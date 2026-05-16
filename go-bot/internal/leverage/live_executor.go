// live leverage executor. decrypts user keys, runs safety checks,
// and places real futures orders on binance with sl/tp.
package leverage

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/circuitbreaker"
	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/opsalert"
)

// decrypts stored api credentials
type KeyDecryptor interface {
	DecryptKeys(userID int) (apiKey, apiSecret string, err error)
}

// places and manages futures orders
type FuturesOrderClient interface {
	SetLeverage(ctx context.Context, symbol string, leverage int, apiKey, apiSecret string) error
	SetMarginType(ctx context.Context, symbol string, marginType string, apiKey, apiSecret string) error
	PlaceOrder(ctx context.Context, symbol string, side exchange.OrderSide, orderType exchange.OrderType, quantity, price float64, apiKey, apiSecret string) (*binance.FuturesOrder, error)
	PlaceStopMarket(ctx context.Context, symbol string, side exchange.OrderSide, quantity, stopPrice float64, apiKey, apiSecret string) (*binance.FuturesOrder, error)
	PlaceTakeProfitMarket(ctx context.Context, symbol string, side exchange.OrderSide, quantity, stopPrice float64, apiKey, apiSecret string) (*binance.FuturesOrder, error)
	CancelOrder(ctx context.Context, symbol string, orderID int64, apiKey, apiSecret string) error
	GetOrder(ctx context.Context, symbol string, orderID int64, apiKey, apiSecret string) (*binance.FuturesOrder, error)
	GetPositions(ctx context.Context, apiKey, apiSecret string) ([]binance.FuturesPosition, error)
}

// executes real leveraged futures trades on binance
type LiveExecutor struct {
	mu        sync.RWMutex
	positions map[string]*LeveragePosition
	closed    []*LeveragePosition
	futures   FuturesOrderClient
	keys      KeyDecryptor
	safety    *SafetyChecker
	funding   *FundingTracker
	prices    MarkPriceProvider
	breaker   *circuitbreaker.Breaker // nil if no circuit breaker configured
	store     LeveragePositionStore   // nil if no persistence configured
	trades    LeverageTradeLogger     // nil if no logging configured
	alerter   opsalert.Notifier       // nil if operational alerting disabled
	nextID    int
}

// creates a new live leverage executor
func NewLiveExecutor(
	futures FuturesOrderClient,
	keys KeyDecryptor,
	safety *SafetyChecker,
	funding *FundingTracker,
	prices MarkPriceProvider,
) *LiveExecutor {
	return &LiveExecutor{
		positions: make(map[string]*LeveragePosition),
		futures:   futures,
		keys:      keys,
		safety:    safety,
		funding:   funding,
		prices:    prices,
	}
}

// SetCircuitBreaker configures portfolio circuit breaker.
func (e *LiveExecutor) SetCircuitBreaker(b *circuitbreaker.Breaker) {
	e.breaker = b
}

// SetStore configures position persistence. Call before Start.
func (e *LiveExecutor) SetStore(store LeveragePositionStore) {
	e.store = store
}

// SetTradeLogger configures trade record logging. Call before Start.
func (e *LiveExecutor) SetTradeLogger(logger LeverageTradeLogger) {
	e.trades = logger
}

// SetAlerter configures operational alerts for live futures failures.
func (e *LiveExecutor) SetAlerter(alerter opsalert.Notifier) {
	e.alerter = alerter
}

// SetNextID sets the starting ID for new positions (used for recovery).
func (e *LiveExecutor) SetNextID(id int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextID = id
}

// RestorePosition adds a recovered position back into the in-memory map (startup only).
func (e *LiveExecutor) RestorePosition(pos *LeveragePosition) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.positions[pos.ID] = pos
}

