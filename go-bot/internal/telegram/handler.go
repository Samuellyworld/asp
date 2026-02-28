// telegram message handler for user registration, setup, watchlist, preferences, and exchange data
package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/preferences"
	"github.com/trading-bot/go-bot/internal/user"
	"github.com/trading-bot/go-bot/internal/watchlist"
)

// botClient defines the interface for sending telegram messages
type botClient interface {
	SendMessage(chatID int64, text string) error
	SendMessageWithKeyboard(chatID int64, text string, keyboard *InlineKeyboardMarkup) error
	EditMessageText(chatID int64, messageID int, text string, keyboard *InlineKeyboardMarkup) error
	AnswerCallbackQuery(queryID string, text string) error
	DeleteMessage(chatID int64, messageID int) error
}

// exchangeClient defines the interface for fetching market data
type exchangeClient interface {
	GetPrice(ctx context.Context, symbol string) (*exchange.Ticker, error)
	GetOrderBook(ctx context.Context, symbol string, depth int) (*exchange.OrderBook, error)
	GetBalance(ctx context.Context, apiKey, apiSecret string) ([]exchange.Balance, error)
}

// Handler processes incoming telegram messages
type Handler struct {
	bot       botClient
	userSvc   *user.Service
	wizard    *user.SetupWizard
	watchSvc  *watchlist.Service
	prefsSvc  *preferences.Service
	exchange  exchangeClient
}

func NewHandler(
	bot botClient,
	userSvc *user.Service,
	wizard *user.SetupWizard,
	watchSvc *watchlist.Service,
	prefsSvc *preferences.Service,
	exch exchangeClient,
) *Handler {
	return &Handler{
		bot:      bot,
		userSvc:  userSvc,
		wizard:   wizard,
		watchSvc: watchSvc,
		prefsSvc: prefsSvc,
		exchange: exch,
	}
}

//  routes an incoming update to the appropriate handler
func (h *Handler) HandleUpdate(ctx context.Context, update Update) {
	// handle callback queries from inline keyboard buttons
	if update.CallbackQuery != nil {
		h.handleCallback(ctx, update.CallbackQuery)
		return
	}

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
	// watchlist commands
	case "watchlist", "wl":
		h.handleWatchlist(ctx, telegramID, chatID)
	case "watchadd", "wa":
		h.handleWatchAdd(ctx, msg, telegramID, chatID)
	case "watchremove", "wr":
		h.handleWatchRemove(ctx, msg, telegramID, chatID)
	case "watchreset":
		h.handleWatchReset(ctx, telegramID, chatID)
	// preferences commands
	case "settings":
		h.handleSettings(ctx, telegramID, chatID)
	case "set":
		h.handleSet(ctx, msg, telegramID, chatID)
	// exchange data commands
	case "price", "p":
		h.handlePrice(ctx, msg, chatID)
	case "balance", "bal":
		h.handleBalance(ctx, telegramID, chatID)
	case "orderbook", "ob":
		h.handleOrderBook(ctx, msg, chatID)
	case "portfolio", "pf":
		h.handlePortfolio(ctx, telegramID, chatID)
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
			"*account*\n"+
			"/start - register or check in\n"+
			"/setup - connect binance api keys\n"+
			"/status - check your account status\n"+
			"/cancel - cancel current setup\n\n"+
			"*exchange*\n"+
			"/price <symbol> - get current price (e.g. /price BTC)\n"+
			"/balance - show your account balances\n"+
			"/portfolio - portfolio overview with value estimate\n"+
			"/orderbook <symbol> - show order book (e.g. /ob BTC)\n\n"+
			"*watchlist*\n"+
			"/watchlist - view your watchlist\n"+
			"/watchadd <symbol> - add a symbol (e.g. /watchadd BTCUSDT)\n"+
			"/watchremove <symbol> - remove a symbol\n"+
			"/watchreset - reset to default top-10\n\n"+
			"*preferences*\n"+
			"/settings - view all your preferences\n"+
			"/set <key> <value> - change a preference\n\n"+
			"/help - show this message",
	)
}

