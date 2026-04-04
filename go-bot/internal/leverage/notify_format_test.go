// tests for leverage notification format helpers.
package leverage

import (
	"strings"
	"testing"
)

func TestFormatLeverageUpdate(t *testing.T) {
	tests := []struct {
		name        string
		pos         *LeveragePosition
		distancePct float64
		wantParts   []string
	}{
		{
			name: "long position update in profit",
			pos: &LeveragePosition{
				Symbol:     "BTCUSDT",
				Side:       SideLong,
				Leverage:   10,
				EntryPrice: 50000,
				MarkPrice:  51000,
				Quantity:   0.1,
				Margin:     500,
			},
			distancePct: 15.5,
			wantParts:   []string{"📊", "BTCUSDT", "10x", "51000", "15.5%"},
		},
		{
			name: "short position update in loss",
			pos: &LeveragePosition{
				Symbol:     "ETHUSDT",
				Side:       SideShort,
				Leverage:   5,
				EntryPrice: 3000,
				MarkPrice:  3100,
				Quantity:   1.0,
				Margin:     600,
			},
			distancePct: 18.0,
			wantParts:   []string{"📊", "ETHUSDT", "5x", "3100", "18.0%"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatLeverageUpdate(tc.pos, tc.distancePct)
			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot: %s", part, got)
				}
			}
		})
	}
}

func TestFmtPrice(t *testing.T) {
	tests := []struct {
		name string
		val  float64
		want string
	}{
		{name: "large value 2 decimals", val: 50000.123456, want: "50000.12"},
		{name: "exactly 1000 uses 2 decimals", val: 1000.0, want: "1000.00"},
		{name: "mid value 4 decimals", val: 150.123456, want: "150.1235"},
		{name: "exactly 1 uses 4 decimals", val: 1.0, want: "1.0000"},
		{name: "small value 6 decimals", val: 0.123456, want: "0.123456"},
		{name: "very small value", val: 0.00001234, want: "0.000012"},
		{name: "value 999 uses 4 decimals", val: 999.1234, want: "999.1234"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fmtPrice(tc.val)
			if got != tc.want {
				t.Errorf("fmtPrice(%f) = %q, want %q", tc.val, got, tc.want)
			}
		})
	}
}

func TestSignStr(t *testing.T) {
	tests := []struct {
		name string
		val  float64
		want string
	}{
		{name: "positive value", val: 10.5, want: "+"},
		{name: "zero", val: 0, want: "+"},
		{name: "negative value", val: -5.3, want: "-"},
		{name: "very small negative", val: -0.0001, want: "-"},
		{name: "very small positive", val: 0.0001, want: "+"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := signStr(tc.val)
			if got != tc.want {
				t.Errorf("signStr(%f) = %q, want %q", tc.val, got, tc.want)
			}
		})
	}
}
