// tests for leverage safety checker and related helpers.
package leverage

import (
	"errors"
	"strings"
	"testing"
)

func TestDefaultSafetyConfig(t *testing.T) {
	cfg := DefaultSafetyConfig()

	if cfg.HardMaxLeverage != 20 {
		t.Errorf("HardMaxLeverage = %d, want 20", cfg.HardMaxLeverage)
	}
	if cfg.UserMaxLeverage != 10 {
		t.Errorf("UserMaxLeverage = %d, want 10", cfg.UserMaxLeverage)
	}
	if cfg.MaxMarginPerTrade != 500 {
		t.Errorf("MaxMarginPerTrade = %f, want 500", cfg.MaxMarginPerTrade)
	}
	if cfg.MinLiquidationDistance != 10 {
		t.Errorf("MinLiquidationDistance = %f, want 10", cfg.MinLiquidationDistance)
	}
	if !cfg.RequireLeverageEnabled {
		t.Error("RequireLeverageEnabled should be true by default")
	}
}

func TestSafetyChecker_Check(t *testing.T) {
	tests := []struct {
		name           string
		config         SafetyConfig
		balance        FuturesBalanceProvider
		leveraged      LeverageStatusProvider
		userID         int
		symbol         string
		leverage       int
		margin         float64
		entryPrice     float64
		side           string
		wantPassed     bool
		wantBlocked    string
		wantCheckCount int
	}{
		{
			name: "all checks pass",
			config: SafetyConfig{
				HardMaxLeverage:        20,
				UserMaxLeverage:        10,
				MaxMarginPerTrade:      500,
				MinLiquidationDistance:  10,
				RequireLeverageEnabled: true,
			},
			balance:        &mockBalance{balance: 1000},
			leveraged:      &mockLeverageStatus{enabled: true},
			userID:         1,
			symbol:         "BTCUSDT",
			leverage:       5,
			margin:         200,
			entryPrice:     50000,
			side:           "LONG",
			wantPassed:     true,
			wantBlocked:    "",
			wantCheckCount: 6,
		},
		{
			name: "leverage not enabled blocks immediately",
			config: SafetyConfig{
				HardMaxLeverage:        20,
				UserMaxLeverage:        10,
				MaxMarginPerTrade:      500,
				MinLiquidationDistance:  10,
				RequireLeverageEnabled: true,
			},
			balance:        &mockBalance{balance: 1000},
			leveraged:      &mockLeverageStatus{enabled: false},
			userID:         1,
			symbol:         "BTCUSDT",
			leverage:       5,
			margin:         200,
			entryPrice:     50000,
			side:           "LONG",
			wantPassed:     false,
			wantBlocked:    "leverage trading not enabled",
			wantCheckCount: 1,
		},
		{
			name: "leverage exceeds hard cap",
			config: SafetyConfig{
				HardMaxLeverage:        20,
				UserMaxLeverage:        25,
				MaxMarginPerTrade:      500,
				MinLiquidationDistance:  5,
				RequireLeverageEnabled: true,
			},
			balance:        &mockBalance{balance: 1000},
			leveraged:      &mockLeverageStatus{enabled: true},
			userID:         1,
			symbol:         "BTCUSDT",
			leverage:       25,
			margin:         200,
			entryPrice:     50000,
			side:           "LONG",
			wantPassed:     false,
			wantBlocked:    "exceeds hard cap",
			wantCheckCount: 6,
		},
		{
			name: "leverage exceeds user cap but within hard cap",
			config: SafetyConfig{
				HardMaxLeverage:        20,
				UserMaxLeverage:        5,
				MaxMarginPerTrade:      500,
				MinLiquidationDistance:  5,
				RequireLeverageEnabled: true,
			},
			balance:        &mockBalance{balance: 1000},
			leveraged:      &mockLeverageStatus{enabled: true},
			userID:         1,
			symbol:         "ETHUSDT",
			leverage:       10,
			margin:         200,
			entryPrice:     3000,
			side:           "LONG",
			wantPassed:     false,
			wantBlocked:    "exceeds your configured max",
			wantCheckCount: 6,
		},
		{
			name: "margin exceeds limit",
			config: SafetyConfig{
				HardMaxLeverage:        20,
				UserMaxLeverage:        10,
				MaxMarginPerTrade:      100,
				MinLiquidationDistance:  5,
				RequireLeverageEnabled: true,
			},
			balance:        &mockBalance{balance: 1000},
			leveraged:      &mockLeverageStatus{enabled: true},
			userID:         1,
			symbol:         "BTCUSDT",
			leverage:       5,
			margin:         200,
			entryPrice:     50000,
			side:           "LONG",
			wantPassed:     false,
			wantBlocked:    "exceeds limit",
			wantCheckCount: 6,
		},
		{
			// 50x LONG: liqPrice = 50000*(1 - 1/50 + 0.004) = 49200
			// distance = (50000-49200)/50000*100 = 1.6% which is below 10%
			name: "liquidation distance too close",
			config: SafetyConfig{
				HardMaxLeverage:        125,
				UserMaxLeverage:        125,
				MaxMarginPerTrade:      500,
				MinLiquidationDistance:  10,
				RequireLeverageEnabled: true,
			},
			balance:        &mockBalance{balance: 1000},
			leveraged:      &mockLeverageStatus{enabled: true},
			userID:         1,
			symbol:         "BTCUSDT",
			leverage:       50,
			margin:         200,
			entryPrice:     50000,
			side:           "LONG",
			wantPassed:     false,
			wantBlocked:    "liq distance",
			wantCheckCount: 6,
		},
		{
			name: "insufficient balance",
			config: SafetyConfig{
				HardMaxLeverage:        20,
				UserMaxLeverage:        10,
				MaxMarginPerTrade:      500,
				MinLiquidationDistance:  10,
				RequireLeverageEnabled: true,
			},
			balance:        &mockBalance{balance: 50},
			leveraged:      &mockLeverageStatus{enabled: true},
			userID:         1,
			symbol:         "BTCUSDT",
			leverage:       5,
			margin:         200,
			entryPrice:     50000,
			side:           "LONG",
			wantPassed:     false,
			wantBlocked:    "insufficient balance",
			wantCheckCount: 6,
		},
		{
			name: "balance check error",
			config: SafetyConfig{
				HardMaxLeverage:        20,
				UserMaxLeverage:        10,
				MaxMarginPerTrade:      500,
				MinLiquidationDistance:  10,
				RequireLeverageEnabled: true,
			},
			balance:        &mockBalance{balance: 0, err: errors.New("connection timeout")},
			leveraged:      &mockLeverageStatus{enabled: true},
			userID:         1,
			symbol:         "BTCUSDT",
			leverage:       5,
			margin:         200,
			entryPrice:     50000,
			side:           "LONG",
			wantPassed:     false,
			wantBlocked:    "failed to check balance",
			wantCheckCount: 6,
		},
		{
			// nil balance provider skips check 6, so 5 checks total
			name: "nil balance provider skips balance check",
			config: SafetyConfig{
				HardMaxLeverage:        20,
				UserMaxLeverage:        10,
				MaxMarginPerTrade:      500,
				MinLiquidationDistance:  10,
				RequireLeverageEnabled: true,
			},
			balance:        nil,
			leveraged:      &mockLeverageStatus{enabled: true},
			userID:         1,
			symbol:         "BTCUSDT",
			leverage:       5,
			margin:         200,
			entryPrice:     50000,
			side:           "LONG",
			wantPassed:     true,
			wantBlocked:    "",
			wantCheckCount: 5,
		},
		{
			// nil leverage status provider skips check 1
			// with RequireLeverageEnabled=true but nil provider: 5 checks
			name: "nil leverage status provider skips leverage_enabled check",
			config: SafetyConfig{
				HardMaxLeverage:        20,
				UserMaxLeverage:        10,
				MaxMarginPerTrade:      500,
				MinLiquidationDistance:  10,
				RequireLeverageEnabled: true,
			},
			balance:        &mockBalance{balance: 1000},
			leveraged:      nil,
			userID:         1,
			symbol:         "BTCUSDT",
			leverage:       5,
			margin:         200,
			entryPrice:     50000,
			side:           "LONG",
			wantPassed:     true,
			wantBlocked:    "",
			wantCheckCount: 5,
		},
		{
			// RequireLeverageEnabled=false skips check 1 even with provider
			name: "require leverage enabled false skips that check",
			config: SafetyConfig{
				HardMaxLeverage:        20,
				UserMaxLeverage:        10,
				MaxMarginPerTrade:      500,
				MinLiquidationDistance:  10,
				RequireLeverageEnabled: false,
			},
			balance:        &mockBalance{balance: 1000},
			leveraged:      &mockLeverageStatus{enabled: false},
			userID:         1,
			symbol:         "BTCUSDT",
			leverage:       5,
			margin:         200,
			entryPrice:     50000,
			side:           "LONG",
			wantPassed:     true,
			wantBlocked:    "",
			wantCheckCount: 5,
		},
		{
			// both leverage_cap and user_leverage_cap fail; first failure is blocked
			name: "multiple checks fail reports first failure",
			config: SafetyConfig{
				HardMaxLeverage:        10,
				UserMaxLeverage:        5,
				MaxMarginPerTrade:      100,
				MinLiquidationDistance:  10,
				RequireLeverageEnabled: false,
			},
			balance:        &mockBalance{balance: 50},
			leveraged:      nil,
			userID:         1,
			symbol:         "BTCUSDT",
			leverage:       15,
			margin:         200,
			entryPrice:     50000,
			side:           "LONG",
			wantPassed:     false,
			wantBlocked:    "exceeds hard cap",
			wantCheckCount: 5,
		},
		{
			// short side 50x: liqPrice = 50000*(1+1/50-0.004) = 50800
			// distance = (50800-50000)/50000*100 = 1.6% below 10%
			name: "short side liquidation distance too close",
			config: SafetyConfig{
				HardMaxLeverage:        125,
				UserMaxLeverage:        125,
				MaxMarginPerTrade:      500,
				MinLiquidationDistance:  10,
				RequireLeverageEnabled: true,
			},
			balance:        &mockBalance{balance: 1000},
			leveraged:      &mockLeverageStatus{enabled: true},
			userID:         1,
			symbol:         "BTCUSDT",
			leverage:       50,
			margin:         200,
			entryPrice:     50000,
			side:           "SHORT",
			wantPassed:     false,
			wantBlocked:    "liq distance",
			wantCheckCount: 6,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checker := NewSafetyChecker(tc.config, tc.balance, tc.leveraged)
			result := checker.Check(tc.userID, tc.symbol, tc.leverage, tc.margin, tc.entryPrice, tc.side)

			if result.Passed != tc.wantPassed {
				t.Errorf("Passed = %v, want %v", result.Passed, tc.wantPassed)
			}
			if tc.wantBlocked != "" && !strings.Contains(result.Blocked, tc.wantBlocked) {
				t.Errorf("Blocked = %q, want substring %q", result.Blocked, tc.wantBlocked)
			}
			if tc.wantBlocked == "" && result.Blocked != "" {
				t.Errorf("Blocked = %q, want empty", result.Blocked)
			}
			if len(result.Checks) != tc.wantCheckCount {
				t.Errorf("check count = %d, want %d", len(result.Checks), tc.wantCheckCount)
				for i, c := range result.Checks {
					t.Logf("  check[%d]: name=%s passed=%v msg=%s", i, c.Name, c.Passed, c.Message)
				}
			}
		})
	}
}

