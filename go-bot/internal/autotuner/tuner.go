package autotuner

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// TradeResult represents a completed trade for performance evaluation
type TradeResult struct {
	Symbol     string
	Direction  string // "long" or "short"
	PnLPct     float64
	Confidence float64
	Regime     string // "trending", "ranging", "volatile", "quiet"
	Timestamp  time.Time
}

// TunableParams holds all parameters the auto-tuner can adjust
type TunableParams struct {
	ConfidenceThreshold float64 // minimum confidence to act (0-100)
	PositionSizePct     float64 // % of portfolio per trade
	MaxLeverage         int     // max leverage allowed
	StopLossPct         float64 // stop loss distance %
	TakeProfitPct       float64 // take profit distance %
	ScanInterval        time.Duration
}

// DefaultParams returns sensible defaults
func DefaultParams() TunableParams {
	return TunableParams{
		ConfidenceThreshold: 70,
		PositionSizePct:     2.0,
		MaxLeverage:         5,
		StopLossPct:         2.0,
		TakeProfitPct:       4.0,
		ScanInterval:        15 * time.Minute,
	}
}

// RegimeParams holds per-regime parameter overrides
type RegimeParams struct {
	Trending TunableParams
	Ranging  TunableParams
	Volatile TunableParams
	Quiet    TunableParams
}

// DefaultRegimeParams returns regime-specific defaults
func DefaultRegimeParams() RegimeParams {
	return RegimeParams{
		Trending: TunableParams{
			ConfidenceThreshold: 60,
			PositionSizePct:     3.0,
			MaxLeverage:         7,
			StopLossPct:         3.0,
			TakeProfitPct:       6.0,
			ScanInterval:        10 * time.Minute,
		},
		Ranging: TunableParams{
			ConfidenceThreshold: 75,
			PositionSizePct:     1.5,
			MaxLeverage:         3,
			StopLossPct:         1.5,
			TakeProfitPct:       3.0,
			ScanInterval:        20 * time.Minute,
		},
		Volatile: TunableParams{
			ConfidenceThreshold: 80,
			PositionSizePct:     1.0,
			MaxLeverage:         2,
			StopLossPct:         4.0,
			TakeProfitPct:       8.0,
			ScanInterval:        5 * time.Minute,
		},
		Quiet: TunableParams{
			ConfidenceThreshold: 70,
			PositionSizePct:     2.0,
			MaxLeverage:         5,
			StopLossPct:         2.0,
			TakeProfitPct:       4.0,
			ScanInterval:        30 * time.Minute,
		},
	}
}

// PerformanceWindow tracks rolling performance metrics
type PerformanceWindow struct {
	WindowSize  int
	Trades      []TradeResult
	WinRate     float64
	AvgPnL      float64
	SharpeRatio float64
	MaxDrawdown float64
}

// AutoTuner dynamically adjusts trading parameters based on rolling performance
type AutoTuner struct {
	mu           sync.RWMutex
	params       RegimeParams
	performance  map[string]*PerformanceWindow // per-regime performance
	globalPerf   *PerformanceWindow
	windowSize   int
	evalInterval time.Duration
	lastEval     time.Time
	tuneHistory  []TuneEvent
}

// TuneEvent records a parameter adjustment
type TuneEvent struct {
	Timestamp time.Time
	Regime    string
	Field     string
	OldValue  float64
	NewValue  float64
	Reason    string
}

// NewAutoTuner creates an auto-tuner with default regime params
func NewAutoTuner(windowSize int, evalInterval time.Duration) *AutoTuner {
	regimes := DefaultRegimeParams()
	perfMap := map[string]*PerformanceWindow{
		"trending": {WindowSize: windowSize},
		"ranging":  {WindowSize: windowSize},
		"volatile": {WindowSize: windowSize},
		"quiet":    {WindowSize: windowSize},
	}

	return &AutoTuner{
		params:       regimes,
		performance:  perfMap,
		globalPerf:   &PerformanceWindow{WindowSize: windowSize},
		windowSize:   windowSize,
		evalInterval: evalInterval,
		lastEval:     time.Now(),
	}
}

