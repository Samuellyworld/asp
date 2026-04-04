package leverage

import (
	"testing"
	"time"
)

func TestUnrealizedPnL(t *testing.T) {
	tests := []struct {
		name     string
		side     PositionSide
		entry    float64
		mark     float64
		qty      float64
		wantPnL  float64
	}{
		{
			name:    "long profit",
			side:    SideLong,
			entry:   50000,
			mark:    51000,
			qty:     0.1,
			wantPnL: 100, // (51000-50000)*0.1
		},
		{
			name:    "long loss",
			side:    SideLong,
			entry:   50000,
			mark:    49000,
			qty:     0.1,
			wantPnL: -100, // (49000-50000)*0.1
		},
		{
			name:    "short profit",
			side:    SideShort,
			entry:   50000,
			mark:    49000,
			qty:     0.1,
			wantPnL: 100, // (50000-49000)*0.1
		},
		{
			name:    "short loss",
			side:    SideShort,
			entry:   50000,
			mark:    51000,
			qty:     0.1,
			wantPnL: -100, // (50000-51000)*0.1
		},
		{
			name:    "no movement",
			side:    SideLong,
			entry:   50000,
			mark:    50000,
			qty:     1.0,
			wantPnL: 0,
		},
		{
			name:    "invalid side",
			side:    "INVALID",
			entry:   50000,
			mark:    51000,
			qty:     0.1,
			wantPnL: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &LeveragePosition{
				Side:       tc.side,
				EntryPrice: tc.entry,
				MarkPrice:  tc.mark,
				Quantity:   tc.qty,
			}
			got := p.UnrealizedPnL()
			if !almostEqual(got, tc.wantPnL, floatTolerance) {
				t.Errorf("got %.6f, want %.6f", got, tc.wantPnL)
			}
		})
	}
}

func TestUnrealizedPnLPercent(t *testing.T) {
	tests := []struct {
		name    string
		side    PositionSide
		entry   float64
		mark    float64
		qty     float64
		margin  float64
		wantPct float64
	}{
		{
			name:    "long 10x profit 2%",
			side:    SideLong,
			entry:   50000,
			mark:    51000,
			qty:     0.1,
			margin:  500, // 5000/10
			wantPct: 20,  // 100/500 * 100 = 20% (leveraged return)
		},
		{
			name:    "short 10x profit 2%",
			side:    SideShort,
			entry:   50000,
			mark:    49000,
			qty:     0.1,
			margin:  500,
			wantPct: 20,
		},
		{
			name:    "long loss with leverage",
			side:    SideLong,
			entry:   50000,
			mark:    49500,
			qty:     0.1,
			margin:  500,
			wantPct: -10, // -50/500 * 100
		},
		{
			name:    "zero margin returns zero",
			side:    SideLong,
			entry:   50000,
			mark:    51000,
			qty:     0.1,
			margin:  0,
			wantPct: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &LeveragePosition{
				Side:       tc.side,
				EntryPrice: tc.entry,
				MarkPrice:  tc.mark,
				Quantity:   tc.qty,
				Margin:     tc.margin,
			}
			got := p.UnrealizedPnLPercent()
			if !almostEqual(got, tc.wantPct, floatTolerance) {
				t.Errorf("got %.6f%%, want %.6f%%", got, tc.wantPct)
			}
		})
	}
}

func TestROI(t *testing.T) {
	tests := []struct {
		name    string
		side    PositionSide
		entry   float64
		mark    float64
		qty     float64
		margin  float64
		wantROI float64
	}{
		{
			name:    "long 20x doubled margin",
			side:    SideLong,
			entry:   50000,
			mark:    52500,
			qty:     0.2,
			margin:  500, // notional 10000, leverage 20x
			wantROI: 100, // pnl 500 / margin 500 * 100
		},
		{
			name:    "short losing half margin",
			side:    SideShort,
			entry:   50000,
			mark:    51250,
			qty:     0.2,
			margin:  500,
			wantROI: -50, // pnl -250 / margin 500 * 100
		},
		{
			name:    "zero margin",
			side:    SideLong,
			entry:   50000,
			mark:    51000,
			qty:     0.1,
			margin:  0,
			wantROI: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &LeveragePosition{
				Side:       tc.side,
				EntryPrice: tc.entry,
				MarkPrice:  tc.mark,
				Quantity:   tc.qty,
				Margin:     tc.margin,
			}
			got := p.ROI()
			if !almostEqual(got, tc.wantROI, floatTolerance) {
				t.Errorf("got %.6f%%, want %.6f%%", got, tc.wantROI)
			}
		})
	}
}

