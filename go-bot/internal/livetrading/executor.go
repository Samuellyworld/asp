// live trade executor. decrypts user keys, runs safety checks,
// and places real orders on binance with sl/tp.
package livetrading

import (
	"fmt"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/opportunity"
)

// decrypts stored credentials for order placement
type KeyDecryptor interface {
	DecryptKeys(userID int) (apiKey, apiSecret string, err error)
}

// a live position with real exchange order ids
type LivePosition struct {
	ID            string
	UserID        int
	Symbol        string
	Side          exchange.OrderSide
	EntryPrice    float64
	Quantity      float64
	PositionSize  float64
	StopLoss      float64
	TakeProfit    float64
	MainOrderID   int64
	SLOrderID     int64
	TPOrderID     int64
	Status        string // "open", "closed"
	CloseReason   string
	ClosePrice    float64
	PnL           float64
	OpenedAt      time.Time
	ClosedAt      *time.Time
	Platform      string
}

// executor configuration
type ExecutorConfig struct {
	Safety SafetyConfig
}

// executes real trades on the exchange with full safety validation
type Executor struct {
	mu        sync.RWMutex
	positions map[string]*LivePosition
	closed    []*LivePosition
	orders    exchange.OrderExecutor
	keys      KeyDecryptor
	safety    *SafetyChecker
	losses    LossTracker
	nextID    int
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

// opens a live position from an approved opportunity.
// decrypts keys, runs safety checks, places market order + sl + tp orders.
func (e *Executor) Execute(opp *opportunity.Opportunity) (*LivePosition, error) {
	if opp.Status != opportunity.StatusApproved && opp.Status != opportunity.StatusModified {
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

	// determine order side and quote asset
	side := exchange.SideBuy
	closeSide := exchange.SideSell
	if opp.Action == claude.ActionSell {
		side = exchange.SideSell
		closeSide = exchange.SideBuy
	}
	asset := "USDT"

	// run safety checks
	if e.safety != nil {
		result := e.safety.Check(opp.UserID, opp.Symbol, plan.PositionSize, asset)
		if !result.Passed {
			return nil, fmt.Errorf("safety check failed: %s", result.Blocked)
		}
	}

	// decrypt user api keys
	apiKey, apiSecret, err := e.keys.DecryptKeys(opp.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt keys: %w", err)
	}

	// place main market order
	mainOrder, err := e.orders.PlaceOrder(
		opp.Symbol, side, exchange.OrderTypeMarket,
		0, 0, // quantity and price determined by exchange for market orders
		apiKey, apiSecret,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	quantity := mainOrder.ExecutedQty
	if quantity <= 0 {
		quantity = plan.PositionSize / mainOrder.AvgPrice
	}

	// place stop loss order
	var slOrderID int64
	if plan.StopLoss > 0 {
		slOrder, err := e.orders.PlaceStopLoss(
			opp.Symbol, closeSide, quantity,
			plan.StopLoss, plan.StopLoss,
			apiKey, apiSecret,
		)
		if err == nil {
			slOrderID = slOrder.OrderID
		}
	}

	// place take profit order
	var tpOrderID int64
	if plan.TakeProfit > 0 {
		tpOrder, err := e.orders.PlaceTakeProfit(
			opp.Symbol, closeSide, quantity,
			plan.TakeProfit, plan.TakeProfit,
			apiKey, apiSecret,
		)
		if err == nil {
			tpOrderID = tpOrder.OrderID
		}
	}

	e.mu.Lock()
	e.nextID++
	id := fmt.Sprintf("live_%d", e.nextID)

	pos := &LivePosition{
		ID:           id,
		UserID:       opp.UserID,
		Symbol:       opp.Symbol,
		Side:         side,
		EntryPrice:   mainOrder.AvgPrice,
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

	return pos, nil
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

	apiKey, apiSecret, err := e.keys.DecryptKeys(pos.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt keys: %w", err)
	}

	// cancel existing sl/tp orders
	if pos.SLOrderID > 0 {
		_ = e.orders.CancelOrder(pos.Symbol, pos.SLOrderID, apiKey, apiSecret)
	}
	if pos.TPOrderID > 0 {
		_ = e.orders.CancelOrder(pos.Symbol, pos.TPOrderID, apiKey, apiSecret)
	}

	// place closing market order
	closeSide := exchange.SideSell
	if pos.Side == exchange.SideSell {
		closeSide = exchange.SideBuy
	}

	closeOrder, err := e.orders.PlaceOrder(
		pos.Symbol, closeSide, exchange.OrderTypeMarket,
		pos.Quantity, 0,
		apiKey, apiSecret,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to close position: %w", err)
	}

	e.mu.Lock()
	now := time.Now()
	pos.Status = "closed"
	pos.CloseReason = reason
	pos.ClosePrice = closeOrder.AvgPrice
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
	delete(e.positions, posID)
	e.mu.Unlock()

	return pos, nil
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
