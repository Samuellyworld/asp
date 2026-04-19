// trading command and component handlers for opportunity approval, position management, and scanning.
package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/trading-bot/go-bot/internal/leverage"
	"github.com/trading-bot/go-bot/internal/livetrading"
	"github.com/trading-bot/go-bot/internal/opportunity"
	"github.com/trading-bot/go-bot/internal/papertrading"
)

// optional trading dependencies injected after construction
type TradingDeps struct {
	OppManager    *opportunity.Manager
	PaperExecutor *papertrading.Executor
	PaperMonitor  *papertrading.Monitor
	LiveExecutor  *livetrading.Executor
	LiveMonitor   *livetrading.Monitor
	Emergency     *livetrading.EmergencyStop
	Confirm       *livetrading.ConfirmationManager
	SafetyConfig  livetrading.SafetyConfig

	// leverage trading
	LevPaperExecutor *leverage.PaperExecutor
	LevLiveExecutor  *leverage.LiveExecutor
	LevMonitor       *leverage.Monitor
}

// attaches trading dependencies to the handler
func (h *Handler) SetTradingDeps(deps *TradingDeps) {
	h.trading = deps
}

// additional slash commands for trading features
func TradingSlashCommands() []ApplicationCommand {
	return []ApplicationCommand{
		{Name: "positions", Description: "view open paper and live positions", Type: 1},
		{Name: "close", Description: "close a position by id", Type: 1, Options: []ApplicationCommandOptionDef{
			{Name: "position_id", Description: "the position id to close", Type: OptionString, Required: true},
		}},
		{Name: "live", Description: "enable live trading mode", Type: 1, Options: []ApplicationCommandOptionDef{
			{Name: "confirmation", Description: "type the confirmation phrase", Type: OptionString, Required: false},
		}},
		{Name: "emergency", Description: "close all live positions immediately", Type: 1},
		{Name: "scan", Description: "check scanner status", Type: 1},
		{Name: "leverage", Description: "manage leverage trading (enable/disable/status)", Type: 1, Options: []ApplicationCommandOptionDef{
			{Name: "action", Description: "enable, disable, or status", Type: OptionString, Required: true},
		}},
	}
}

// routes trading slash commands. returns true if handled.
func (h *Handler) handleTradingCommand(ctx context.Context, interaction *Interaction) bool {
	if interaction.Data == nil {
		return false
	}

	switch interaction.Data.Name {
	case "positions":
		h.handlePositions(ctx, interaction)
	case "close":
		h.handleClosePosition(ctx, interaction)
	case "live":
		h.handleLiveMode(ctx, interaction)
	case "emergency":
		h.handleEmergencyStop(ctx, interaction)
	case "scan":
		h.handleScan(interaction)
	case "leverage":
		h.handleLeverageCommand(ctx, interaction)
	default:
		return false
	}
	return true
}

// routes trading component interactions. returns true if handled.
func (h *Handler) handleTradingComponent(ctx context.Context, interaction *Interaction) bool {
	if interaction.Data == nil {
		return false
	}

	data := interaction.Data.CustomID
	switch {
	case strings.HasPrefix(data, "opp_approve:"):
		oppID := strings.TrimPrefix(data, "opp_approve:")
		h.componentOppApprove(ctx, interaction, oppID)
	case strings.HasPrefix(data, "opp_reject:"):
		oppID := strings.TrimPrefix(data, "opp_reject:")
		h.componentOppReject(ctx, interaction, oppID)
	case strings.HasPrefix(data, "opp_modify:"):
		oppID := strings.TrimPrefix(data, "opp_modify:")
		h.componentOppModify(interaction, oppID)
	case strings.HasPrefix(data, "opp_mod_confirm:"):
		oppID := strings.TrimPrefix(data, "opp_mod_confirm:")
		h.componentOppModConfirm(ctx, interaction, oppID)
	case strings.HasPrefix(data, "opp_mod_cancel:"):
		oppID := strings.TrimPrefix(data, "opp_mod_cancel:")
		h.componentOppModCancel(interaction, oppID)
	case strings.HasPrefix(data, "pt_close:"):
		posID := strings.TrimPrefix(data, "pt_close:")
		h.componentPaperClose(ctx, interaction, posID)
	case strings.HasPrefix(data, "pt_adj_sl:"), strings.HasPrefix(data, "pt_adj_tp:"):
		h.respondEphemeral(interaction, "use `/close <id>` to manage this position.")
	case strings.HasPrefix(data, "live_close:"):
		posID := strings.TrimPrefix(data, "live_close:")
		h.componentLiveClose(ctx, interaction, posID)
	case strings.HasPrefix(data, "lev_"):
		h.componentLeverageSelection(ctx, interaction, data)
	default:
		return false
	}
	return true
}

