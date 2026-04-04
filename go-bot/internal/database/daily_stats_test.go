package database

import (
	"testing"
	"time"
)

func TestDailyStatsRecord_Fields(t *testing.T) {
	rec := &DailyStatsRecord{
		UserID:              1,
		Date:                time.Now(),
		TotalTrades:         5,
		WinningTrades:       3,
		LosingTrades:        2,
		RealizedPnL:         150.50,
		FeesPaid:            2.50,
		FundingPaid:         1.20,
		AIDecisionsMade:     10,
		AIDecisionsApproved: 5,
		NotificationsSent:   5,
	}

	if rec.TotalTrades != 5 {
		t.Errorf("expected 5 total trades, got %d", rec.TotalTrades)
	}
	if rec.WinningTrades+rec.LosingTrades != rec.TotalTrades {
		t.Errorf("wins (%d) + losses (%d) should equal total (%d)",
			rec.WinningTrades, rec.LosingTrades, rec.TotalTrades)
	}
	if rec.RealizedPnL != 150.50 {
		t.Errorf("expected PnL 150.50, got %f", rec.RealizedPnL)
	}
}

func TestDailyStatsRecord_WinRate(t *testing.T) {
	cases := []struct {
		name    string
		wins    int
		total   int
		wantPct float64
	}{
		{"all wins", 5, 5, 100.0},
		{"no wins", 0, 5, 0.0},
		{"half", 3, 6, 50.0},
		{"no trades", 0, 0, 0.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &DailyStatsRecord{
				WinningTrades: tc.wins,
				TotalTrades:   tc.total,
			}
			var pct float64
			if rec.TotalTrades > 0 {
				pct = float64(rec.WinningTrades) / float64(rec.TotalTrades) * 100
			}
			if pct != tc.wantPct {
				t.Errorf("expected win rate %.1f%%, got %.1f%%", tc.wantPct, pct)
			}
		})
	}
}

func TestNewDailyStatsRepository(t *testing.T) {
	repo := NewDailyStatsRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}
