// telegram message handler for user registration and setup
package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/trading-bot/go-bot/internal/user"
)

// Handler processes incoming telegram messages
type Handler struct {
	bot     *Bot
	userSvc *user.Service
	wizard  *user.SetupWizard
}

func NewHandler(bot *Bot, userSvc *user.Service, wizard *user.SetupWizard) *Handler {
	return &Handler{
		bot:     bot,
		userSvc: userSvc,
		wizard:  wizard,
	}
}

// HandleUpdate routes an incoming update to the appropriate handler
func (h *Handler) HandleUpdate(ctx context.Context, update Update) {
	if update.Message == nil || update.Message.From == nil || update.Message.From.IsBot {
		return
	}

	msg := update.Message
	telegramID := msg.From.ID
	chatID := msg.Chat.ID

	// check if user is in setup wizard
	if h.wizard.IsInSetup(telegramID) {
		h.handleWizardInput(ctx, msg, telegramID, chatID)
		return
	}

	// check for commands
	command, _ := ParseCommand(msg.Text)
	switch command {
	case "start":
		h.handleStart(ctx, msg, telegramID, chatID)
	case "setup":
		h.handleSetup(ctx, msg, telegramID, chatID)
	case "status":
		h.handleStatus(ctx, telegramID, chatID)
	case "cancel":
		h.handleCancel(telegramID, chatID)
	case "help":
		h.handleHelp(chatID)
	default:
		if command != "" {
			h.send(chatID, "unknown command. type /help for available commands.")
		}
	}
}

func (h *Handler) handleStart(ctx context.Context, msg *Message, telegramID int64, chatID int64) {
	username := msg.From.Username
	if username == "" {
		username = msg.From.FirstName
	}

	result, err := h.userSvc.Register(ctx, telegramID, username)
	if err != nil {
		log.Printf("error registering user %d: %v", telegramID, err)
		h.send(chatID, "something went wrong during registration. please try again.")
		return
	}

	if result.IsNewUser {
		h.send(chatID, fmt.Sprintf(
			"welcome, %s! 🤖\n\n"+
				"your account has been created.\n\n"+
				"to start trading, you need to connect your binance api keys.\n"+
				"use /setup to begin the api key setup wizard.\n\n"+
				"type /help for all available commands.",
			username,
		))
	} else {
		activated, hasKeys, _ := h.userSvc.GetStatus(ctx, result.User.ID)
		if activated && hasKeys {
			h.send(chatID, fmt.Sprintf("welcome back, %s! your account is active and ready.", username))
		} else {
			h.send(chatID, fmt.Sprintf(
				"welcome back, %s!\n\nyour account exists but hasn't been set up yet.\n"+
					"use /setup to connect your binance api keys.",
				username,
			))
		}
	}
}

func (h *Handler) handleSetup(ctx context.Context, msg *Message, telegramID int64, chatID int64) {
	// check if user exists
	result, err := h.userSvc.Register(ctx, telegramID, msg.From.Username)
	if err != nil {
		h.send(chatID, "something went wrong. please try /start first.")
		return
	}

	// check if already has keys
	_, hasKeys, _ := h.userSvc.GetStatus(ctx, result.User.ID)
	if hasKeys {
		h.send(chatID,
			"you already have api keys configured.\n\n"+
				"if you want to update them, send /setup again and your existing keys will be replaced.\n\n"+
				"⚠️ *important*: only proceed if you want to replace your current keys.",
		)
	}

	// start the wizard
	h.wizard.Start(telegramID, result.User.ID)

	h.send(chatID,
		"🔐 *binance api key setup*\n\n"+
			"i'll walk you through connecting your binance account.\n\n"+
			"⚠️ *security requirements*:\n"+
			"• create keys with *only spot and/or futures* trading enabled\n"+
			"• *do NOT enable withdrawal* permission\n"+
			"• keys with withdrawal permission will be rejected\n\n"+
			"create your api keys at:\nhttps://www.binance.com/en/my/settings/api-management\n\n"+
			"*step 1/2*: please send your *api key*\n\n"+
			"type /cancel to abort setup.",
	)
}