// shows all open positions
func (h *Handler) handlePositions(ctx context.Context, interaction *Interaction) {
	if h.trading == nil {
		h.respondEphemeral(interaction, "trading is not enabled.")
		return
	}

	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	var text strings.Builder
	hasPositions := false

	if h.trading.PaperExecutor != nil {
		positions := h.trading.PaperExecutor.OpenPositions(userID)
		if len(positions) > 0 {
			hasPositions = true
			text.WriteString("**Paper Positions**\n\n")
			for _, pos := range positions {
				pnl := pos.PnL()
				pct := pos.PnLPercent()
				sign := "+"
				if pnl < 0 {
					sign = "-"
				}
				text.WriteString(fmt.Sprintf("• %s: %s$%.2f (%s%.2f%%)\n",
					pos.Symbol, sign, abs(pnl), sign, abs(pct)))
			}
			text.WriteString("\n")
		}
	}

	if h.trading.LiveExecutor != nil {
		positions := h.trading.LiveExecutor.OpenPositions(userID)
		if len(positions) > 0 {
			hasPositions = true
			text.WriteString("**Live Positions**\n\n")
			for _, pos := range positions {
				text.WriteString(fmt.Sprintf("• %s | Entry: $%.2f | Size: $%.2f\n",
					pos.Symbol, pos.EntryPrice, pos.PositionSize))
				text.WriteString(fmt.Sprintf("  SL: $%.2f | TP: $%.2f\n",
					pos.StopLoss, pos.TakeProfit))
			}
		}
	}

	if !hasPositions {
		h.respondEphemeral(interaction, "no open positions.")
		return
	}

	h.respond(interaction, text.String(), nil, nil)
}

// closes a position by id
func (h *Handler) handleClosePosition(ctx context.Context, interaction *Interaction) {
	if h.trading == nil {
		h.respondEphemeral(interaction, "trading is not enabled.")
		return
	}

	posID := getOption(interaction, "position_id")
	if posID == "" {
		h.respondEphemeral(interaction, "position_id is required.")
		return
	}

	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	// try paper first
	if h.trading.PaperExecutor != nil {
		pos := h.trading.PaperExecutor.Get(posID)
		if pos != nil && pos.UserID == userID {
			closed, err := h.trading.PaperExecutor.Close(posID, "manual", pos.CurrentPrice)
			if err != nil {
				h.respondEphemeral(interaction, fmt.Sprintf("failed to close: %v", err))
				return
			}
			h.respond(interaction, papertrading.FormatManualClose(closed), nil, nil)
			return
		}
	}

	// try live
	if h.trading.LiveExecutor != nil {
		pos := h.trading.LiveExecutor.Get(posID)
		if pos != nil && pos.UserID == userID {
			closed, err := h.trading.LiveExecutor.Close(posID, "manual")
			if err != nil {
				h.respondEphemeral(interaction, fmt.Sprintf("failed to close: %v", err))
				return
			}
			h.respond(interaction, livetrading.FormatPositionClosed(closed), nil, nil)
			return
		}
	}

	h.respondEphemeral(interaction, "position not found.")
}

