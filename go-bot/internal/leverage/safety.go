// pre-trade safety checks specific to leverage trading.
// composes the existing spot safety checks with additional leverage-specific validations.
package leverage

import (
	"context"
	"fmt"
)

// result of a single safety check
type CheckResult struct {
	Name    string
	Passed  bool
	Message string
}

// aggregate result of all leverage safety checks
type SafetyResult struct {
	Passed  bool
	Checks  []CheckResult
	Blocked string
}

// leverage safety configuration
type SafetyConfig struct {
	HardMaxLeverage        int     // absolute maximum leverage allowed
	UserMaxLeverage        int     // user's configured maximum
	MaxMarginPerTrade      float64 // max margin per position in usd
	MinLiquidationDistance  float64 // minimum liquidation distance % at entry (e.g. 10)
	RequireLeverageEnabled bool    // user must opt in to leverage
}

// provides balance information for futures account
type FuturesBalanceProvider interface {
	GetFuturesBalance(ctx context.Context, userID int, asset string) (float64, error)
}

// provides leverage opt-in status for a user
type LeverageStatusProvider interface {
	IsLeverageEnabled(userID int) bool
}

// validates leverage trades before execution
type SafetyChecker struct {
	config    SafetyConfig
	balance   FuturesBalanceProvider
	leveraged LeverageStatusProvider
}

// creates a new leverage safety checker
func NewSafetyChecker(config SafetyConfig, balance FuturesBalanceProvider, leveraged LeverageStatusProvider) *SafetyChecker {
	return &SafetyChecker{
		config:    config,
		balance:   balance,
		leveraged: leveraged,
	}
}

// returns a default safety config
func DefaultSafetyConfig() SafetyConfig {
	return SafetyConfig{
		HardMaxLeverage:        20,
		UserMaxLeverage:        10,
		MaxMarginPerTrade:      500,
		MinLiquidationDistance:  10,
		RequireLeverageEnabled: true,
	}
}

// runs all leverage-specific safety checks before allowing a trade
func (s *SafetyChecker) Check(userID int, symbol string, leverage int, margin float64, entryPrice float64, side string) SafetyResult {
	var checks []CheckResult

	// 1. leverage enabled check
	if s.config.RequireLeverageEnabled && s.leveraged != nil {
		enabled := s.leveraged.IsLeverageEnabled(userID)
		check := CheckResult{
			Name:    "leverage_enabled",
			Passed:  enabled,
			Message: "leverage trading enabled",
		}
		if !enabled {
			check.Message = "leverage trading not enabled. use /leverage enable"
		}
		checks = append(checks, check)
		if !enabled {
			return SafetyResult{Passed: false, Checks: checks, Blocked: check.Message}
		}
	}

	// 2. leverage within hard cap
	check := CheckResult{
		Name:    "leverage_cap",
		Passed:  leverage <= s.config.HardMaxLeverage,
		Message: fmt.Sprintf("leverage %dx within hard cap %dx", leverage, s.config.HardMaxLeverage),
	}
	if !check.Passed {
		check.Message = fmt.Sprintf("leverage %dx exceeds hard cap %dx", leverage, s.config.HardMaxLeverage)
	}
	checks = append(checks, check)

	// 3. leverage within user's configured max
	check = CheckResult{
		Name:    "user_leverage_cap",
		Passed:  leverage <= s.config.UserMaxLeverage,
		Message: fmt.Sprintf("leverage %dx within user max %dx", leverage, s.config.UserMaxLeverage),
	}
	if !check.Passed {
		check.Message = fmt.Sprintf("leverage %dx exceeds your configured max %dx", leverage, s.config.UserMaxLeverage)
	}
	checks = append(checks, check)

	// 4. margin within limits
	check = CheckResult{
		Name:    "margin_limit",
		Passed:  margin <= s.config.MaxMarginPerTrade,
		Message: fmt.Sprintf("margin $%.2f within limit $%.0f", margin, s.config.MaxMarginPerTrade),
	}
	if !check.Passed {
		check.Message = fmt.Sprintf("margin $%.2f exceeds limit $%.0f", margin, s.config.MaxMarginPerTrade)
	}
	checks = append(checks, check)

	// 5. minimum liquidation distance at entry
	if entryPrice > 0 && leverage > 0 {
		liqPrice := CalculateLiquidationPrice(entryPrice, leverage, side, 0.004)
		dist := DistanceToLiquidation(entryPrice, liqPrice, side)
		check = CheckResult{
			Name:    "liquidation_distance",
			Passed:  dist >= s.config.MinLiquidationDistance,
			Message: fmt.Sprintf("liq distance %.1f%% >= min %.1f%%", dist, s.config.MinLiquidationDistance),
		}
		if !check.Passed {
			check.Message = fmt.Sprintf("liq distance %.1f%% below min %.1f%%", dist, s.config.MinLiquidationDistance)
		}
		checks = append(checks, check)
	}

	// 6. futures balance check
	if s.balance != nil {
		balance, err := s.balance.GetFuturesBalance(context.Background(), userID, "USDT")
		if err != nil {
			checks = append(checks, CheckResult{
				Name:    "balance",
				Passed:  false,
				Message: fmt.Sprintf("failed to check balance: %v", err),
			})
		} else {
			check = CheckResult{
				Name:    "balance",
				Passed:  balance >= margin,
				Message: fmt.Sprintf("balance $%.2f sufficient for $%.2f margin", balance, margin),
			}
			if !check.Passed {
				check.Message = fmt.Sprintf("insufficient balance $%.2f for $%.2f margin", balance, margin)
			}
			checks = append(checks, check)
		}
	}

	// aggregate result
	result := SafetyResult{Passed: true, Checks: checks}
	for _, c := range checks {
		if !c.Passed {
			result.Passed = false
			result.Blocked = c.Message
			break
		}
	}
	return result
}

// formats a safety check failure message for notification
func FormatSafetyFailure(result SafetyResult) string {
	msg := "🛡️ Leverage Trade Blocked\n\n"
	for _, check := range result.Checks {
		icon := "✅"
		if !check.Passed {
			icon = "❌"
		}
		msg += fmt.Sprintf("  %s %s: %s\n", icon, check.Name, check.Message)
	}
	return msg
}
