// paper trade executor. simulates fills at current market price and manages
// virtual positions with open/close/adjust operations.
package papertrading

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/circuitbreaker"
	"github.com/trading-bot/go-bot/internal/opportunity"
)

// dbCtx returns a context with a 5-second timeout for best-effort DB operations
func dbCtx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	return ctx
}

// provides current market prices for position management
type PriceProvider interface {
	GetPrice(symbol string) (float64, error)
}

// optional persistence layer for paper positions (nil = in-memory only)
type PositionStore interface {
	SavePosition(ctx context.Context, pos *Position) error
	ClosePosition(ctx context.Context, pos *Position) error
	AdjustPosition(ctx context.Context, posID string, sl, tp float64) error
}

// optional trade record logger (nil = no logging)
type TradeLogger interface {
	LogOpen(ctx context.Context, pos *Position) error
	LogClose(ctx context.Context, pos *Position) error
}

// manages virtual paper trading positions
type Executor struct {
	mu        sync.RWMutex
	positions map[string]*Position
	closed    []*Position
	prices    PriceProvider
	store     PositionStore // nil if no persistence configured
	trades    TradeLogger   // nil if no logging configured
	breaker   *circuitbreaker.Breaker // nil if no circuit breaker configured
	nextID    int
}

func NewExecutor(prices PriceProvider) *Executor {
	return &Executor{
		positions: make(map[string]*Position),
		prices:    prices,
	}
}

// SetStore configures position persistence. Call before Start.
func (e *Executor) SetStore(store PositionStore) {
	e.store = store
}

// SetTradeLogger configures trade record logging. Call before Start.
func (e *Executor) SetTradeLogger(logger TradeLogger) {
	e.trades = logger
}

// SetCircuitBreaker configures portfolio circuit breaker. Call before Start.
func (e *Executor) SetCircuitBreaker(b *circuitbreaker.Breaker) {
	e.breaker = b
}

// SetNextID sets the starting ID for new positions (used for recovery).
func (e *Executor) SetNextID(id int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextID = id
}

// RestorePosition adds a recovered position back into the in-memory map (startup only).
func (e *Executor) RestorePosition(pos *Position) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.positions[pos.ID] = pos
}

// opens a paper position from an approved or modified opportunity.
// simulates an immediate fill at the current market price.
func (e *Executor) Execute(opp *opportunity.Opportunity) (*Position, error) {
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

	// check circuit breaker before executing
	if e.breaker != nil {
		if ok, reason := e.breaker.AllowTrade(opp.UserID); !ok {
			return nil, fmt.Errorf("circuit breaker: %s", reason)
		}
	}

	price, err := e.prices.GetPrice(opp.Symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get price for %s: %w", opp.Symbol, err)
	}

	if plan.PositionSize <= 0 {
		return nil, fmt.Errorf("invalid position size: %.2f", plan.PositionSize)
	}

	quantity := plan.PositionSize / price
	if quantity <= 0 {
		return nil, fmt.Errorf("calculated quantity is zero")
	}

	e.mu.Lock()
	e.nextID++
	id := fmt.Sprintf("pt_%d", e.nextID)

	pos := &Position{
		ID:            id,
		UserID:        opp.UserID,
		Symbol:        opp.Symbol,
		Action:        opp.Action,
		EntryPrice:    price,
		CurrentPrice:  price,
		Quantity:      quantity,
		StopLoss:      plan.StopLoss,
		TakeProfit:    plan.TakeProfit,
		PositionSize:  plan.PositionSize,
		Status:        PositionOpen,
		OpenedAt:      time.Now(),
		HitMilestones: make(map[float64]bool),
		Platform:      opp.Platform,
	}

	e.positions[id] = pos
	e.mu.Unlock()

	// persist to database (best-effort — don't fail the trade on DB error)
	if e.store != nil {
		if err := e.store.SavePosition(dbCtx(), pos); err != nil {
			slog.Error("failed to persist paper position", "id", id, "error", err)
		}
	}

	// log trade record (best-effort)
	if e.trades != nil {
		if err := e.trades.LogOpen(dbCtx(), pos); err != nil {
			slog.Error("failed to log paper trade open", "id", id, "error", err)
		}
	}

	return pos, nil
}

