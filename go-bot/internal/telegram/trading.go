// trading command and callback handlers for opportunity approval, position management, and scanning.
package telegram

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/trading-bot/go-bot/internal/dca"
	"github.com/trading-bot/go-bot/internal/leverage"
	"github.com/trading-bot/go-bot/internal/livetrading"
	"github.com/trading-bot/go-bot/internal/opportunity"
	"github.com/trading-bot/go-bot/internal/papertrading"
)

// optional trading dependencies injected after construction.
// nil fields mean the feature is not available.
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

	// DCA execution
	DCAExecutor *dca.Executor
}

// attaches trading dependencies to the handler
func (h *Handler) SetTradingDeps(deps *TradingDeps) {
	h.trading = deps
}

// routes trading-related commands. returns true if handled.
func (h *Handler) handleTradingCommand(ctx context.Context, command, args string, telegramID int64, chatID int64) bool {
	switch command {
	case "positions", "pos":
		h.handlePositions(ctx, telegramID, chatID)
	case "close":
		h.handleClosePosition(ctx, args, telegramID, chatID)
	case "live":
		h.handleLiveMode(ctx, args, telegramID, chatID)
	case "emergency":
		h.handleEmergencyStop(ctx, telegramID, chatID)
	case "scan":
		h.handleScan(chatID)
	case "leverage":
		return h.handleLeverageCommand(ctx, args, telegramID, chatID)
	case "dca":
		h.handleDCA(ctx, args, telegramID, chatID)
	default:
		return false
	}
	return true
}

// routes trading-related callbacks. returns true if handled.
func (h *Handler) handleTradingCallback(ctx context.Context, cb *CallbackQuery) bool {
	data := cb.Data
	chatID := cb.Message.Chat.ID
	messageID := cb.Message.MessageID
	telegramID := cb.From.ID

	switch {
	// opportunity actions
	case strings.HasPrefix(data, "opp_approve:"):
		oppID := strings.TrimPrefix(data, "opp_approve:")
		h.callbackOppApprove(ctx, cb.ID, telegramID, chatID, messageID, oppID)

	case strings.HasPrefix(data, "opp_reject:"):
		oppID := strings.TrimPrefix(data, "opp_reject:")
		h.callbackOppReject(ctx, cb.ID, telegramID, chatID, messageID, oppID)

	case strings.HasPrefix(data, "opp_modify:"):
		oppID := strings.TrimPrefix(data, "opp_modify:")
		h.callbackOppModify(cb.ID, chatID, messageID, oppID)

	case strings.HasPrefix(data, "opp_mod_confirm:"):
		oppID := strings.TrimPrefix(data, "opp_mod_confirm:")
		h.callbackOppModConfirm(ctx, cb.ID, telegramID, chatID, messageID, oppID)

	case strings.HasPrefix(data, "opp_mod_cancel:"):
		oppID := strings.TrimPrefix(data, "opp_mod_cancel:")
		h.callbackOppModCancel(cb.ID, chatID, messageID, oppID)

	// paper trading position actions
	case strings.HasPrefix(data, "pt_close:"):
		posID := strings.TrimPrefix(data, "pt_close:")
		h.callbackPaperClose(ctx, cb.ID, telegramID, chatID, posID)

	case strings.HasPrefix(data, "pt_adj_sl:"):
		posID := strings.TrimPrefix(data, "pt_adj_sl:")
		h.answerCallback(cb.ID, fmt.Sprintf("reply with: /set sl %s <price>", posID))

	case strings.HasPrefix(data, "pt_adj_tp:"):
		posID := strings.TrimPrefix(data, "pt_adj_tp:")
		h.answerCallback(cb.ID, fmt.Sprintf("reply with: /set tp %s <price>", posID))

	// live trading position actions
	case strings.HasPrefix(data, "live_close:"):
		posID := strings.TrimPrefix(data, "live_close:")
		h.callbackLiveClose(ctx, cb.ID, telegramID, chatID, posID)

	// handle leverage selection: lev_long_3:oppID, lev_short_5:oppID, etc.
	case strings.HasPrefix(data, "lev_"):
		return h.handleLeverageSelection(ctx, cb, data)

	default:
		return false
	}
	return true
}

// --- command handlers ---

