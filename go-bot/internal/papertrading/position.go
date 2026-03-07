// paper trading position types and p&l calculations.
// tracks virtual positions with entry/exit prices, milestones, and lifecycle state.
package papertrading

import (
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
)

// position lifecycle state
type PositionStatus string

const (
	PositionOpen   PositionStatus = "open"
	PositionClosed PositionStatus = "closed"
)

// reason a position was closed
type CloseReason string

const (
	CloseTP     CloseReason = "take_profit"
	CloseSL     CloseReason = "stop_loss"
	CloseManual CloseReason = "manual"
)

// default milestone thresholds (percentage)
var (
	DefaultProfitMilestones = []float64{1.0, 2.0, 3.0, 5.0}
	DefaultLossMilestones   = []float64{-0.5, -1.0, -1.5}

	// ai suggestions trigger at or above this milestone
	AITriggerThreshold = 2.0
)

// a virtual trading position tracked by the paper executor
type Position struct {
	ID            string
	UserID        int
	Symbol        string
	Action        claude.Action
	EntryPrice    float64
	CurrentPrice  float64
	Quantity      float64
	StopLoss      float64
	TakeProfit    float64
	PositionSize  float64 // notional value in usd
	Status        PositionStatus
	CloseReason   CloseReason
	ClosePrice    float64
	OpenedAt      time.Time
	ClosedAt      *time.Time
	HitMilestones map[float64]bool
	LastNotified  time.Time
	Platform      string // "telegram" or "discord"
}

// unrealized profit/loss based on current price
func (p *Position) PnL() float64 {
	if p.Action == claude.ActionBuy {
		return (p.CurrentPrice - p.EntryPrice) * p.Quantity
	}
	return (p.EntryPrice - p.CurrentPrice) * p.Quantity
}

// unrealized p&l as a percentage of entry
func (p *Position) PnLPercent() float64 {
	if p.EntryPrice == 0 {
		return 0
	}
	if p.Action == claude.ActionBuy {
		return ((p.CurrentPrice - p.EntryPrice) / p.EntryPrice) * 100
	}
	return ((p.EntryPrice - p.CurrentPrice) / p.EntryPrice) * 100
}

// checks if take profit level has been reached
func (p *Position) IsTPHit() bool {
	if p.TakeProfit == 0 {
		return false
	}
	if p.Action == claude.ActionBuy {
		return p.CurrentPrice >= p.TakeProfit
	}
	return p.CurrentPrice <= p.TakeProfit
}

// checks if stop loss level has been triggered
func (p *Position) IsSLHit() bool {
	if p.StopLoss == 0 {
		return false
	}
	if p.Action == claude.ActionBuy {
		return p.CurrentPrice <= p.StopLoss
	}
	return p.CurrentPrice >= p.StopLoss
}

// returns milestone thresholds that were just reached but not yet recorded
func (p *Position) NewMilestones() []float64 {
	pctChange := p.PnLPercent()
	var triggered []float64

	for _, m := range DefaultProfitMilestones {
		if p.HitMilestones[m] {
			continue
		}
		if pctChange >= m {
			triggered = append(triggered, m)
		}
	}
	for _, m := range DefaultLossMilestones {
		if p.HitMilestones[m] {
			continue
		}
		if pctChange <= m {
			triggered = append(triggered, m)
		}
	}
	return triggered
}

// realized profit/loss after the position is closed
func (p *Position) ClosedPnL() float64 {
	if p.Action == claude.ActionBuy {
		return (p.ClosePrice - p.EntryPrice) * p.Quantity
	}
	return (p.EntryPrice - p.ClosePrice) * p.Quantity
}

// realized p&l as a percentage
func (p *Position) ClosedPnLPercent() float64 {
	if p.EntryPrice == 0 {
		return 0
	}
	if p.Action == claude.ActionBuy {
		return ((p.ClosePrice - p.EntryPrice) / p.EntryPrice) * 100
	}
	return ((p.EntryPrice - p.ClosePrice) / p.EntryPrice) * 100
}

// aggregated daily trading performance
type DailySummary struct {
	UserID        int
	Date          time.Time
	OpenCount     int
	ClosedCount   int
	Wins          int
	Losses        int
	TotalPnL      float64
	BestTrade     *Position
	WorstTrade    *Position
	OpenPositions []*Position
}
