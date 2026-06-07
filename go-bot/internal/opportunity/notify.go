// channel routing and notification formatting for opportunities.
// determines which platform to notify based on user's last active channel.
package opportunity

import (
	"fmt"
	"strings"

	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/pipeline"
	"github.com/trading-bot/go-bot/internal/user"
)

// determines which platform to notify the user on.
// rules:
//   - only telegram connected → telegram
//   - only discord connected → discord
//   - both connected → whichever they last interacted on
//   - neither connected → empty string (should not happen)
func ResolveChannel(u *user.User) string {
	hasTelegram := u.TelegramID != nil
	hasDiscord := u.DiscordID != nil

	if hasTelegram && !hasDiscord {
		return "telegram"
	}
	if hasDiscord && !hasTelegram {
		return "discord"
	}
	if hasTelegram && hasDiscord {
		if u.LastActiveChannel != nil {
			ch := strings.ToLower(*u.LastActiveChannel)
			if ch == "telegram" || ch == "discord" {
				return ch
			}
		}
		// default to telegram if no last active recorded
		return "telegram"
	}
	return ""
}

// builds a telegram message for an opportunity notification with action buttons.
// the callback data prefix is "opp:" followed by the opportunity id and action.
func FormatTelegramOpportunity(opp *Opportunity) string {
	var b strings.Builder

	result := opp.Result
	msg := pipeline.FormatTelegramMessage(result)
	b.WriteString("🎯 *Opportunity Detected*\n\n")
	b.WriteString(msg)
	b.WriteString("\n\n⏰ Limited-time opportunity")

	return b.String()
}

// returns the inline keyboard buttons for telegram.
// three buttons: approve, reject, modify.
func TelegramButtons(oppID string) [][]ButtonData {
	return [][]ButtonData{
		{
			{Text: "✅ Approve", Data: "opp_approve:" + oppID},
			{Text: "❌ Reject", Data: "opp_reject:" + oppID},
			{Text: "⚙️ Modify", Data: "opp_modify:" + oppID},
		},
	}
}

// returns the discord components for the opportunity notification.
func DiscordButtons(oppID string) []ButtonData {
	return []ButtonData{
		{Text: "✅ Approve", Data: "opp_approve:" + oppID, Style: ButtonStyleSuccess},
		{Text: "❌ Reject", Data: "opp_reject:" + oppID, Style: ButtonStyleDanger},
		{Text: "⚙️ Modify", Data: "opp_modify:" + oppID, Style: ButtonStyleSecondary},
	}
}

// returns telegram buttons for choosing a futures leverage trade.
func TelegramLeverageButtons(oppID string, action claude.Action) [][]ButtonData {
	buttons := leverageButtonsForAction(oppID, action, false)
	buttons = append(buttons, []ButtonData{
		{Text: "❌ Reject", Data: "opp_reject:" + oppID},
	})
	return buttons
}

// returns discord buttons for choosing a futures leverage trade.
func DiscordLeverageButtons(oppID string, action claude.Action) []ButtonData {
	rows := leverageButtonsForAction(oppID, action, true)
	var buttons []ButtonData
	for _, row := range rows {
		buttons = append(buttons, row...)
	}
	buttons = append(buttons,
		ButtonData{Text: "❌ Reject", Data: "opp_reject:" + oppID, Style: ButtonStyleSecondary},
	)
	return buttons
}

// generic button data for both platforms
type ButtonData struct {
	Text  string
	Data  string
	Style int
}

// button style constants matching discord's component styles
const (
	ButtonStylePrimary   = 1
	ButtonStyleSecondary = 2
	ButtonStyleSuccess   = 3
	ButtonStyleDanger    = 4
)

