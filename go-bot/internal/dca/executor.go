// package dca implements dollar-cost-averaging strategy for staged position entry.
// it wraps existing executors to split a single trade plan into multiple rounds.
package dca

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/opportunity"
)

// Config controls how DCA splits entries
type Config struct {
	Rounds        int           // number of entry rounds (default 3)
	Interval      time.Duration // time between rounds (default 1h)
	ScaleFactor   float64       // multiplier per round (1.0 = equal, 1.5 = 50% more each round)
	PriceDropPct  float64       // only enter next round if price drops this % (0 = time-only)
	MaxTotalSize  float64       // cap on total position size in USD
}

// DefaultConfig returns sensible DCA defaults
func DefaultConfig() Config {
	return Config{
		Rounds:       3,
		Interval:     1 * time.Hour,
		ScaleFactor:  1.0,
		PriceDropPct: 0,
		MaxTotalSize: 1500,
	}
}

// Round represents a single DCA entry
type Round struct {
	Number     int
	Size       float64 // USD amount for this round
	Executed   bool
	Price      float64 // execution price (0 if not yet executed)
	ExecutedAt *time.Time
	Skipped    bool   // true if price condition not met
	Error      string // non-empty if execution failed
}

// Plan is the full DCA schedule for an opportunity
type Plan struct {
	ID            string
	OpportunityID string
	Symbol        string
	Action        claude.Action
	Rounds        []Round
	StopLoss      float64
	TakeProfit    float64
	TotalSize     float64 // sum of all round sizes
	TotalFilled   float64 // sum of executed sizes
	AvgEntryPrice float64
	Status        string  // "active", "completed", "cancelled"
	CreatedAt     time.Time
}

// PriceProvider gets current price for DCA condition checks
type PriceProvider interface {
	GetPrice(ctx context.Context, symbol string) (*exchange.Ticker, error)
}

// OrderPlacer places individual orders (wraps exchange with credentials)
type OrderPlacer interface {
	PlaceMarketOrder(ctx context.Context, symbol string, side exchange.OrderSide, quantity float64) (*exchange.Order, error)
}

// Callback is called after each round executes
type Callback func(plan *Plan, round *Round)

// Executor manages DCA plans and executes rounds on schedule
type Executor struct {
	mu       sync.RWMutex
	plans    map[string]*Plan
	config   Config
	price    PriceProvider
	stopCh   chan struct{}
	running  bool
	onRound  Callback
}

// NewExecutor creates a DCA executor
func NewExecutor(cfg Config, price PriceProvider) *Executor {
	return &Executor{
		plans:  make(map[string]*Plan),
		config: cfg,
		price:  price,
		stopCh: make(chan struct{}),
	}
}

// OnRound sets a callback for after each round executes
func (e *Executor) OnRound(cb Callback) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onRound = cb
}

// CreatePlan builds a DCA plan from an approved opportunity
func (e *Executor) CreatePlan(opp *opportunity.Opportunity) (*Plan, error) {
	if opp.Result == nil || opp.Result.Decision == nil {
		return nil, fmt.Errorf("opportunity has no decision")
	}

	totalSize := opp.Result.Decision.Plan.PositionSize
	if totalSize > e.config.MaxTotalSize {
		totalSize = e.config.MaxTotalSize
	}

	rounds := e.computeRoundSizes(totalSize)

	plan := &Plan{
		ID:            fmt.Sprintf("dca_%s", opp.ID),
		OpportunityID: opp.ID,
		Symbol:        opp.Symbol,
		Action:        opp.Result.Decision.Action,
		Rounds:        rounds,
		StopLoss:      opp.Result.Decision.Plan.StopLoss,
		TakeProfit:    opp.Result.Decision.Plan.TakeProfit,
		TotalSize:     totalSize,
		Status:        "active",
		CreatedAt:     time.Now(),
	}

	e.mu.Lock()
	e.plans[plan.ID] = plan
	e.mu.Unlock()

	return plan, nil
}

// computeRoundSizes distributes total size across rounds using the scale factor
func (e *Executor) computeRoundSizes(totalSize float64) []Round {
	n := e.config.Rounds
	if n <= 0 {
		n = 1
	}

	// compute raw weights: 1, scaleFactor, scaleFactor^2, ...
	weights := make([]float64, n)
	totalWeight := 0.0
	for i := 0; i < n; i++ {
		weights[i] = math.Pow(e.config.ScaleFactor, float64(i))
		totalWeight += weights[i]
	}

	rounds := make([]Round, n)
	for i := 0; i < n; i++ {
		rounds[i] = Round{
			Number: i + 1,
			Size:   totalSize * weights[i] / totalWeight,
		}
	}
	return rounds
}

// GetPlan retrieves a DCA plan by ID
func (e *Executor) GetPlan(id string) *Plan {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.plans[id]
}