// resolves a telegram id to an internal user id
func (h *Handler) getUserID(ctx context.Context, telegramID int64, chatID int64) (int, bool) {
	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.send(chatID, "something went wrong. please try /start first.")
		return 0, false
	}
	activated, hasKeys, _ := h.userSvc.GetStatus(ctx, result.User.ID)
	if !activated || !hasKeys {
		h.send(chatID, "you need to complete setup first. use /setup to connect your binance api keys.")
		return 0, false
	}
	return result.User.ID, true
}

// watchlist handlers

func (h *Handler) handleWatchlist(ctx context.Context, telegramID int64, chatID int64) {
	userID, ok := h.getUserID(ctx, telegramID, chatID)
	if !ok {
		return
	}

	items, err := h.watchSvc.List(ctx, userID)
	if err != nil {
		log.Printf("error listing watchlist for user %d: %v", userID, err)
		h.send(chatID, "failed to load watchlist. please try again.")
		return
	}

	if len(items) == 0 {
		h.send(chatID, "your watchlist is empty.\n\nuse /watchadd <symbol> to add one, or /watchreset for the default top-10.")
		return
	}

	msg := fmt.Sprintf("📋 *your watchlist* (%d symbols)\n\n", len(items))
	var buttons [][]InlineKeyboardButton

	for i, item := range items {
		// try to get live price for each symbol
		ticker, err := h.exchange.GetPrice(ctx, item.Symbol)
		if err == nil && ticker != nil {
			changeEmoji := "📈"
			if ticker.ChangePct < 0 {
				changeEmoji = "📉"
			}
			msg += fmt.Sprintf("%d. `%s` — `$%s` %s `%.2f%%`\n", i+1, item.Symbol, formatPrice(ticker.Price), changeEmoji, ticker.ChangePct)
		} else {
			msg += fmt.Sprintf("%d. `%s`\n", i+1, item.Symbol)
		}

		// add a button row for every 3 symbols (compact layout)
		if i%3 == 0 {
			buttons = append(buttons, []InlineKeyboardButton{})
		}
		row := len(buttons) - 1
		buttons[row] = append(buttons[row], InlineKeyboardButton{
			Text:         item.Symbol,
			CallbackData: "wl_price:" + item.Symbol,
		})
	}

	msg += "\nuse /watchadd, /watchremove, or /watchreset to manage."

	keyboard := &InlineKeyboardMarkup{InlineKeyboard: buttons}
	h.sendWithKeyboard(chatID, msg, keyboard)
}

func (h *Handler) handleWatchAdd(ctx context.Context, msg *Message, telegramID int64, chatID int64) {
	userID, ok := h.getUserID(ctx, telegramID, chatID)
	if !ok {
		return
	}

	_, args := ParseCommand(msg.Text)
	if args == "" {
		h.send(chatID, "usage: /watchadd <symbol>\n\nexample: /watchadd BTCUSDT")
		return
	}

	symbol := strings.TrimSpace(args)
	if err := h.watchSvc.Add(ctx, userID, symbol); err != nil {
		h.send(chatID, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

	h.send(chatID, fmt.Sprintf("✅ added to watchlist. use /watchlist to see your list."))
}

func (h *Handler) handleWatchRemove(ctx context.Context, msg *Message, telegramID int64, chatID int64) {
	userID, ok := h.getUserID(ctx, telegramID, chatID)
	if !ok {
		return
	}

	_, args := ParseCommand(msg.Text)
	if args == "" {
		h.send(chatID, "usage: /watchremove <symbol>\n\nexample: /watchremove BTCUSDT")
		return
	}

	symbol := strings.TrimSpace(args)
	if err := h.watchSvc.Remove(ctx, userID, symbol); err != nil {
		h.send(chatID, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

	h.send(chatID, "✅ removed from watchlist.")
}

func (h *Handler) handleWatchReset(ctx context.Context, telegramID int64, chatID int64) {
	_, ok := h.getUserID(ctx, telegramID, chatID)
	if !ok {
		return
	}

	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "✅ Yes, reset", CallbackData: "watchreset_confirm"},
				{Text: "❌ Cancel", CallbackData: "watchreset_cancel"},
			},
		},
	}

	h.sendWithKeyboard(chatID,
		"⚠️ *are you sure?*\n\nthis will replace your current watchlist with the default top-10 symbols.",
		keyboard,
	)
}

