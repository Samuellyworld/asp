package backtest

import (
	"math"
	"testing"
	"time"
)

// --- ComputeMetrics tests ---

func TestComputeMetrics_NoTrades(t *testing.T) {
	result := &Result{
		Config:      Config{InitialCapital: 10000},
		Trades:      nil,
		FinalEquity: 10000,
		EquityCurve: []EquityPoint{
			{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Equity: 10000},
		},
	}

	m := ComputeMetrics(result)
	if m.TotalTrades != 0 {
		t.Errorf("total trades = %d, want 0", m.TotalTrades)
	}
	if m.TotalReturn != 0 {
		t.Errorf("total return = %f, want 0", m.TotalReturn)
	}
	if m.WinRate != 0 {
		t.Errorf("win rate = %f, want 0", m.WinRate)
	}
}

func TestComputeMetrics_BasicProfitable(t *testing.T) {
	result := &Result{
		Config:      Config{InitialCapital: 10000},
		FinalEquity: 11500,
		Trades: []Trade{
			{PnL: 500, PnLPercent: 5, EntryFee: 5, ExitFee: 5, Bars: 10},
			{PnL: -200, PnLPercent: -2, EntryFee: 3, ExitFee: 3, Bars: 5},
			{PnL: 800, PnLPercent: 8, EntryFee: 8, ExitFee: 8, Bars: 15},
			{PnL: 400, PnLPercent: 4, EntryFee: 4, ExitFee: 4, Bars: 8},
		},
		EquityCurve: []EquityPoint{
			{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Equity: 10000},
			{Time: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Equity: 10500},
			{Time: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), Equity: 10300},
			{Time: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), Equity: 11500},
		},
	}

	m := ComputeMetrics(result)

	if m.TotalTrades != 4 {
		t.Errorf("total trades = %d, want 4", m.TotalTrades)
	}
	if m.WinningTrades != 3 {
		t.Errorf("winning = %d, want 3", m.WinningTrades)
	}
	if m.LosingTrades != 1 {
		t.Errorf("losing = %d, want 1", m.LosingTrades)
	}
	if m.WinRate != 75 {
		t.Errorf("win rate = %f, want 75", m.WinRate)
	}
	if m.TotalReturn != 1500 {
		t.Errorf("total return = %f, want 1500", m.TotalReturn)
	}
	if m.TotalReturnPct != 15 {
		t.Errorf("return pct = %f, want 15", m.TotalReturnPct)
	}
	if m.LargestWin != 800 {
		t.Errorf("largest win = %f, want 800", m.LargestWin)
	}
	if m.LargestLoss != -200 {
		t.Errorf("largest loss = %f, want -200", m.LargestLoss)
	}

	// profit factor: gross_profit=1700, gross_loss=200
	expectedPF := 1700.0 / 200.0
	if math.Abs(m.ProfitFactor-expectedPF) > 0.01 {
		t.Errorf("profit factor = %f, want %f", m.ProfitFactor, expectedPF)
	}

	// avg bars held
	expectedBars := float64(10+5+15+8) / 4
	if math.Abs(m.AvgBarsHeld-expectedBars) > 0.01 {
		t.Errorf("avg bars = %f, want %f", m.AvgBarsHeld, expectedBars)
	}

	// total fees
	expectedFees := 5.0 + 5 + 3 + 3 + 8 + 8 + 4 + 4
	if math.Abs(m.TotalFees-expectedFees) > 0.01 {
		t.Errorf("total fees = %f, want %f", m.TotalFees, expectedFees)
	}
}

func TestComputeMetrics_AllLosing(t *testing.T) {
	result := &Result{
		Config:      Config{InitialCapital: 10000},
		FinalEquity: 8000,
		Trades: []Trade{
			{PnL: -500, PnLPercent: -5, Bars: 3},
			{PnL: -800, PnLPercent: -8, Bars: 5},
			{PnL: -700, PnLPercent: -7, Bars: 4},
		},
		EquityCurve: []EquityPoint{
			{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Equity: 10000},
			{Time: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Equity: 9000},
			{Time: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), Equity: 8000},
		},
	}

	m := ComputeMetrics(result)
	if m.WinRate != 0 {
		t.Errorf("win rate = %f, want 0", m.WinRate)
	}
	if m.MaxConsecLosses != 3 {
		t.Errorf("max consec losses = %d, want 3", m.MaxConsecLosses)
	}
	if m.ProfitFactor != 0 {
		t.Errorf("profit factor = %f, want 0", m.ProfitFactor)
	}
}

