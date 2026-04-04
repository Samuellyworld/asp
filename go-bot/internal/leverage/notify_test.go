// tests for leverage notification formatters.
package leverage

import (
	"strings"
	"testing"
)

func TestFormatLeverageOpened(t *testing.T) {
	tests := []struct {
		name      string
		pos       *LeveragePosition
		wantParts []string
	}{
		{
			name: "long live position",
			pos: &LeveragePosition{
				Symbol:           "BTCUSDT",
				Side:             SideLong,
				Leverage:         10,
				EntryPrice:       50000,
				Margin:           500,
				NotionalValue:    5000,
				IsPaper:          false,
				LiquidationPrice: 45200,
				StopLoss:         48000,
				TakeProfit:       55000,
			},
			wantParts: []string{
				"📗", "LIVE", "Long", "BTCUSDT", "10x",
				"50000", "500", "5000", "45200", "48000", "55000",
			},
		},
		{
			name: "short paper position",
			pos: &LeveragePosition{
				Symbol:           "ETHUSDT",
				Side:             SideShort,
				Leverage:         5,
				EntryPrice:       3000,
				Margin:           200,
				NotionalValue:    1000,
				IsPaper:          true,
				LiquidationPrice: 3580,
			},
			wantParts: []string{
				"📕", "PAPER", "Short", "ETHUSDT", "5x",
				"3000", "200", "1000", "3580",
			},
		},
		{
			name: "position without sl/tp",
			pos: &LeveragePosition{
				Symbol:           "SOLUSDT",
				Side:             SideLong,
				Leverage:         3,
				EntryPrice:       150,
				Margin:           100,
				NotionalValue:    300,
				IsPaper:          false,
				LiquidationPrice: 100,
			},
			wantParts: []string{
				"📗", "LIVE", "Long", "SOLUSDT", "3x",
			},
		},
		{
			name: "position without liquidation price",
			pos: &LeveragePosition{
				Symbol:        "BTCUSDT",
				Side:          SideLong,
				Leverage:      2,
				EntryPrice:    50000,
				Margin:        1000,
				NotionalValue: 2000,
				IsPaper:       false,
			},
			wantParts: []string{
				"📗", "LIVE", "Long", "BTCUSDT",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatLeverageOpened(tc.pos)
			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot: %s", part, got)
				}
			}
		})
	}
}

func TestFormatLiquidationWarning(t *testing.T) {
	tests := []struct {
		name        string
		pos         *LeveragePosition
		distancePct float64
		wantParts   []string
	}{
		{
			name: "standard warning",
			pos: &LeveragePosition{
				Symbol:           "BTCUSDT",
				Leverage:         10,
				MarkPrice:        48000,
				LiquidationPrice: 45200,
			},
			distancePct: 5.83,
			wantParts:   []string{"⚠️", "BTCUSDT", "10x", "48000", "45200", "5.83"},
		},
		{
			name: "low price token",
			pos: &LeveragePosition{
				Symbol:           "DOGEUSDT",
				Leverage:         5,
				MarkPrice:        0.08,
				LiquidationPrice: 0.07,
			},
			distancePct: 8.5,
			wantParts:   []string{"⚠️", "DOGEUSDT", "5x", "0.080000", "0.070000", "8.50"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatLiquidationWarning(tc.pos, tc.distancePct)
			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot: %s", part, got)
				}
			}
		})
	}
}

func TestFormatLiquidationCritical(t *testing.T) {
	tests := []struct {
		name        string
		pos         *LeveragePosition
		distancePct float64
		wantParts   []string
	}{
		{
			name: "critical alert",
			pos: &LeveragePosition{
				Symbol:           "BTCUSDT",
				Leverage:         20,
				MarkPrice:        49000,
				LiquidationPrice: 47700,
			},
			distancePct: 2.65,
			wantParts:   []string{"🚨", "CRITICAL", "BTCUSDT", "20x", "49000", "47700", "2.65"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatLiquidationCritical(tc.pos, tc.distancePct)
			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot: %s", part, got)
				}
			}
		})
	}
}

func TestFormatLiquidationAutoClose(t *testing.T) {
	tests := []struct {
		name        string
		pos         *LeveragePosition
		distancePct float64
		wantParts   []string
	}{
		{
			name: "auto close with loss",
			pos: &LeveragePosition{
				Symbol:     "BTCUSDT",
				Side:       SideLong,
				Leverage:   20,
				EntryPrice: 50000,
				MarkPrice:  47800,
				Quantity:   0.1,
				Margin:     250,
				ClosePrice: 47800,
				PnL:        -220,
			},
			distancePct: 1.5,
			wantParts:   []string{"🔴", "AUTO-CLOSED", "BTCUSDT", "20x", "1.50%", "47800", "220"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatLiquidationAutoClose(tc.pos, tc.distancePct)
			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot: %s", part, got)
				}
			}
		})
	}
}

func TestFormatFundingFee(t *testing.T) {
	tests := []struct {
		name      string
		pos       *LeveragePosition
		rate      float64
		amount    float64
		wantParts []string
	}{
		{
			name: "positive funding fee received",
			pos: &LeveragePosition{
				Symbol:      "BTCUSDT",
				Leverage:    10,
				FundingPaid: 2.50,
			},
			rate:      0.0001,
			amount:    0.50,
			wantParts: []string{"💰", "BTCUSDT", "10x", "0.0100%", "0.5000", "2.5000"},
		},
		{
			name: "negative funding fee paid",
			pos: &LeveragePosition{
				Symbol:      "ETHUSDT",
				Leverage:    5,
				FundingPaid: -3.75,
			},
			rate:      0.0003,
			amount:    -1.25,
			wantParts: []string{"💸", "ETHUSDT", "5x", "0.0300%", "1.2500", "3.7500"},
		},
		{
			name: "zero cumulative funding",
			pos: &LeveragePosition{
				Symbol:      "SOLUSDT",
				Leverage:    3,
				FundingPaid: 0,
			},
			rate:      0.0001,
			amount:    0.10,
			wantParts: []string{"💰", "SOLUSDT", "3x", "0.1000"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatFundingFee(tc.pos, tc.rate, tc.amount)
			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot: %s", part, got)
				}
			}
		})
	}
}

