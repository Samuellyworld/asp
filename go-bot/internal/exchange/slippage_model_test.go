package exchange

import (
	"testing"
)

func TestSlippageModelEstimateDefault(t *testing.T) {
	model := NewSlippageModel(nil, 5.0)
	est := model.EstimateBps("BTCUSDT")
	if est != 5.0 {
		t.Errorf("expected fallback 5.0, got %.2f", est)
	}
}

func TestSlippageModelUpdate(t *testing.T) {
	model := NewSlippageModel(nil, 5.0)

	for i := 0; i < 10; i++ {
		model.Update(&SlippageRecord{
			Symbol:        "BTCUSDT",
			Side:          "BUY",
			ExpectedPrice: 50000,
			ActualPrice:   50005,
			SlippageBps:   1.0,
		})
	}

	est := model.EstimateBps("BTCUSDT")
	if est == 5.0 {
		t.Error("expected estimate to change from fallback after updates")
	}
	if est < 0 || est > 5 {
		t.Errorf("expected reasonable estimate, got %.2f", est)
	}
}

func TestSlippageModelWorstCase(t *testing.T) {
	model := NewSlippageModel(nil, 5.0)

	// add varied slippage values
	for i := 0; i < 20; i++ {
		bps := float64(i) * 0.5
		model.Update(&SlippageRecord{
			Symbol:      "ETHUSDT",
			Side:        "BUY",
			SlippageBps: bps,
		})
	}

	worst := model.WorstCaseBps("ETHUSDT")
	if worst <= 0 {
		t.Errorf("expected positive worst case, got %.2f", worst)
	}
	est := model.EstimateBps("ETHUSDT")
	if worst < est {
		t.Errorf("worst case (%.2f) should be >= estimate (%.2f)", worst, est)
	}
}

func TestSlippageModelStats(t *testing.T) {
	model := NewSlippageModel(nil, 5.0)
	model.Update(&SlippageRecord{Symbol: "BTCUSDT", SlippageBps: 2.0})
	model.Update(&SlippageRecord{Symbol: "ETHUSDT", SlippageBps: 3.0})

	stats := model.Stats()
	if len(stats) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(stats))
	}
	if stats["BTCUSDT"].SampleCount != 1 {
		t.Errorf("expected 1 sample for BTC, got %d", stats["BTCUSDT"].SampleCount)
	}
}

func TestSlippageTrackerWithModel(t *testing.T) {
	model := NewSlippageModel(nil, 5.0)
	tracker := NewSlippageTracker(100)
	tracker.SetModel(model)

	tracker.Record("BTCUSDT", "BUY", 50000, 50010, 0.1, false)
	tracker.Record("BTCUSDT", "BUY", 50000, 50015, 0.1, false)
	tracker.Record("BTCUSDT", "BUY", 50000, 50005, 0.1, false)

	stats := model.Stats()
	if stats["BTCUSDT"].SampleCount != 3 {
		t.Errorf("expected 3 samples, got %d", stats["BTCUSDT"].SampleCount)
	}
}

func TestPercentile(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	p50 := percentile(values, 50)
	if p50 < 5 || p50 > 6 {
		t.Errorf("expected p50 ~5.5, got %.2f", p50)
	}
	p95 := percentile(values, 95)
	if p95 < 9 || p95 > 10 {
		t.Errorf("expected p95 ~9.5, got %.2f", p95)
	}
}

func TestPercentileEmpty(t *testing.T) {
	p := percentile([]float64{}, 50)
	if p != 0 {
		t.Errorf("expected 0 for empty, got %.2f", p)
	}
}