// opens a live leveraged position on binance futures.
// decrypts keys, runs safety checks, configures leverage and margin type,
// places market order, then sets sl/tp orders.
func (e *LiveExecutor) OpenPosition(
	userID int,
	symbol string,
	side PositionSide,
	leverage int,
	margin float64,
	stopLoss, takeProfit float64,
	platform string,
) (*LeveragePosition, error) {
	// create a timeout context for exchange operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// get current mark price for quantity calculation and safety checks
	markPrice, err := e.prices.GetMarkPrice(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get mark price: %w", err)
	}
	if markPrice <= 0 {
		return nil, fmt.Errorf("invalid mark price %.8f for %s", markPrice, symbol)
	}

	// check circuit breaker before executing
	if e.breaker != nil {
		if ok, reason := e.breaker.AllowTrade(userID); !ok {
			return nil, fmt.Errorf("circuit breaker: %s", reason)
		}
	}

	// run safety checks if checker is configured
	if e.safety != nil {
		result := e.safety.Check(userID, symbol, leverage, margin, markPrice, string(side))
		if !result.Passed {
			return nil, fmt.Errorf("safety check failed: %s", result.Blocked)
		}
	}

	// decrypt user api keys
	apiKey, apiSecret, err := e.keys.DecryptKeys(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt keys: %w", err)
	}

	// configure leverage on exchange
	if err := e.futures.SetLeverage(ctx, symbol, leverage, apiKey, apiSecret); err != nil {
		return nil, fmt.Errorf("failed to set leverage: %w", err)
	}

	// set isolated margin mode
	if err := e.futures.SetMarginType(ctx, symbol, "ISOLATED", apiKey, apiSecret); err != nil {
		return nil, fmt.Errorf("failed to set margin type: %w", err)
	}

	// determine exchange order side
	orderSide := exchange.SideBuy
	closeSide := exchange.SideSell
	if side == SideShort {
		orderSide = exchange.SideSell
		closeSide = exchange.SideBuy
	}

	// calculate quantity from margin and leverage
	notional := margin * float64(leverage)
	quantity := notional / markPrice

	// place market order
	mainOrder, err := e.futures.PlaceOrder(
		ctx, symbol, orderSide, exchange.OrderTypeMarket,
		quantity, 0,
		apiKey, apiSecret,
	)
	if err != nil {
		e.notifyAlert(opsalert.Alert{
			Key:       fmt.Sprintf("futures-main-order-failed:%d:%s", userID, symbol),
			Severity:  opsalert.SeverityCritical,
			Component: "leverage_trading",
			Title:     "Futures order placement failed",
			Summary:   "The live futures entry order was rejected or could not reach the exchange.",
			UserID:    userID,
			Symbol:    symbol,
			Error:     err.Error(),
			Fields: map[string]string{
				"side":     string(side),
				"leverage": fmt.Sprintf("%d", leverage),
				"margin":   fmt.Sprintf("%.2f", margin),
			},
		})
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	// use actual fill price if available, otherwise fall back to mark price
	entryPrice := mainOrder.AvgPrice
	if entryPrice <= 0 {
		entryPrice = markPrice
	}

	// use actual executed quantity if available
	filledQty := mainOrder.ExecutedQty
	if filledQty <= 0 {
		filledQty = quantity
	}

	// recalculate actual notional based on fill
	actualNotional := entryPrice * filledQty

	// calculate liquidation price
	liqPrice := CalculateLiquidationPrice(entryPrice, leverage, string(side), DefaultMaintenanceMarginRate)

	// place stop loss order — abort if this fails (leveraged position without SL is extremely dangerous)
	var slOrderID int64
	if stopLoss > 0 {
		slOrder, err := e.futures.PlaceStopMarket(
			ctx, symbol, closeSide, filledQty, stopLoss,
			apiKey, apiSecret,
		)
		if err != nil {
			// close the position immediately — cannot have leverage without SL
			slog.Error("failed to place SL on leveraged position, closing immediately",
				"symbol", symbol, "leverage", leverage, "error", err)
			_, reverseErr := e.futures.PlaceOrder(
				ctx, symbol, closeSide, exchange.OrderTypeMarket,
				filledQty, 0, apiKey, apiSecret,
			)
			if reverseErr != nil {
				// CRITICAL: leveraged position is open on exchange with NO stop loss and reversal FAILED
				slog.Error("CRITICAL: failed to reverse leveraged position after SL failure — OPEN LEVERAGED POSITION WITHOUT PROTECTION",
					"symbol", symbol, "quantity", filledQty, "side", side, "leverage", leverage,
					"sl_error", err, "reversal_error", reverseErr)
				e.notifyAlert(opsalert.Alert{
					Key:       fmt.Sprintf("futures-naked-position:%d:%s:%d", userID, symbol, mainOrder.OrderID),
					Severity:  opsalert.SeverityCritical,
					Component: "leverage_trading",
					Title:     "Open futures position has no stop loss",
					Summary:   "Stop-loss placement failed and the emergency reversal also failed. Manual exchange review is required immediately.",
					UserID:    userID,
					Symbol:    symbol,
					Error:     fmt.Sprintf("stop_loss=%s; reversal=%s", err, reverseErr),
					Fields: map[string]string{
						"main_order_id": fmt.Sprintf("%d", mainOrder.OrderID),
						"quantity":      fmt.Sprintf("%.8f", filledQty),
						"leverage":      fmt.Sprintf("%d", leverage),
					},
				})
				return nil, fmt.Errorf("CRITICAL: SL failed and reversal failed — naked leveraged position on exchange: sl_err=%w, reversal_err=%v", err, reverseErr)
			}
			e.notifyAlert(opsalert.Alert{
				Key:       fmt.Sprintf("futures-sl-failed-reversed:%d:%s:%d", userID, symbol, mainOrder.OrderID),
				Severity:  opsalert.SeverityWarning,
				Component: "leverage_trading",
				Title:     "Futures stop loss failed, position reversed",
				Summary:   "The futures entry filled, stop-loss placement failed, and the bot reversed the position with a market order.",
				UserID:    userID,
				Symbol:    symbol,
				Error:     err.Error(),
				Fields: map[string]string{
					"main_order_id": fmt.Sprintf("%d", mainOrder.OrderID),
					"leverage":      fmt.Sprintf("%d", leverage),
				},
			})
			return nil, fmt.Errorf("failed to place stop loss on leveraged position (position reversed): %w", err)
		}
		slOrderID = slOrder.OrderID
	}

	// place take profit order — log warning but don't abort (SL protects us)
	var tpOrderID int64
	if takeProfit > 0 {
		tpOrder, err := e.futures.PlaceTakeProfitMarket(
			ctx, symbol, closeSide, filledQty, takeProfit,
			apiKey, apiSecret,
		)
		if err != nil {
			slog.Warn("failed to place TP on leveraged position, will rely on SL only",
				"symbol", symbol, "error", err)
			e.notifyAlert(opsalert.Alert{
				Key:       fmt.Sprintf("futures-tp-placement-failed:%d:%s:%d", userID, symbol, mainOrder.OrderID),
				Severity:  opsalert.SeverityWarning,
				Component: "leverage_trading",
				Title:     "Futures take profit placement failed",
				Summary:   "The live futures position is open and protected by stop loss, but the take-profit order was not placed.",
				UserID:    userID,
				Symbol:    symbol,
				Error:     err.Error(),
				Fields: map[string]string{
					"main_order_id": fmt.Sprintf("%d", mainOrder.OrderID),
					"leverage":      fmt.Sprintf("%d", leverage),
				},
			})
		} else {
			tpOrderID = tpOrder.OrderID
		}
	}

	// try to get exchange liquidation price for higher accuracy
	positions, err := e.futures.GetPositions(ctx, apiKey, apiSecret)
	if err == nil {
		for _, p := range positions {
			if p.Symbol == symbol && p.LiquidationPrice > 0 {
				liqPrice = p.LiquidationPrice
				break
			}
		}
	}

	e.mu.Lock()
	e.nextID++
	id := fmt.Sprintf("lev_%d", e.nextID)

	pos := &LeveragePosition{
		ID:               id,
		UserID:           userID,
		Symbol:           symbol,
		Side:             side,
		Leverage:         leverage,
		EntryPrice:       entryPrice,
		MarkPrice:        entryPrice,
		Quantity:         filledQty,
		Margin:           margin,
		NotionalValue:    actualNotional,
		LiquidationPrice: liqPrice,
		StopLoss:         stopLoss,
		TakeProfit:       takeProfit,
		MarginType:       "isolated",
		IsPaper:          false,
		Status:           "open",
		OpenedAt:         time.Now(),
		Platform:         platform,
		MainOrderID:      mainOrder.OrderID,
		SLOrderID:        slOrderID,
		TPOrderID:        tpOrderID,
	}

	e.positions[id] = pos
	e.mu.Unlock()

	e.alertMissingProtection(pos)

	// persist to database (best-effort — position is already on exchange)
	if e.store != nil {
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := e.store.SavePosition(dbCtx, pos); err != nil {
			slog.Error("failed to persist live leverage position", "id", id, "error", err)
		}
		dbCancel()
	}

	// log trade open record (best-effort)
	if e.trades != nil {
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := e.trades.LogOpen(dbCtx, pos); err != nil {
			slog.Error("failed to log live leverage trade open", "id", id, "error", err)
		}
		dbCancel()
	}

	return pos, nil
}

func (e *LiveExecutor) alertMissingProtection(pos *LeveragePosition) {
	if pos == nil {
		return
	}
	if pos.SLOrderID == 0 {
		e.notifyAlert(opsalert.Alert{
			Key:        fmt.Sprintf("futures-missing-sl:%d:%s", pos.UserID, pos.ID),
			Severity:   opsalert.SeverityCritical,
			Component:  "leverage_trading",
			Title:      "Futures position missing stop loss",
			Summary:    "A live futures position is open without an exchange stop-loss order. Review the exchange account immediately.",
			UserID:     pos.UserID,
			Symbol:     pos.Symbol,
			PositionID: pos.ID,
			Fields: map[string]string{
				"main_order_id": fmt.Sprintf("%d", pos.MainOrderID),
				"quantity":      fmt.Sprintf("%.8f", pos.Quantity),
				"leverage":      fmt.Sprintf("%d", pos.Leverage),
			},
		})
	}
	if pos.TPOrderID == 0 {
		e.notifyAlert(opsalert.Alert{
			Key:        fmt.Sprintf("futures-missing-tp:%d:%s", pos.UserID, pos.ID),
			Severity:   opsalert.SeverityWarning,
			Component:  "leverage_trading",
			Title:      "Futures position missing take profit",
			Summary:    "A live futures position is open without an exchange take-profit order. Stop-loss protection is the priority, but exit automation is incomplete.",
			UserID:     pos.UserID,
			Symbol:     pos.Symbol,
			PositionID: pos.ID,
			Fields: map[string]string{
				"main_order_id": fmt.Sprintf("%d", pos.MainOrderID),
				"leverage":      fmt.Sprintf("%d", pos.Leverage),
			},
		})
	}
}

func (e *LiveExecutor) notifyAlert(alert opsalert.Alert) {
	if e.alerter == nil {
		return
	}
	if err := e.alerter.Notify(context.Background(), alert); err != nil {
		slog.Warn("failed to send leverage alert", "key", alert.Key, "error", err)
	}
}

// closes a live leveraged position by canceling sl/tp and placing a closing order.
// calculates realized pnl including cumulative funding fees.
func (e *LiveExecutor) Close(posID string, reason string) (*LeveragePosition, error) {
	// create a timeout context for exchange operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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

	// decrypt user api keys
	apiKey, apiSecret, err := e.keys.DecryptKeys(pos.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt keys: %w", err)
	}

	// cancel existing sl/tp orders (log failures — stale orders could fill unexpectedly)
	if pos.SLOrderID > 0 {
		if err := e.futures.CancelOrder(ctx, pos.Symbol, pos.SLOrderID, apiKey, apiSecret); err != nil {
			slog.Warn("failed to cancel SL order during leverage close",
				"position", posID, "sl_order", pos.SLOrderID, "error", err)
		}
	}
	if pos.TPOrderID > 0 {
		if err := e.futures.CancelOrder(ctx, pos.Symbol, pos.TPOrderID, apiKey, apiSecret); err != nil {
			slog.Warn("failed to cancel TP order during leverage close",
				"position", posID, "tp_order", pos.TPOrderID, "error", err)
		}
	}

	// determine closing side
	closeSide := exchange.SideSell
	if pos.Side == SideShort {
		closeSide = exchange.SideBuy
	}

	// place closing market order
	closeOrder, err := e.futures.PlaceOrder(
		ctx, pos.Symbol, closeSide, exchange.OrderTypeMarket,
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

// CloseFromFilledOrder reconciles a position as closed by an already-filled
// exchange order. It does not place another market order.
func (e *LiveExecutor) CloseFromFilledOrder(posID string, reason string, closeOrder *binance.FuturesOrder) (*LeveragePosition, error) {
	if closeOrder == nil {
		return nil, fmt.Errorf("filled order is nil")
	}
	if pos := e.Get(posID); pos != nil {
		e.cancelProtectionOrdersExcept(pos, closeOrder.OrderID)
	}

	closePrice := closeOrder.AvgPrice
	if closePrice <= 0 {
		closePrice = closeOrder.Price
	}
	if closePrice <= 0 {
		closePrice = closeOrder.StopPrice
	}
	return e.finalizeClose(posID, reason, closePrice, closeOrder.OrderID)
}

func (e *LiveExecutor) cancelProtectionOrdersExcept(pos *LeveragePosition, filledOrderID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	apiKey, apiSecret, err := e.keys.DecryptKeys(pos.UserID)
	if err != nil {
		slog.Warn("failed to decrypt keys for leverage protection cleanup", "position", pos.ID, "error", err)
		return
	}
	if pos.SLOrderID > 0 && pos.SLOrderID != filledOrderID {
		if err := e.futures.CancelOrder(ctx, pos.Symbol, pos.SLOrderID, apiKey, apiSecret); err != nil {
			slog.Warn("failed to cancel sibling leverage SL order after fill", "position", pos.ID, "order", pos.SLOrderID, "error", err)
		}
	}
	if pos.TPOrderID > 0 && pos.TPOrderID != filledOrderID {
		if err := e.futures.CancelOrder(ctx, pos.Symbol, pos.TPOrderID, apiKey, apiSecret); err != nil {
			slog.Warn("failed to cancel sibling leverage TP order after fill", "position", pos.ID, "order", pos.TPOrderID, "error", err)
		}
	}
}

func (e *LiveExecutor) finalizeClose(posID string, reason string, closePrice float64, closeOrderID int64) (*LeveragePosition, error) {
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
	if closePrice <= 0 {
		closePrice = pos.MarkPrice
	}
	if closePrice <= 0 {
		closePrice = pos.EntryPrice
	}
	// calculate pnl
	var pnl float64
	if pos.Side == SideLong {
		pnl = (closePrice - pos.EntryPrice) * pos.Quantity
	} else {
		pnl = (pos.EntryPrice - closePrice) * pos.Quantity
	}

	// subtract funding fees from pnl
	var fundingFees float64
	if e.funding != nil {
		fundingFees = e.funding.CumulativeFees(posID)
		e.funding.Cleanup(posID)
	}
	pnl -= fundingFees

	now := time.Now()
	pos.Status = "closed"
	pos.CloseReason = reason
	pos.ClosePrice = closePrice
	pos.CloseOrderID = closeOrderID
	pos.ClosedAt = &now
	pos.PnL = pnl
	pos.FundingPaid = fundingFees

	e.closed = append(e.closed, pos)
	if len(e.closed) > 1000 {
		e.closed = e.closed[len(e.closed)-1000:]
	}
	delete(e.positions, posID)
	e.mu.Unlock()

	e.persistClosed(pos)
	return pos, nil
}

func (e *LiveExecutor) persistClosed(pos *LeveragePosition) {
	// persist to database (best-effort)
	if e.store != nil {
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := e.store.ClosePosition(dbCtx, pos); err != nil {
			slog.Error("failed to persist live leverage position close", "id", pos.ID, "error", err)
		}
		dbCancel()
	}

	// log trade close record (best-effort)
	if e.trades != nil {
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := e.trades.LogClose(dbCtx, pos); err != nil {
			slog.Error("failed to log live leverage trade close", "id", pos.ID, "error", err)
		}
		dbCancel()
	}
}

// returns a position by id from open or closed positions
func (e *LiveExecutor) Get(posID string) *LeveragePosition {
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

// returns all open positions for a specific user
func (e *LiveExecutor) OpenPositions(userID int) []*LeveragePosition {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []*LeveragePosition
	for _, pos := range e.positions {
		if pos.UserID == userID {
			result = append(result, pos)
		}
	}
	return result
}

// returns all open positions across all users
func (e *LiveExecutor) AllOpen() []*LeveragePosition {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*LeveragePosition, 0, len(e.positions))
	for _, pos := range e.positions {
		result = append(result, pos)
	}
	return result
}

// returns the total count of open positions
func (e *LiveExecutor) Count() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.positions)
}

// updates the mark price on an open position under the executor's mutex
func (e *LiveExecutor) UpdateMarkPrice(posID string, price float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if pos, ok := e.positions[posID]; ok {
		pos.MarkPrice = price
	}
}