func TestSafetyChecker_CheckNames(t *testing.T) {
	// verify all check names are present in order when all pass
	checker := NewSafetyChecker(SafetyConfig{
		HardMaxLeverage:        20,
		UserMaxLeverage:        10,
		MaxMarginPerTrade:      500,
		MinLiquidationDistance:  10,
		RequireLeverageEnabled: true,
	}, &mockBalance{balance: 1000}, &mockLeverageStatus{enabled: true})

	result := checker.Check(1, "BTCUSDT", 5, 200, 50000, "LONG")

	wantNames := []string{
		"leverage_enabled",
		"leverage_cap",
		"user_leverage_cap",
		"margin_limit",
		"liquidation_distance",
		"balance",
	}

	if len(result.Checks) != len(wantNames) {
		t.Fatalf("check count = %d, want %d", len(result.Checks), len(wantNames))
	}
	for i, name := range wantNames {
		if result.Checks[i].Name != name {
			t.Errorf("check[%d].Name = %q, want %q", i, result.Checks[i].Name, name)
		}
	}
}

func TestFormatSafetyFailure(t *testing.T) {
	tests := []struct {
		name       string
		result     SafetyResult
		wantParts  []string
	}{
		{
			name: "mixed pass and fail checks",
			result: SafetyResult{
				Passed: false,
				Checks: []CheckResult{
					{Name: "leverage_enabled", Passed: true, Message: "leverage trading enabled"},
					{Name: "leverage_cap", Passed: true, Message: "leverage 5x within hard cap 20x"},
					{Name: "margin_limit", Passed: false, Message: "margin $600.00 exceeds limit $500"},
				},
				Blocked: "margin $600.00 exceeds limit $500",
			},
			wantParts: []string{
				"Leverage Trade Blocked",
				"✅",
				"❌",
				"leverage_enabled",
				"leverage_cap",
				"margin_limit",
				"leverage trading enabled",
				"exceeds limit",
			},
		},
		{
			name: "all checks passed",
			result: SafetyResult{
				Passed: true,
				Checks: []CheckResult{
					{Name: "leverage_cap", Passed: true, Message: "ok"},
				},
			},
			wantParts: []string{
				"✅",
				"leverage_cap",
			},
		},
		{
			name: "single failed check",
			result: SafetyResult{
				Passed: false,
				Checks: []CheckResult{
					{Name: "balance", Passed: false, Message: "insufficient balance"},
				},
				Blocked: "insufficient balance",
			},
			wantParts: []string{
				"❌",
				"balance",
				"insufficient balance",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatSafetyFailure(tc.result)
			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("output missing %q\ngot: %s", part, got)
				}
			}
		})
	}
}