// closes a position with the given reason and final price
func (e *Executor) Close(posID string, reason CloseReason, price float64) (*Position, error) {
	e.mu.Lock()

	pos, ok := e.positions[posID]
	if !ok {
		e.mu.Unlock()
		return nil, fmt.Errorf("position not found: %s", posID)
	}

	if pos.Status == PositionClosed {
		e.mu.Unlock()
		return nil, fmt.Errorf("position already closed: %s", posID)
	}

	now := time.Now()
	pos.Status = PositionClosed
	pos.CloseReason = reason
	pos.ClosePrice = price
	pos.CurrentPrice = price
	pos.ClosedAt = &now

	e.closed = append(e.closed, pos)
	delete(e.positions, posID)
	e.mu.Unlock()

	// persist close to database
	if e.store != nil {
		if err := e.store.ClosePosition(dbCtx(), pos); err != nil {
			slog.Error("failed to persist paper position close", "id", posID, "error", err)
		}
	}

	// log trade close record (best-effort)
	if e.trades != nil {
		if err := e.trades.LogClose(dbCtx(), pos); err != nil {
			slog.Error("failed to log paper trade close", "id", posID, "error", err)
		}
	}

	return pos, nil
}

// modifies the stop loss or take profit on an open position
func (e *Executor) Adjust(posID string, field string, value float64) error {
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

	// persist adjustment (best-effort)
	if e.store != nil {
		if err := e.store.AdjustPosition(dbCtx(), posID, pos.StopLoss, pos.TakeProfit); err != nil {
			slog.Error("failed to persist paper position adjust", "id", posID, "error", err)
		}
	}

	return nil
}

// returns a position by id (open or closed)
func (e *Executor) Get(posID string) *Position {
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
func (e *Executor) OpenPositions(userID int) []*Position {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []*Position
	for _, pos := range e.positions {
		if pos.UserID == userID {
			result = append(result, pos)
		}
	}
	return result
}

// returns all currently open positions across all users
func (e *Executor) AllOpen() []*Position {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*Position, 0, len(e.positions))
	for _, pos := range e.positions {
		result = append(result, pos)
	}
	return result
}

// returns closed positions for a specific user
func (e *Executor) ClosedPositions(userID int) []*Position {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []*Position
	for _, pos := range e.closed {
		if pos.UserID == userID {
			result = append(result, pos)
		}
	}
	return result
}

// updates the current market price on a position
func (e *Executor) UpdatePrice(posID string, price float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if pos, ok := e.positions[posID]; ok {
		pos.CurrentPrice = price
	}
}

// generates a daily summary of trading activity for a user
func (e *Executor) Summary(userID int, date time.Time) *DailySummary {
	e.mu.RLock()
	defer e.mu.RUnlock()

	summary := &DailySummary{
		UserID: userID,
		Date:   date,
	}

	for _, pos := range e.positions {
		if pos.UserID == userID {
			summary.OpenCount++
			summary.OpenPositions = append(summary.OpenPositions, pos)
		}
	}

	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	for _, pos := range e.closed {
		if pos.UserID != userID {
			continue
		}
		if pos.ClosedAt == nil || pos.ClosedAt.Before(dayStart) || !pos.ClosedAt.Before(dayEnd) {
			continue
		}

		summary.ClosedCount++
		pnl := pos.ClosedPnL()
		summary.TotalPnL += pnl

		if pnl > 0 {
			summary.Wins++
		} else {
			summary.Losses++
		}

		if summary.BestTrade == nil || pnl > summary.BestTrade.ClosedPnL() {
			summary.BestTrade = pos
		}
		if summary.WorstTrade == nil || pnl < summary.WorstTrade.ClosedPnL() {
			summary.WorstTrade = pos
		}
	}

	return summary
}

// returns total count of open positions
func (e *Executor) Count() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.positions)
}
