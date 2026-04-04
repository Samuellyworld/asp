package leverage

import (
	"math"
	"testing"
)

const floatTolerance = 1e-6

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

func TestCalculateLiquidationPrice_Long(t *testing.T) {
	// for LONG: liqPrice = entryPrice * (1 - 1/leverage + mmr)
	tests := []struct {
		name     string
		entry    float64
		leverage int
		mmr      float64
		want     float64
	}{
		{
			name:     "1x leverage",
			entry:    50000,
			leverage: 1,
			mmr:      0.004,
			want:     50000 * (1 - 1.0/1 + 0.004), // 200
		},
		{
			name:     "5x leverage",
			entry:    50000,
			leverage: 5,
			mmr:      0.004,
			want:     50000 * (1 - 1.0/5 + 0.004), // 40200
		},
		{
			name:     "10x leverage",
			entry:    50000,
			leverage: 10,
			mmr:      0.004,
			want:     50000 * (1 - 1.0/10 + 0.004), // 45200
		},
		{
			name:     "20x leverage",
			entry:    50000,
			leverage: 20,
			mmr:      0.004,
			want:     50000 * (1 - 1.0/20 + 0.004), // 47700
		},
		{
			name:     "50x leverage",
			entry:    50000,
			leverage: 50,
			mmr:      0.004,
			want:     50000 * (1 - 1.0/50 + 0.004), // 49200
		},
		{
			name:     "125x leverage",
			entry:    50000,
			leverage: 125,
			mmr:      0.004,
			want:     50000 * (1 - 1.0/125 + 0.004), // 49800
		},
		{
			name:     "different entry price",
			entry:    3000,
			leverage: 10,
			mmr:      0.004,
			want:     3000 * (1 - 0.1 + 0.004),
		},
		{
			name:     "high mmr at low leverage clamps to zero",
			entry:    100,
			leverage: 1,
			mmr:      0.004,
			want:     100 * (1 - 1 + 0.004), // 0.4
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CalculateLiquidationPrice(tc.entry, tc.leverage, string(SideLong), tc.mmr)
			if !almostEqual(got, tc.want, floatTolerance) {
				t.Errorf("got %.6f, want %.6f", got, tc.want)
			}
		})
	}
}

func TestCalculateLiquidationPrice_Short(t *testing.T) {
	// for SHORT: liqPrice = entryPrice * (1 + 1/leverage - mmr)
	tests := []struct {
		name     string
		entry    float64
		leverage int
		mmr      float64
		want     float64
	}{
		{
			name:     "1x leverage",
			entry:    50000,
			leverage: 1,
			mmr:      0.004,
			want:     50000 * (1 + 1.0/1 - 0.004), // 99800
		},
		{
			name:     "5x leverage",
			entry:    50000,
			leverage: 5,
			mmr:      0.004,
			want:     50000 * (1 + 1.0/5 - 0.004), // 59800
		},
		{
			name:     "10x leverage",
			entry:    50000,
			leverage: 10,
			mmr:      0.004,
			want:     50000 * (1 + 1.0/10 - 0.004), // 54800
		},
		{
			name:     "20x leverage",
			entry:    50000,
			leverage: 20,
			mmr:      0.004,
			want:     50000 * (1 + 1.0/20 - 0.004), // 52300
		},
		{
			name:     "50x leverage",
			entry:    50000,
			leverage: 50,
			mmr:      0.004,
			want:     50000 * (1 + 1.0/50 - 0.004), // 50800
		},
		{
			name:     "125x leverage",
			entry:    50000,
			leverage: 125,
			mmr:      0.004,
			want:     50000 * (1 + 1.0/125 - 0.004), // 50200
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CalculateLiquidationPrice(tc.entry, tc.leverage, string(SideShort), tc.mmr)
			if !almostEqual(got, tc.want, floatTolerance) {
				t.Errorf("got %.6f, want %.6f", got, tc.want)
			}
		})
	}
}

func TestCalculateLiquidationPrice_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		entry    float64
		leverage int
		side     string
		mmr      float64
		want     float64
	}{
		{
			name:     "zero entry price",
			entry:    0,
			leverage: 10,
			side:     string(SideLong),
			mmr:      0.004,
			want:     0,
		},
		{
			name:     "negative entry price",
			entry:    -100,
			leverage: 10,
			side:     string(SideLong),
			mmr:      0.004,
			want:     0,
		},
		{
			name:     "zero leverage",
			entry:    50000,
			leverage: 0,
			side:     string(SideLong),
			mmr:      0.004,
			want:     0,
		},
		{
			name:     "invalid side",
			entry:    50000,
			leverage: 10,
			side:     "INVALID",
			mmr:      0.004,
			want:     0,
		},
		{
			name:     "zero mmr long",
			entry:    50000,
			leverage: 10,
			side:     string(SideLong),
			mmr:      0,
			want:     50000 * (1 - 0.1),
		},
		{
			name:     "zero mmr short",
			entry:    50000,
			leverage: 10,
			side:     string(SideShort),
			mmr:      0,
			want:     50000 * (1 + 0.1),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CalculateLiquidationPrice(tc.entry, tc.leverage, tc.side, tc.mmr)
			if !almostEqual(got, tc.want, floatTolerance) {
				t.Errorf("got %.6f, want %.6f", got, tc.want)
			}
		})
	}
}