// enables live trading mode
func (h *Handler) handleLiveMode(ctx context.Context, interaction *Interaction) {
	if h.trading == nil || h.trading.Confirm == nil {
		h.respondEphemeral(interaction, "live trading is not available.")
		return
	}

	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	exchangeName, err := h.userSvc.GetPrimaryCredentialExchange(ctx, userID)
	if err == nil && exchangeName != "binance" {
		h.respondEphemeral(interaction, livetrading.FormatUnsupportedSpotExchange(exchangeName))
		return
	}

	input := getOption(interaction, "confirmation")
	if input == "" {
		if h.trading.Confirm.IsConfirmed(userID) {
			h.respondEphemeral(interaction, "live trading is already enabled.")
		} else {
			h.respondEphemeral(interaction, livetrading.FormatConfirmPrompt(h.trading.SafetyConfig))
		}
		return
	}

	if h.trading.Confirm.Confirm(userID, input) {
		h.respond(interaction, livetrading.FormatConfirmSuccess(h.trading.SafetyConfig), nil, nil)
	} else {
		h.respondEphemeral(interaction, fmt.Sprintf("incorrect phrase. type exactly: %s", h.trading.Confirm.Phrase()))
	}
}

// closes all live positions
func (h *Handler) handleEmergencyStop(ctx context.Context, interaction *Interaction) {
	if h.trading == nil || h.trading.Emergency == nil {
		h.respondEphemeral(interaction, "emergency stop is not available.")
		return
	}

	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	closed, errors := h.trading.Emergency.Execute(userID)
	h.respond(interaction, livetrading.FormatEmergencyStop(userID, closed, errors), nil, nil)
}

// shows scanner status
func (h *Handler) handleScan(interaction *Interaction) {
	h.respond(interaction, "scanner is running. opportunities will be sent when detected.", nil, nil)
}

// approves an opportunity and routes to executor
func (h *Handler) componentOppApprove(ctx context.Context, interaction *Interaction, oppID string) {
	if h.trading == nil || h.trading.OppManager == nil {
		h.respondEphemeral(interaction, "trading not available.")
		return
	}

	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	opp := h.trading.OppManager.GetForUser(oppID, userID)
	if opp == nil {
		h.respondEphemeral(interaction, "opportunity not found.")
		return
	}

	if !h.trading.OppManager.Approve(oppID, userID) {
		h.updateMessage(interaction, "opportunity already resolved.", nil, nil)
		return
	}

	// route to live or paper executor
	if h.trading.Confirm != nil && h.trading.Confirm.IsConfirmed(userID) && h.trading.LiveExecutor != nil {
		pos, err := h.trading.LiveExecutor.Execute(opp)
		if err != nil {
			h.updateMessage(interaction, opportunity.FormatApprovedMessage(opp), nil, nil)
			h.bot.SendMessage(interaction.ChannelID, fmt.Sprintf("live execution failed: %v", err))
			return
		}
		h.updateMessage(interaction, opportunity.FormatApprovedMessage(opp), nil, nil)
		h.bot.SendMessage(interaction.ChannelID, livetrading.FormatTradeExecuted(pos))
	} else if h.trading.PaperExecutor != nil {
		pos, err := h.trading.PaperExecutor.Execute(opp)
		if err != nil {
			h.updateMessage(interaction, opportunity.FormatApprovedMessage(opp), nil, nil)
			h.bot.SendMessage(interaction.ChannelID, fmt.Sprintf("paper execution failed: %v", err))
			return
		}
		h.updateMessage(interaction, opportunity.FormatApprovedMessage(opp), nil, nil)
		h.bot.SendMessage(interaction.ChannelID, papertrading.FormatTradeExecuted(pos))
	} else {
		h.updateMessage(interaction, opportunity.FormatApprovedMessage(opp), nil, nil)
	}
}

// rejects an opportunity
func (h *Handler) componentOppReject(ctx context.Context, interaction *Interaction, oppID string) {
	if h.trading == nil || h.trading.OppManager == nil {
		h.respondEphemeral(interaction, "trading not available.")
		return
	}

	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	opp := h.trading.OppManager.GetForUser(oppID, userID)
	if opp == nil {
		h.respondEphemeral(interaction, "opportunity not found.")
		return
	}

	if !h.trading.OppManager.Reject(oppID, userID) {
		h.updateMessage(interaction, "opportunity already resolved.", nil, nil)
		return
	}

	h.updateMessage(interaction, opportunity.FormatRejectedMessage(opp), nil, nil)
}

