package leverage

import (
	"fmt"
	"math"
)

// recommended position size for a given leverage level
type SizeRecommendation struct {
	Leverage        int
	Margin          float64 // USDT
	PositionSize    float64 // USDT (margin * leverage)
	RiskAmount      float64 // USDT at risk (% of balance)
	LiquidationDist float64 // approximate liquidation distance %
}

// calculates optimal position sizes across multiple leverage levels.
// riskPct: fraction of balance willing to risk (e.g. 0.02 = 2%)
// slDistPct: stop-loss distance as fraction of entry price (e.g. 0.03 = 3%)
// balance: available futures balance in USDT
// maxLeverage: hard cap on leverage
func CalculatePositionSizes(balance, riskPct, slDistPct float64, maxLeverage int) ([]SizeRecommendation, error) {
	if balance <= 0 {
		return nil, fmt.Errorf("balance must be positive")
	}
	if riskPct <= 0 || riskPct > 1 {
		return nil, fmt.Errorf("risk percentage must be between 0 and 1")
	}
	if slDistPct <= 0 || slDistPct > 1 {
		return nil, fmt.Errorf("stop-loss distance must be between 0 and 1")
	}
	if maxLeverage < 1 {
		return nil, fmt.Errorf("max leverage must be at least 1")
	}

	// risk amount in USDT that we're willing to lose
	riskAmount := balance * riskPct

	// from risk formula: margin = riskAmount / (slDistPct * leverage)
	// position size = margin * leverage
	// loss at SL = positionSize * slDistPct = margin * leverage * slDistPct
	// we want: margin * leverage * slDistPct = riskAmount
	// therefore: margin = riskAmount / (leverage * slDistPct)

	leverages := []int{1, 2, 3, 5, 10, 15, 20, 25, 50, 75, 100, 125}

	var recs []SizeRecommendation
	for _, lev := range leverages {
		if lev > maxLeverage {
			break
		}

		margin := riskAmount / (float64(lev) * slDistPct)

		// cap margin at balance
		if margin > balance {
			margin = balance
		}

		posSize := margin * float64(lev)

		// approximate liquidation distance (simplified: 1/leverage with maintenance margin)
		// in practice, Binance uses tiered maintenance margins, but this is a good approximation
		maintenanceRate := 0.004 // 0.4% standard maintenance margin rate
		liqDist := (1.0/float64(lev) - maintenanceRate) * 100

		recs = append(recs, SizeRecommendation{
			Leverage:        lev,
			Margin:          math.Round(margin*100) / 100,
			PositionSize:    math.Round(posSize*100) / 100,
			RiskAmount:      math.Round(riskAmount*100) / 100,
			LiquidationDist: math.Round(liqDist*100) / 100,
		})
	}

	return recs, nil
}

// formats position sizing recommendations as a readable string
func FormatSizeRecommendations(recs []SizeRecommendation, symbol string, entry float64) string {
	s := fmt.Sprintf("📐 Position Sizing for %s @ $%.2f\n", symbol, entry)
	s += fmt.Sprintf("Risk: $%.2f\n\n", recs[0].RiskAmount)
	s += fmt.Sprintf("%-5s %-12s %-12s %-8s\n", "Lev", "Margin", "Position", "Liq%")
	s += "──────────────────────────────────\n"

	for _, r := range recs {
		s += fmt.Sprintf("%-5dx $%-11.2f $%-11.2f %.1f%%\n",
			r.Leverage, r.Margin, r.PositionSize, r.LiquidationDist)
	}
	return s
}