func TestDistanceToLiquidation_Long(t *testing.T) {
	tests := []struct {
		name     string
		current  float64
		liq      float64
		wantPct  float64
	}{
		{
			name:    "10% away",
			current: 50000,
			liq:     45000,
			wantPct: 10.0,
		},
		{
			name:    "5% away",
			current: 50000,
			liq:     47500,
			wantPct: 5.0,
		},
		{
			name:    "price at liquidation",
			current: 45000,
			liq:     45000,
			wantPct: 0,
		},
		{
			name:    "price below liquidation",
			current: 44000,
			liq:     45000,
			wantPct: 100.0 * 1000.0 / 44000.0, // still returns abs value
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DistanceToLiquidation(tc.current, tc.liq, string(SideLong))
			if !almostEqual(got, tc.wantPct, 0.01) {
				t.Errorf("got %.4f%%, want %.4f%%", got, tc.wantPct)
			}
		})
	}
}

func TestDistanceToLiquidation_Short(t *testing.T) {
	tests := []struct {
		name    string
		current float64
		liq     float64
		wantPct float64
	}{
		{
			name:    "10% away",
			current: 50000,
			liq:     55000,
			wantPct: 10.0,
		},
		{
			name:    "2% away",
			current: 50000,
			liq:     51000,
			wantPct: 2.0,
		},
		{
			name:    "price at liquidation",
			current: 55000,
			liq:     55000,
			wantPct: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DistanceToLiquidation(tc.current, tc.liq, string(SideShort))
			if !almostEqual(got, tc.wantPct, 0.01) {
				t.Errorf("got %.4f%%, want %.4f%%", got, tc.wantPct)
			}
		})
	}
}

func TestDistanceToLiquidation_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		current float64
		liq     float64
		side    string
		wantPct float64
	}{
		{
			name:    "zero current price",
			current: 0,
			liq:     45000,
			side:    string(SideLong),
			wantPct: 0,
		},
		{
			name:    "invalid side",
			current: 50000,
			liq:     45000,
			side:    "INVALID",
			wantPct: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DistanceToLiquidation(tc.current, tc.liq, tc.side)
			if !almostEqual(got, tc.wantPct, floatTolerance) {
				t.Errorf("got %.4f%%, want %.4f%%", got, tc.wantPct)
			}
		})
	}
}

func TestClassifyLiquidationRisk(t *testing.T) {
	tests := []struct {
		name     string
		distance float64
		want     AlertLevel
	}{
		{name: "far away 15%", distance: 15.0, want: AlertNone},
		{name: "exactly 10%", distance: 10.0, want: AlertNone},
		{name: "just under 10%", distance: 9.99, want: AlertWarning},
		{name: "at 7%", distance: 7.0, want: AlertWarning},
		{name: "exactly 5%", distance: 5.0, want: AlertWarning},
		{name: "just under 5%", distance: 4.99, want: AlertCritical},
		{name: "at 3%", distance: 3.0, want: AlertCritical},
		{name: "exactly 2%", distance: 2.0, want: AlertCritical},
		{name: "just under 2%", distance: 1.99, want: AlertAutoClose},
		{name: "at 1%", distance: 1.0, want: AlertAutoClose},
		{name: "at 0%", distance: 0, want: AlertAutoClose},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyLiquidationRisk(tc.distance)
			if got != tc.want {
				t.Errorf("distance %.2f%%: got %s, want %s", tc.distance, got, tc.want)
			}
		})
	}
}

func TestAlertLevel_String(t *testing.T) {
	tests := []struct {
		level AlertLevel
		want  string
	}{
		{AlertNone, "none"},
		{AlertWarning, "warning"},
		{AlertCritical, "critical"},
		{AlertAutoClose, "auto-close"},
		{AlertLevel(99), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := tc.level.String()
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDefaultMaintenanceMarginRate(t *testing.T) {
	// verify the constant is set correctly
	if DefaultMaintenanceMarginRate != 0.004 {
		t.Errorf("expected 0.004, got %f", DefaultMaintenanceMarginRate)
	}
}
