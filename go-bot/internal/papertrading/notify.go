// notification formatting for all paper trading lifecycle events.
// produces telegram messages, buttons, and daily summaries.
package papertrading

import (
	"fmt"
	"math"
	"strings"

	"github.com/trading-bot/go-bot/internal/claude"
)

// formats a "trade opened" notification
func FormatTradeExecuted(pos *Position) string {
	var b strings.Builder
	emoji := "📗"
	action := "Bought"
	if pos.Action == claude.ActionSell {
		emoji = "📕"
		action = "Sold"
	}

	b.WriteString(fmt.Sprintf("%s Paper Trade: %s %s %s @ $%s\n",
		emoji, action, formatQty(pos.Quantity), pos.Symbol, formatPrice(pos.EntryPrice)))
	b.WriteString(fmt.Sprintf("   SL: $%s | TP: $%s\n",
		formatPrice(pos.StopLoss), formatPrice(pos.TakeProfit)))
	b.WriteString(fmt.Sprintf("   Size: $%s", formatPrice(pos.PositionSize)))
	return b.String()
}

// formats a milestone breach notification
func FormatMilestone(pos *Position, milestone float64) string {
	emoji := "📈"
	if milestone < 0 {
		emoji = "📉"
	}
	pnl := pos.PnL()
	return fmt.Sprintf("%s Milestone: %s %+.1f%% | P&L: %s$%.2f",
		emoji, pos.Symbol, milestone, pnlSign(pnl), absVal(pnl))
}

// formats an ai adjustment suggestion
func FormatAISuggestion(suggestion string) string {
	return fmt.Sprintf("🤖 Claude suggests: \"%s\"", suggestion)
}

// formats a take profit hit notification
func FormatTPHit(pos *Position) string {
	pnl := pos.ClosedPnL()
	pct := pos.ClosedPnLPercent()
	return fmt.Sprintf("🎉 Take Profit Hit! Profit: +$%.2f (+%.2f%%)\n   %s closed @ $%s",
		pnl, pct, pos.Symbol, formatPrice(pos.ClosePrice))
}

// formats a stop loss hit notification
func FormatSLHit(pos *Position) string {
	pnl := pos.ClosedPnL()
	pct := pos.ClosedPnLPercent()
	return fmt.Sprintf("⚠️ Stop Loss Hit! Loss: -$%.2f (%.2f%%)\n   %s closed @ $%s",
		absVal(pnl), pct, pos.Symbol, formatPrice(pos.ClosePrice))
}

// formats a manual close notification
func FormatManualClose(pos *Position) string {
	pnl := pos.ClosedPnL()
	pct := pos.ClosedPnLPercent()
	sign := pnlSign(pnl)
	return fmt.Sprintf("👤 Position Closed: %s %s$%.2f (%s%.2f%%)\n   %s @ $%s",
		pos.Symbol, sign, absVal(pnl), sign, absVal(pct), pos.Symbol, formatPrice(pos.ClosePrice))
}

// formats a periodic price update
func FormatPeriodicUpdate(pos *Position) string {
	pnl := pos.PnL()
	pct := pos.PnLPercent()
	sign := pnlSign(pnl)
	return fmt.Sprintf("📊 Update: %s @ $%s | P&L: %s$%.2f (%s%.2f%%)",
		pos.Symbol, formatPrice(pos.CurrentPrice), sign, absVal(pnl), sign, absVal(pct))
}

// formats the daily summary for a user
func FormatDailySummary(s *DailySummary) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("📊 Daily Summary — %s\n\n", s.Date.Format("Jan 2, 2006")))

	if s.ClosedCount > 0 {
		b.WriteString(fmt.Sprintf("Closed: %d trades | %dW %dL\n", s.ClosedCount, s.Wins, s.Losses))
		b.WriteString(fmt.Sprintf("P&L: %s$%.2f\n", pnlSign(s.TotalPnL), absVal(s.TotalPnL)))

		if s.BestTrade != nil {
			b.WriteString(fmt.Sprintf("Best: %s +$%.2f\n", s.BestTrade.Symbol, s.BestTrade.ClosedPnL()))
		}
		if s.WorstTrade != nil {
			pnl := s.WorstTrade.ClosedPnL()
			b.WriteString(fmt.Sprintf("Worst: %s %s$%.2f\n", s.WorstTrade.Symbol, pnlSign(pnl), absVal(pnl)))
		}
	} else {
		b.WriteString("No closed trades today\n")
	}

	if s.OpenCount > 0 {
		b.WriteString(fmt.Sprintf("\nOpen: %d positions\n", s.OpenCount))
		for _, pos := range s.OpenPositions {
			pnl := pos.PnL()
			sign := pnlSign(pnl)
			b.WriteString(fmt.Sprintf("  %s: %s$%.2f (%s%.2f%%)\n",
				pos.Symbol, sign, absVal(pnl), sign, absVal(pos.PnLPercent())))
		}
	}

	return b.String()
}

// button data for trade adjustment interactions
type ButtonData struct {
	Text  string
	Data  string
	Style int
}

const (
	ButtonStylePrimary   = 1
	ButtonStyleSecondary = 2
	ButtonStyleSuccess   = 3
	ButtonStyleDanger    = 4
)

// returns buttons for responding to an ai adjustment suggestion
func AdjustButtons(posID string) []ButtonData {
	return []ButtonData{
		{Text: "Yes, adjust ✓", Data: fmt.Sprintf("pt_adjust_yes:%s", posID), Style: ButtonStyleSuccess},
		{Text: "No, keep original", Data: fmt.Sprintf("pt_adjust_no:%s", posID), Style: ButtonStyleSecondary},
	}
}

// returns a manual close button for a position
func CloseButton(posID string) ButtonData {
	return ButtonData{
		Text:  "Close Position",
		Data:  fmt.Sprintf("pt_close:%s", posID),
		Style: ButtonStyleDanger,
	}
}

// returns buttons for position management (adjust sl/tp, close)
func PositionButtons(posID string) []ButtonData {
	return []ButtonData{
		{Text: "🛑 Adjust SL", Data: fmt.Sprintf("pt_adj_sl:%s", posID), Style: ButtonStyleSecondary},
		{Text: "🎯 Adjust TP", Data: fmt.Sprintf("pt_adj_tp:%s", posID), Style: ButtonStyleSecondary},
		{Text: "❌ Close", Data: fmt.Sprintf("pt_close:%s", posID), Style: ButtonStyleDanger},
	}
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