func TestIsTPHit(t *testing.T) {
	tests := []struct {
		name string
		side PositionSide
		mark float64
		tp   float64
		want bool
	}{
		{name: "long tp not set", side: SideLong, mark: 55000, tp: 0, want: false},
		{name: "long below tp", side: SideLong, mark: 54000, tp: 55000, want: false},
		{name: "long at tp", side: SideLong, mark: 55000, tp: 55000, want: true},
		{name: "long above tp", side: SideLong, mark: 56000, tp: 55000, want: true},
		{name: "short tp not set", side: SideShort, mark: 45000, tp: 0, want: false},
		{name: "short above tp", side: SideShort, mark: 46000, tp: 45000, want: false},
		{name: "short at tp", side: SideShort, mark: 45000, tp: 45000, want: true},
		{name: "short below tp", side: SideShort, mark: 44000, tp: 45000, want: true},
		{name: "invalid side", side: "INVALID", mark: 55000, tp: 55000, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &LeveragePosition{
				Side:       tc.side,
				MarkPrice:  tc.mark,
				TakeProfit: tc.tp,
			}
			if got := p.IsTPHit(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsSLHit(t *testing.T) {
	tests := []struct {
		name string
		side PositionSide
		mark float64
		sl   float64
		want bool
	}{
		{name: "long sl not set", side: SideLong, mark: 48000, sl: 0, want: false},
		{name: "long above sl", side: SideLong, mark: 49000, sl: 48000, want: false},
		{name: "long at sl", side: SideLong, mark: 48000, sl: 48000, want: true},
		{name: "long below sl", side: SideLong, mark: 47000, sl: 48000, want: true},
		{name: "short sl not set", side: SideShort, mark: 52000, sl: 0, want: false},
		{name: "short below sl", side: SideShort, mark: 51000, sl: 52000, want: false},
		{name: "short at sl", side: SideShort, mark: 52000, sl: 52000, want: true},
		{name: "short above sl", side: SideShort, mark: 53000, sl: 52000, want: true},
		{name: "invalid side", side: "INVALID", mark: 48000, sl: 48000, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &LeveragePosition{
				Side:      tc.side,
				MarkPrice: tc.mark,
				StopLoss:  tc.sl,
			}
			if got := p.IsSLHit(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestShouldAutoClose(t *testing.T) {
	tests := []struct {
		name string
		side PositionSide
		mark float64
		liq  float64
		want bool
	}{
		{
			name: "long far from liquidation",
			side: SideLong,
			mark: 50000,
			liq:  40000, // 20% away
			want: false,
		},
		{
			name: "long within auto-close zone",
			side: SideLong,
			mark: 50000,
			liq:  49500, // 1% away
			want: true,
		},
		{
			name: "long at critical but not auto-close",
			side: SideLong,
			mark: 50000,
			liq:  48500, // 3% away
			want: false,
		},
		{
			name: "short far from liquidation",
			side: SideShort,
			mark: 50000,
			liq:  60000, // 20% away
			want: false,
		},
		{
			name: "short within auto-close zone",
			side: SideShort,
			mark: 50000,
			liq:  50500, // 1% away
			want: true,
		},
		{
			name: "short at warning level",
			side: SideShort,
			mark: 50000,
			liq:  54000, // 8% away
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &LeveragePosition{
				Side:             tc.side,
				MarkPrice:        tc.mark,
				LiquidationPrice: tc.liq,
			}
			if got := p.ShouldAutoClose(DefaultMaintenanceMarginRate); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLeveragePositionFields(t *testing.T) {
	// verify a fully populated position can be constructed
	now := time.Now()
	closedAt := now.Add(time.Hour)
	p := LeveragePosition{
		ID:               "pos_123",
		UserID:           42,
		Symbol:           "BTCUSDT",
		Side:             SideLong,
		Leverage:         10,
		EntryPrice:       50000,
		MarkPrice:        51000,
		Quantity:         0.1,
		Margin:           500,
		NotionalValue:    5000,
		LiquidationPrice: 45200,
		StopLoss:         48000,
		TakeProfit:       55000,
		FundingPaid:      1.23,
		MarginType:       "isolated",
		IsPaper:          true,
		Status:           "open",
		CloseReason:      "",
		ClosePrice:       0,
		PnL:              0,
		OpenedAt:         now,
		ClosedAt:         &closedAt,
		Platform:         "telegram",
		MainOrderID:      0,
		SLOrderID:        0,
		TPOrderID:        0,
	}

	if p.ID != "pos_123" {
		t.Errorf("unexpected ID: %s", p.ID)
	}
	if p.Side != SideLong {
		t.Errorf("unexpected side: %s", p.Side)
	}
	if p.IsPaper != true {
		t.Error("expected IsPaper to be true")
	}
	if p.ClosedAt == nil || !p.ClosedAt.Equal(closedAt) {
		t.Error("unexpected ClosedAt value")
	}
}

func TestPositionSideConstants(t *testing.T) {
	if SideLong != "LONG" {
		t.Errorf("SideLong = %q, want LONG", SideLong)
	}
	if SideShort != "SHORT" {
		t.Errorf("SideShort = %q, want SHORT", SideShort)
	}
}