// preferences handlers

func (h *Handler) handleSettings(ctx context.Context, telegramID int64, chatID int64) {
	userID, ok := h.getUserID(ctx, telegramID, chatID)
	if !ok {
		return
	}

	scan, err := h.prefsSvc.GetScanning(ctx, userID)
	if err != nil {
		log.Printf("error getting scanning prefs for user %d: %v", userID, err)
		h.send(chatID, "failed to load preferences. please try again.")
		return
	}

	notif, err := h.prefsSvc.GetNotification(ctx, userID)
	if err != nil {
		log.Printf("error getting notification prefs for user %d: %v", userID, err)
		h.send(chatID, "failed to load preferences. please try again.")
		return
	}

	trade, err := h.prefsSvc.GetTrading(ctx, userID)
	if err != nil {
		log.Printf("error getting trading prefs for user %d: %v", userID, err)
		h.send(chatID, "failed to load preferences. please try again.")
		return
	}

	scanStatus := "disabled"
	if scan.IsScanningEnabled {
		scanStatus = "enabled"
	}

	msg := "⚙️ *your preferences*\n\n"

	msg += "*scanning*\n"
	msg += fmt.Sprintf("• status: %s\n", scanStatus)
	msg += fmt.Sprintf("• min confidence: %d%%\n", scan.MinConfidence)
	msg += fmt.Sprintf("• scan interval: %d min\n", scan.ScanIntervalMins)
	msg += fmt.Sprintf("• timeframes: %s\n", strings.Join(scan.EnabledTimeframes, ", "))
	msg += "\n"

	msg += "*notifications*\n"
	msg += fmt.Sprintf("• max daily: %d\n", notif.MaxDailyNotifications)
	msg += fmt.Sprintf("• timezone: %s\n", notif.Timezone)
	msg += fmt.Sprintf("• daily summary: hour %d\n", notif.DailySummaryHour)
	msg += "\n"

	msg += "*trading*\n"
	msg += fmt.Sprintf("• position size: $%.2f (max $%.2f)\n", trade.DefaultPositionSize, trade.MaxPositionSize)
	msg += fmt.Sprintf("• stop loss: %.1f%%\n", trade.DefaultStopLossPct)
	msg += fmt.Sprintf("• take profit: %.1f%%\n", trade.DefaultTakeProfitPct)
	msg += fmt.Sprintf("• max leverage: %dx\n", trade.MaxLeverage)
	msg += fmt.Sprintf("• risk per trade: %.1f%%\n", trade.RiskPerTradePct)
	msg += "\n"

	msg += "use `/set <key> <value>` to change.\n"
	msg += "keys: confidence, interval, maxnotifs, timezone, summaryhour, positionsize, stoploss, takeprofit, leverage, risk, scanning"

	h.send(chatID, msg)
}

