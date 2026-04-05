package autotuner

import (
	"context"
	"testing"
	"time"
)

func TestDefaultParams(t *testing.T) {
	p := DefaultParams()
	if p.ConfidenceThreshold != 70 {
		t.Errorf("expected confidence 70, got %f", p.ConfidenceThreshold)
	}
	if p.MaxLeverage != 5 {
		t.Errorf("expected leverage 5, got %d", p.MaxLeverage)
	}
}

func TestDefaultRegimeParams(t *testing.T) {
	rp := DefaultRegimeParams()

	if rp.Trending.ConfidenceThreshold != 60 {
		t.Errorf("trending confidence expected 60, got %f", rp.Trending.ConfidenceThreshold)
	}
	if rp.Volatile.MaxLeverage != 2 {
		t.Errorf("volatile leverage expected 2, got %d", rp.Volatile.MaxLeverage)
	}
	if rp.Ranging.PositionSizePct != 1.5 {
		t.Errorf("ranging position size expected 1.5, got %f", rp.Ranging.PositionSizePct)
	}
}

func TestGetParams(t *testing.T) {
	tuner := NewAutoTuner(50, time.Hour)

	tests := []struct {
		regime   string
		expected float64
	}{
		{"trending", 60},
		{"ranging", 75},
		{"volatile", 80},
		{"quiet", 70},
		{"unknown", 70}, // default
	}

	for _, tt := range tests {
		p := tuner.GetParams(tt.regime)
		if p.ConfidenceThreshold != tt.expected {
			t.Errorf("regime %s: expected confidence %f, got %f", tt.regime, tt.expected, p.ConfidenceThreshold)
		}
	}
}

func TestRecordTrade(t *testing.T) {
	tuner := NewAutoTuner(50, time.Hour)

	trade := TradeResult{
		Symbol:     "BTC/USDT",
		Direction:  "long",
		PnLPct:     2.5,
		Confidence: 0.8,
		Regime:     "trending",
		Timestamp:  time.Now(),
	}

	tuner.RecordTrade(trade)

	perf := tuner.GetPerformance("trending")
	if len(perf.Trades) != 1 {
		t.Errorf("expected 1 trade, got %d", len(perf.Trades))
	}

	globalPerf := tuner.GetPerformance("global")
	if len(globalPerf.Trades) != 1 {
		t.Errorf("expected 1 global trade, got %d", len(globalPerf.Trades))
	}
}

func TestRecalcMetrics(t *testing.T) {
	tuner := NewAutoTuner(50, time.Hour)

	trades := []TradeResult{
		{PnLPct: 3.0, Regime: "trending"},
		{PnLPct: -1.0, Regime: "trending"},
		{PnLPct: 2.0, Regime: "trending"},
		{PnLPct: 1.5, Regime: "trending"},
		{PnLPct: -0.5, Regime: "trending"},
	}

	for _, tr := range trades {
		tr.Timestamp = time.Now()
		tuner.RecordTrade(tr)
	}

	perf := tuner.GetPerformance("trending")
	if perf.WinRate != 0.6 {
		t.Errorf("expected win rate 0.6, got %f", perf.WinRate)
	}
	if perf.AvgPnL != 1.0 {
		t.Errorf("expected avg pnl 1.0, got %f", perf.AvgPnL)
	}
}

func TestWindowSizeLimit(t *testing.T) {
	tuner := NewAutoTuner(5, time.Hour)

	for i := 0; i < 10; i++ {
		tuner.RecordTrade(TradeResult{
			PnLPct:    float64(i),
			Regime:    "trending",
			Timestamp: time.Now(),
		})
	}

	perf := tuner.GetPerformance("trending")
	if len(perf.Trades) != 5 {
		t.Errorf("expected 5 trades (window limit), got %d", len(perf.Trades))
	}
}

