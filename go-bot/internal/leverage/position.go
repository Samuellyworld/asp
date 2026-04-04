// leveraged position types for futures trading (paper and live).
// provides pnl calculations, tp/sl hit detection, and auto-close logic.
package leverage

import (
	"time"

	"github.com/trading-bot/go-bot/internal/trailingstop"
)

// position side for futures
type PositionSide string

const (
	SideLong  PositionSide = "LONG"
	SideShort PositionSide = "SHORT"
)

// a leveraged position (paper or live)
type LeveragePosition struct {
	ID               string
	UserID           int
	Symbol           string
	Side             PositionSide
	Leverage         int
	EntryPrice       float64
	MarkPrice        float64
	Quantity         float64
	Margin           float64    // initial margin = notional / leverage
	NotionalValue    float64    // quantity * entry price
	LiquidationPrice float64
	StopLoss         float64
	TakeProfit       float64
	FundingPaid      float64    // cumulative funding fees
	MarginType       string     // "isolated"
	IsPaper          bool
	Status           string     // "open", "closed"
	CloseReason      string
	ClosePrice       float64
	PnL              float64
	OpenedAt         time.Time
	ClosedAt         *time.Time
	Platform         string
	MainOrderID      int64 // binance order id (0 for paper)
	SLOrderID        int64
	TPOrderID        int64
	TrailingStop     trailingstop.TrailingStop
}

// unrealized pnl based on mark price (amplified by leverage)
func (p *LeveragePosition) UnrealizedPnL() float64 {
	switch p.Side {
	case SideLong:
		return (p.MarkPrice - p.EntryPrice) * p.Quantity
	case SideShort:
		return (p.EntryPrice - p.MarkPrice) * p.Quantity
	default:
		return 0
	}
}

// unrealized pnl as a percentage of margin
func (p *LeveragePosition) UnrealizedPnLPercent() float64 {
	if p.Margin == 0 {
		return 0
	}
	return (p.UnrealizedPnL() / p.Margin) * 100
}

// return on margin (ROI) — how much margin is gained/lost percentage-wise
func (p *LeveragePosition) ROI() float64 {
	if p.Margin == 0 {
		return 0
	}
	return (p.UnrealizedPnL() / p.Margin) * 100
}

// checks if the mark price has hit the take profit level
func (p *LeveragePosition) IsTPHit() bool {
	if p.TakeProfit == 0 {
		return false
	}
	switch p.Side {
	case SideLong:
		return p.MarkPrice >= p.TakeProfit
	case SideShort:
		return p.MarkPrice <= p.TakeProfit
	default:
		return false
	}
}

// checks if the mark price has hit the stop loss level
func (p *LeveragePosition) IsSLHit() bool {
	if p.StopLoss == 0 {
		return false
	}
	switch p.Side {
	case SideLong:
		return p.MarkPrice <= p.StopLoss
	case SideShort:
		return p.MarkPrice >= p.StopLoss
	default:
		return false
	}
}

// checks if position should be auto-closed based on liquidation proximity
func (p *LeveragePosition) ShouldAutoClose(maintenanceMarginRate float64) bool {
	dist := DistanceToLiquidation(p.MarkPrice, p.LiquidationPrice, string(p.Side))
	risk := ClassifyLiquidationRisk(dist)
	return risk == AlertAutoClose
}

// checks if the trailing stop has been triggered
func (p *LeveragePosition) IsTrailingStopHit() bool {
	if !p.TrailingStop.Enabled() {
		return false
	}
	switch p.Side {
	case SideLong:
		return p.TrailingStop.IsHitLong(p.MarkPrice)
	case SideShort:
		return p.TrailingStop.IsHitShort(p.MarkPrice)
	default:
		return false
	}
}

// updates the trailing stop based on current mark price
func (p *LeveragePosition) UpdateTrailingStop() bool {
	if !p.TrailingStop.Enabled() {
		return false
	}
	switch p.Side {
	case SideLong:
		_, updated := p.TrailingStop.UpdateLong(p.EntryPrice, p.MarkPrice)
		return updated
	case SideShort:
		_, updated := p.TrailingStop.UpdateShort(p.EntryPrice, p.MarkPrice)
		return updated
	default:
		return false
	}
}
