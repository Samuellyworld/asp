// trailing stop logic for dynamic stop losses that follow favorable price movement.
// call Update() on every price tick to adjust the stop level.
package trailingstop

// TrailingStop tracks a dynamic stop loss that follows price upward (for longs)
// or downward (for shorts). The stop never moves backward.
type TrailingStop struct {
	// TrailPercent is the trailing distance as a percentage (e.g. 2.0 for 2%).
	// If zero, trailing stop is disabled.
	TrailPercent float64

	// ActivationPct is the minimum profit percentage before trailing begins.
	// If zero, trailing starts immediately.
	ActivationPct float64

	// HighWaterMark tracks the highest price seen (used for longs).
	HighWaterMark float64

	// LowWaterMark tracks the lowest price seen (used for shorts).
	LowWaterMark float64

	// StopPrice is the current computed trailing stop level.
	StopPrice float64

	// Activated indicates whether the trailing stop has been triggered.
	Activated bool
}

// Enabled returns true if trailing stop is configured.
func (ts *TrailingStop) Enabled() bool {
	return ts.TrailPercent > 0
}

// UpdateLong recalculates the trailing stop for a long position.
// entryPrice is the original entry, currentPrice is the latest tick.
// Returns the new stop price and whether the stop was updated.
func (ts *TrailingStop) UpdateLong(entryPrice, currentPrice float64) (float64, bool) {
	if !ts.Enabled() || entryPrice == 0 {
		return ts.StopPrice, false
	}

	// check activation threshold
	if ts.ActivationPct > 0 {
		profitPct := ((currentPrice - entryPrice) / entryPrice) * 100
		if profitPct < ts.ActivationPct {
			return ts.StopPrice, false
		}
		if !ts.Activated {
			ts.Activated = true
			ts.HighWaterMark = currentPrice
		}
	}

	// update high water mark
	if currentPrice > ts.HighWaterMark {
		ts.HighWaterMark = currentPrice
	}

	// compute new trailing stop
	newStop := ts.HighWaterMark * (1 - ts.TrailPercent/100)

	// stop never moves backward
	if newStop > ts.StopPrice {
		ts.StopPrice = newStop
		return ts.StopPrice, true
	}

	return ts.StopPrice, false
}

// UpdateShort recalculates the trailing stop for a short position.
// entryPrice is the original entry, currentPrice is the latest tick.
// Returns the new stop price and whether the stop was updated.
func (ts *TrailingStop) UpdateShort(entryPrice, currentPrice float64) (float64, bool) {
	if !ts.Enabled() || entryPrice == 0 {
		return ts.StopPrice, false
	}

	// check activation threshold
	if ts.ActivationPct > 0 {
		profitPct := ((entryPrice - currentPrice) / entryPrice) * 100
		if profitPct < ts.ActivationPct {
			return ts.StopPrice, false
		}
		if !ts.Activated {
			ts.Activated = true
			ts.LowWaterMark = currentPrice
		}
	}

	// update low water mark (track lowest price for shorts)
	if ts.LowWaterMark == 0 || currentPrice < ts.LowWaterMark {
		ts.LowWaterMark = currentPrice
	}

	// compute new trailing stop
	newStop := ts.LowWaterMark * (1 + ts.TrailPercent/100)

	// stop never moves backward (for shorts, lower is better)
	if ts.StopPrice == 0 || newStop < ts.StopPrice {
		ts.StopPrice = newStop
		return ts.StopPrice, true
	}

	return ts.StopPrice, false
}

// IsHit returns true if the current price has triggered the trailing stop.
func (ts *TrailingStop) IsHitLong(currentPrice float64) bool {
	if !ts.Enabled() || ts.StopPrice == 0 {
		return false
	}
	return currentPrice <= ts.StopPrice
}

// IsHitShort returns true if the current price has triggered the trailing stop (short).
func (ts *TrailingStop) IsHitShort(currentPrice float64) bool {
	if !ts.Enabled() || ts.StopPrice == 0 {
		return false
	}
	return currentPrice >= ts.StopPrice
}
