// live trade executor. decrypts user keys, runs safety checks,
// and places real orders on binance with sl/tp.
package livetrading

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/circuitbreaker"
	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/opportunity"
)

// dbCtx returns a context with a 5-second timeout for best-effort DB operations
func dbCtx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	return ctx
}

// optional persistence layer for live positions (nil = in-memory only)
type PositionStore interface {
	SavePosition(ctx context.Context, pos *LivePosition) error
	ClosePosition(ctx context.Context, pos *LivePosition) error
}

// optional trade record logger for live trades (nil = no logging)
type TradeLogger interface {
	LogOpen(ctx context.Context, pos *LivePosition) error
	LogClose(ctx context.Context, pos *LivePosition) error
}

// records failed order placements for manual review (dead-letter queue)
type FailedOrderRecorder interface {
	RecordFailedOrder(ctx context.Context, userID int, positionID, symbol, side, orderType string, quantity, price, stopPrice float64, tradeType, errorMsg string) error
}

// decrypts stored credentials for order placement
type KeyDecryptor interface {
	DecryptKeys(userID int) (apiKey, apiSecret string, err error)
}

type exchangeKeyDecryptor interface {
	DecryptKeysForExchange(userID int, exchange string) (apiKey, apiSecret string, err error)
}

// resolves the user's active exchange so live spot execution can refuse
// unsupported runtimes instead of silently assuming Binance.
type PrimaryExchangeResolver interface {
	PrimaryExchange(userID int) (string, error)
}

// resolves the user's active exchange and its order executor.
type OrderRouter interface {
	OrderExecutorFor(userID int) (exchangeName string, orders exchange.OrderExecutor, err error)
}

type namedOrderRouter interface {
	OrderExecutor(exchangeName string) (exchange.OrderExecutor, error)
}

// a live position with real exchange order ids
type LivePosition struct {
	ID           string
	UserID       int
	Exchange     string
	Symbol       string
	Side         exchange.OrderSide
	EntryPrice   float64
	Quantity     float64
	PositionSize float64
	StopLoss     float64
	TakeProfit   float64
	MainOrderID  int64
	SLOrderID    int64
	TPOrderID    int64
	CloseOrderID int64
	Status       string // "open", "closed"
	CloseReason  string
	ClosePrice   float64
	PnL          float64
	OpenedAt     time.Time
	ClosedAt     *time.Time
	Platform     string
}

// executor configuration
type ExecutorConfig struct {
	Safety SafetyConfig
}

// records slippage observations for execution quality tracking
type SlippageRecorder interface {
	Record(symbol, side string, expectedPrice, actualPrice, quantity float64, isPaper bool) exchange.SlippageRecord
}

// executes real trades on the exchange with full safety validation
type Executor struct {
	mu           sync.RWMutex
	positions    map[string]*LivePosition
	closed       []*LivePosition
	orders       exchange.OrderExecutor
	keys         KeyDecryptor
	safety       *SafetyChecker
	losses       LossTracker
	breaker      *circuitbreaker.Breaker // nil if no circuit breaker configured
	store        PositionStore           // nil if no persistence configured
	trades       TradeLogger             // nil if no logging configured
	slippage     SlippageRecorder        // nil if no slippage tracking configured
	failedOrders FailedOrderRecorder     // nil if no dead-letter queue configured
	exchanges    PrimaryExchangeResolver // nil if exchange routing is not wired
	orderRouter  OrderRouter             // nil = use orders/default exchange
	nextID       int
}

func NewExecutor(orders exchange.OrderExecutor, keys KeyDecryptor, safety *SafetyChecker, losses LossTracker) *Executor {
	return &Executor{
		positions: make(map[string]*LivePosition),
		orders:    orders,
		keys:      keys,
		safety:    safety,
		losses:    losses,
	}
}

// SetSafetyChecker updates the pre-trade safety checker.
func (e *Executor) SetSafetyChecker(safety *SafetyChecker) {
	e.safety = safety
}