func TestComputeMetrics_ConsecutiveStreaks(t *testing.T) {
	result := &Result{
		Config:      Config{InitialCapital: 10000},
		FinalEquity: 10500,
		Trades: []Trade{
			{PnL: 100, PnLPercent: 1},
			{PnL: 200, PnLPercent: 2},
			{PnL: 150, PnLPercent: 1.5},
			{PnL: -50, PnLPercent: -0.5},
			{PnL: -100, PnLPercent: -1},
			{PnL: 300, PnLPercent: 3},
		},
		EquityCurve: []EquityPoint{
			{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Equity: 10000},
			{Time: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC), Equity: 10500},
		},
	}

	m := ComputeMetrics(result)
	if m.MaxConsecWins != 3 {
		t.Errorf("max consec wins = %d, want 3", m.MaxConsecWins)
	}
	if m.MaxConsecLosses != 2 {
		t.Errorf("max consec losses = %d, want 2", m.MaxConsecLosses)
	}
}

// --- MaxDrawdown tests ---

func TestMaxDrawdown_NoDrawdown(t *testing.T) {
	equity := []EquityPoint{
		{Equity: 100}, {Equity: 110}, {Equity: 120}, {Equity: 130},
	}
	pct, usd := maxDrawdown(equity)
	if pct != 0 {
		t.Errorf("drawdown pct = %f, want 0", pct)
	}
	if usd != 0 {
		t.Errorf("drawdown usd = %f, want 0", usd)
	}
}

func TestMaxDrawdown_BasicDrawdown(t *testing.T) {
	equity := []EquityPoint{
		{Equity: 100}, {Equity: 120}, {Equity: 90}, {Equity: 110},
	}
	pct, usd := maxDrawdown(equity)
	// peak = 120, trough = 90, drawdown = 30/120 = 25%
	if math.Abs(pct-25) > 0.01 {
		t.Errorf("drawdown pct = %f, want 25", pct)
	}
	if math.Abs(usd-30) > 0.01 {
		t.Errorf("drawdown usd = %f, want 30", usd)
	}
}

func TestMaxDrawdown_Empty(t *testing.T) {
	pct, usd := maxDrawdown(nil)
	if pct != 0 || usd != 0 {
		t.Error("expected 0 for empty equity curve")
	}
}

// --- Sharpe/Sortino tests ---

func TestSharpeRatio_InsufficientTrades(t *testing.T) {
	got := sharpeRatio([]Trade{{PnLPercent: 5}}, 1)
	if got != 0 {
		t.Errorf("sharpe with 1 trade = %f, want 0", got)
	}
}

func TestSharpeRatio_PositiveTrades(t *testing.T) {
	trades := []Trade{
		{PnLPercent: 5}, {PnLPercent: 3}, {PnLPercent: 7},
		{PnLPercent: -2}, {PnLPercent: 4}, {PnLPercent: 6},
	}
	got := sharpeRatio(trades, 1)
	if got <= 0 {
		t.Errorf("sharpe should be positive for mostly profitable trades, got %f", got)
	}
}

func TestSortinoRatio_NoDownside(t *testing.T) {
	trades := []Trade{
		{PnLPercent: 5}, {PnLPercent: 3}, {PnLPercent: 7},
	}
	got := sortinoRatio(trades, 1)
	if got != 0 {
		t.Errorf("sortino with no downside = %f, want 0", got)
	}
}

// --- TradesByPnL tests ---

func TestTradesByPnL_Sort(t *testing.T) {
	trades := []Trade{
		{PnL: -100}, {PnL: 500}, {PnL: 200}, {PnL: -50},
	}
	sorted := TradesByPnL(trades)
	if sorted[0].PnL != 500 {
		t.Errorf("first = %f, want 500", sorted[0].PnL)
	}
	if sorted[3].PnL != -100 {
		t.Errorf("last = %f, want -100", sorted[3].PnL)
	}
	// original unchanged
	if trades[0].PnL != -100 {
		t.Error("original slice was modified")
	}
}

// --- helper function tests ---

func TestAvg(t *testing.T) {
	if avg(nil) != 0 {
		t.Error("avg(nil) should be 0")
	}
	if avg([]float64{2, 4, 6}) != 4 {
		t.Error("avg should be 4")
	}
}

func TestStddev(t *testing.T) {
	if stddev(nil, 0) != 0 {
		t.Error("stddev(nil) should be 0")
	}
	vals := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	mean := avg(vals)
	sd := stddev(vals, mean)
	if sd < 1 || sd > 3 {
		t.Errorf("stddev = %f, expected ~2", sd)
	}
}