// returns the modify sub-flow buttons for adjusting trade parameters
func ModifyButtons(oppID string) [][]ButtonData {
	return [][]ButtonData{
		{
			{Text: "📏 Position Size", Data: "opp_mod_size:" + oppID},
			{Text: "🎯 Entry Price", Data: "opp_mod_entry:" + oppID},
		},
		{
			{Text: "🛑 Stop Loss", Data: "opp_mod_sl:" + oppID},
			{Text: "🎯 Take Profit", Data: "opp_mod_tp:" + oppID},
		},
		{
			{Text: "✅ Confirm Modifications", Data: "opp_mod_confirm:" + oppID},
			{Text: "↩️ Back", Data: "opp_mod_cancel:" + oppID},
		},
	}
}

// formats the expired message for a timed-out opportunity
func FormatExpiredMessage(opp *Opportunity) string {
	return fmt.Sprintf("⏰ *Opportunity Expired*\n\n%s %s opportunity for %s has expired.",
		statusEmoji(opp.Action), opp.Action, opp.Symbol)
}

// formats the approved confirmation message
func FormatApprovedMessage(opp *Opportunity) string {
	plan := opp.Result.Decision.Plan
	if opp.ModifiedPlan != nil {
		plan = *opp.ModifiedPlan
	}
	return fmt.Sprintf("✅ *Trade Approved*\n\n%s %s %s\nEntry: $%.2f | SL: $%.2f | TP: $%.2f\nPosition: $%.2f | R/R: 1:%.1f",
		statusEmoji(opp.Action), opp.Action, opp.Symbol,
		plan.Entry, plan.StopLoss, plan.TakeProfit, plan.PositionSize, plan.RiskReward)
}

// formats the rejected confirmation message
func FormatRejectedMessage(opp *Opportunity) string {
	return fmt.Sprintf("❌ *Trade Rejected*\n\n%s %s opportunity for %s was rejected.",
		statusEmoji(opp.Action), opp.Action, opp.Symbol)
}

// formats the modified confirmation message
func FormatModifiedMessage(opp *Opportunity) string {
	if opp.ModifiedPlan == nil {
		return FormatApprovedMessage(opp)
	}
	plan := opp.ModifiedPlan
	return fmt.Sprintf("⚙️ *Trade Modified & Approved*\n\n%s %s %s\nEntry: $%.2f | SL: $%.2f | TP: $%.2f\nPosition: $%.2f | R/R: 1:%.1f",
		statusEmoji(opp.Action), opp.Action, opp.Symbol,
		plan.Entry, plan.StopLoss, plan.TakeProfit, plan.PositionSize, plan.RiskReward)
}

// returns leverage option buttons for an opportunity.
// Kept for generic callers; scanner alerts use action-aware buttons.
func LeverageButtons(oppID string) [][]ButtonData {
	rows := leverageButtonsForAction(oppID, claude.ActionBuy, false)
	rows = append(rows, leverageButtonsForAction(oppID, claude.ActionSell, false)...)
	return rows
}

func leverageButtonsForAction(oppID string, action claude.Action, withStyle bool) [][]ButtonData {
	side := "long"
	label := "Long"
	style := ButtonStyleSuccess
	if action == claude.ActionSell {
		side = "short"
		label = "Short"
		style = ButtonStyleDanger
	}

	leverages := []int{2, 3, 5, 10}
	row := make([]ButtonData, 0, len(leverages))
	for _, lev := range leverages {
		button := ButtonData{
			Text: fmt.Sprintf("%dx %s", lev, label),
			Data: fmt.Sprintf("lev_%s_%d:%s", side, lev, oppID),
		}
		if withStyle {
			button.Style = style
		}
		row = append(row, button)
	}
	return [][]ButtonData{row}
}

// formats leverage selection confirmation
func FormatLeverageSelected(opp *Opportunity) string {
	if !opp.UseLeverage {
		return ""
	}
	return fmt.Sprintf("\n⚡ Leverage: %dx %s", opp.Leverage, opp.PositionSide)
}

func statusEmoji(action claude.Action) string {
	switch action {
	case claude.ActionBuy:
		return "🟢"
	case claude.ActionSell:
		return "🔴"
	default:
		return "⏸️"
	}
}