// shows all open positions (paper + live)
func (h *Handler) handlePositions(ctx context.Context, telegramID int64, chatID int64) {
	if h.trading == nil {
		h.send(chatID, "trading is not enabled.")
		return
	}

	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.send(chatID, "account error.")
		return
	}
	userID := result.User.ID

	var text strings.Builder
	hasPositions := false

	// paper positions
	if h.trading.PaperExecutor != nil {
		positions := h.trading.PaperExecutor.OpenPositions(userID)
		if len(positions) > 0 {
			hasPositions = true
			text.WriteString("📝 *Paper Positions*\n\n")
			for _, pos := range positions {
				pnl := pos.PnL()
				pct := pos.PnLPercent()
				sign := "+"
				if pnl < 0 {
					sign = "-"
				}
				text.WriteString(fmt.Sprintf("• %s: %s$%.2f (%s%.2f%%)\n",
					pos.Symbol, sign, math.Abs(pnl), sign, math.Abs(pct)))
			}
			text.WriteString("\n")
		}
	}

	// live positions
	if h.trading.LiveExecutor != nil {
		positions := h.trading.LiveExecutor.OpenPositions(userID)
		if len(positions) > 0 {
			hasPositions = true
			text.WriteString("🔴 *Live Positions*\n\n")
			for _, pos := range positions {
				text.WriteString(fmt.Sprintf("• %s | Entry: $%.2f | Size: $%.2f\n",
					pos.Symbol, pos.EntryPrice, pos.PositionSize))
				text.WriteString(fmt.Sprintf("  SL: $%.2f | TP: $%.2f\n",
					pos.StopLoss, pos.TakeProfit))
			}
		}
	}

	if !hasPositions {
		h.send(chatID, "no open positions.")
		return
	}

	h.send(chatID, text.String())
}

// closes a position by id. usage: /close <posID>
func (h *Handler) handleClosePosition(ctx context.Context, args string, telegramID int64, chatID int64) {
	if h.trading == nil {
		h.send(chatID, "trading is not enabled.")
		return
	}

	posID := strings.TrimSpace(args)
	if posID == "" {
		h.send(chatID, "usage: /close <position_id>")
		return
	}

	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.send(chatID, "account error.")
		return
	}
	userID := result.User.ID

	// try paper first
	if h.trading.PaperExecutor != nil {
		pos := h.trading.PaperExecutor.Get(posID)
		if pos != nil && pos.UserID == userID {
			price := pos.CurrentPrice
			closed, err := h.trading.PaperExecutor.Close(posID, "manual", price)
			if err != nil {
				h.send(chatID, fmt.Sprintf("failed to close: %v", err))
				return
			}
			h.send(chatID, papertrading.FormatManualClose(closed))
			return
		}
	}

	// try live
	if h.trading.LiveExecutor != nil {
		pos := h.trading.LiveExecutor.Get(posID)
		if pos != nil && pos.UserID == userID {
			closed, err := h.trading.LiveExecutor.Close(posID, "manual")
			if err != nil {
				h.send(chatID, fmt.Sprintf("failed to close: %v", err))
				return
			}
			h.send(chatID, livetrading.FormatPositionClosed(closed))
			return
		}
	}

	h.send(chatID, "position not found.")
}

// enables live trading mode or shows confirmation prompt
func (h *Handler) handleLiveMode(ctx context.Context, args string, telegramID int64, chatID int64) {
	if h.trading == nil || h.trading.Confirm == nil {
		h.send(chatID, "live trading is not available.")
		return
	}

	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.send(chatID, "account error.")
		return
	}
	userID := result.User.ID

	exchangeName, err := h.userSvc.GetPrimaryCredentialExchange(ctx, userID)
	if err == nil && exchangeName != "binance" {
		h.send(chatID, livetrading.FormatUnsupportedSpotExchange(exchangeName))
		return
	}

	input := strings.TrimSpace(args)
	if input == "" {
		// show prompt or status
		if h.trading.Confirm.IsConfirmed(userID) {
			h.send(chatID, "✅ live trading is already enabled.")
		} else {
			h.send(chatID, livetrading.FormatConfirmPrompt(h.trading.SafetyConfig))
		}
		return
	}

	// try to confirm
	if h.trading.Confirm.Confirm(userID, input) {
		h.send(chatID, livetrading.FormatConfirmSuccess(h.trading.SafetyConfig))
	} else {
		h.send(chatID, fmt.Sprintf("incorrect phrase. type exactly: %s", h.trading.Confirm.Phrase()))
	}
}

// closes all live positions for the user
func (h *Handler) handleEmergencyStop(ctx context.Context, telegramID int64, chatID int64) {
	if h.trading == nil || h.trading.Emergency == nil {
		h.send(chatID, "emergency stop is not available.")
		return
	}

	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.send(chatID, "account error.")
		return
	}
	userID := result.User.ID

	closed, errors := h.trading.Emergency.Execute(userID)
	h.send(chatID, livetrading.FormatEmergencyStop(userID, closed, errors))
}