// GetParams returns the current parameters for a given regime
func (t *AutoTuner) GetParams(regime string) TunableParams {
	t.mu.RLock()
	defer t.mu.RUnlock()

	switch regime {
	case "trending":
		return t.params.Trending
	case "ranging":
		return t.params.Ranging
	case "volatile":
		return t.params.Volatile
	case "quiet":
		return t.params.Quiet
	default:
		return DefaultParams()
	}
}

// RecordTrade records a completed trade and triggers evaluation if needed
func (t *AutoTuner) RecordTrade(result TradeResult) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// add to regime-specific window
	if perf, ok := t.performance[result.Regime]; ok {
		perf.Trades = append(perf.Trades, result)
		if len(perf.Trades) > t.windowSize {
			perf.Trades = perf.Trades[len(perf.Trades)-t.windowSize:]
		}
		t.recalcMetrics(perf)
	}

	// add to global window
	t.globalPerf.Trades = append(t.globalPerf.Trades, result)
	if len(t.globalPerf.Trades) > t.windowSize {
		t.globalPerf.Trades = t.globalPerf.Trades[len(t.globalPerf.Trades)-t.windowSize:]
	}
	t.recalcMetrics(t.globalPerf)

	// evaluate and tune if interval has passed
	if time.Since(t.lastEval) >= t.evalInterval {
		t.evaluate()
		t.lastEval = time.Now()
	}
}

// GetPerformance returns performance metrics for a regime (or "global")
func (t *AutoTuner) GetPerformance(regime string) PerformanceWindow {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if regime == "global" {
		return *t.globalPerf
	}
	if perf, ok := t.performance[regime]; ok {
		return *perf
	}
	return PerformanceWindow{}
}

// GetTuneHistory returns recent tuning events
func (t *AutoTuner) GetTuneHistory() []TuneEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make([]TuneEvent, len(t.tuneHistory))
	copy(out, t.tuneHistory)
	return out
}

func (t *AutoTuner) recalcMetrics(perf *PerformanceWindow) {
	if len(perf.Trades) == 0 {
		return
	}

	wins := 0
	totalPnL := 0.0
	pnls := make([]float64, len(perf.Trades))

	for i, tr := range perf.Trades {
		if tr.PnLPct > 0 {
			wins++
		}
		totalPnL += tr.PnLPct
		pnls[i] = tr.PnLPct
	}

	perf.WinRate = float64(wins) / float64(len(perf.Trades))
	perf.AvgPnL = totalPnL / float64(len(perf.Trades))

	// sharpe ratio (simplified: mean/std of returns)
	mean := perf.AvgPnL
	variance := 0.0
	for _, p := range pnls {
		variance += (p - mean) * (p - mean)
	}
	if len(pnls) > 1 {
		stdDev := math.Sqrt(variance / float64(len(pnls)-1))
		if stdDev > 0 {
			perf.SharpeRatio = mean / stdDev
		}
	}

	// max drawdown
	peak := 0.0
	cumulative := 0.0
	maxDD := 0.0
	for _, p := range pnls {
		cumulative += p
		if cumulative > peak {
			peak = cumulative
		}
		dd := peak - cumulative
		if dd > maxDD {
			maxDD = dd
		}
	}
	perf.MaxDrawdown = maxDD
}

