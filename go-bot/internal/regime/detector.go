// market regime detection using ADX (trend strength) and ATR (volatility).
// classifies markets as trending, ranging, volatile, or quiet to help
// Claude make better trading decisions.
package regime

import "math"

// MarketRegime classifies the current market state
type MarketRegime string

const (
	RegimeTrending MarketRegime = "trending"
	RegimeRanging  MarketRegime = "ranging"
	RegimeVolatile MarketRegime = "volatile"
	RegimeQuiet    MarketRegime = "quiet"
)

// Candle represents OHLCV price data
type Candle struct {
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// Detection holds the full regime analysis output
type Detection struct {
	Regime       MarketRegime
	ADX          float64 // 0-100, trend strength
	ATR          float64 // absolute average true range
	ATRPercent   float64 // ATR as % of current price
	TrendDir     string  // "up", "down", or "neutral"
	Confidence   float64 // 0-100, how confident in the regime classification
	Description  string  // human-readable explanation
}

// thresholds for regime classification
const (
	adxTrendThreshold     = 25.0 // above = trending
	adxStrongThreshold    = 40.0 // above = strong trend
	atrHighVolThreshold   = 3.0  // ATR% above = high volatility
	atrLowVolThreshold    = 1.0  // ATR% below = low volatility
)

// Detect analyzes candles and returns the market regime.
// Requires at least 28 candles (14 for ATR smoothing + 14 for ADX).
func Detect(candles []Candle, currentPrice float64) *Detection {
	if len(candles) < 28 || currentPrice == 0 {
		return &Detection{
			Regime:      RegimeRanging,
			Description: "insufficient data for regime detection",
		}
	}

	adx, plusDI, minusDI := calculateADX(candles, 14)
	atr := calculateATR(candles, 14)
	atrPct := (atr / currentPrice) * 100

	// determine trend direction from DI lines
	trendDir := "neutral"
	if plusDI > minusDI+5 {
		trendDir = "up"
	} else if minusDI > plusDI+5 {
		trendDir = "down"
	}

	// classify regime
	det := &Detection{
		ADX:        adx,
		ATR:        atr,
		ATRPercent: atrPct,
		TrendDir:   trendDir,
	}

	isTrending := adx >= adxTrendThreshold
	isHighVol := atrPct >= atrHighVolThreshold
	isLowVol := atrPct <= atrLowVolThreshold

	switch {
	case isTrending && isHighVol:
		det.Regime = RegimeVolatile
		det.Confidence = math.Min(adx, 80)
		det.Description = describeVolatileTrend(adx, atrPct, trendDir)
	case isTrending && !isHighVol:
		det.Regime = RegimeTrending
		det.Confidence = math.Min(adx*1.2, 90)
		det.Description = describeTrend(adx, trendDir)
	case !isTrending && isHighVol:
		det.Regime = RegimeVolatile
		det.Confidence = math.Min(atrPct*20, 80)
		det.Description = describeVolatileRange(atrPct)
	case !isTrending && isLowVol:
		det.Regime = RegimeQuiet
		det.Confidence = math.Min((adxTrendThreshold-adx)*4, 80)
		det.Description = describeQuiet(adx, atrPct)
	default:
		det.Regime = RegimeRanging
		det.Confidence = math.Min((adxTrendThreshold-adx)*3, 70)
		det.Description = describeRanging(adx, atrPct)
	}

	return det
}

// calculateADX computes the Average Directional Index.
// Returns ADX, +DI, and -DI values.
func calculateADX(candles []Candle, period int) (float64, float64, float64) {
	n := len(candles)
	if n < period*2 {
		return 0, 0, 0
	}

	// calculate True Range, +DM, -DM for each candle
	trueRanges := make([]float64, n-1)
	plusDMs := make([]float64, n-1)
	minusDMs := make([]float64, n-1)

	for i := 1; i < n; i++ {
		high := candles[i].High
		low := candles[i].Low
		prevHigh := candles[i-1].High
		prevLow := candles[i-1].Low
		prevClose := candles[i-1].Close

		// true range
		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)
		trueRanges[i-1] = math.Max(tr1, math.Max(tr2, tr3))

		// directional movement
		upMove := high - prevHigh
		downMove := prevLow - low

		if upMove > downMove && upMove > 0 {
			plusDMs[i-1] = upMove
		}
		if downMove > upMove && downMove > 0 {
			minusDMs[i-1] = downMove
		}
	}

	// smoothed averages using Wilder's smoothing
	smoothTR := wilderSmooth(trueRanges, period)
	smoothPlusDM := wilderSmooth(plusDMs, period)
	smoothMinusDM := wilderSmooth(minusDMs, period)

	if len(smoothTR) == 0 {
		return 0, 0, 0
	}

	// calculate DI values
	nDI := len(smoothTR)
	plusDIs := make([]float64, nDI)
	minusDIs := make([]float64, nDI)
	dxValues := make([]float64, nDI)

	for i := 0; i < nDI; i++ {
		if smoothTR[i] != 0 {
			plusDIs[i] = (smoothPlusDM[i] / smoothTR[i]) * 100
			minusDIs[i] = (smoothMinusDM[i] / smoothTR[i]) * 100
		}

		sum := plusDIs[i] + minusDIs[i]
		if sum != 0 {
			dxValues[i] = (math.Abs(plusDIs[i]-minusDIs[i]) / sum) * 100
		}
	}

	// smooth DX to get ADX
	if len(dxValues) < period {
		return 0, 0, 0
	}

	adxSmoothed := wilderSmooth(dxValues, period)
	if len(adxSmoothed) == 0 {
		return 0, 0, 0
	}

	adx := adxSmoothed[len(adxSmoothed)-1]
	plusDI := plusDIs[len(plusDIs)-1]
	minusDI := minusDIs[len(minusDIs)-1]

	return adx, plusDI, minusDI
}