// shows scanner status
func (h *Handler) handleScan(chatID int64) {
	h.send(chatID, "🔍 scanner is running. opportunities will be sent when detected.")
}

// --- callback handlers ---

// approves an opportunity and routes to paper or live executor
func (h *Handler) callbackOppApprove(ctx context.Context, queryID string, telegramID int64, chatID int64, messageID int, oppID string) {
	if h.trading == nil || h.trading.OppManager == nil {
		h.answerCallback(queryID, "trading not available")
		return
	}

	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.answerCallback(queryID, "account error")
		return
	}
	userID := result.User.ID

	opp := h.trading.OppManager.GetForUser(oppID, userID)
	if opp == nil {
		h.answerCallback(queryID, "opportunity not found")
		return
	}

	if !h.trading.OppManager.Approve(oppID, userID) {
		h.answerCallback(queryID, "opportunity already resolved")
		return
	}

	// update the message to show approved status
	h.editMessage(chatID, messageID, opportunity.FormatApprovedMessage(opp), nil)

	// route to appropriate executor
	if h.trading.Confirm != nil && h.trading.Confirm.IsConfirmed(userID) && h.trading.LiveExecutor != nil {
		pos, err := h.trading.LiveExecutor.Execute(opp)
		if err != nil {
			h.answerCallback(queryID, "approved")
			h.send(chatID, fmt.Sprintf("❌ live execution failed: %v", err))
			return
		}
		h.answerCallback(queryID, "trade executed!")
		h.send(chatID, livetrading.FormatTradeExecuted(pos))
	} else if h.trading.PaperExecutor != nil {
		pos, err := h.trading.PaperExecutor.Execute(opp)
		if err != nil {
			h.answerCallback(queryID, "approved")
			h.send(chatID, fmt.Sprintf("❌ paper execution failed: %v", err))
			return
		}
		h.answerCallback(queryID, "paper trade opened!")
		h.send(chatID, papertrading.FormatTradeExecuted(pos))
	} else {
		h.answerCallback(queryID, "approved (no executor available)")
	}
}

// rejects an opportunity
func (h *Handler) callbackOppReject(ctx context.Context, queryID string, telegramID int64, chatID int64, messageID int, oppID string) {
	if h.trading == nil || h.trading.OppManager == nil {
		h.answerCallback(queryID, "trading not available")
		return
	}

	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.answerCallback(queryID, "account error")
		return
	}

	opp := h.trading.OppManager.GetForUser(oppID, result.User.ID)
	if opp == nil {
		h.answerCallback(queryID, "opportunity not found")
		return
	}

	if !h.trading.OppManager.Reject(oppID, result.User.ID) {
		h.answerCallback(queryID, "opportunity already resolved")
		return
	}

	h.answerCallback(queryID, "rejected")
	h.editMessage(chatID, messageID, opportunity.FormatRejectedMessage(opp), nil)
}

// shows the modify sub-flow buttons
func (h *Handler) callbackOppModify(queryID string, chatID int64, messageID int, oppID string) {
	if h.trading == nil || h.trading.OppManager == nil {
		h.answerCallback(queryID, "trading not available")
		return
	}

	buttons := opportunity.ModifyButtons(oppID)
	keyboard := toTelegramKeyboard(buttons)

	h.answerCallback(queryID, "choose parameter to modify")
	h.editMessage(chatID, messageID, "⚙️ modify trade parameters:", keyboard)
}

// confirms modifications and approves the opportunity
func (h *Handler) callbackOppModConfirm(ctx context.Context, queryID string, telegramID int64, chatID int64, messageID int, oppID string) {
	if h.trading == nil || h.trading.OppManager == nil {
		h.answerCallback(queryID, "trading not available")
		return
	}

	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.answerCallback(queryID, "account error")
		return
	}

	// approve the modified opportunity
	if !h.trading.OppManager.Approve(oppID, result.User.ID) {
		h.answerCallback(queryID, "opportunity already resolved")
		return
	}

	opp := h.trading.OppManager.Get(oppID)
	h.answerCallback(queryID, "modifications confirmed")
	h.editMessage(chatID, messageID, opportunity.FormatModifiedMessage(opp), nil)
}

// cancels modification and restores original buttons
func (h *Handler) callbackOppModCancel(queryID string, chatID int64, messageID int, oppID string) {
	if h.trading == nil || h.trading.OppManager == nil {
		h.answerCallback(queryID, "trading not available")
		return
	}

	opp := h.trading.OppManager.Get(oppID)
	if opp == nil {
		h.answerCallback(queryID, "opportunity not found")
		return
	}

	buttons := opportunity.TelegramButtons(oppID)
	keyboard := toTelegramKeyboard(buttons)
	msg := opportunity.FormatTelegramOpportunity(opp)

	h.answerCallback(queryID, "cancelled modifications")
	h.editMessage(chatID, messageID, msg, keyboard)
}

