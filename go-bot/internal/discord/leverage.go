// leverage command and component handlers for discord.
// handles /leverage slash command and inline leverage selection buttons.
package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/trading-bot/go-bot/internal/leverage"
	"github.com/trading-bot/go-bot/internal/opportunity"
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
	if _, err := fmt.Sscanf(levStr, "%d", &lev); err != nil {
		h.respondEphemeral(interaction, "invalid leverage value.")
		return
	}
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

	opp, claimed := h.trading.OppManager.BeginExecution(oppID, userID)
	if !claimed {
		h.updateMessage(interaction, "opportunity already resolved.", nil, nil)
		return
	}

	plan := opp.Result.Decision.Plan
	if opp.ModifiedPlan != nil {
		plan = *opp.ModifiedPlan
	}
	if plan.PositionSize <= 0 {
		h.trading.OppManager.FailExecution(oppID, userID)
		h.updateMessage(interaction, "leverage execution failed: invalid position size.", nil, nil)
		return
	}

	positionSide := leverage.PositionSide(side)
	if h.trading.Confirm != nil && h.trading.Confirm.IsConfirmed(userID) && h.trading.LevLiveExecutor != nil {
		margin := plan.PositionSize
		if h.trading.FuturesBalanceProvider != nil {
			balance, err := h.trading.FuturesBalanceProvider.GetFuturesBalance(ctx, userID, "USDT")
			if err != nil {
				h.trading.OppManager.FailExecution(oppID, userID)
				h.updateMessage(interaction, fmt.Sprintf("live futures execution failed: failed to check futures balance: %v", err), nil, nil)
				return
			}
			if balance <= 0 {
				h.trading.OppManager.FailExecution(oppID, userID)
				h.updateMessage(interaction, "live futures execution failed: no available USDT futures balance.", nil, nil)
				return
			}
			if margin > balance {
				margin = balance
			}
		}

		pos, err := h.trading.LevLiveExecutor.OpenPosition(
			userID,
			opp.Symbol,
			positionSide,
			lev,
			margin,
			plan.StopLoss,
			plan.TakeProfit,
			"discord",
		)
		if err != nil {
			if !strings.Contains(err.Error(), "CRITICAL") {
				h.trading.OppManager.FailExecution(oppID, userID)
			}
			h.updateMessage(interaction, fmt.Sprintf("live futures execution failed: %v", err), nil, nil)
			return
		}
		h.trading.OppManager.CompleteExecution(oppID, userID)
		h.updateMessage(interaction, opportunity.FormatApprovedMessage(opp)+opportunity.FormatLeverageSelected(opp), nil, nil)
		if margin < plan.PositionSize {
			_ = h.bot.SendMessage(interaction.ChannelID, fmt.Sprintf("Position margin capped to available futures balance: $%.2f (AI suggested $%.2f).", margin, plan.PositionSize))
		}
		_ = h.bot.SendMessage(interaction.ChannelID, leverage.FormatLeverageOpened(pos))
		return
	}

	if h.trading.LevPaperExecutor != nil {
		pos, err := h.trading.LevPaperExecutor.OpenPosition(
			userID,
			opp.Symbol,
			positionSide,
			lev,
			plan.PositionSize,
			plan.StopLoss,
			plan.TakeProfit,
			"discord",
		)
		if err != nil {
			h.trading.OppManager.FailExecution(oppID, userID)
			h.updateMessage(interaction, fmt.Sprintf("paper futures execution failed: %v", err), nil, nil)
			return
		}
		h.trading.OppManager.CompleteExecution(oppID, userID)
		h.updateMessage(interaction, opportunity.FormatApprovedMessage(opp)+opportunity.FormatLeverageSelected(opp), nil, nil)
		_ = h.bot.SendMessage(interaction.ChannelID, leverage.FormatLeverageOpened(pos))
		return
	}

	h.trading.OppManager.FailExecution(oppID, userID)
	h.updateMessage(interaction, "futures execution is not available.", nil, nil)
}