func (h *Handler) handleSet(ctx context.Context, msg *Message, telegramID int64, chatID int64) {
	userID, ok := h.getUserID(ctx, telegramID, chatID)
	if !ok {
		return
	}

	_, args := ParseCommand(msg.Text)
	parts := strings.Fields(args)
	if len(parts) < 2 {
		h.send(chatID, "usage: /set <key> <value>\n\nexample: /set confidence 70\n\n"+
			"keys: confidence, interval, maxnotifs, timezone, summaryhour, positionsize, stoploss, takeprofit, leverage, risk, scanning")
		return
	}

	key := strings.ToLower(parts[0])
	value := parts[1]

	var setErr error
	switch key {
	case "confidence":
		v, err := strconv.Atoi(value)
		if err != nil {
			h.send(chatID, "confidence must be a number (0-100)")
			return
		}
		setErr = h.prefsSvc.SetMinConfidence(ctx, userID, v)

	case "interval":
		v, err := strconv.Atoi(value)
		if err != nil {
			h.send(chatID, "interval must be a number (1-60 minutes)")
			return
		}
		setErr = h.prefsSvc.SetScanInterval(ctx, userID, v)

	case "maxnotifs":
		v, err := strconv.Atoi(value)
		if err != nil {
			h.send(chatID, "maxnotifs must be a number (1-100)")
			return
		}
		setErr = h.prefsSvc.SetMaxDailyNotifications(ctx, userID, v)

	case "timezone":
		setErr = h.prefsSvc.SetTimezone(ctx, userID, value)

	case "summaryhour":
		v, err := strconv.Atoi(value)
		if err != nil {
			h.send(chatID, "summaryhour must be a number (0-23)")
			return
		}
		setErr = h.prefsSvc.SetDailySummaryHour(ctx, userID, v)

	case "positionsize":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			h.send(chatID, "positionsize must be a number")
			return
		}
		maxSize := v * 3
		if len(parts) > 2 {
			m, err := strconv.ParseFloat(parts[2], 64)
			if err == nil {
				maxSize = m
			}
		}
		setErr = h.prefsSvc.SetPositionSize(ctx, userID, v, maxSize)

	case "stoploss":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			h.send(chatID, "stoploss must be a number")
			return
		}
		setErr = h.prefsSvc.SetStopLoss(ctx, userID, v)

	case "takeprofit":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			h.send(chatID, "takeprofit must be a number")
			return
		}
		setErr = h.prefsSvc.SetTakeProfit(ctx, userID, v)

	case "leverage":
		v, err := strconv.Atoi(value)
		if err != nil {
			h.send(chatID, "leverage must be a number (1-125)")
			return
		}
		setErr = h.prefsSvc.SetMaxLeverage(ctx, userID, v)

	case "risk":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			h.send(chatID, "risk must be a number")
			return
		}
		setErr = h.prefsSvc.SetRiskPerTrade(ctx, userID, v)

	case "scanning":
		v := strings.ToLower(value)
		if v != "on" && v != "off" {
			h.send(chatID, "scanning must be 'on' or 'off'")
			return
		}
		setErr = h.prefsSvc.ToggleScanning(ctx, userID, v == "on")

	default:
		h.send(chatID, fmt.Sprintf("unknown setting: %s\n\n"+
			"keys: confidence, interval, maxnotifs, timezone, summaryhour, positionsize, stoploss, takeprofit, leverage, risk, scanning", key))
		return
	}

	if setErr != nil {
		h.send(chatID, fmt.Sprintf("❌ %s", setErr.Error()))
		return
	}

	h.send(chatID, fmt.Sprintf("✅ %s updated. use /settings to see all preferences.", key))
}

// exchange data handlers

func (h *Handler) handlePrice(ctx context.Context, msg *Message, chatID int64) {
	_, args := ParseCommand(msg.Text)
	if args == "" {
		h.send(chatID, "usage: /price <symbol>\n\nexamples:\n/price BTC\n/price ETHUSDT\n/price SOL/USDT")
		return
	}

	symbol := normalizeSymbolForExchange(args)

	ticker, err := h.exchange.GetPrice(ctx, symbol)
	if err != nil {
		h.send(chatID, fmt.Sprintf("❌ failed to get price: %s", err.Error()))
		return
	}

	text := formatTickerMessage(ticker)

	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🔄 Refresh", CallbackData: "price:" + symbol},
				{Text: "📊 Order Book", CallbackData: "ob:" + symbol},
			},
			{
				{Text: "⭐ Add to Watchlist", CallbackData: "wa:" + symbol},
			},
		},
	}

	h.sendWithKeyboard(chatID, text, keyboard)
}