// calculateATR computes the Average True Range using Wilder's smoothing.
func calculateATR(candles []Candle, period int) float64 {
	n := len(candles)
	if n < period+1 {
		return 0
	}

	trueRanges := make([]float64, n-1)
	for i := 1; i < n; i++ {
		tr1 := candles[i].High - candles[i].Low
		tr2 := math.Abs(candles[i].High - candles[i-1].Close)
		tr3 := math.Abs(candles[i].Low - candles[i-1].Close)
		trueRanges[i-1] = math.Max(tr1, math.Max(tr2, tr3))
	}

	smoothed := wilderSmooth(trueRanges, period)
	if len(smoothed) == 0 {
		return 0
	}

	return smoothed[len(smoothed)-1]
}

// wilderSmooth applies Wilder's smoothing (exponential) to a data series.
func wilderSmooth(data []float64, period int) []float64 {
	if len(data) < period {
		return nil
	}

	result := make([]float64, len(data)-period+1)

	// first value is simple average
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += data[i]
	}
	result[0] = sum / float64(period)

	// subsequent values use Wilder's smoothing
	for i := period; i < len(data); i++ {
		result[i-period+1] = (result[i-period]*float64(period-1) + data[i]) / float64(period)
	}

	return result
}

func describeTrend(adx float64, dir string) string {
	strength := "moderate"
	if adx >= adxStrongThreshold {
		strength = "strong"
	}
	return strength + " " + dir + "trend — favor trend-following strategies"
}

func describeVolatileTrend(adx, atrPct float64, dir string) string {
	return "volatile " + dir + "trend — use wider stops, reduce position size"
}

func describeVolatileRange(atrPct float64) string {
	return "high volatility with no clear trend — avoid or use mean-reversion with tight risk"
}

func describeRanging(adx, atrPct float64) string {
	return "ranging market — favor mean-reversion at support/resistance, avoid breakout entries"
}

func describeQuiet(adx, atrPct float64) string {
	return "quiet/compressed market — potential breakout setup, wait for confirmation"
}
