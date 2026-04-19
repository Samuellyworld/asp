// portfolio correlation analysis and rebalancing suggestions.
// detects correlated positions that amplify risk and suggests adjustments.
package portfolio

import (
	"fmt"
	"math"
	"sort"
)

// represents a position in the portfolio
type Position struct {
	Symbol    string
	Side      string  // "LONG" or "SHORT"
	Size      float64 // notional value in USDT
	EntryPx   float64
	CurrentPx float64
	PnLPct    float64
}

// correlation between two positions
type Correlation struct {
	SymbolA string
	SymbolB string
	Value   float64 // -1 to 1
	Risk    string  // "HIGH", "MEDIUM", "LOW"
}

// portfolio-level risk assessment
type RiskAssessment struct {
	TotalExposure     float64
	LongExposure      float64
	ShortExposure     float64
	NetExposure       float64 // long - short
	Correlations      []Correlation
	HighRiskPairs     int
	Suggestions       []string
	ConcentrationPct  float64 // largest position as % of total
}

// calculates Pearson correlation coefficient from two price return series
func PearsonCorrelation(x, y []float64) float64 {
	n := len(x)
	if n != len(y) || n < 2 {
		return 0
	}

	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := 0; i < n; i++ {
		sumX += x[i]
		sumY += y[i]
		sumXY += x[i] * y[i]
		sumX2 += x[i] * x[i]
		sumY2 += y[i] * y[i]
	}

	num := float64(n)*sumXY - sumX*sumY
	den := math.Sqrt((float64(n)*sumX2 - sumX*sumX) * (float64(n)*sumY2 - sumY*sumY))

	if den == 0 {
		return 0
	}
	return num / den
}

// converts price series to returns (pct change)
func PriceToReturns(prices []float64) []float64 {
	if len(prices) < 2 {
		return nil
	}
	returns := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		if prices[i-1] != 0 {
			returns[i-1] = (prices[i] - prices[i-1]) / prices[i-1]
		}
	}
	return returns
}

// analyzes portfolio risk: exposure, concentration, and generates suggestions
func AnalyzeRisk(positions []Position) *RiskAssessment {
	if len(positions) == 0 {
		return &RiskAssessment{}
	}

	ra := &RiskAssessment{}

	for _, p := range positions {
		absSize := math.Abs(p.Size)
		ra.TotalExposure += absSize
		if p.Side == "LONG" {
			ra.LongExposure += absSize
		} else {
			ra.ShortExposure += absSize
		}
	}
	ra.NetExposure = ra.LongExposure - ra.ShortExposure

	// concentration: largest position as pct of total
	if ra.TotalExposure > 0 {
		var maxSize float64
		for _, p := range positions {
			if math.Abs(p.Size) > maxSize {
				maxSize = math.Abs(p.Size)
			}
		}
		ra.ConcentrationPct = (maxSize / ra.TotalExposure) * 100
	}

	// generate suggestions
	if ra.ConcentrationPct > 50 {
		ra.Suggestions = append(ra.Suggestions,
			fmt.Sprintf("⚠️ High concentration: single position is %.0f%% of portfolio. Consider reducing.", ra.ConcentrationPct))
	}

	if ra.TotalExposure > 0 {
		netPct := (ra.NetExposure / ra.TotalExposure) * 100
		if math.Abs(netPct) > 80 {
			direction := "long"
			if netPct < 0 {
				direction = "short"
			}
			ra.Suggestions = append(ra.Suggestions,
				fmt.Sprintf("⚠️ Portfolio heavily %s-biased (%.0f%% net). Consider hedging.", direction, math.Abs(netPct)))
		}
	}

	if len(positions) > 5 {
		ra.Suggestions = append(ra.Suggestions,
			"ℹ️ Large number of positions. Ensure you have capacity to monitor all.")
	}

	return ra
}

// analyzes correlations given return series for each symbol and active positions
func AnalyzeCorrelations(positions []Position, returnSeries map[string][]float64) []Correlation {
	var corrs []Correlation

	symbols := make([]string, 0, len(positions))
	seen := make(map[string]bool)
	for _, p := range positions {
		if !seen[p.Symbol] {
			symbols = append(symbols, p.Symbol)
			seen[p.Symbol] = true
		}
	}
	sort.Strings(symbols)

	for i := 0; i < len(symbols); i++ {
		for j := i + 1; j < len(symbols); j++ {
			retA := returnSeries[symbols[i]]
			retB := returnSeries[symbols[j]]
			if len(retA) == 0 || len(retB) == 0 {
				continue
			}

			// align lengths
			n := len(retA)
			if len(retB) < n {
				n = len(retB)
			}

			val := PearsonCorrelation(retA[:n], retB[:n])

			risk := "LOW"
			if math.Abs(val) > 0.8 {
				risk = "HIGH"
			} else if math.Abs(val) > 0.5 {
				risk = "MEDIUM"
			}

			corrs = append(corrs, Correlation{
				SymbolA: symbols[i],
				SymbolB: symbols[j],
				Value:   math.Round(val*1000) / 1000,
				Risk:    risk,
			})
		}
	}

	return corrs
}

// formats risk assessment as readable message
func FormatRiskAssessment(ra *RiskAssessment) string {
	s := "📊 Portfolio Risk Assessment\n\n"
	s += fmt.Sprintf("Total Exposure:  $%.2f\n", ra.TotalExposure)
	s += fmt.Sprintf("Long Exposure:   $%.2f\n", ra.LongExposure)
	s += fmt.Sprintf("Short Exposure:  $%.2f\n", ra.ShortExposure)
	s += fmt.Sprintf("Net Exposure:    $%.2f\n", ra.NetExposure)
	s += fmt.Sprintf("Concentration:   %.1f%%\n", ra.ConcentrationPct)

	if len(ra.Correlations) > 0 {
		s += "\nCorrelated Pairs:\n"
		for _, c := range ra.Correlations {
			if c.Risk == "HIGH" || c.Risk == "MEDIUM" {
				s += fmt.Sprintf("  %s/%s: %.3f (%s)\n", c.SymbolA, c.SymbolB, c.Value, c.Risk)
			}
		}
	}

	if len(ra.Suggestions) > 0 {
		s += "\nSuggestions:\n"
		for _, sug := range ra.Suggestions {
			s += fmt.Sprintf("  %s\n", sug)
		}
	}

	return s
}
