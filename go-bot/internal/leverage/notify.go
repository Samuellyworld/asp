// notification formatting for leverage trading lifecycle events.
// includes liquidation alerts, funding fees, and position updates.
package leverage

import (
	"fmt"
	"math"
	"strings"
)

// formats a leverage position opened notification
func FormatLeverageOpened(pos *LeveragePosition) string {
	var b strings.Builder

	emoji := "📗"
	action := "Long"
	if pos.Side == SideShort {
		emoji = "📕"
		action = "Short"
	}

	modeLabel := "LIVE"
	if pos.IsPaper {
		modeLabel = "PAPER"
	}

	b.WriteString(fmt.Sprintf("%s %s %s: %s %dx @ $%s\n",
		emoji, modeLabel, action, pos.Symbol, pos.Leverage, fmtPrice(pos.EntryPrice)))
	b.WriteString(fmt.Sprintf("   Margin: $%.2f | Notional: $%.2f\n",
		pos.Margin, pos.NotionalValue))

	if pos.StopLoss > 0 || pos.TakeProfit > 0 {
		b.WriteString(fmt.Sprintf("   SL: $%s | TP: $%s\n",
			fmtPrice(pos.StopLoss), fmtPrice(pos.TakeProfit)))
	}

	if pos.LiquidationPrice > 0 {
		b.WriteString(fmt.Sprintf("   Liq. price: $%s", fmtPrice(pos.LiquidationPrice)))
	}

	return b.String()
}

// formats a liquidation warning alert (5-10% from liquidation)
func FormatLiquidationWarning(pos *LeveragePosition, distancePct float64) string {
	return fmt.Sprintf("⚠️ Liquidation Warning: %s %dx\n"+
		"   Mark: $%s | Liq: $%s | Distance: %.2f%%\n"+
		"   Consider reducing position or adding margin.",
		pos.Symbol, pos.Leverage,
		fmtPrice(pos.MarkPrice), fmtPrice(pos.LiquidationPrice), distancePct)
}

// formats a liquidation critical alert (2-5% from liquidation)
func FormatLiquidationCritical(pos *LeveragePosition, distancePct float64) string {
	return fmt.Sprintf("🚨 CRITICAL: %s %dx near liquidation!\n"+
		"   Mark: $%s | Liq: $%s | Distance: %.2f%%\n"+
		"   CLOSE POSITION IMMEDIATELY or risk total loss.",
		pos.Symbol, pos.Leverage,
		fmtPrice(pos.MarkPrice), fmtPrice(pos.LiquidationPrice), distancePct)
}

// formats a liquidation auto-close notification (< 2% from liquidation)
func FormatLiquidationAutoClose(pos *LeveragePosition, distancePct float64) string {
	return fmt.Sprintf("🔴 AUTO-CLOSED: %s %dx (%.2f%% from liquidation)\n"+
		"   Closed @ $%s to prevent liquidation.\n"+
		"   P&L: %s$%.2f (%.2f%% ROI)",
		pos.Symbol, pos.Leverage, distancePct,
		fmtPrice(pos.ClosePrice),
		signStr(pos.PnL), math.Abs(pos.PnL), pos.ROI())
}

// formats a funding fee notification
func FormatFundingFee(pos *LeveragePosition, rate, amount float64) string {
	emoji := "💰"
	if amount < 0 {
		emoji = "💸"
	}
	return fmt.Sprintf("%s Funding Fee: %s %dx\n"+
		"   Rate: %.4f%% | Amount: %s$%.4f\n"+
		"   Cumulative: %s$%.4f",
		emoji, pos.Symbol, pos.Leverage,
		rate*100, signStr(amount), math.Abs(amount),
		signStr(pos.FundingPaid), math.Abs(pos.FundingPaid))
}

// formats a leverage position closed notification
func FormatLeverageClosed(pos *LeveragePosition) string {
	var b strings.Builder

	emoji := "🎉"
	if pos.PnL < 0 {
		emoji = "⚠️"
	}
	if pos.CloseReason == "auto_close" {
		emoji = "🔴"
	}

	reason := "Position Closed"
	switch pos.CloseReason {
	case "take_profit":
		reason = "Take Profit Hit"
		emoji = "🎉"
	case "stop_loss":
		reason = "Stop Loss Hit"
		emoji = "⚠️"
	case "auto_close":
		reason = "Auto-Closed (Near Liquidation)"
	case "emergency_stop":
		reason = "Emergency Stop"
		emoji = "🚨"
	case "manual":
		reason = "Manually Closed"
		emoji = "👤"
	}

	modeLabel := "LIVE"
	if pos.IsPaper {
		modeLabel = "PAPER"
	}

	b.WriteString(fmt.Sprintf("%s %s %s: %s %dx\n", emoji, modeLabel, reason, pos.Symbol, pos.Leverage))
	b.WriteString(fmt.Sprintf("   P&L: %s$%.2f | ROI: %s%.2f%%\n",
		signStr(pos.PnL), math.Abs(pos.PnL), signStr(pos.PnL), math.Abs(pos.ROI())))
	b.WriteString(fmt.Sprintf("   Closed @ $%s | Entry @ $%s",
		fmtPrice(pos.ClosePrice), fmtPrice(pos.EntryPrice)))

	if pos.FundingPaid != 0 {
		b.WriteString(fmt.Sprintf("\n   Funding fees: %s$%.4f", signStr(pos.FundingPaid), math.Abs(pos.FundingPaid)))
	}

	return b.String()
}

// formats the leverage opt-in prompt message
func FormatLeverageOptInPrompt(maxLeverage int, maxMargin float64) string {
	var b strings.Builder
	b.WriteString("⚠️ Leverage Trading Mode\n\n")
	b.WriteString("Leverage amplifies both gains AND losses.\n")
	b.WriteString("You can lose MORE than your margin.\n\n")
	b.WriteString("Safety limits:\n")
	b.WriteString(fmt.Sprintf("  Max leverage: %dx\n", maxLeverage))
	b.WriteString(fmt.Sprintf("  Max margin per trade: $%.0f\n", maxMargin))
	b.WriteString("  Auto-close at 2%% from liquidation\n\n")
	b.WriteString("Type 'I UNDERSTAND LEVERAGE RISKS' to enable.")
	return b.String()
}

// formats a periodic leverage update
func FormatLeverageUpdate(pos *LeveragePosition, distancePct float64) string {
	return fmt.Sprintf("📊 %s %dx | Mark: $%s | P&L: %s$%.2f (%.2f%% ROI) | Liq: %.1f%% away",
		pos.Symbol, pos.Leverage,
		fmtPrice(pos.MarkPrice),
		signStr(pos.UnrealizedPnL()), math.Abs(pos.UnrealizedPnL()),
		math.Abs(pos.ROI()),
		distancePct)
}

func fmtPrice(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.2f", v)
	}
	if v >= 1 {
		return fmt.Sprintf("%.4f", v)
	}
	return fmt.Sprintf("%.6f", v)
}

func signStr(v float64) string {
	if v >= 0 {
		return "+"
	}
	return "-"
}