// shows modify buttons
func (h *Handler) componentOppModify(interaction *Interaction, oppID string) {
	if h.trading == nil || h.trading.OppManager == nil {
		h.respondEphemeral(interaction, "trading not available.")
		return
	}

	buttons := opportunity.ModifyButtons(oppID)
	components := toDiscordComponents(buttons)
	h.updateMessage(interaction, "modify trade parameters:", nil, components)
}

// confirms modifications
func (h *Handler) componentOppModConfirm(ctx context.Context, interaction *Interaction, oppID string) {
	if h.trading == nil || h.trading.OppManager == nil {
		h.respondEphemeral(interaction, "trading not available.")
		return
	}

	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	if !h.trading.OppManager.Approve(oppID, userID) {
		h.updateMessage(interaction, "opportunity already resolved.", nil, nil)
		return
	}

	opp := h.trading.OppManager.Get(oppID)
	h.updateMessage(interaction, opportunity.FormatModifiedMessage(opp), nil, nil)
}

// cancels modification flow
func (h *Handler) componentOppModCancel(interaction *Interaction, oppID string) {
	if h.trading == nil || h.trading.OppManager == nil {
		h.respondEphemeral(interaction, "trading not available.")
		return
	}

	opp := h.trading.OppManager.Get(oppID)
	if opp == nil {
		h.respondEphemeral(interaction, "opportunity not found.")
		return
	}

	buttons := opportunity.DiscordButtons(oppID)
	components := toDiscordButtonRow(buttons)
	h.updateMessage(interaction, opportunity.FormatTelegramOpportunity(opp), nil, components)
}

// closes a paper position
func (h *Handler) componentPaperClose(ctx context.Context, interaction *Interaction, posID string) {
	if h.trading == nil || h.trading.PaperExecutor == nil {
		h.respondEphemeral(interaction, "paper trading not available.")
		return
	}

	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	pos := h.trading.PaperExecutor.Get(posID)
	if pos == nil || pos.UserID != userID {
		h.respondEphemeral(interaction, "position not found.")
		return
	}

	closed, err := h.trading.PaperExecutor.Close(posID, "manual", pos.CurrentPrice)
	if err != nil {
		h.respondEphemeral(interaction, fmt.Sprintf("close failed: %v", err))
		return
	}

	h.respond(interaction, papertrading.FormatManualClose(closed), nil, nil)
}

// closes a live position
func (h *Handler) componentLiveClose(ctx context.Context, interaction *Interaction, posID string) {
	if h.trading == nil || h.trading.LiveExecutor == nil {
		h.respondEphemeral(interaction, "live trading not available.")
		return
	}

	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	pos := h.trading.LiveExecutor.Get(posID)
	if pos == nil || pos.UserID != userID {
		h.respondEphemeral(interaction, "position not found.")
		return
	}

	closed, err := h.trading.LiveExecutor.Close(posID, "manual")
	if err != nil {
		h.respondEphemeral(interaction, fmt.Sprintf("close failed: %v", err))
		return
	}

	h.respond(interaction, livetrading.FormatPositionClosed(closed), nil, nil)
}

// converts opportunity button rows to discord action row components
func toDiscordComponents(rows [][]opportunity.ButtonData) []Component {
	var actionRows []Component
	for _, row := range rows {
		var buttons []Component
		for _, btn := range row {
			style := btn.Style
			if style == 0 {
				style = 2 // secondary default
			}
			buttons = append(buttons, Component{
				Type:     ComponentButton,
				Style:    style,
				Label:    btn.Text,
				CustomID: btn.Data,
			})
		}
		actionRows = append(actionRows, Component{
			Type:       ComponentActionRow,
			Components: buttons,
		})
	}
	return actionRows
}

// converts a flat list of buttons to a single action row
func toDiscordButtonRow(buttons []opportunity.ButtonData) []Component {
	var comps []Component
	for _, btn := range buttons {
		style := btn.Style
		if style == 0 {
			style = 2
		}
		comps = append(comps, Component{
			Type:     ComponentButton,
			Style:    style,
			Label:    btn.Text,
			CustomID: btn.Data,
		})
	}
	return []Component{{Type: ComponentActionRow, Components: comps}}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