// SetCircuitBreaker configures portfolio circuit breaker.
func (e *Executor) SetCircuitBreaker(b *circuitbreaker.Breaker) {
	e.breaker = b
}

// SetStore configures position persistence. Call before Start.
func (e *Executor) SetStore(store PositionStore) {
	e.store = store
}

// SetTradeLogger configures trade record logging. Call before Start.
func (e *Executor) SetTradeLogger(logger TradeLogger) {
	e.trades = logger
}

// SetSlippageTracker configures slippage recording. Call before Start.
func (e *Executor) SetSlippageTracker(tracker SlippageRecorder) {
	e.slippage = tracker
}

// SetFailedOrderRecorder configures the dead-letter queue for failed orders.
func (e *Executor) SetFailedOrderRecorder(recorder FailedOrderRecorder) {
	e.failedOrders = recorder
}

// SetPrimaryExchangeResolver configures user-level exchange resolution for
// explicit live spot routing checks.
func (e *Executor) SetPrimaryExchangeResolver(resolver PrimaryExchangeResolver) {
	e.exchanges = resolver
}

// SetOrderRouter configures per-user live spot exchange routing.
func (e *Executor) SetOrderRouter(router OrderRouter) {
	e.orderRouter = router
}

// SetNextID sets the starting ID for new positions (used for recovery).
func (e *Executor) SetNextID(id int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextID = id
}

// RestorePosition adds a recovered position back into the in-memory map (startup only).
func (e *Executor) RestorePosition(pos *LivePosition) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.positions[pos.ID] = pos
}