func (h *Handler) handleWizardInput(ctx context.Context, msg *Message, telegramID int64, chatID int64) {
	text := strings.TrimSpace(msg.Text)

	// check for cancel command even during wizard
	if text == "/cancel" {
		h.handleCancel(telegramID, chatID)
		return
	}

	session := h.wizard.GetSession(telegramID)
	if session == nil {
		return
	}

	switch session.Step {
	case user.StepAPIKey:
		// delete the message containing the api key for security
		h.deleteMessage(chatID, msg.MessageID)

		if err := h.wizard.SetAPIKey(telegramID, text); err != nil {
			h.send(chatID, "something went wrong. please try /setup again.")
			h.wizard.Cancel(telegramID)
			return
		}

		h.send(chatID,
			"✅ api key received and message deleted for security.\n\n"+
				"*step 2/2*: now send your *api secret*\n\n"+
				"type /cancel to abort setup.",
		)

	case user.StepAPISecret:
		// delete the message containing the api secret for security
		h.deleteMessage(chatID, msg.MessageID)

		if err := h.wizard.SetAPISecret(telegramID, text); err != nil {
			h.send(chatID, "something went wrong. please try /setup again.")
			h.wizard.Cancel(telegramID)
			return
		}

		// complete the wizard and validate keys
		userID, apiKey, apiSecret, err := h.wizard.Complete(telegramID)
		if err != nil {
			h.send(chatID, "something went wrong. please try /setup again.")
			h.wizard.Cancel(telegramID)
			return
		}

		h.send(chatID, "🔄 validating your api keys with binance...")

		setupResult, err := h.userSvc.SetupAPIKeys(ctx, userID, apiKey, apiSecret)
		if err != nil {
			h.send(chatID, fmt.Sprintf("❌ setup failed: %s\n\nplease check your keys and try /setup again.", err.Error()))
			return
		}

		// build permissions summary
		permsMsg := "detected permissions:\n"
		if setupResult.Permissions.Spot {
			permsMsg += "• ✅ spot trading\n"
		}
		if setupResult.Permissions.Futures {
			permsMsg += "• ✅ futures trading\n"
		}

		h.send(chatID, fmt.Sprintf(
			"✅ *setup complete!*\n\n"+
				"%s\n"+
				"your api keys have been encrypted and stored securely.\n"+
				"your account is now active.\n\n"+
				"your default watchlist (top 10 coins) and preferences have been set up.\n\n"+
				"type /help to see what you can do next.",
			permsMsg,
		))
	}
}

func (h *Handler) handleStatus(ctx context.Context, telegramID int64, chatID int64) {
	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.send(chatID, "something went wrong. please try /start first.")
		return
	}

	activated, hasKeys, _ := h.userSvc.GetStatus(ctx, result.User.ID)

	status := "📊 *account status*\n\n"
	if activated && hasKeys {
		status += "• account: ✅ active\n"
		status += "• api keys: ✅ connected\n"
		status += fmt.Sprintf("• trading mode: %s\n", result.User.TradingMode)
	} else {
		status += "• account: ⏳ pending setup\n"
		status += "• api keys: ❌ not connected\n"
		status += "\nuse /setup to connect your binance api keys."
	}

	h.send(chatID, status)
}

func (h *Handler) handleCancel(telegramID int64, chatID int64) {
	if h.wizard.IsInSetup(telegramID) {
		h.wizard.Cancel(telegramID)
		h.send(chatID, "setup cancelled. your data has been cleared.\n\nuse /setup to start again.")
	} else {
		h.send(chatID, "nothing to cancel.")
	}
}

func (h *Handler) handleHelp(chatID int64) {
	h.send(chatID,
		"*available commands*\n\n"+
			"/start - register or check in\n"+
			"/setup - connect binance api keys\n"+
			"/status - check your account status\n"+
			"/cancel - cancel current setup\n"+
			"/help - show this message",
	)
}

func (h *Handler) send(chatID int64, text string) {
	if err := h.bot.SendMessage(chatID, text); err != nil {
		log.Printf("error sending message to chat %d: %v", chatID, err)
	}
}

func (h *Handler) deleteMessage(chatID int64, messageID int) {
	if err := h.bot.DeleteMessage(chatID, messageID); err != nil {
		log.Printf("warning: failed to delete message %d in chat %d: %v", messageID, chatID, err)
	}
}