func TestFormatLeverageClosed(t *testing.T) {
	tests := []struct {
		name      string
		pos       *LeveragePosition
		wantParts []string
	}{
		{
			name: "profitable close",
			pos: &LeveragePosition{
				Symbol:      "BTCUSDT",
				Side:        SideLong,
				Leverage:    10,
				EntryPrice:  50000,
				MarkPrice:   52000,
				Quantity:    0.1,
				Margin:      500,
				ClosePrice:  52000,
				PnL:         200,
				IsPaper:     false,
				CloseReason: "take_profit",
			},
			wantParts: []string{"🎉", "LIVE", "Take Profit", "BTCUSDT", "10x", "200", "52000", "50000"},
		},
		{
			name: "loss close with stop loss",
			pos: &LeveragePosition{
				Symbol:      "ETHUSDT",
				Side:        SideShort,
				Leverage:    5,
				EntryPrice:  3000,
				MarkPrice:   3200,
				Quantity:    1.0,
				Margin:      600,
				ClosePrice:  3200,
				PnL:         -200,
				IsPaper:     true,
				CloseReason: "stop_loss",
			},
			wantParts: []string{"⚠️", "PAPER", "Stop Loss", "ETHUSDT", "5x", "200", "3200", "3000"},
		},
		{
			name: "auto close near liquidation",
			pos: &LeveragePosition{
				Symbol:      "BTCUSDT",
				Side:        SideLong,
				Leverage:    20,
				EntryPrice:  50000,
				MarkPrice:   47800,
				Quantity:    0.1,
				Margin:      250,
				ClosePrice:  47800,
				PnL:         -220,
				IsPaper:     false,
				CloseReason: "auto_close",
			},
			wantParts: []string{"🔴", "LIVE", "Auto-Closed", "BTCUSDT", "20x", "47800", "50000"},
		},
		{
			name: "manual close",
			pos: &LeveragePosition{
				Symbol:      "SOLUSDT",
				Side:        SideLong,
				Leverage:    3,
				EntryPrice:  150,
				MarkPrice:   155,
				Quantity:    10,
				Margin:      500,
				ClosePrice:  155,
				PnL:         50,
				IsPaper:     false,
				CloseReason: "manual",
			},
			wantParts: []string{"👤", "LIVE", "Manually Closed", "SOLUSDT", "3x"},
		},
		{
			name: "emergency stop",
			pos: &LeveragePosition{
				Symbol:      "BTCUSDT",
				Side:        SideLong,
				Leverage:    10,
				EntryPrice:  50000,
				MarkPrice:   45000,
				Quantity:    0.1,
				Margin:      500,
				ClosePrice:  45000,
				PnL:         -500,
				IsPaper:     false,
				CloseReason: "emergency_stop",
			},
			wantParts: []string{"🚨", "LIVE", "Emergency Stop", "BTCUSDT"},
		},
		{
			name: "close with funding fees",
			pos: &LeveragePosition{
				Symbol:      "BTCUSDT",
				Side:        SideLong,
				Leverage:    10,
				EntryPrice:  50000,
				MarkPrice:   51000,
				Quantity:    0.1,
				Margin:      500,
				ClosePrice:  51000,
				PnL:         100,
				FundingPaid: -1.25,
				IsPaper:     false,
				CloseReason: "take_profit",
			},
			wantParts: []string{"Funding fees", "1.2500"},
		},
		{
			name: "close without funding fees",
			pos: &LeveragePosition{
				Symbol:      "BTCUSDT",
				Side:        SideLong,
				Leverage:    10,
				EntryPrice:  50000,
				MarkPrice:   51000,
				Quantity:    0.1,
				Margin:      500,
				ClosePrice:  51000,
				PnL:         100,
				FundingPaid: 0,
				IsPaper:     false,
				CloseReason: "take_profit",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatLeverageClosed(tc.pos)
			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot: %s", part, got)
				}
			}
			// verify no funding fees line when FundingPaid is zero
			if tc.pos.FundingPaid == 0 && strings.Contains(got, "Funding fees") {
				t.Errorf("should not contain funding fees line when FundingPaid is 0\ngot: %s", got)
			}
		})
	}
}

func TestFormatLeverageOptInPrompt(t *testing.T) {
	tests := []struct {
		name         string
		maxLeverage  int
		maxMargin    float64
		wantParts    []string
	}{
		{
			name:        "default limits",
			maxLeverage: 10,
			maxMargin:   500,
			wantParts: []string{
				"Leverage Trading",
				"10x",
				"500",
				"gains AND losses",
				"MORE than your margin",
				"Auto-close",
				"I UNDERSTAND LEVERAGE RISKS",
			},
		},
		{
			name:        "high limits",
			maxLeverage: 125,
			maxMargin:   5000,
			wantParts: []string{
				"125x",
				"5000",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatLeverageOptInPrompt(tc.maxLeverage, tc.maxMargin)
			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot: %s", part, got)
				}
			}
		})
	}
}