func (h *Handler) handleBalance(ctx context.Context, telegramID int64, chatID int64) {
	userID, ok := h.getUserID(ctx, telegramID, chatID)
	if !ok {
		return
	}

	apiKey, apiSecret, err := h.userSvc.GetDecryptedCredentials(ctx, userID)
	if err != nil {
		h.send(chatID, "❌ failed to retrieve your api keys. try /setup to reconfigure.")
		return
	}

	balances, err := h.exchange.GetBalance(ctx, apiKey, apiSecret)
	if err != nil {
		h.send(chatID, fmt.Sprintf("❌ failed to get balance: %s", err.Error()))
		return
	}

	if len(balances) == 0 {
		h.send(chatID, "💼 your account has no balances.")
		return
	}

	text := "💼 *your balances*\n\n"
	for _, b := range balances {
		line := fmt.Sprintf("• *%s*: `%s`", b.Asset, formatBalance(b.Free))
		if b.Locked > 0 {
			line += fmt.Sprintf(" (locked: `%s`)", formatBalance(b.Locked))
		}
		text += line + "\n"
	}

	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🔄 Refresh", CallbackData: "refresh_balance"},
				{Text: "📊 Portfolio", CallbackData: "portfolio"},
			},
		},
	}

	h.sendWithKeyboard(chatID, text, keyboard)
}

func (h *Handler) handleOrderBook(ctx context.Context, msg *Message, chatID int64) {
	_, args := ParseCommand(msg.Text)
	if args == "" {
		h.send(chatID, "usage: /orderbook <symbol>\n\nexamples:\n/orderbook BTC\n/ob ETHUSDT")
		return
	}

	parts := strings.Fields(args)
	symbol := normalizeSymbolForExchange(parts[0])
	depth := 5
	if len(parts) > 1 {
		if d, err := strconv.Atoi(parts[1]); err == nil && d > 0 && d <= 20 {
			depth = d
		}
	}

	book, err := h.exchange.GetOrderBook(ctx, symbol, depth)
	if err != nil {
		h.send(chatID, fmt.Sprintf("❌ failed to get order book: %s", err.Error()))
		return
	}

	text := formatOrderBookMessage(book)

	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🔄 Refresh", CallbackData: fmt.Sprintf("ob:%s:%d", symbol, depth)},
				{Text: "💰 Price", CallbackData: "price:" + symbol},
			},
		},
	}

	h.sendWithKeyboard(chatID, text, keyboard)
}

func (h *Handler) handlePortfolio(ctx context.Context, telegramID int64, chatID int64) {
	userID, ok := h.getUserID(ctx, telegramID, chatID)
	if !ok {
		return
	}

	apiKey, apiSecret, err := h.userSvc.GetDecryptedCredentials(ctx, userID)
	if err != nil {
		h.send(chatID, "❌ failed to retrieve your api keys. try /setup to reconfigure.")
		return
	}

	balances, err := h.exchange.GetBalance(ctx, apiKey, apiSecret)
	if err != nil {
		h.send(chatID, fmt.Sprintf("❌ failed to get balance: %s", err.Error()))
		return
	}

	if len(balances) == 0 {
		h.send(chatID, "💼 your portfolio is empty.")
		return
	}

	text, _ := h.buildPortfolioText(ctx, balances)

	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🔄 Refresh", CallbackData: "portfolio"},
				{Text: "💼 Balances", CallbackData: "refresh_balance"},
			},
		},
	}

	h.sendWithKeyboard(chatID, text, keyboard)
}

// builds portfolio text with estimated usd values
func (h *Handler) buildPortfolioText(ctx context.Context, balances []exchange.Balance) (string, float64) {
	text := "📊 *portfolio overview*\n\n"
	totalUSD := 0.0

	type assetLine struct {
		asset  string
		amount float64
		usd    float64
		priced bool
	}
	var lines []assetLine

	for _, b := range balances {
		total := b.Free + b.Locked
		if total == 0 {
			continue
		}

		line := assetLine{asset: b.Asset, amount: total}

		// stablecoins are 1:1 usd
		if isStablecoin(b.Asset) {
			line.usd = total
			line.priced = true
		} else {
			// try to fetch price
			symbol := b.Asset + "/USDT"
			ticker, err := h.exchange.GetPrice(ctx, symbol)
			if err == nil && ticker != nil {
				line.usd = total * ticker.Price
				line.priced = true
			}
		}

		if line.priced {
			totalUSD += line.usd
		}
		lines = append(lines, line)
	}

	for _, l := range lines {
		if l.priced {
			text += fmt.Sprintf("• *%s*: `%s` ≈ `$%s`\n", l.asset, formatBalance(l.amount), formatPrice(l.usd))
		} else {
			text += fmt.Sprintf("• *%s*: `%s`\n", l.asset, formatBalance(l.amount))
		}
	}

	text += fmt.Sprintf("\n💰 *estimated total*: `$%s`", formatPrice(totalUSD))
	return text, totalUSD
}