func TestEvaluateLowWinRate(t *testing.T) {
	tuner := NewAutoTuner(50, time.Millisecond) // very short interval

	// record 15 losing trades
	for i := 0; i < 15; i++ {
		tuner.RecordTrade(TradeResult{
			PnLPct:    -1.0,
			Regime:    "ranging",
			Timestamp: time.Now(),
		})
	}

	// force evaluation
	time.Sleep(2 * time.Millisecond)
	tuner.RecordTrade(TradeResult{PnLPct: -1.0, Regime: "ranging", Timestamp: time.Now()})

	params := tuner.GetParams("ranging")
	if params.ConfidenceThreshold <= 75 {
		t.Errorf("expected confidence threshold increased above 75, got %f", params.ConfidenceThreshold)
	}
}

func TestEvaluateHighWinRate(t *testing.T) {
	tuner := NewAutoTuner(50, time.Millisecond)

	for i := 0; i < 15; i++ {
		tuner.RecordTrade(TradeResult{
			PnLPct:    2.0,
			Regime:    "trending",
			Timestamp: time.Now(),
		})
	}

	time.Sleep(2 * time.Millisecond)
	tuner.RecordTrade(TradeResult{PnLPct: 2.0, Regime: "trending", Timestamp: time.Now()})

	params := tuner.GetParams("trending")
	if params.ConfidenceThreshold >= 60 {
		t.Errorf("expected confidence threshold decreased below 60, got %f", params.ConfidenceThreshold)
	}
}

func TestTuneHistory(t *testing.T) {
	tuner := NewAutoTuner(50, time.Millisecond)

	// trigger tuning with losing trades
	for i := 0; i < 15; i++ {
		tuner.RecordTrade(TradeResult{PnLPct: -2.0, Regime: "volatile", Timestamp: time.Now()})
	}
	time.Sleep(2 * time.Millisecond)
	tuner.RecordTrade(TradeResult{PnLPct: -2.0, Regime: "volatile", Timestamp: time.Now()})

	history := tuner.GetTuneHistory()
	if len(history) == 0 {
		t.Error("expected tune history entries, got none")
	}

	for _, event := range history {
		if event.Regime == "" {
			t.Error("expected regime in tune event")
		}
		if event.Reason == "" {
			t.Error("expected reason in tune event")
		}
	}
}

func TestMaxDrawdownReducesPosition(t *testing.T) {
	tuner := NewAutoTuner(50, time.Millisecond)

	// create high drawdown scenario
	for i := 0; i < 6; i++ {
		tuner.RecordTrade(TradeResult{PnLPct: 5.0, Regime: "trending", Timestamp: time.Now()})
	}
	for i := 0; i < 10; i++ {
		tuner.RecordTrade(TradeResult{PnLPct: -3.0, Regime: "trending", Timestamp: time.Now()})
	}

	time.Sleep(2 * time.Millisecond)
	tuner.RecordTrade(TradeResult{PnLPct: -3.0, Regime: "trending", Timestamp: time.Now()})

	params := tuner.GetParams("trending")
	if params.PositionSizePct >= 3.0 {
		t.Errorf("expected position size reduced below 3.0, got %f", params.PositionSizePct)
	}
}

func TestRunContextCancellation(t *testing.T) {
	tuner := NewAutoTuner(50, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		tuner.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(500 * time.Millisecond):
		t.Error("Run did not exit after context cancellation")
	}
}

func TestPerformanceUnknownRegime(t *testing.T) {
	tuner := NewAutoTuner(50, time.Hour)
	perf := tuner.GetPerformance("nonexistent")
	if len(perf.Trades) != 0 {
		t.Error("expected empty performance for unknown regime")
	}
}

func TestSharpeRatio(t *testing.T) {
	tuner := NewAutoTuner(50, time.Hour)

	// consistent positive returns -> high sharpe
	for i := 0; i < 20; i++ {
		tuner.RecordTrade(TradeResult{
			PnLPct:    1.0 + float64(i)*0.01,
			Regime:    "trending",
			Timestamp: time.Now(),
		})
	}

	perf := tuner.GetPerformance("trending")
	if perf.SharpeRatio <= 0 {
		t.Errorf("expected positive sharpe ratio, got %f", perf.SharpeRatio)
	}
}
