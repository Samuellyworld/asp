package leverage

import (
	"testing"
)

func TestCalculatePositionSizes(t *testing.T) {
	recs, err := CalculatePositionSizes(10000, 0.02, 0.03, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}

	// verify risk amount is 2% of 10000 = 200
	if recs[0].RiskAmount != 200 {
		t.Fatalf("expected risk 200, got %f", recs[0].RiskAmount)
	}

	// 1x: margin = 200 / (1 * 0.03) = 6666.67
	if recs[0].Leverage != 1 {
		t.Fatal("first should be 1x")
	}
	if recs[0].Margin != 6666.67 {
		t.Fatalf("1x margin should be ~6666.67, got %f", recs[0].Margin)
	}

	// 10x: margin = 200 / (10 * 0.03) = 666.67
	var found bool
	for _, r := range recs {
		if r.Leverage == 10 {
			found = true
			if r.Margin != 666.67 {
				t.Fatalf("10x margin should be ~666.67, got %f", r.Margin)
			}
			if r.PositionSize != 6666.67 {
				t.Fatalf("10x pos should be ~6666.67, got %f", r.PositionSize)
			}
		}
	}
	if !found {
		t.Fatal("expected 10x in results")
	}

	// should not exceed max leverage
	for _, r := range recs {
		if r.Leverage > 20 {
			t.Fatalf("leverage %d exceeds max 20", r.Leverage)
		}
	}
}

func TestCalculatePositionSizesInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		balance float64
		risk    float64
		sl      float64
		maxLev  int
	}{
		{"zero balance", 0, 0.02, 0.03, 20},
		{"negative risk", 10000, -0.02, 0.03, 20},
		{"risk > 1", 10000, 1.5, 0.03, 20},
		{"zero sl", 10000, 0.02, 0, 20},
		{"zero max leverage", 10000, 0.02, 0.03, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CalculatePositionSizes(tt.balance, tt.risk, tt.sl, tt.maxLev)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestFormatSizeRecommendations(t *testing.T) {
	recs, _ := CalculatePositionSizes(10000, 0.02, 0.03, 10)
	result := FormatSizeRecommendations(recs, "BTCUSDT", 67000)
	if result == "" {
		t.Fatal("expected non-empty format")
	}
}

func TestMarginCappedAtBalance(t *testing.T) {
	// Very small SL + low leverage = margin would exceed balance
	recs, err := CalculatePositionSizes(100, 0.5, 0.001, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range recs {
		if r.Margin > 100 {
			t.Fatalf("margin %f exceeds balance 100", r.Margin)
		}
	}
}