// opens a live position from an approved opportunity.
// decrypts keys, runs safety checks, places market order + sl + tp orders.
func (e *Executor) Execute(opp *opportunity.Opportunity) (*LivePosition, error) {
	if opp.Status != opportunity.StatusApproved && opp.Status != opportunity.StatusModified && opp.Status != opportunity.StatusExecuting {
		return nil, fmt.Errorf("opportunity not approved: %s", opp.Status)
	}

	if opp.Result == nil || opp.Result.Decision == nil {
		return nil, fmt.Errorf("opportunity missing analysis result")
	}

	plan := opp.Result.Decision.Plan
	if opp.ModifiedPlan != nil {
		plan = *opp.ModifiedPlan
	}

	if plan.PositionSize <= 0 {
		return nil, fmt.Errorf("invalid position size: %.2f", plan.PositionSize)
	}

	exchangeName, orders, err := e.resolveOrders(opp.UserID)
	if err != nil {
		return nil, err
	}

	// determine order side and quote asset
	side := exchange.SideBuy
	closeSide := exchange.SideSell
	if opp.Action == claude.ActionSell {
		side = exchange.SideSell
		closeSide = exchange.SideBuy
	}
	asset := "USDT"

	// check circuit breaker before executing
	if e.breaker != nil {
		if ok, reason := e.breaker.AllowTrade(opp.UserID); !ok {
			return nil, fmt.Errorf("circuit breaker: %s", reason)
		}
	}

	// run safety checks
	if e.safety != nil {
		result := e.safety.Check(opp.UserID, opp.Symbol, plan.PositionSize, asset)
		if !result.Passed {
			return nil, fmt.Errorf("safety check failed: %s", result.Blocked)
		}
	}

	// decrypt user api keys
	apiKey, apiSecret, err := e.decryptKeys(opp.UserID, exchangeName)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt keys: %w", err)
	}

	quantity, quoteOrderQty, err := marketOrderSizing(exchangeName, side, plan)
	if err != nil {
		return nil, err
	}

	// place main market order
	mainOrder, err := orders.PlaceOrder(
		opp.Symbol, side, exchange.OrderTypeMarket,
		quantity, quoteOrderQty,
		apiKey, apiSecret,
	)
	if err != nil {
		if e.failedOrders != nil {
			_ = e.failedOrders.RecordFailedOrder(dbCtx(), opp.UserID, "", opp.Symbol,
				string(side), "MARKET", plan.PositionSize, plan.Entry, 0, "SPOT", err.Error())
		}
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	fillPrice := mainOrder.AvgPrice
	if fillPrice <= 0 {
		fillPrice = plan.Entry
	}
	quantity = mainOrder.ExecutedQty
	if quantity <= 0 {
		quantity = mainOrder.Quantity
	}
	if quantity <= 0 && fillPrice > 0 {
		quantity = plan.PositionSize / fillPrice
	}
	if quantity <= 0 {
		return nil, fmt.Errorf("exchange returned no executed quantity for %s market order", exchangeName)
	}

	// place stop loss order — abort if this fails (position would be unprotected)
	var slOrderID int64
	if plan.StopLoss > 0 {
		slOrder, err := orders.PlaceStopLoss(
			opp.Symbol, closeSide, quantity,
			plan.StopLoss, plan.StopLoss,
			apiKey, apiSecret,
		)
		if err != nil {
			// close the main order — position must not exist without a stop loss
			slog.Error("failed to place stop loss, closing main order",
				"symbol", opp.Symbol, "error", err)
			_, reverseErr := orders.PlaceOrder(
				opp.Symbol, closeSide, exchange.OrderTypeMarket,
				quantity, 0, apiKey, apiSecret,
			)
			if reverseErr != nil {
				// CRITICAL: position is open on exchange with NO stop loss and reversal FAILED
				slog.Error("CRITICAL: failed to reverse position after SL failure — OPEN POSITION WITHOUT PROTECTION",
					"symbol", opp.Symbol, "quantity", quantity, "side", side,
					"sl_error", err, "reversal_error", reverseErr)
				if e.failedOrders != nil {
					_ = e.failedOrders.RecordFailedOrder(dbCtx(), opp.UserID, "", opp.Symbol,
						string(closeSide), "EMERGENCY_REVERSAL", quantity, 0, 0, "SPOT",
						fmt.Sprintf("SL failed: %s; reversal also failed: %s", err, reverseErr))
				}
				return nil, fmt.Errorf("CRITICAL: SL failed and reversal failed — naked position on exchange: sl_err=%w, reversal_err=%v", err, reverseErr)
			}
			if e.failedOrders != nil {
				_ = e.failedOrders.RecordFailedOrder(dbCtx(), opp.UserID, "", opp.Symbol,
					string(closeSide), "STOP_LOSS_LIMIT", quantity, plan.StopLoss, plan.StopLoss, "SPOT", err.Error())
			}
			return nil, fmt.Errorf("failed to place stop loss (main order reversed): %w", err)
		}
		slOrderID = slOrder.OrderID
	}

	// place take profit order — log warning but don't abort (SL protects us)
	var tpOrderID int64
	if plan.TakeProfit > 0 {
		tpOrder, err := orders.PlaceTakeProfit(
			opp.Symbol, closeSide, quantity,
			plan.TakeProfit, plan.TakeProfit,
			apiKey, apiSecret,
		)
		if err != nil {
			slog.Warn("failed to place take profit order, position will rely on SL only",
				"symbol", opp.Symbol, "error", err)
		} else {
			tpOrderID = tpOrder.OrderID
		}
	}

	e.mu.Lock()
	e.nextID++
	id := fmt.Sprintf("live_%d", e.nextID)

	pos := &LivePosition{
		ID:           id,
		UserID:       opp.UserID,
		Exchange:     exchangeName,
		Symbol:       opp.Symbol,
		Side:         side,
		EntryPrice:   fillPrice,
		Quantity:     quantity,
		PositionSize: plan.PositionSize,
		StopLoss:     plan.StopLoss,
		TakeProfit:   plan.TakeProfit,
		MainOrderID:  mainOrder.OrderID,
		SLOrderID:    slOrderID,
		TPOrderID:    tpOrderID,
		Status:       "open",
		OpenedAt:     time.Now(),
		Platform:     opp.Platform,
	}

	e.positions[id] = pos
	e.mu.Unlock()

	// record slippage (expected from AI decision vs actual fill)
	if e.slippage != nil && plan.Entry > 0 {
		rec := e.slippage.Record(opp.Symbol, string(side), plan.Entry, fillPrice, quantity, false)
		slog.Debug("slippage recorded", "symbol", opp.Symbol, "bps", rec.SlippageBps)
	}

	// persist to database (best-effort — position is already on exchange)
	if e.store != nil {
		if err := e.store.SavePosition(dbCtx(), pos); err != nil {
			slog.Error("failed to persist live position", "id", id, "error", err)
		}
	}

	// log trade open record (best-effort)
	if e.trades != nil {
		if err := e.trades.LogOpen(dbCtx(), pos); err != nil {
			slog.Error("failed to log live trade open", "id", id, "error", err)
		}
	}

	return pos, nil
}

