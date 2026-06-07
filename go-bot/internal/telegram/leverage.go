// leverage command and callback handlers for telegram.
// handles /leverage enable|disable|status and inline leverage selection buttons.
package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/trading-bot/go-bot/internal/leverage"
	"github.com/trading-bot/go-bot/internal/opportunity"
)

// handles the /leverage enable|disable|status command
func (h *Handler) handleLeverageCommand(ctx context.Context, args string, telegramID int64, chatID int64) bool {
	if h.trading == nil {
		h.send(chatID, "trading not available")
		return true
	}

	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.send(chatID, "failed to look up user")
		return true
	}

	switch strings.TrimSpace(args) {
	case "enable":
		msg := leverage.FormatLeverageOptInPrompt(20, 500)
		h.send(chatID, msg)
	case "disable":
		err := h.userSvc.DisableLeverage(ctx, result.User.ID)
		if err != nil {
			h.send(chatID, "failed to disable leverage")
			return true
		}
		h.send(chatID, "leverage trading disabled")
	case "status":
		enabled, err := h.userSvc.IsLeverageEnabled(ctx, result.User.ID)
		if err != nil {
			h.send(chatID, "failed to check leverage status")
			return true
		}
		if enabled {
			h.send(chatID, "⚡ Leverage trading is enabled")
		} else {
			h.send(chatID, "Leverage trading is disabled. Use /leverage enable to opt in.")
		}
	default:
		h.send(chatID, "usage: /leverage enable|disable|status")
	}
	return true
}

// handles the explicit leverage risk acknowledgement phrase.
func (h *Handler) handleLeverageConfirmationText(ctx context.Context, telegramID int64, chatID int64, text string) bool {
	if strings.TrimSpace(text) != "I UNDERSTAND LEVERAGE RISKS" {
		return false
	}
	if h.trading == nil {
		h.send(chatID, "trading not available")
		return true
	}

	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.send(chatID, "failed to look up user")
		return true
	}

	if err := h.userSvc.EnableLeverage(ctx, result.User.ID); err != nil {
		h.send(chatID, "failed to enable leverage")
		return true
	}

	h.send(chatID, "⚡ Leverage trading enabled. Use /futuresbalance to confirm your futures USDT balance.")
	return true
}

