// paper leverage executor. simulates futures/leverage trades without hitting
// the exchange. manages virtual leveraged positions with open/close/adjust.
package leverage

import (
	"fmt"
	"sync"
	"time"
)

// provides current market prices for position management
type PriceProvider interface {
	GetPrice(symbol string) (float64, error)
}

// manages simulated leverage positions for paper trading
type PaperExecutor struct {
	mu        sync.RWMutex
	positions map[string]*LeveragePosition
	closed    []*LeveragePosition
	prices    PriceProvider
	safety    *SafetyChecker
	funding   *FundingTracker
	nextID    int
}

// creates a new paper executor with the given dependencies
func NewPaperExecutor(prices PriceProvider, safety *SafetyChecker, funding *FundingTracker) *PaperExecutor {
	return &PaperExecutor{
		positions: make(map[string]*LeveragePosition),
		prices:    prices,
		safety:    safety,
		funding:   funding,
	}
}

// opens a simulated leverage position at the current market price.
// runs safety checks if a safety checker is configured.
func (e *PaperExecutor) OpenPosition(
	userID int,
	symbol string,
	side PositionSide,
	leverage int,
	margin float64,
	stopLoss, takeProfit float64,
	platform string,
) (*LeveragePosition, error) {
	if margin <= 0 {
		return nil, fmt.Errorf("margin must be positive, got %.2f", margin)
	}
	if leverage <= 0 {
		return nil, fmt.Errorf("leverage must be positive, got %d", leverage)
	}

	price, err := e.prices.GetPrice(symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get price for %s: %w", symbol, err)
	}

	// run safety checks if configured
	if e.safety != nil {
		result := e.safety.Check(userID, symbol, leverage, margin, price, string(side))
		if !result.Passed {
			return nil, fmt.Errorf("safety check failed: %s", result.Blocked)
		}
	}

	notional := margin * float64(leverage)
	quantity := notional / price
	liqPrice := CalculateLiquidationPrice(price, leverage, string(side), DefaultMaintenanceMarginRate)

	e.mu.Lock()
	e.nextID++
	id := fmt.Sprintf("lp_%d", e.nextID)

	pos := &LeveragePosition{
		ID:               id,
		UserID:           userID,
		Symbol:           symbol,
		Side:             side,
		Leverage:         leverage,
		EntryPrice:       price,
		MarkPrice:        price,
		Quantity:         quantity,
		Margin:           margin,
		NotionalValue:    notional,
		LiquidationPrice: liqPrice,
		StopLoss:         stopLoss,
		TakeProfit:       takeProfit,
		MarginType:       "isolated",
		IsPaper:          true,
		Status:           "open",
		OpenedAt:         time.Now(),
		Platform:         platform,
	}

	e.positions[id] = pos
	e.mu.Unlock()

	return pos, nil
}

// closes a position with the given reason. calculates final pnl based on
// current market price, position side, and accumulated funding fees.
func (e *PaperExecutor) Close(posID string, reason string) (*LeveragePosition, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	pos, ok := e.positions[posID]
	if !ok {
		// check if already in closed list
		for _, cp := range e.closed {
			if cp.ID == posID {
				return nil, fmt.Errorf("position already closed: %s", posID)
			}
		}
		return nil, fmt.Errorf("position not found: %s", posID)
	}

	if pos.Status == "closed" {
		return nil, fmt.Errorf("position already closed: %s", posID)
	}

	// get current price for pnl calculation (unlock temporarily)
	e.mu.Unlock()
	closePrice, err := e.prices.GetPrice(pos.Symbol)
	e.mu.Lock()
	if err != nil {
		return nil, fmt.Errorf("failed to get close price for %s: %w", pos.Symbol, err)
	}

	// calculate raw pnl based on side
	var rawPnL float64
	switch pos.Side {
	case SideLong:
		rawPnL = (closePrice - pos.EntryPrice) * pos.Quantity
	case SideShort:
		rawPnL = (pos.EntryPrice - closePrice) * pos.Quantity
	}

	// subtract funding fees from pnl
	pos.PnL = rawPnL - pos.FundingPaid

	now := time.Now()
	pos.Status = "closed"
	pos.CloseReason = reason
	pos.ClosePrice = closePrice
	pos.MarkPrice = closePrice
	pos.ClosedAt = &now

	e.closed = append(e.closed, pos)
	delete(e.positions, posID)

	return pos, nil
}

// modifies the stop loss or take profit on an open position
func (e *PaperExecutor) Adjust(posID string, field string, value float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	pos, ok := e.positions[posID]
	if !ok {
		return fmt.Errorf("position not found: %s", posID)
	}

	switch field {
	case "sl", "stop_loss":
		pos.StopLoss = value
	case "tp", "take_profit":
		pos.TakeProfit = value
	default:
		return fmt.Errorf("unknown field: %s", field)
	}

	return nil
}

// returns a position by id (open or closed)
func (e *PaperExecutor) Get(posID string) *LeveragePosition {
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
func (e *PaperExecutor) OpenPositions(userID int) []*LeveragePosition {
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

// returns all currently open positions across all users
func (e *PaperExecutor) AllOpen() []*LeveragePosition {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*LeveragePosition, 0, len(e.positions))
	for _, pos := range e.positions {
		result = append(result, pos)
	}
	return result
}

// updates the mark price on an open position
func (e *PaperExecutor) UpdateMarkPrice(posID string, price float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if pos, ok := e.positions[posID]; ok {
		pos.MarkPrice = price
	}
}

// returns total count of open positions
func (e *PaperExecutor) Count() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.positions)
}