func (e *Executor) ensureSupportedSpotExchange(userID int) error {
	if e.exchanges == nil {
		return nil
	}

	exchangeName, err := e.exchanges.PrimaryExchange(userID)
	if err != nil {
		return fmt.Errorf("live spot trading unavailable: %w", err)
	}
	if exchangeName != supportedLiveSpotExchange {
		return errors.New(FormatUnsupportedSpotExchange(exchangeName))
	}
	return nil
}

func (e *Executor) resolveOrders(userID int) (string, exchange.OrderExecutor, error) {
	if e.orderRouter != nil {
		exchangeName, orders, err := e.orderRouter.OrderExecutorFor(userID)
		if err != nil {
			return "", nil, err
		}
		exchangeName = normalizeExchangeName(exchangeName)
		if exchangeName == "" {
			return "", nil, fmt.Errorf("live spot trading unavailable: empty exchange for user %d", userID)
		}
		if orders == nil {
			return "", nil, fmt.Errorf("live spot trading unavailable: no order executor for %s", exchangeName)
		}
		return exchangeName, orders, nil
	}

	exchangeName := supportedLiveSpotExchange
	if e.exchanges != nil {
		name, err := e.exchanges.PrimaryExchange(userID)
		if err != nil {
			return "", nil, fmt.Errorf("live spot trading unavailable: %w", err)
		}
		exchangeName = normalizeExchangeName(name)
		if exchangeName != supportedLiveSpotExchange {
			return "", nil, errors.New(FormatUnsupportedSpotExchange(exchangeName))
		}
	}
	if e.orders == nil {
		return "", nil, fmt.Errorf("live spot trading unavailable: no default order executor")
	}
	return exchangeName, e.orders, nil
}

func (e *Executor) ordersForPosition(pos *LivePosition) (exchange.OrderExecutor, error) {
	if e.orderRouter == nil {
		if e.orders == nil {
			return nil, fmt.Errorf("live spot trading unavailable: no default order executor")
		}
		return e.orders, nil
	}

	if pos.Exchange != "" {
		if router, ok := e.orderRouter.(namedOrderRouter); ok {
			return router.OrderExecutor(pos.Exchange)
		}
	}

	exchangeName, orders, err := e.orderRouter.OrderExecutorFor(pos.UserID)
	if err != nil {
		return nil, err
	}
	if pos.Exchange != "" && normalizeExchangeName(exchangeName) != normalizeExchangeName(pos.Exchange) {
		return nil, fmt.Errorf("position %s belongs to %s but active credentials route to %s", pos.ID, pos.Exchange, exchangeName)
	}
	if orders == nil {
		return nil, fmt.Errorf("live spot trading unavailable: no order executor for %s", exchangeName)
	}
	return orders, nil
}

func (e *Executor) decryptKeys(userID int, exchangeName string) (string, string, error) {
	return decryptKeysForExchange(e.keys, userID, exchangeName)
}

func decryptKeysForExchange(keys KeyDecryptor, userID int, exchangeName string) (string, string, error) {
	if keys == nil {
		return "", "", fmt.Errorf("no key decryptor configured")
	}
	if kd, ok := keys.(exchangeKeyDecryptor); ok {
		return kd.DecryptKeysForExchange(userID, exchangeName)
	}
	return keys.DecryptKeys(userID)
}