// closes a paper trading position from an inline button
func (h *Handler) callbackPaperClose(ctx context.Context, queryID string, telegramID int64, chatID int64, posID string) {
	if h.trading == nil || h.trading.PaperExecutor == nil {
		h.answerCallback(queryID, "paper trading not available")
		return
	}

	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.answerCallback(queryID, "account error")
		return
	}

	pos := h.trading.PaperExecutor.Get(posID)
	if pos == nil || pos.UserID != result.User.ID {
		h.answerCallback(queryID, "position not found")
		return
	}

	closed, err := h.trading.PaperExecutor.Close(posID, "manual", pos.CurrentPrice)
	if err != nil {
		h.answerCallback(queryID, fmt.Sprintf("close failed: %v", err))
		return
	}

	h.answerCallback(queryID, "position closed")
	h.send(chatID, papertrading.FormatManualClose(closed))
}

// closes a live trading position from an inline button
func (h *Handler) callbackLiveClose(ctx context.Context, queryID string, telegramID int64, chatID int64, posID string) {
	if h.trading == nil || h.trading.LiveExecutor == nil {
		h.answerCallback(queryID, "live trading not available")
		return
	}

	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.answerCallback(queryID, "account error")
		return
	}

	pos := h.trading.LiveExecutor.Get(posID)
	if pos == nil || pos.UserID != result.User.ID {
		h.answerCallback(queryID, "position not found")
		return
	}

	closed, err := h.trading.LiveExecutor.Close(posID, "manual")
	if err != nil {
		h.answerCallback(queryID, fmt.Sprintf("close failed: %v", err))
		return
	}

	h.answerCallback(queryID, "position closed")
	h.send(chatID, livetrading.FormatPositionClosed(closed))
}

// converts opportunity ButtonData rows to telegram inline keyboard
func toTelegramKeyboard(rows [][]opportunity.ButtonData) *InlineKeyboardMarkup {
	var keyboard [][]InlineKeyboardButton
	for _, row := range rows {
		var buttons []InlineKeyboardButton
		for _, btn := range row {
			buttons = append(buttons, InlineKeyboardButton{
				Text:         btn.Text,
				CallbackData: btn.Data,
			})
		}
		keyboard = append(keyboard, buttons)
	}
	return &InlineKeyboardMarkup{InlineKeyboard: keyboard}
}

// handleDCA handles /dca commands: status, cancel <id>
func (h *Handler) handleDCA(ctx context.Context, args string, telegramID int64, chatID int64) {
	if h.trading == nil || h.trading.DCAExecutor == nil {
		h.send(chatID, "DCA trading is not available.")
		return
	}

	parts := strings.Fields(args)

	subCmd := ""
	if len(parts) > 0 {
		subCmd = strings.ToLower(parts[0])
	}

	switch subCmd {
	case "cancel":
		if len(parts) < 2 {
			h.send(chatID, "usage: /dca cancel <plan_id>")
			return
		}
		planID := parts[1]
		if h.trading.DCAExecutor.CancelPlan(planID) {
			h.send(chatID, fmt.Sprintf("✅ DCA plan %s cancelled", planID))
		} else {
			h.send(chatID, fmt.Sprintf("❌ DCA plan %s not found or already completed", planID))
		}

	default:
		// show DCA status
		plans := h.trading.DCAExecutor.ActivePlans()
		stats := h.trading.DCAExecutor.Stats()

		if len(plans) == 0 {
			h.send(chatID, fmt.Sprintf(
				"📊 *DCA Status*\n\nNo active plans.\n\nTotal: %d active, %d completed, %d cancelled",
				stats["active"], stats["completed"], stats["cancelled"],
			))
			return
		}

		msg := "📊 *DCA Active Plans*\n\n"
		for _, p := range plans {
			executed := 0
			for _, r := range p.Rounds {
				if r.Executed {
					executed++
				}
			}
			msg += fmt.Sprintf(
				"• *%s* (%s)\n  ID: `%s`\n  Rounds: %d/%d | Filled: $%.2f/$%.2f\n  Avg Entry: $%.2f\n\n",
				p.Symbol, p.Action, p.ID, executed, len(p.Rounds),
				p.TotalFilled, p.TotalSize, p.AvgEntryPrice,
			)
		}
		msg += fmt.Sprintf("Total: %d active, %d completed, %d cancelled\n\nUse /dca cancel <plan_id> to stop a plan.",
			stats["active"], stats["completed"], stats["cancelled"])
		h.send(chatID, msg)
	}
}
