// leverage command and callback handlers for telegram.
// handles /leverage enable|disable|status and inline leverage selection buttons.
package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/trading-bot/go-bot/internal/leverage"
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
	fmt.Sscanf(levStr, "%d", &lev)
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

	// auto-approve after leverage selection
	h.trading.OppManager.Approve(oppID, result.User.ID)
	opp := h.trading.OppManager.Get(oppID)
	if opp == nil {
		return true
	}

	h.answerCallback(cb.ID, fmt.Sprintf("%dx %s leverage selected", lev, side))
	h.send(cb.Message.Chat.ID, fmt.Sprintf("⚡ %dx %s leverage approved for %s", lev, side, opp.Symbol))
	return true
}