// callback query handler — routes button presses to appropriate actions
func (h *Handler) handleCallback(ctx context.Context, cb *CallbackQuery) {
	if cb.From == nil || cb.Message == nil {
		return
	}

	telegramID := cb.From.ID
	chatID := cb.Message.Chat.ID
	messageID := cb.Message.MessageID
	data := cb.Data

	switch {
	case strings.HasPrefix(data, "price:"):
		symbol := strings.TrimPrefix(data, "price:")
		h.callbackPrice(ctx, cb.ID, chatID, messageID, symbol)

	case strings.HasPrefix(data, "ob:"):
		h.callbackOrderBook(ctx, cb.ID, chatID, messageID, data)

	case strings.HasPrefix(data, "wa:"):
		symbol := strings.TrimPrefix(data, "wa:")
		h.callbackWatchAdd(ctx, cb.ID, telegramID, chatID, symbol)

	case data == "refresh_balance":
		h.callbackBalance(ctx, cb.ID, telegramID, chatID, messageID)

	case data == "portfolio":
		h.callbackPortfolio(ctx, cb.ID, telegramID, chatID, messageID)

	case data == "watchreset_confirm":
		h.callbackWatchResetConfirm(ctx, cb.ID, telegramID, chatID, messageID)

	case data == "watchreset_cancel":
		h.answerCallback(cb.ID, "cancelled")
		h.editMessage(chatID, messageID, "watchlist reset cancelled.", nil)

	case strings.HasPrefix(data, "wl_price:"):
		symbol := strings.TrimPrefix(data, "wl_price:")
		h.callbackPrice(ctx, cb.ID, chatID, messageID, symbol)

	default:
		h.answerCallback(cb.ID, "")
	}
}

// refreshes price data in-place via inline button
func (h *Handler) callbackPrice(ctx context.Context, queryID string, chatID int64, messageID int, symbol string) {
	ticker, err := h.exchange.GetPrice(ctx, symbol)
	if err != nil {
		h.answerCallback(queryID, "failed to refresh price")
		return
	}

	text := formatTickerMessage(ticker)
	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🔄 Refresh", CallbackData: "price:" + symbol},
				{Text: "📊 Order Book", CallbackData: "ob:" + symbol},
			},
			{
				{Text: "⭐ Add to Watchlist", CallbackData: "wa:" + symbol},
			},
		},
	}

	h.answerCallback(queryID, "price updated")
	h.editMessage(chatID, messageID, text, keyboard)
}

// refreshes order book data in-place via inline button
func (h *Handler) callbackOrderBook(ctx context.Context, queryID string, chatID int64, messageID int, data string) {
	// format: "ob:SYMBOL" or "ob:SYMBOL:DEPTH"
	parts := strings.Split(strings.TrimPrefix(data, "ob:"), ":")
	symbol := parts[0]
	depth := 5
	if len(parts) > 1 {
		if d, err := strconv.Atoi(parts[1]); err == nil && d > 0 {
			depth = d
		}
	}

	book, err := h.exchange.GetOrderBook(ctx, symbol, depth)
	if err != nil {
		h.answerCallback(queryID, "failed to refresh order book")
		return
	}

	text := formatOrderBookMessage(book)
	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🔄 Refresh", CallbackData: fmt.Sprintf("ob:%s:%d", symbol, depth)},
				{Text: "💰 Price", CallbackData: "price:" + symbol},
			},
		},
	}

	h.answerCallback(queryID, "order book updated")
	h.editMessage(chatID, messageID, text, keyboard)
}

