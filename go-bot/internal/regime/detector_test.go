package regime

import (
	"math"
	"testing"
)

func almostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) < tolerance
}

// generates a trending upward candle sequence
func trendingUpCandles(n int) []Candle {
	candles := make([]Candle, n)
	basePrice := 100.0
	for i := 0; i < n; i++ {
		price := basePrice + float64(i)*2.0 // steady uptrend
		candles[i] = Candle{
			Open:   price - 0.5,
			High:   price + 1.0,
			Low:    price - 1.0,
			Close:  price + 0.5,
			Volume: 1000,
		}
	}
	return candles
}

// generates a ranging/sideways candle sequence
func rangingCandles(n int) []Candle {
	candles := make([]Candle, n)
	basePrice := 100.0
	for i := 0; i < n; i++ {
		// oscillate between 98 and 102
		offset := math.Sin(float64(i)*0.5) * 2.0
		price := basePrice + offset
		candles[i] = Candle{
			Open:   price - 0.3,
			High:   price + 0.5,
			Low:    price - 0.5,
			Close:  price + 0.3,
			Volume: 800,
		}
	}
	return candles
}

// generates highly volatile candles
func volatileCandles(n int) []Candle {
	candles := make([]Candle, n)
	basePrice := 100.0
	for i := 0; i < n; i++ {
		offset := math.Sin(float64(i)*0.8) * 10.0
		price := basePrice + offset
		candles[i] = Candle{
			Open:   price - 3.0,
			High:   price + 5.0,
			Low:    price - 5.0,
			Close:  price + 3.0,
			Volume: 2000,
		}
	}
	return candles
}

// generates quiet/compressed candles
func quietCandles(n int) []Candle {
	candles := make([]Candle, n)
	basePrice := 100.0
	for i := 0; i < n; i++ {
		// very tight range
		candles[i] = Candle{
			Open:   basePrice - 0.05,
			High:   basePrice + 0.1,
			Low:    basePrice - 0.1,
			Close:  basePrice + 0.05,
			Volume: 500,
		}
	}
	return candles
}

func TestDetect_InsufficientData(t *testing.T) {
	candles := make([]Candle, 10)
	det := Detect(candles, 100)

	if det.Regime != RegimeRanging {
		t.Errorf("expected ranging for insufficient data, got %s", det.Regime)
	}
	if det.Description != "insufficient data for regime detection" {
		t.Errorf("unexpected description: %s", det.Description)
	}
}

func TestDetect_ZeroPrice(t *testing.T) {
	candles := make([]Candle, 50)
	det := Detect(candles, 0)

	if det.Regime != RegimeRanging {
		t.Errorf("expected ranging for zero price, got %s", det.Regime)
	}
}

func TestDetect_TrendingUp(t *testing.T) {
	candles := trendingUpCandles(50)
	lastPrice := candles[len(candles)-1].Close
	det := Detect(candles, lastPrice)

	if det.ADX == 0 {
		t.Error("expected non-zero ADX for trending data")
	}
	if det.ATR == 0 {
		t.Error("expected non-zero ATR")
	}
	if det.ATRPercent == 0 {
		t.Error("expected non-zero ATR percent")
	}
	// trending up data should have upward direction
	if det.TrendDir != "up" && det.TrendDir != "neutral" {
		t.Errorf("expected up or neutral trend direction for uptrend, got %s", det.TrendDir)
	}
}

func TestDetect_Ranging(t *testing.T) {
	candles := rangingCandles(50)
	det := Detect(candles, 100)

	// ranging data should have low ADX
	if det.ADX > adxTrendThreshold+10 {
		t.Errorf("expected low ADX for ranging data, got %.2f", det.ADX)
	}
}

func TestDetect_Quiet(t *testing.T) {
	candles := quietCandles(50)
	det := Detect(candles, 100)

	if det.ATRPercent > 2.0 {
		t.Errorf("expected low ATR%% for quiet data, got %.2f", det.ATRPercent)
	}
	// quiet candles should give quiet or ranging regime
	if det.Regime != RegimeQuiet && det.Regime != RegimeRanging {
		t.Errorf("expected quiet or ranging regime for compressed data, got %s", det.Regime)
	}
}

func TestDetect_Volatile(t *testing.T) {
	candles := volatileCandles(50)
	det := Detect(candles, 100)

	if det.ATRPercent < 1.0 {
		t.Errorf("expected high ATR%% for volatile data, got %.2f", det.ATRPercent)
	}
}

