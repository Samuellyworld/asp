// liquidation price calculation and risk classification for binance futures.
// implements the standard isolated-margin formulas with configurable
// maintenance margin rates.
package leverage

import "math"

// default maintenance margin rate for the lowest binance futures tier
const DefaultMaintenanceMarginRate = 0.004

// risk alert level based on distance to liquidation
type AlertLevel int

const (
	AlertNone     AlertLevel = iota // > 10% away
	AlertWarning                    // 5-10% away
	AlertCritical                   // 2-5% away
	AlertAutoClose                  // < 2% away
)

// calculates the estimated liquidation price for a futures position.
// for LONG:  liqPrice = entryPrice * (1 - 1/leverage + maintenanceMarginRate)
// for SHORT: liqPrice = entryPrice * (1 + 1/leverage - maintenanceMarginRate)
func CalculateLiquidationPrice(entryPrice float64, leverage int, side string, maintenanceMarginRate float64) float64 {
	if entryPrice <= 0 || leverage <= 0 {
		return 0
	}
	invLev := 1.0 / float64(leverage)

	switch PositionSide(side) {
	case SideLong:
		price := entryPrice * (1 - invLev + maintenanceMarginRate)
		if price < 0 {
			return 0
		}
		return price
	case SideShort:
		return entryPrice * (1 + invLev - maintenanceMarginRate)
	default:
		return 0
	}
}

// returns the percentage distance from current price to liquidation price.
// always returns a positive number (distance is always >= 0 in theory).
func DistanceToLiquidation(currentPrice, liquidationPrice float64, side string) float64 {
	if currentPrice <= 0 {
		return 0
	}

	var dist float64
	switch PositionSide(side) {
	case SideLong:
		// for longs, liquidation is below current price
		dist = (currentPrice - liquidationPrice) / currentPrice * 100
	case SideShort:
		// for shorts, liquidation is above current price
		dist = (liquidationPrice - currentPrice) / currentPrice * 100
	default:
		return 0
	}
	return math.Abs(dist)
}

// classifies the liquidation risk based on distance percentage
func ClassifyLiquidationRisk(distancePct float64) AlertLevel {
	switch {
	case distancePct < 2:
		return AlertAutoClose
	case distancePct < 5:
		return AlertCritical
	case distancePct < 10:
		return AlertWarning
	default:
		return AlertNone
	}
}

// returns a human-readable description of the alert level
func (a AlertLevel) String() string {
	switch a {
	case AlertNone:
		return "none"
	case AlertWarning:
		return "warning"
	case AlertCritical:
		return "critical"
	case AlertAutoClose:
		return "auto-close"
	default:
		return "unknown"
	}
}
