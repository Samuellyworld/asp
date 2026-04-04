package exchange

import (
	"math"
	"testing"
)

func TestCalculateSlippageBps_BuyUnfavorable(t *testing.T) {
	// expected 42000, got filled at 42042 (0.1% = 10 bps)
	bps := CalculateSlippageBps(42000, 42042, "BUY")
	if math.Abs(bps-10) > 0.1 {
		t.Errorf("expected ~10 bps, got %.2f", bps)
	}
}

func TestCalculateSlippageBps_BuyFavorable(t *testing.T) {
	// expected 42000, got filled at 41958 (-1 bps, favorable)
	bps := CalculateSlippageBps(42000, 41958, "BUY")
	if bps >= 0 {
		t.Errorf("expected negative (favorable) bps for buy with lower actual, got %.2f", bps)
	}
}

func TestCalculateSlippageBps_SellUnfavorable(t *testing.T) {
	// expected sell at 42000, got 41958 (unfavorable — got less)
	bps := CalculateSlippageBps(42000, 41958, "SELL")
	if bps <= 0 {
		t.Errorf("expected positive (unfavorable) bps for sell at lower price, got %.2f", bps)
	}
}

func TestCalculateSlippageBps_SellFavorable(t *testing.T) {
	// expected sell at 42000, got 42042 (favorable — got more)
	bps := CalculateSlippageBps(42000, 42042, "SELL")
	if bps >= 0 {
		t.Errorf("expected negative (favorable) bps for sell at higher price, got %.2f", bps)
	}
}

func TestCalculateSlippageBps_ZeroExpected(t *testing.T) {
	bps := CalculateSlippageBps(0, 42000, "BUY")
	if bps != 0 {
		t.Errorf("expected 0 for zero expected price, got %.2f", bps)
	}
}

func TestCalculateSlippageBps_NoSlippage(t *testing.T) {
	bps := CalculateSlippageBps(42000, 42000, "BUY")
	if bps != 0 {
		t.Errorf("expected 0 bps for no slippage, got %.2f", bps)
	}
}

func TestSlippageTracker_Record(t *testing.T) {
	tracker := NewSlippageTracker(100)

	rec := tracker.Record("BTC/USDT", "BUY", 42000, 42042, 0.5, true)

	if rec.Symbol != "BTC/USDT" {
		t.Errorf("expected BTC/USDT, got %s", rec.Symbol)
	}
	if rec.SlippageBps <= 0 {
		t.Errorf("expected positive slippage for unfavorable buy, got %.2f", rec.SlippageBps)
	}
}

func TestSlippageTracker_StatsForSymbol(t *testing.T) {
	tracker := NewSlippageTracker(100)

	tracker.Record("BTC/USDT", "BUY", 42000, 42042, 0.5, true)  // ~10 bps
	tracker.Record("BTC/USDT", "BUY", 42000, 42021, 0.5, true)  // ~5 bps
	tracker.Record("ETH/USDT", "SELL", 3000, 2997, 2.0, false)  // different symbol

	stats := tracker.StatsForSymbol("BTC/USDT")

	if stats.TradeCount != 2 {
		t.Errorf("expected 2 trades, got %d", stats.TradeCount)
	}
	if stats.AvgSlippageBps < 5 || stats.AvgSlippageBps > 10 {
		t.Errorf("expected avg slippage between 5-10 bps, got %.2f", stats.AvgSlippageBps)
	}
}

func TestSlippageTracker_MaxSize(t *testing.T) {
	tracker := NewSlippageTracker(5)

	for i := 0; i < 10; i++ {
		tracker.Record("BTC/USDT", "BUY", 42000, 42000+float64(i), 1, true)
	}

	recent := tracker.RecentRecords(100)
	if len(recent) != 5 {
		t.Errorf("expected 5 records (max size), got %d", len(recent))
	}
}

func TestSlippageTracker_AllStats(t *testing.T) {
	tracker := NewSlippageTracker(100)

	tracker.Record("BTC/USDT", "BUY", 42000, 42042, 0.5, true)
	tracker.Record("ETH/USDT", "BUY", 3000, 3003, 2.0, true)

	stats := tracker.AllStats()
	if len(stats) != 2 {
		t.Errorf("expected 2 symbol stats, got %d", len(stats))
	}
}

func TestSlippageTracker_RecentRecords(t *testing.T) {
	tracker := NewSlippageTracker(100)

	tracker.Record("BTC/USDT", "BUY", 42000, 42010, 1, true)
	tracker.Record("BTC/USDT", "SELL", 42100, 42090, 1, true)
	tracker.Record("ETH/USDT", "BUY", 3000, 3005, 1, true)

	recent := tracker.RecentRecords(2)
	if len(recent) != 2 {
		t.Errorf("expected 2 recent records, got %d", len(recent))
	}

	// most recent should be ETH
	if recent[1].Symbol != "ETH/USDT" {
		t.Errorf("expected last record to be ETH/USDT, got %s", recent[1].Symbol)
	}
}

func TestSlippageTracker_EmptyStats(t *testing.T) {
	tracker := NewSlippageTracker(100)

	stats := tracker.StatsForSymbol("BTC/USDT")
	if stats.TradeCount != 0 {
		t.Errorf("expected 0 trades, got %d", stats.TradeCount)
	}
	if stats.AvgSlippageBps != 0 {
		t.Errorf("expected 0 avg slippage, got %.2f", stats.AvgSlippageBps)
	}
}

func TestNewSlippageTracker_DefaultSize(t *testing.T) {
	tracker := NewSlippageTracker(0)
	if tracker.maxSize != 10000 {
		t.Errorf("expected default max size 10000, got %d", tracker.maxSize)
	}
}