func TestDetect_Description_NotEmpty(t *testing.T) {
	candles := trendingUpCandles(50)
	det := Detect(candles, candles[len(candles)-1].Close)

	if det.Description == "" {
		t.Error("expected non-empty description")
	}
}

func TestDetect_Confidence_InRange(t *testing.T) {
	tests := []struct {
		name    string
		candles []Candle
		price   float64
	}{
		{"trending", trendingUpCandles(50), 200},
		{"ranging", rangingCandles(50), 100},
		{"quiet", quietCandles(50), 100},
		{"volatile", volatileCandles(50), 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			det := Detect(tc.candles, tc.price)
			if det.Confidence < 0 || det.Confidence > 100 {
				t.Errorf("confidence out of range: %.2f", det.Confidence)
			}
		})
	}
}

func TestCalculateATR(t *testing.T) {
	candles := trendingUpCandles(50)
	atr := calculateATR(candles, 14)

	if atr <= 0 {
		t.Errorf("expected positive ATR, got %.4f", atr)
	}
}

func TestCalculateATR_InsufficientData(t *testing.T) {
	candles := make([]Candle, 5)
	atr := calculateATR(candles, 14)

	if atr != 0 {
		t.Errorf("expected zero ATR for insufficient data, got %.4f", atr)
	}
}

func TestCalculateADX(t *testing.T) {
	candles := trendingUpCandles(50)
	adx, plusDI, minusDI := calculateADX(candles, 14)

	if adx < 0 || adx > 100 {
		t.Errorf("ADX out of range: %.2f", adx)
	}
	if plusDI < 0 || plusDI > 100 {
		t.Errorf("+DI out of range: %.2f", plusDI)
	}
	if minusDI < 0 || minusDI > 100 {
		t.Errorf("-DI out of range: %.2f", minusDI)
	}

	// for an uptrend, +DI should be greater than -DI
	if plusDI < minusDI {
		t.Logf("warning: +DI (%.2f) < -DI (%.2f) for uptrend data", plusDI, minusDI)
	}
}

func TestCalculateADX_InsufficientData(t *testing.T) {
	candles := make([]Candle, 10)
	adx, _, _ := calculateADX(candles, 14)

	if adx != 0 {
		t.Errorf("expected zero ADX for insufficient data, got %.4f", adx)
	}
}

func TestWilderSmooth(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := wilderSmooth(data, 3)

	if len(result) != 8 {
		t.Fatalf("expected 8 smoothed values, got %d", len(result))
	}

	// first value should be simple average of first 3
	expected := (1.0 + 2.0 + 3.0) / 3.0
	if !almostEqual(result[0], expected, 0.001) {
		t.Errorf("expected first value %.4f, got %.4f", expected, result[0])
	}

	// values should be smoothed (between min and max of input)
	for i, v := range result {
		if v < 0 || v > 10 {
			t.Errorf("smoothed value[%d] out of range: %.4f", i, v)
		}
	}
}

func TestWilderSmooth_InsufficientData(t *testing.T) {
	data := []float64{1, 2}
	result := wilderSmooth(data, 5)

	if result != nil {
		t.Errorf("expected nil for insufficient data, got %v", result)
	}
}

func TestRegimeConstants(t *testing.T) {
	// verify regime string values
	if RegimeTrending != "trending" {
		t.Error("unexpected trending constant")
	}
	if RegimeRanging != "ranging" {
		t.Error("unexpected ranging constant")
	}
	if RegimeVolatile != "volatile" {
		t.Error("unexpected volatile constant")
	}
	if RegimeQuiet != "quiet" {
		t.Error("unexpected quiet constant")
	}
}

func TestDetect_TrendingDown(t *testing.T) {
	// create downtrend candles
	candles := make([]Candle, 50)
	basePrice := 200.0
	for i := 0; i < 50; i++ {
		price := basePrice - float64(i)*2.0
		candles[i] = Candle{
			Open:   price + 0.5,
			High:   price + 1.0,
			Low:    price - 1.0,
			Close:  price - 0.5,
			Volume: 1000,
		}
	}

	det := Detect(candles, candles[len(candles)-1].Close)

	if det.ADX == 0 {
		t.Error("expected non-zero ADX for downtrend")
	}
	// downtrend should show "down" direction
	if det.TrendDir != "down" && det.TrendDir != "neutral" {
		t.Errorf("expected down or neutral direction for downtrend, got %s", det.TrendDir)
	}
}