// handles leverage selection from inline buttons (lev_long_3:oppID etc.)
func (h *Handler) handleLeverageSelection(ctx context.Context, cb *CallbackQuery, data string) bool {
	if h.trading == nil || h.trading.OppManager == nil {
		return false
	}

	// parse: lev_long_3:oppID or lev_short_5:oppID
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return false
	}

	oppID := parts[1]
	levParts := strings.Split(parts[0], "_") // ["lev", "long", "3"]
	if len(levParts) != 3 {
		return false
	}

	side := strings.ToUpper(levParts[1]) // "LONG" or "SHORT"
	levStr := levParts[2]
	lev := 0
	if _, err := fmt.Sscanf(levStr, "%d", &lev); err != nil {
		return false
	}
	if lev <= 0 {
		return false
	}

	result, err := h.userSvc.Register(ctx, cb.From.ID, "")
	if err != nil {
		return false
	}

	ok := h.trading.OppManager.SetLeverage(oppID, result.User.ID, lev, side)
	if !ok {
		h.answerCallback(cb.ID, "opportunity not found or already resolved")
		return true
	}

	opp, claimed := h.trading.OppManager.BeginExecution(oppID, result.User.ID)
	if !claimed {
		h.answerCallback(cb.ID, "opportunity already resolved")
		return true
	}

	plan := opp.Result.Decision.Plan
	if opp.ModifiedPlan != nil {
		plan = *opp.ModifiedPlan
	}
	if plan.PositionSize <= 0 {
		h.trading.OppManager.FailExecution(oppID, result.User.ID)
		h.answerCallback(cb.ID, "invalid position size")
		h.send(cb.Message.Chat.ID, "❌ leverage execution failed: invalid position size")
		return true
	}

	positionSide := leverage.PositionSide(side)
	if h.isLiveEnabled(ctx, result.User.ID) && h.trading.LevLiveExecutor != nil {
		margin, err := leverage.MarginForPositionSize(plan.PositionSize, lev)
		if err != nil {
			h.trading.OppManager.FailExecution(oppID, result.User.ID)
			h.answerCallback(cb.ID, "invalid leverage sizing")
			h.send(cb.Message.Chat.ID, fmt.Sprintf("❌ leverage execution failed: %v", err))
			return true
		}
		if h.trading.FuturesBalanceProvider != nil {
			balance, err := h.trading.FuturesBalanceProvider.GetFuturesBalance(ctx, result.User.ID, "USDT")
			if err != nil {
				h.trading.OppManager.FailExecution(oppID, result.User.ID)
				h.answerCallback(cb.ID, "futures balance check failed")
				h.send(cb.Message.Chat.ID, fmt.Sprintf("❌ live futures execution failed: failed to check futures balance: %v", err))
				return true
			}
			if balance <= 0 {
				h.trading.OppManager.FailExecution(oppID, result.User.ID)
				h.answerCallback(cb.ID, "insufficient futures balance")
				h.send(cb.Message.Chat.ID, "❌ live futures execution failed: no available USDT futures balance")
				return true
			}
			margin = leverage.CapMarginToAvailableBalance(margin, balance)
			if margin <= 0 {
				h.trading.OppManager.FailExecution(oppID, result.User.ID)
				h.answerCallback(cb.ID, "insufficient futures balance")
				h.send(cb.Message.Chat.ID, "❌ live futures execution failed: no usable USDT futures balance after exchange buffer")
				return true
			}
		}

		pos, err := h.trading.LevLiveExecutor.OpenPosition(
			result.User.ID,
			opp.Symbol,
			positionSide,
			lev,
			margin,
			plan.StopLoss,
			plan.TakeProfit,
			"telegram",
		)
		if err != nil {
			if !strings.Contains(err.Error(), "CRITICAL") {
				h.trading.OppManager.FailExecution(oppID, result.User.ID)
			}
			h.answerCallback(cb.ID, "futures execution failed")
			h.send(cb.Message.Chat.ID, fmt.Sprintf("❌ live futures execution failed: %v", err))
			return true
		}
		h.trading.OppManager.CompleteExecution(oppID, result.User.ID)
		h.editMessage(cb.Message.Chat.ID, cb.Message.MessageID, opportunity.FormatApprovedMessage(opp)+opportunity.FormatLeverageSelected(opp), nil)
		h.answerCallback(cb.ID, "live futures opened")
		plannedMargin := plan.PositionSize / float64(lev)
		if margin < plannedMargin {
			h.send(cb.Message.Chat.ID, fmt.Sprintf("ℹ️ Position margin capped to available futures balance: $%.2f (AI suggested $%.2f margin).", margin, plannedMargin))
		}
		h.send(cb.Message.Chat.ID, leverage.FormatLeverageOpened(pos))
		return true
	}

	if h.trading.LevPaperExecutor != nil {
		margin, err := leverage.MarginForPositionSize(plan.PositionSize, lev)
		if err != nil {
			h.trading.OppManager.FailExecution(oppID, result.User.ID)
			h.answerCallback(cb.ID, "invalid leverage sizing")
			h.send(cb.Message.Chat.ID, fmt.Sprintf("❌ leverage execution failed: %v", err))
			return true
		}
		pos, err := h.trading.LevPaperExecutor.OpenPosition(
			result.User.ID,
			opp.Symbol,
			positionSide,
			lev,
			margin,
			plan.StopLoss,
			plan.TakeProfit,
			"telegram",
		)
		if err != nil {
			h.trading.OppManager.FailExecution(oppID, result.User.ID)
			h.answerCallback(cb.ID, "paper futures failed")
			h.send(cb.Message.Chat.ID, fmt.Sprintf("❌ paper futures execution failed: %v", err))
			return true
		}
		h.trading.OppManager.CompleteExecution(oppID, result.User.ID)
		h.editMessage(cb.Message.Chat.ID, cb.Message.MessageID, opportunity.FormatApprovedMessage(opp)+opportunity.FormatLeverageSelected(opp), nil)
		h.answerCallback(cb.ID, "paper futures opened")
		h.send(cb.Message.Chat.ID, leverage.FormatLeverageOpened(pos))
		return true
	}

	h.trading.OppManager.FailExecution(oppID, result.User.ID)
	h.answerCallback(cb.ID, "futures executor unavailable")
	h.send(cb.Message.Chat.ID, "futures execution is not available.")
	return true
}