func marketOrderSizing(exchangeName string, side exchange.OrderSide, plan claude.TradePlan) (quantity, quoteOrderQty float64, err error) {
	if plan.PositionSize <= 0 {
		return 0, 0, fmt.Errorf("invalid position size: %.2f", plan.PositionSize)
	}

	// Binance spot supports quoteOrderQty for market buys, which avoids buying
	// too much or too little when the current price has moved from the AI entry.
	if normalizeExchangeName(exchangeName) == supportedLiveSpotExchange && side == exchange.SideBuy {
		return 0, plan.PositionSize, nil
	}

	if plan.Entry <= 0 {
		return 0, 0, fmt.Errorf("market order quantity requires positive entry price for %s %s", exchangeName, side)
	}
	return plan.PositionSize / plan.Entry, 0, nil
}

func normalizeExchangeName(exchangeName string) string {
	return strings.TrimSpace(strings.ToLower(exchangeName))
}

// closes a live position by placing a market order and canceling sl/tp
func (e *Executor) Close(posID string, reason string) (*LivePosition, error) {
	e.mu.Lock()
	pos, ok := e.positions[posID]
	if !ok {
		e.mu.Unlock()
		return nil, fmt.Errorf("position not found: %s", posID)
	}
	if pos.Status == "closed" {
		e.mu.Unlock()
		return nil, fmt.Errorf("position already closed: %s", posID)
	}
	e.mu.Unlock()

	orders, err := e.ordersForPosition(pos)
	if err != nil {
		return nil, err
	}

	exchangeName := pos.Exchange
	if exchangeName == "" {
		exchangeName = supportedLiveSpotExchange
	}
	apiKey, apiSecret, err := e.decryptKeys(pos.UserID, exchangeName)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt keys: %w", err)
	}

	// cancel existing sl/tp orders (log failures — stale orders could fill unexpectedly)
	if pos.SLOrderID > 0 {
		if err := orders.CancelOrder(pos.Symbol, pos.SLOrderID, apiKey, apiSecret); err != nil {
			slog.Warn("failed to cancel SL order during close — may still fill on exchange",
				"position", posID, "sl_order", pos.SLOrderID, "error", err)
		}
	}
	if pos.TPOrderID > 0 {
		if err := orders.CancelOrder(pos.Symbol, pos.TPOrderID, apiKey, apiSecret); err != nil {
			slog.Warn("failed to cancel TP order during close — may still fill on exchange",
				"position", posID, "tp_order", pos.TPOrderID, "error", err)
		}
	}

	// place closing market order
	closeSide := exchange.SideSell
	if pos.Side == exchange.SideSell {
		closeSide = exchange.SideBuy
	}

	closeOrder, err := orders.PlaceOrder(
		pos.Symbol, closeSide, exchange.OrderTypeMarket,
		pos.Quantity, 0,
		apiKey, apiSecret,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to close position: %w", err)
	}

	closePrice := closeOrder.AvgPrice
	if closePrice <= 0 {
		closePrice = closeOrder.Price
	}
	return e.finalizeClose(posID, reason, closePrice, closeOrder.OrderID)
}

func (e *Executor) CloseFromFilledOrder(posID string, reason string, order *exchange.Order) (*LivePosition, error) {
	if order == nil {
		return nil, fmt.Errorf("filled order is nil")
	}
	if pos := e.Get(posID); pos != nil {
		e.cancelProtectionOrdersExcept(pos, order.OrderID)
	}
	closePrice := order.AvgPrice
	if closePrice <= 0 {
		closePrice = order.Price
	}
	if closePrice <= 0 {
		closePrice = order.StopPrice
	}
	return e.finalizeClose(posID, reason, closePrice, order.OrderID)
}