// GetPlanForOpportunity finds the DCA plan for an opportunity
func (e *Executor) GetPlanForOpportunity(oppID string) *Plan {
	e.mu.RLock()
	defer e.mu.RUnlock()
	id := fmt.Sprintf("dca_%s", oppID)
	return e.plans[id]
}

// CancelPlan stops a DCA plan from executing further rounds
func (e *Executor) CancelPlan(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	plan, ok := e.plans[id]
	if !ok || plan.Status != "active" {
		return false
	}
	plan.Status = "cancelled"
	return true
}

// ExecuteRound executes the next pending round for a plan
func (e *Executor) ExecuteRound(ctx context.Context, plan *Plan, placer OrderPlacer) error {
	e.mu.Lock()
	var round *Round
	for i := range plan.Rounds {
		if !plan.Rounds[i].Executed && !plan.Rounds[i].Skipped {
			round = &plan.Rounds[i]
			break
		}
	}
	if round == nil {
		plan.Status = "completed"
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()

	// check price condition
	if e.config.PriceDropPct > 0 && plan.AvgEntryPrice > 0 {
		ticker, err := e.price.GetPrice(ctx, plan.Symbol)
		if err != nil {
			round.Error = fmt.Sprintf("price check failed: %v", err)
			return fmt.Errorf("price check: %w", err)
		}
		requiredDrop := plan.AvgEntryPrice * (1 - e.config.PriceDropPct/100)
		if plan.Action == claude.ActionBuy && ticker.Price > requiredDrop {
			round.Skipped = true
			return nil
		}
	}

	// determine order side
	side := exchange.SideBuy
	if plan.Action == claude.ActionSell {
		side = exchange.SideSell
	}

	// get current price for quantity calculation
	ticker, err := e.price.GetPrice(ctx, plan.Symbol)
	if err != nil {
		round.Error = fmt.Sprintf("price fetch failed: %v", err)
		return fmt.Errorf("price fetch: %w", err)
	}

	quantity := round.Size / ticker.Price
	order, err := placer.PlaceMarketOrder(ctx, plan.Symbol, side, quantity)
	if err != nil {
		round.Error = fmt.Sprintf("order failed: %v", err)
		return fmt.Errorf("place order round %d: %w", round.Number, err)
	}

	now := time.Now()
	round.Executed = true
	round.Price = order.AvgPrice
	round.ExecutedAt = &now

	// update plan totals
	e.mu.Lock()
	plan.TotalFilled += round.Size
	// recalculate average entry price
	totalQty := 0.0
	totalCost := 0.0
	for _, r := range plan.Rounds {
		if r.Executed {
			qty := r.Size / r.Price
			totalQty += qty
			totalCost += r.Size
		}
	}
	if totalQty > 0 {
		plan.AvgEntryPrice = totalCost / totalQty
	}

	// check if all rounds done
	allDone := true
	for _, r := range plan.Rounds {
		if !r.Executed && !r.Skipped {
			allDone = false
			break
		}
	}
	if allDone {
		plan.Status = "completed"
	}
	cb := e.onRound
	e.mu.Unlock()

	if cb != nil {
		cb(plan, round)
	}

	return nil
}

// Start begins the background DCA scheduler
func (e *Executor) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.mu.Unlock()

	go e.loop()
}

// Stop halts the background DCA scheduler
func (e *Executor) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return
	}
	e.running = false
	close(e.stopCh)
}

// loop checks for pending rounds at the configured interval
func (e *Executor) loop() {
	ticker := time.NewTicker(e.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.tickPlans()
		}
	}
}

// tickPlans finds active plans with pending rounds and logs them
// actual execution requires an OrderPlacer, so the scheduler signals readiness
func (e *Executor) tickPlans() {
	e.mu.RLock()
	var activePlans []*Plan
	for _, plan := range e.plans {
		if plan.Status == "active" {
			activePlans = append(activePlans, plan)
		}
	}
	e.mu.RUnlock()

	for _, plan := range activePlans {
		// check if enough time has passed since last execution
		var lastExec time.Time
		for _, r := range plan.Rounds {
			if r.ExecutedAt != nil && r.ExecutedAt.After(lastExec) {
				lastExec = *r.ExecutedAt
			}
		}
		if lastExec.IsZero() || time.Since(lastExec) >= e.config.Interval {
			// plan is ready for next round — handled by the caller via ExecuteRound
			// we just track the readiness state
			_ = plan // placeholder for notification/callback integration
		}
	}
}

// ActivePlans returns all currently active DCA plans
func (e *Executor) ActivePlans() []*Plan {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var result []*Plan
	for _, plan := range e.plans {
		if plan.Status == "active" {
			result = append(result, plan)
		}
	}
	return result
}

// Stats returns DCA statistics
func (e *Executor) Stats() map[string]int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	stats := map[string]int{
		"active":    0,
		"completed": 0,
		"cancelled": 0,
	}
	for _, plan := range e.plans {
		stats[plan.Status]++
	}
	return stats
}
