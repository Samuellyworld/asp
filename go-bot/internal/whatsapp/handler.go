package whatsapp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// handles incoming WhatsApp webhook commands
type CommandHandler struct {
	bot         *Bot
	verifyToken string
	commands    map[string]CommandFunc
}

// function signature for command handlers
type CommandFunc func(from string, args []string) string

func NewCommandHandler(bot *Bot, verifyToken string) *CommandHandler {
	h := &CommandHandler{
		bot:         bot,
		verifyToken: verifyToken,
		commands:    make(map[string]CommandFunc),
	}

	// register built-in commands
	h.commands["help"] = h.handleHelp
	h.commands["status"] = h.handleStatus
	h.commands["market"] = h.handleMarket
	h.commands["balance"] = h.handleBalance
	h.commands["positions"] = h.handlePositions
	h.commands["alerts"] = h.handleAlerts

	return h
}

// registers a custom command handler
func (h *CommandHandler) RegisterCommand(name string, fn CommandFunc) {
	h.commands[strings.ToLower(name)] = fn
}

// webhook verification (GET) — Meta sends this to verify the endpoint
func (h *CommandHandler) HandleVerification(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == h.verifyToken {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, challenge)
		return
	}
	http.Error(w, "forbidden", http.StatusForbidden)
}

// webhook message handler (POST) — processes incoming messages
func (h *CommandHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.HandleVerification(w, r)
		return
	}

	var webhook Webhook
	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		slog.Error("whatsapp: failed to decode webhook", "error", err)
		w.WriteHeader(http.StatusOK) // always return 200 to Meta
		return
	}

	for _, entry := range webhook.Entry {
		for _, change := range entry.Changes {
			for _, msg := range change.Value.Messages {
				if msg.Type != "text" {
					continue
				}
				h.processMessage(msg.From, msg.Text.Body)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (h *CommandHandler) processMessage(from, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	// parse command: /command arg1 arg2 or just command arg1 arg2
	text = strings.TrimPrefix(text, "/")
	parts := strings.Fields(text)
	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	handler, ok := h.commands[cmd]
	if !ok {
		response := fmt.Sprintf("Unknown command: %s\nType /help for available commands.", cmd)
		if err := h.bot.SendMessage(from, response); err != nil {
			slog.Error("whatsapp: failed to send reply", "to", from, "error", err)
		}
		return
	}

	response := handler(from, args)
	if response != "" {
		if err := h.bot.SendMessage(from, response); err != nil {
			slog.Error("whatsapp: failed to send reply", "to", from, "error", err)
		}
	}
}

func (h *CommandHandler) handleHelp(_ string, _ []string) string {
	return `📱 Trading Bot Commands:

/status — Bot and account status
/market [SYMBOL] — Market overview
/balance — Account balance
/positions — Open positions
/alerts — Active price alerts
/help — This message`
}

func (h *CommandHandler) handleStatus(_ string, _ []string) string {
	return "✅ Bot is running"
}

func (h *CommandHandler) handleMarket(_ string, args []string) string {
	if len(args) == 0 {
		return "Usage: /market BTCUSDT"
	}
	return fmt.Sprintf("📊 Market data for %s — use Telegram or Discord for full analysis", strings.ToUpper(args[0]))
}

func (h *CommandHandler) handleBalance(_ string, _ []string) string {
	return "💰 Balance check — connect via Telegram/Discord for live data"
}

func (h *CommandHandler) handlePositions(_ string, _ []string) string {
	return "📈 Positions — connect via Telegram/Discord for live data"
}

func (h *CommandHandler) handleAlerts(_ string, _ []string) string {
	return "🔔 Alerts — connect via Telegram/Discord to manage alerts"
}