// adds symbol to watchlist from inline button
func (h *Handler) callbackWatchAdd(ctx context.Context, queryID string, telegramID int64, chatID int64, symbol string) {
	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.answerCallback(queryID, "account error")
		return
	}
	activated, hasKeys, _ := h.userSvc.GetStatus(ctx, result.User.ID)
	if !activated || !hasKeys {
		h.answerCallback(queryID, "complete setup first")
		return
	}

	if err := h.watchSvc.Add(ctx, result.User.ID, symbol); err != nil {
		h.answerCallback(queryID, err.Error())
		return
	}

	h.answerCallback(queryID, fmt.Sprintf("%s added to watchlist", symbol))
}

// refreshes balance in-place via inline button
func (h *Handler) callbackBalance(ctx context.Context, queryID string, telegramID int64, chatID int64, messageID int) {
	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.answerCallback(queryID, "account error")
		return
	}
	userID := result.User.ID

	apiKey, apiSecret, err := h.userSvc.GetDecryptedCredentials(ctx, userID)
	if err != nil {
		h.answerCallback(queryID, "failed to get credentials")
		return
	}

	balances, err := h.exchange.GetBalance(ctx, apiKey, apiSecret)
	if err != nil {
		h.answerCallback(queryID, "failed to refresh balance")
		return
	}

	if len(balances) == 0 {
		h.answerCallback(queryID, "no balances")
		h.editMessage(chatID, messageID, "💼 your account has no balances.", nil)
		return
	}

	text := "💼 *your balances*\n\n"
	for _, b := range balances {
		line := fmt.Sprintf("• *%s*: `%s`", b.Asset, formatBalance(b.Free))
		if b.Locked > 0 {
			line += fmt.Sprintf(" (locked: `%s`)", formatBalance(b.Locked))
		}
		text += line + "\n"
	}

	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🔄 Refresh", CallbackData: "refresh_balance"},
				{Text: "📊 Portfolio", CallbackData: "portfolio"},
			},
		},
	}

	h.answerCallback(queryID, "balance updated")
	h.editMessage(chatID, messageID, text, keyboard)
}

// refreshes portfolio in-place via inline button
func (h *Handler) callbackPortfolio(ctx context.Context, queryID string, telegramID int64, chatID int64, messageID int) {
	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.answerCallback(queryID, "account error")
		return
	}
	userID := result.User.ID

	apiKey, apiSecret, err := h.userSvc.GetDecryptedCredentials(ctx, userID)
	if err != nil {
		h.answerCallback(queryID, "failed to get credentials")
		return
	}

	balances, err := h.exchange.GetBalance(ctx, apiKey, apiSecret)
	if err != nil {
		h.answerCallback(queryID, "failed to refresh portfolio")
		return
	}

	text, _ := h.buildPortfolioText(ctx, balances)
	keyboard := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "🔄 Refresh", CallbackData: "portfolio"},
				{Text: "💼 Balances", CallbackData: "refresh_balance"},
			},
		},
	}

	h.answerCallback(queryID, "portfolio updated")
	h.editMessage(chatID, messageID, text, keyboard)
}

// confirms watchlist reset via inline button
func (h *Handler) callbackWatchResetConfirm(ctx context.Context, queryID string, telegramID int64, chatID int64, messageID int) {
	result, err := h.userSvc.Register(ctx, telegramID, "")
	if err != nil {
		h.answerCallback(queryID, "account error")
		return
	}
	userID := result.User.ID

	if err := h.watchSvc.Reset(ctx, userID); err != nil {
		h.answerCallback(queryID, "failed to reset watchlist")
		return
	}

	h.answerCallback(queryID, "watchlist reset")
	h.editMessage(chatID, messageID, "✅ watchlist reset to default top-10 symbols.\n\nuse /watchlist to see them.", nil)
}

// formats a complete ticker message
func formatTickerMessage(ticker *exchange.Ticker) string {
	changeEmoji := "📈"
	if ticker.ChangePct < 0 {
		changeEmoji = "📉"
	}

	return fmt.Sprintf(
		"💰 *%s*\n\n"+
			"price: `$%s`\n"+
			"24h change: %s `%s (%.2f%%)`\n"+
			"24h volume: `%s`",
		ticker.Symbol,
		formatPrice(ticker.Price),
		changeEmoji,
		formatPriceChange(ticker.PriceChange),
		ticker.ChangePct,
		formatVolume(ticker.QuoteVolume),
	)
}

