// leverage command and component handlers for discord.
// handles /leverage slash command and inline leverage selection buttons.
package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/trading-bot/go-bot/internal/leverage"
)

// handles the /leverage slash command
func (h *Handler) handleLeverageCommand(ctx context.Context, interaction *Interaction) {
	if h.trading == nil {
		h.respondEphemeral(interaction, "trading not available.")
		return
	}

	action := getOption(interaction, "action")

	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	switch strings.TrimSpace(action) {
	case "enable":
		msg := leverage.FormatLeverageOptInPrompt(20, 500)
		h.respondEphemeral(interaction, msg)
	case "disable":
		err := h.userSvc.DisableLeverage(ctx, userID)
		if err != nil {
			h.respondEphemeral(interaction, "failed to disable leverage.")
			return
		}
		h.respond(interaction, "leverage trading disabled.", nil, nil)
	case "status":
		enabled, err := h.userSvc.IsLeverageEnabled(ctx, userID)
		if err != nil {
			h.respondEphemeral(interaction, "failed to check leverage status.")
			return
		}
		if enabled {
			h.respond(interaction, "⚡ Leverage trading is enabled", nil, nil)
		} else {
			h.respond(interaction, "Leverage trading is disabled. Use /leverage enable to opt in.", nil, nil)
		}
	default:
		h.respondEphemeral(interaction, "usage: /leverage action:enable|disable|status")
	}
}

// handles leverage selection from component buttons
func (h *Handler) componentLeverageSelection(ctx context.Context, interaction *Interaction, data string) {
	if h.trading == nil || h.trading.OppManager == nil {
		h.respondEphemeral(interaction, "trading not available.")
		return
	}

	// parse: lev_long_3:oppID or lev_short_5:oppID
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		h.respondEphemeral(interaction, "invalid leverage selection.")
		return
	}

	oppID := parts[1]
	levParts := strings.Split(parts[0], "_") // ["lev", "long", "3"]
	if len(levParts) != 3 {
		h.respondEphemeral(interaction, "invalid leverage selection.")
		return
	}

	side := strings.ToUpper(levParts[1]) // "LONG" or "SHORT"
	levStr := levParts[2]
	lev := 0
	fmt.Sscanf(levStr, "%d", &lev)
	if lev <= 0 {
		h.respondEphemeral(interaction, "invalid leverage value.")
		return
	}

	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	setOk := h.trading.OppManager.SetLeverage(oppID, userID, lev, side)
	if !setOk {
		h.updateMessage(interaction, "opportunity not found or already resolved.", nil, nil)
		return
	}

	// auto-approve after leverage selection
	h.trading.OppManager.Approve(oppID, userID)
	opp := h.trading.OppManager.Get(oppID)
	if opp == nil {
		h.updateMessage(interaction, "opportunity not found.", nil, nil)
		return
	}

	h.updateMessage(interaction, fmt.Sprintf("⚡ %dx %s leverage approved for %s", lev, side, opp.Symbol), nil, nil)
}