func (t *AutoTuner) evaluate() {
	for regime, perf := range t.performance {
		if len(perf.Trades) < 10 {
			continue
		}

		params := t.getParamsPtr(regime)
		if params == nil {
			continue
		}

		// rule 1: if win rate < 40%, increase confidence threshold
		if perf.WinRate < 0.4 {
			old := params.ConfidenceThreshold
			params.ConfidenceThreshold = math.Min(params.ConfidenceThreshold+5, 95)
			if params.ConfidenceThreshold != old {
				t.logTune(regime, "ConfidenceThreshold", old, params.ConfidenceThreshold,
					fmt.Sprintf("win rate %.1f%% < 40%%", perf.WinRate*100))
			}
		}

		// rule 2: if win rate > 65%, can lower threshold to capture more trades
		if perf.WinRate > 0.65 {
			old := params.ConfidenceThreshold
			params.ConfidenceThreshold = math.Max(params.ConfidenceThreshold-3, 50)
			if params.ConfidenceThreshold != old {
				t.logTune(regime, "ConfidenceThreshold", old, params.ConfidenceThreshold,
					fmt.Sprintf("win rate %.1f%% > 65%%", perf.WinRate*100))
			}
		}

		// rule 3: if max drawdown > 10%, reduce position size
		if perf.MaxDrawdown > 10 {
			old := params.PositionSizePct
			params.PositionSizePct = math.Max(params.PositionSizePct*0.8, 0.5)
			if params.PositionSizePct != old {
				t.logTune(regime, "PositionSizePct", old, params.PositionSizePct,
					fmt.Sprintf("max drawdown %.1f%% > 10%%", perf.MaxDrawdown))
			}
		}

		// rule 4: if sharpe ratio > 1.5, can increase position size
		if perf.SharpeRatio > 1.5 && perf.MaxDrawdown < 5 {
			old := params.PositionSizePct
			params.PositionSizePct = math.Min(params.PositionSizePct*1.1, 5.0)
			if params.PositionSizePct != old {
				t.logTune(regime, "PositionSizePct", old, params.PositionSizePct,
					fmt.Sprintf("sharpe %.2f > 1.5, drawdown %.1f%% < 5%%", perf.SharpeRatio, perf.MaxDrawdown))
			}
		}

		// rule 5: if avg PnL is negative, widen stops
		if perf.AvgPnL < 0 {
			old := params.StopLossPct
			params.StopLossPct = math.Min(params.StopLossPct*1.15, 8.0)
			if params.StopLossPct != old {
				t.logTune(regime, "StopLossPct", old, params.StopLossPct,
					fmt.Sprintf("avg PnL %.2f%% < 0", perf.AvgPnL))
			}
		}

		// rule 6: reduce leverage in volatile regime with losses
		if regime == "volatile" && perf.AvgPnL < -0.5 {
			old := float64(params.MaxLeverage)
			params.MaxLeverage = max(params.MaxLeverage-1, 1)
			if float64(params.MaxLeverage) != old {
				t.logTune(regime, "MaxLeverage", old, float64(params.MaxLeverage),
					fmt.Sprintf("volatile regime avg PnL %.2f%%", perf.AvgPnL))
			}
		}
	}
}

func (t *AutoTuner) getParamsPtr(regime string) *TunableParams {
	switch regime {
	case "trending":
		return &t.params.Trending
	case "ranging":
		return &t.params.Ranging
	case "volatile":
		return &t.params.Volatile
	case "quiet":
		return &t.params.Quiet
	default:
		return nil
	}
}

func (t *AutoTuner) logTune(regime, field string, old, new float64, reason string) {
	event := TuneEvent{
		Timestamp: time.Now(),
		Regime:    regime,
		Field:     field,
		OldValue:  old,
		NewValue:  new,
		Reason:    reason,
	}
	t.tuneHistory = append(t.tuneHistory, event)
	if len(t.tuneHistory) > 100 {
		t.tuneHistory = t.tuneHistory[len(t.tuneHistory)-100:]
	}
	log.Printf("[autotuner] %s.%s: %.2f → %.2f (%s)", regime, field, old, new, reason)
}

// Run starts the auto-tuner evaluation loop (blocking, use in goroutine)
func (t *AutoTuner) Run(ctx context.Context) {
	ticker := time.NewTicker(t.evalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.mu.Lock()
			t.evaluate()
			t.lastEval = time.Now()
			t.mu.Unlock()
		}
	}
}
