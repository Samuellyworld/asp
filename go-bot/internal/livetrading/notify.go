// notification formatting for live trades.
// same lifecycle notifications as paper trading but with real order ids
// and exchange-specific details.
package livetrading

import (
	"fmt"
	"math"
	"strings"
)

const supportedLiveSpotExchange = "binance"

// formats a live trade execution notification with order ids
func FormatTradeExecuted(pos *LivePosition) string {
	var b strings.Builder

	emoji := "📗"
	action := "Bought"
	if pos.Side == "SELL" {
		emoji = "📕"
		action = "Sold"
	}

	b.WriteString(fmt.Sprintf("%s LIVE: %s %s %s @ $%s\n",
		emoji, action, formatQty(pos.Quantity), pos.Symbol, formatPrice(pos.EntryPrice)))
	b.WriteString(fmt.Sprintf("   Binance Order ID: %d\n", pos.MainOrderID))

	slStatus := "✓"
	if pos.SLOrderID == 0 {
		slStatus = "✗"
	}
	tpStatus := "✓"
	if pos.TPOrderID == 0 {
		tpStatus = "✗"
	}
	b.WriteString(fmt.Sprintf("   SL order placed %s | TP order placed %s\n", slStatus, tpStatus))

	if pos.StopLoss > 0 || pos.TakeProfit > 0 {
		b.WriteString(fmt.Sprintf("   SL: $%s | TP: $%s",
			formatPrice(pos.StopLoss), formatPrice(pos.TakeProfit)))
	}

	return b.String()
}

// formats a position closed notification with pnl
func FormatPositionClosed(pos *LivePosition) string {
	var b strings.Builder

	emoji := "🎉"
	if pos.PnL < 0 {
		emoji = "⚠️"
	}
	if pos.CloseReason == "emergency_stop" {
		emoji = "🚨"
	}

	reason := "Position Closed"
	switch pos.CloseReason {
	case "take_profit":
		reason = "Take Profit Hit"
		emoji = "🎉"
	case "stop_loss":
		reason = "Stop Loss Hit"
		emoji = "⚠️"
	case "emergency_stop":
		reason = "Emergency Stop"
	case "manual":
		reason = "Manually Closed"
		emoji = "👤"
	}

	pct := 0.0
	if pos.EntryPrice > 0 {
		if pos.Side == "BUY" {
			pct = ((pos.ClosePrice - pos.EntryPrice) / pos.EntryPrice) * 100
		} else {
			pct = ((pos.EntryPrice - pos.ClosePrice) / pos.EntryPrice) * 100
		}
	}

	sign := pnlSign(pos.PnL)
	b.WriteString(fmt.Sprintf("%s %s: %s %s$%.2f (%s%.2f%%)\n",
		emoji, reason, pos.Symbol, sign, absVal(pos.PnL), sign, absVal(pct)))
	b.WriteString(fmt.Sprintf("   Closed @ $%s | Entry @ $%s",
		formatPrice(pos.ClosePrice), formatPrice(pos.EntryPrice)))

	return b.String()
}

// formats the emergency stop result notification
func FormatEmergencyStop(userID int, closed []*LivePosition, errors []error) string {
	var b strings.Builder

	if len(closed) == 0 && len(errors) == 0 {
		return "🚨 No open positions to close."
	}

	b.WriteString("🚨 Emergency Stop Executed\n\n")

	totalPnL := 0.0
	for _, pos := range closed {
		sign := pnlSign(pos.PnL)
		b.WriteString(fmt.Sprintf("  ✓ %s: %s$%.2f @ $%s\n",
			pos.Symbol, sign, absVal(pos.PnL), formatPrice(pos.ClosePrice)))
		totalPnL += pos.PnL
	}

	if len(errors) > 0 {
		b.WriteString(fmt.Sprintf("\n  ✗ %d positions failed to close\n", len(errors)))
	}

	sign := pnlSign(totalPnL)
	b.WriteString(fmt.Sprintf("\nTotal P&L: %s$%.2f\n", sign, absVal(totalPnL)))
	b.WriteString("Trading disabled. Re-confirm to resume.")

	return b.String()
}

// formats the live mode confirmation prompt
func FormatConfirmPrompt(config SafetyConfig) string {
	var b strings.Builder
	b.WriteString("⚠️ Live Trading Mode\n\n")
	b.WriteString("You are about to enable REAL trading with REAL money.\n")
	b.WriteString("Please review the safety limits:\n\n")
	b.WriteString(fmt.Sprintf("  Max position: $%.0f\n", config.MaxPositionSize))
	b.WriteString(fmt.Sprintf("  Max open positions: %d\n", config.MaxOpenPositions))
	b.WriteString(fmt.Sprintf("  Daily loss limit: $%.0f\n", config.DailyLossLimit))
	b.WriteString(fmt.Sprintf("\nType '%s' to confirm:", DefaultConfirmPhrase))
	return b.String()
}

// formats the confirmation success message
func FormatConfirmSuccess(config SafetyConfig) string {
	return fmt.Sprintf("✅ Live mode ON | Max pos: $%.0f | Daily loss limit: $%.0f",
		config.MaxPositionSize, config.DailyLossLimit)
}

// formats the refusal shown when live spot trading is requested for an
// exchange the runtime does not route explicitly yet.
func FormatUnsupportedSpotExchange(exchangeName string) string {
	exchangeName = strings.TrimSpace(strings.ToLower(exchangeName))
	if exchangeName == "" {
		exchangeName = "this exchange"
	}
	return fmt.Sprintf(
		"real %s live spot trading is disabled right now. the live runtime still assumes %s routing and quantity sizing. use paper trading or connect %s spot credentials before enabling live mode.",
		exchangeName,
		supportedLiveSpotExchange,
		supportedLiveSpotExchange,
	)
}

// formats a safety check failure message
func FormatSafetyFailure(result SafetyResult) string {
	var b strings.Builder
	b.WriteString("🛡️ Trade Blocked — Safety Check Failed\n\n")
	for _, check := range result.Checks {
		icon := "✅"
		if !check.Passed {
			icon = "❌"
		}
		b.WriteString(fmt.Sprintf("  %s %s: %s\n", icon, check.Name, check.Message))
	}
	return b.String()
}

func formatPrice(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.2f", v)
	}
	if v >= 1 {
		return fmt.Sprintf("%.4f", v)
	}
	return fmt.Sprintf("%.6f", v)
}

func formatQty(v float64) string {
	if v >= 1 {
		return fmt.Sprintf("%.4f", v)
	}
	return fmt.Sprintf("%.5f", v)
}

func pnlSign(v float64) string {
	if v >= 0 {
		return "+"
	}
	return "-"
}

func absVal(v float64) float64 {
	return math.Abs(v)
}