func (e *Executor) cancelProtectionOrdersExcept(pos *LivePosition, filledOrderID int64) {
	orders, err := e.ordersForPosition(pos)
	if err != nil {
		slog.Warn("failed to resolve order executor for protection cleanup", "position", pos.ID, "error", err)
		return
	}
	exchangeName := pos.Exchange
	if exchangeName == "" {
		exchangeName = supportedLiveSpotExchange
	}
	apiKey, apiSecret, err := e.decryptKeys(pos.UserID, exchangeName)
	if err != nil {
		slog.Warn("failed to decrypt keys for protection cleanup", "position", pos.ID, "error", err)
		return
	}
	if pos.SLOrderID > 0 && pos.SLOrderID != filledOrderID {
		if err := orders.CancelOrder(pos.Symbol, pos.SLOrderID, apiKey, apiSecret); err != nil {
			slog.Warn("failed to cancel sibling SL order after fill", "position", pos.ID, "order", pos.SLOrderID, "error", err)
		}
	}
	if pos.TPOrderID > 0 && pos.TPOrderID != filledOrderID {
		if err := orders.CancelOrder(pos.Symbol, pos.TPOrderID, apiKey, apiSecret); err != nil {
			slog.Warn("failed to cancel sibling TP order after fill", "position", pos.ID, "order", pos.TPOrderID, "error", err)
		}
	}
}

func (e *Executor) finalizeClose(posID string, reason string, closePrice float64, closeOrderID int64) (*LivePosition, error) {
	e.mu.Lock()
	pos, ok := e.positions[posID]
	if !ok {
		e.mu.Unlock()
		return nil, fmt.Errorf("position not found: %s", posID)
	}
	if pos.Status == "closed" {
		e.mu.Unlock()
		return nil, fmt.Errorf("position already closed: %s", posID)
	}
	now := time.Now()
	if closePrice <= 0 {
		closePrice = pos.EntryPrice
	}
	pos.Status = "closed"
	pos.CloseReason = reason
	pos.ClosePrice = closePrice
	pos.CloseOrderID = closeOrderID
	pos.ClosedAt = &now

	if pos.Side == exchange.SideBuy {
		pos.PnL = (pos.ClosePrice - pos.EntryPrice) * pos.Quantity
	} else {
		pos.PnL = (pos.EntryPrice - pos.ClosePrice) * pos.Quantity
	}

	// track losses
	if e.losses != nil && pos.PnL < 0 {
		e.losses.RecordLoss(pos.UserID, pos.PnL)
	}

	e.closed = append(e.closed, pos)
	if len(e.closed) > 1000 {
		e.closed = e.closed[len(e.closed)-1000:]
	}
	delete(e.positions, posID)
	e.mu.Unlock()

	e.persistClosed(pos)
	return pos, nil
}

func (e *Executor) persistClosed(pos *LivePosition) {
	// persist to database (best-effort)
	if e.store != nil {
		if err := e.store.ClosePosition(dbCtx(), pos); err != nil {
			slog.Error("failed to persist live position close", "id", pos.ID, "error", err)
		}
	}

	// log trade close record (best-effort)
	if e.trades != nil {
		if err := e.trades.LogClose(dbCtx(), pos); err != nil {
			slog.Error("failed to log live trade close", "id", pos.ID, "error", err)
		}
	}
}

// returns a position by id
func (e *Executor) Get(posID string) *LivePosition {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if pos, ok := e.positions[posID]; ok {
		return pos
	}
	for _, pos := range e.closed {
		if pos.ID == posID {
			return pos
		}
	}
	return nil
}

// returns all open positions for a user
func (e *Executor) OpenPositions(userID int) []*LivePosition {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []*LivePosition
	for _, pos := range e.positions {
		if pos.UserID == userID {
			result = append(result, pos)
		}
	}
	return result
}

// returns all open positions across all users
func (e *Executor) AllOpen() []*LivePosition {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*LivePosition, 0, len(e.positions))
	for _, pos := range e.positions {
		result = append(result, pos)
	}
	return result
}

// returns the open position count for a user (implements PositionCounter)
func (e *Executor) OpenPositionCount(userID int) int {
	return len(e.OpenPositions(userID))
}

// returns count of all open positions
func (e *Executor) Count() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.positions)
}