// formats a complete order book message
func formatOrderBookMessage(book *exchange.OrderBook) string {
	text := fmt.Sprintf("📊 *order book — %s*\n\n", book.Symbol)

	text += "*asks (sell)*\n"
	for i := len(book.Asks) - 1; i >= 0; i-- {
		a := book.Asks[i]
		text += fmt.Sprintf("`$%s` × `%s`\n", formatPrice(a.Price), formatBalance(a.Quantity))
	}

	text += "————————\n"

	text += "*bids (buy)*\n"
	for _, b := range book.Bids {
		text += fmt.Sprintf("`$%s` × `%s`\n", formatPrice(b.Price), formatBalance(b.Quantity))
	}

	return text
}

// returns true for known stablecoin tickers
func isStablecoin(asset string) bool {
	switch asset {
	case "USDT", "USDC", "BUSD", "DAI", "TUSD", "USDP", "FDUSD":
		return true
	}
	return false
}

// normalizes user input like "BTC" or "BTCUSDT" to "BTC/USDT"
func normalizeSymbolForExchange(input string) string {
	input = strings.ToUpper(strings.TrimSpace(input))

	// already has slash
	if strings.Contains(input, "/") {
		return input
	}

	// try to split at known quote currencies
	for _, quote := range []string{"USDT", "BUSD", "USDC", "BTC", "ETH", "BNB"} {
		if strings.HasSuffix(input, quote) {
			base := strings.TrimSuffix(input, quote)
			if len(base) > 0 {
				return base + "/" + quote
			}
		}
	}

	// assume USDT pair if no quote currency detected
	return input + "/USDT"
}

// formats a price for display (removes trailing zeros)
func formatPrice(price float64) string {
	if price >= 1 {
		return fmt.Sprintf("%.2f", price)
	}
	// for sub-dollar prices, show more decimals
	return strconv.FormatFloat(price, 'f', -1, 64)
}

// formats a price change with sign
func formatPriceChange(change float64) string {
	if change >= 0 {
		return "+" + formatPrice(change)
	}
	return formatPrice(change)
}

// formats large volumes with K/M/B suffixes
func formatVolume(vol float64) string {
	switch {
	case vol >= 1_000_000_000:
		return fmt.Sprintf("$%.1fB", vol/1_000_000_000)
	case vol >= 1_000_000:
		return fmt.Sprintf("$%.1fM", vol/1_000_000)
	case vol >= 1_000:
		return fmt.Sprintf("$%.1fK", vol/1_000)
	default:
		return fmt.Sprintf("$%.2f", vol)
	}
}

// formats a balance value
func formatBalance(val float64) string {
	if val == 0 {
		return "0"
	}
	return strconv.FormatFloat(val, 'f', -1, 64)
}

func (h *Handler) send(chatID int64, text string) {
	if err := h.bot.SendMessage(chatID, text); err != nil {
		log.Printf("error sending message to chat %d: %v", chatID, err)
	}
}

func (h *Handler) sendWithKeyboard(chatID int64, text string, keyboard *InlineKeyboardMarkup) {
	if err := h.bot.SendMessageWithKeyboard(chatID, text, keyboard); err != nil {
		log.Printf("error sending message with keyboard to chat %d: %v", chatID, err)
	}
}

func (h *Handler) editMessage(chatID int64, messageID int, text string, keyboard *InlineKeyboardMarkup) {
	if err := h.bot.EditMessageText(chatID, messageID, text, keyboard); err != nil {
		log.Printf("error editing message %d in chat %d: %v", messageID, chatID, err)
	}
}

func (h *Handler) answerCallback(queryID string, text string) {
	if err := h.bot.AnswerCallbackQuery(queryID, text); err != nil {
		log.Printf("error answering callback query %s: %v", queryID, err)
	}
}

func (h *Handler) deleteMessage(chatID int64, messageID int) {
	if err := h.bot.DeleteMessage(chatID, messageID); err != nil {
		log.Printf("warning: failed to delete message %d in chat %d: %v", messageID, chatID, err)
	}
}
