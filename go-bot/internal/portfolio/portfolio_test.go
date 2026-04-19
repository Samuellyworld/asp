package portfolio

import (
	"math"
	"testing"
)

func TestPearsonCorrelation(t *testing.T) {
	// perfectly correlated
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{2, 4, 6, 8, 10}
	if c := PearsonCorrelation(x, y); math.Abs(c-1.0) > 0.001 {
		t.Fatalf("expected 1.0, got %f", c)
	}

	// perfectly anti-correlated
	z := []float64{10, 8, 6, 4, 2}
	if c := PearsonCorrelation(x, z); math.Abs(c-(-1.0)) > 0.001 {
		t.Fatalf("expected -1.0, got %f", c)
	}

	// inversely correlated
	a := []float64{1, 3, 2, 5, 4}
	b := []float64{5, 2, 4, 1, 3}
	c := PearsonCorrelation(a, b)
	if c > 0 {
		t.Fatalf("expected negative correlation, got %f", c)
	}
}

func TestPriceToReturns(t *testing.T) {
	prices := []float64{100, 110, 105, 115}
	returns := PriceToReturns(prices)
	if len(returns) != 3 {
		t.Fatalf("expected 3 returns, got %d", len(returns))
	}
	// 100->110 = 10%
	if math.Abs(returns[0]-0.1) > 0.001 {
		t.Fatalf("expected 0.1, got %f", returns[0])
	}
}

func TestAnalyzeRisk(t *testing.T) {
	positions := []Position{
		{Symbol: "BTCUSDT", Side: "LONG", Size: 5000},
		{Symbol: "ETHUSDT", Side: "LONG", Size: 3000},
		{Symbol: "BNBUSDT", Side: "SHORT", Size: 2000},
	}

	ra := AnalyzeRisk(positions)
	if ra.TotalExposure != 10000 {
		t.Fatalf("expected 10000, got %f", ra.TotalExposure)
	}
	if ra.LongExposure != 8000 {
		t.Fatalf("expected 8000, got %f", ra.LongExposure)
	}
	if ra.ShortExposure != 2000 {
		t.Fatalf("expected 2000, got %f", ra.ShortExposure)
	}
	if ra.ConcentrationPct != 50 {
		t.Fatalf("expected 50%%, got %f", ra.ConcentrationPct)
	}
}

func TestAnalyzeRiskHighConcentration(t *testing.T) {
	positions := []Position{
		{Symbol: "BTCUSDT", Side: "LONG", Size: 9000},
		{Symbol: "ETHUSDT", Side: "LONG", Size: 1000},
	}

	ra := AnalyzeRisk(positions)
	if ra.ConcentrationPct != 90 {
		t.Fatalf("expected 90%%, got %f", ra.ConcentrationPct)
	}
	if len(ra.Suggestions) == 0 {
		t.Fatal("expected concentration warning")
	}
}

func TestAnalyzeCorrelations(t *testing.T) {
	positions := []Position{
		{Symbol: "BTCUSDT", Side: "LONG", Size: 5000},
		{Symbol: "ETHUSDT", Side: "LONG", Size: 3000},
	}

	// BTC and ETH highly correlated
	returns := map[string][]float64{
		"BTCUSDT": {0.01, 0.02, -0.01, 0.03, -0.02},
		"ETHUSDT": {0.015, 0.025, -0.015, 0.035, -0.025},
	}

	corrs := AnalyzeCorrelations(positions, returns)
	if len(corrs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(corrs))
	}
	if corrs[0].Value < 0.9 {
		t.Fatalf("expected high correlation, got %f", corrs[0].Value)
	}
	if corrs[0].Risk != "HIGH" {
		t.Fatalf("expected HIGH risk, got %s", corrs[0].Risk)
	}
}

func TestFormatRiskAssessment(t *testing.T) {
	ra := &RiskAssessment{
		TotalExposure: 10000,
		LongExposure:  8000,
		ShortExposure: 2000,
		NetExposure:   6000,
		Suggestions:   []string{"test suggestion"},
	}
	msg := FormatRiskAssessment(ra)
	if msg == "" {
		t.Fatal("expected non-empty format")
	}
}

func TestEmptyPortfolio(t *testing.T) {
	ra := AnalyzeRisk(nil)
	if ra.TotalExposure != 0 {
		t.Fatal("expected zero exposure for empty portfolio")
	}
}
