// discord interaction handler for slash commands, buttons, and user management
package discord

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

// botClient defines the interface for sending discord messages
type botClient interface {
	SendMessage(channelID string, content string) error
	SendEmbed(channelID string, content string, embeds []Embed, components []Component) error
	RespondInteraction(interactionID, interactionToken string, resp *InteractionResponse) error
	EditInteractionResponse(interactionToken string, content string, embeds []Embed, components []Component) error
}

// exchangeClient defines the interface for fetching market data
type exchangeClient interface {
	GetPrice(ctx context.Context, symbol string) (*exchange.Ticker, error)
	GetOrderBook(ctx context.Context, symbol string, depth int) (*exchange.OrderBook, error)
	GetBalance(ctx context.Context, apiKey, apiSecret string) ([]exchange.Balance, error)
}

// Handler processes incoming discord interactions
type Handler struct {
	bot      botClient
	userSvc  *user.Service
	watchSvc *watchlist.Service
	prefsSvc *preferences.Service
	exchange exchangeClient
	trading  *TradingDeps // optional, set via SetTradingDeps
}

func NewHandler(
	bot botClient,
	userSvc *user.Service,
	watchSvc *watchlist.Service,
	prefsSvc *preferences.Service,
	exch exchangeClient,
) *Handler {
	return &Handler{
		bot:      bot,
		userSvc:  userSvc,
		watchSvc: watchSvc,
		prefsSvc: prefsSvc,
		exchange: exch,
	}
}

// slash command definitions for registration with the discord api
func SlashCommands() []ApplicationCommand {
	return []ApplicationCommand{
		{Name: "start", Description: "register or check in", Type: 1},
		{Name: "setup", Description: "connect binance api keys (ephemeral)", Type: 1, Options: []ApplicationCommandOptionDef{
			{Name: "api_key", Description: "your binance api key", Type: OptionString, Required: true},
			{Name: "api_secret", Description: "your binance api secret", Type: OptionString, Required: true},
		}},
		{Name: "status", Description: "check your account status", Type: 1},
		{Name: "help", Description: "show available commands", Type: 1},
		{Name: "price", Description: "get current price for a symbol", Type: 1, Options: []ApplicationCommandOptionDef{
			{Name: "symbol", Description: "trading symbol (e.g. BTC, ETH/USDT)", Type: OptionString, Required: true},
		}},
		{Name: "balance", Description: "show your account balances", Type: 1},
		{Name: "portfolio", Description: "portfolio overview with value estimate", Type: 1},
		{Name: "orderbook", Description: "show order book for a symbol", Type: 1, Options: []ApplicationCommandOptionDef{
			{Name: "symbol", Description: "trading symbol (e.g. BTC)", Type: OptionString, Required: true},
			{Name: "depth", Description: "number of levels (1-20, default 5)", Type: OptionInteger, Required: false},
		}},
		{Name: "watchlist", Description: "view your watchlist", Type: 1},
		{Name: "watchadd", Description: "add a symbol to your watchlist", Type: 1, Options: []ApplicationCommandOptionDef{
			{Name: "symbol", Description: "symbol to add (e.g. BTCUSDT)", Type: OptionString, Required: true},
		}},
		{Name: "watchremove", Description: "remove a symbol from your watchlist", Type: 1, Options: []ApplicationCommandOptionDef{
			{Name: "symbol", Description: "symbol to remove (e.g. BTCUSDT)", Type: OptionString, Required: true},
		}},
		{Name: "watchreset", Description: "reset watchlist to default top-10", Type: 1},
		{Name: "settings", Description: "view all your preferences", Type: 1},
		{Name: "set", Description: "change a preference", Type: 1, Options: []ApplicationCommandOptionDef{
			{Name: "key", Description: "preference key (e.g. confidence, stoploss)", Type: OptionString, Required: true},
			{Name: "value", Description: "new value", Type: OptionString, Required: true},
		}},
		{Name: "link", Description: "link your discord to an existing telegram account", Type: 1, Options: []ApplicationCommandOptionDef{
			{Name: "telegram_id", Description: "your telegram user id (find it with /start on telegram)", Type: OptionString, Required: true},
		}},
	}
}

// routes incoming interactions
func (h *Handler) HandleInteraction(ctx context.Context, interaction *Interaction) {
	switch interaction.Type {
	case InteractionPing:
		h.bot.RespondInteraction(interaction.ID, interaction.Token, &InteractionResponse{Type: ResponsePong})
	case InteractionCommand:
		h.handleCommand(ctx, interaction)
	case InteractionComponent:
		h.handleComponent(ctx, interaction)
	}
}

func (h *Handler) handleCommand(ctx context.Context, interaction *Interaction) {
	if interaction.Data == nil {
		return
	}

	switch interaction.Data.Name {
	case "start":
		h.handleStart(ctx, interaction)
	case "setup":
		h.handleSetup(ctx, interaction)
	case "status":
		h.handleStatus(ctx, interaction)
	case "help":
		h.handleHelp(interaction)
	case "price":
		h.handlePrice(ctx, interaction)
	case "balance":
		h.handleBalance(ctx, interaction)
	case "portfolio":
		h.handlePortfolio(ctx, interaction)
	case "orderbook":
		h.handleOrderBook(ctx, interaction)
	case "watchlist":
		h.handleWatchlist(ctx, interaction)
	case "watchadd":
		h.handleWatchAdd(ctx, interaction)
	case "watchremove":
		h.handleWatchRemove(ctx, interaction)
	case "watchreset":
		h.handleWatchReset(interaction)
	case "settings":
		h.handleSettings(ctx, interaction)
	case "set":
		h.handleSet(ctx, interaction)
	case "link":
		h.handleLink(ctx, interaction)
	default:
		if !h.handleTradingCommand(ctx, interaction) {
			h.respond(interaction, "unknown command. use /help for available commands.", nil, nil)
		}
	}
}

// button callback routing
func (h *Handler) handleComponent(ctx context.Context, interaction *Interaction) {
	if interaction.Data == nil {
		return
	}

	data := interaction.Data.CustomID
	switch {
	case strings.HasPrefix(data, "price:"):
		symbol := strings.TrimPrefix(data, "price:")
		h.componentPrice(ctx, interaction, symbol)
	case strings.HasPrefix(data, "ob:"):
		h.componentOrderBook(ctx, interaction, data)
	case strings.HasPrefix(data, "wa:"):
		symbol := strings.TrimPrefix(data, "wa:")
		h.componentWatchAdd(ctx, interaction, symbol)
	case data == "refresh_balance":
		h.componentBalance(ctx, interaction)
	case data == "portfolio":
		h.componentPortfolio(ctx, interaction)
	case data == "watchreset_confirm":
		h.componentWatchResetConfirm(ctx, interaction)
	case data == "watchreset_cancel":
		h.updateMessage(interaction, "watchlist reset cancelled.", nil, nil)
	case strings.HasPrefix(data, "wl_price:"):
		symbol := strings.TrimPrefix(data, "wl_price:")
		h.componentPrice(ctx, interaction, symbol)
	default:
		h.handleTradingComponent(ctx, interaction)
	}
}

// --- command handlers ---

func (h *Handler) handleStart(ctx context.Context, interaction *Interaction) {
	discordUser := getInteractionUser(interaction)
	if discordUser == nil {
		h.respond(interaction, "could not identify your account.", nil, nil)
		return
	}

	discordID, err := strconv.ParseInt(discordUser.ID, 10, 64)
	if err != nil {
		h.respond(interaction, "invalid discord user id.", nil, nil)
		return
	}

	result, err := h.userSvc.RegisterDiscord(ctx, discordID, discordUser.Username)
	if err != nil {
		log.Printf("error registering discord user %s: %v", discordUser.ID, err)
		h.respond(interaction, "something went wrong during registration. please try again.", nil, nil)
		return
	}

	if result.IsNewUser {
		h.respond(interaction, fmt.Sprintf(
			"welcome, %s! 🤖\n\n"+
				"your account has been created.\n\n"+
				"to start trading, connect your binance api keys.\n"+
				"use `/setup` to begin.\n\n"+
				"type `/help` for all available commands.",
			discordUser.Username,
		), nil, nil)
	} else {
		activated, hasKeys, _ := h.userSvc.GetStatus(ctx, result.User.ID)
		if activated && hasKeys {
			h.respond(interaction, fmt.Sprintf("welcome back, %s! your account is active and ready.", discordUser.Username), nil, nil)
		} else {
			h.respond(interaction, fmt.Sprintf(
				"welcome back, %s!\n\nyour account exists but hasn't been set up yet.\n"+
					"use `/setup` to connect your binance api keys.",
				discordUser.Username,
			), nil, nil)
		}
	}
}

func (h *Handler) handleSetup(ctx context.Context, interaction *Interaction) {
	discordUser := getInteractionUser(interaction)
	if discordUser == nil {
		h.respondEphemeral(interaction, "could not identify your account.")
		return
	}

	discordID, err := strconv.ParseInt(discordUser.ID, 10, 64)
	if err != nil {
		h.respondEphemeral(interaction, "invalid discord user id.")
		return
	}

	apiKey := getOption(interaction, "api_key")
	apiSecret := getOption(interaction, "api_secret")
	if apiKey == "" || apiSecret == "" {
		h.respondEphemeral(interaction, "both api_key and api_secret are required.")
		return
	}

	// register user if needed
	result, err := h.userSvc.RegisterDiscord(ctx, discordID, discordUser.Username)
	if err != nil {
		h.respondEphemeral(interaction, "something went wrong. please try /start first.")
		return
	}

	// validate and store keys
	setupResult, err := h.userSvc.SetupAPIKeys(ctx, result.User.ID, apiKey, apiSecret)
	if err != nil {
		h.respondEphemeral(interaction, fmt.Sprintf("❌ setup failed: %s", err.Error()))
		return
	}

	permsMsg := "detected permissions:\n"
	if setupResult.Permissions.Spot {
		permsMsg += "• ✅ spot trading\n"
	}
	if setupResult.Permissions.Futures {
		permsMsg += "• ✅ futures trading\n"
	}

	h.respondEphemeral(interaction, fmt.Sprintf(
		"✅ **setup complete!**\n\n"+
			"%s\n"+
			"your api keys have been encrypted and stored securely.\n"+
			"your account is now active.\n\n"+
			"type `/help` to see what you can do next.",
		permsMsg,
	))
}

func (h *Handler) handleStatus(ctx context.Context, interaction *Interaction) {
	result, ok := h.resolveUserResult(ctx, interaction)
	if !ok {
		return
	}

	userID := result.User.ID
	activated, hasKeys, _ := h.userSvc.GetStatus(ctx, userID)

	embed := Embed{Title: "📊 Account Status", Color: ColorBlue}

	if activated && hasKeys {
		tradingMode := "paper"
		if result != nil {
			tradingMode = result.User.TradingMode
		}
		embed.Fields = []EmbedField{
			{Name: "Account", Value: "✅ Active", Inline: true},
			{Name: "API Keys", Value: "✅ Connected", Inline: true},
			{Name: "Trading Mode", Value: tradingMode, Inline: true},
		}
	} else {
		embed.Fields = []EmbedField{
			{Name: "Account", Value: "⏳ Pending Setup", Inline: true},
			{Name: "API Keys", Value: "❌ Not Connected", Inline: true},
		}
		embed.Footer = &EmbedFooter{Text: "use /setup to connect your binance api keys"}
	}

	h.respond(interaction, "", []Embed{embed}, nil)
}

func (h *Handler) handleHelp(interaction *Interaction) {
	embed := Embed{
		Title: "Available Commands",
		Color: ColorBlue,
		Fields: []EmbedField{
			{Name: "Account", Value: "`/start` - register or check in\n`/setup` - connect binance api keys\n`/status` - check your account status"},
			{Name: "Exchange", Value: "`/price` - get current price\n`/balance` - show your balances\n`/portfolio` - portfolio overview\n`/orderbook` - show order book"},
			{Name: "Watchlist", Value: "`/watchlist` - view your watchlist\n`/watchadd` - add a symbol\n`/watchremove` - remove a symbol\n`/watchreset` - reset to default top-10"},
			{Name: "Preferences", Value: "`/settings` - view all preferences\n`/set` - change a preference"},
		},
	}
	h.respond(interaction, "", []Embed{embed}, nil)
}

func (h *Handler) handlePrice(ctx context.Context, interaction *Interaction) {
	symbolInput := getOption(interaction, "symbol")
	if symbolInput == "" {
		h.respond(interaction, "please provide a symbol. example: `/price BTC`", nil, nil)
		return
	}

	symbol := normalizeSymbol(symbolInput)
	ticker, err := h.exchange.GetPrice(ctx, symbol)
	if err != nil {
		h.respond(interaction, fmt.Sprintf("❌ failed to get price: %s", err.Error()), nil, nil)
		return
	}

	embed := buildTickerEmbed(ticker)
	buttons := []Component{
		actionRow(
			button(ButtonPrimary, "🔄 Refresh", "price:"+symbol),
			button(ButtonSecondary, "📊 Order Book", "ob:"+symbol),
		),
		actionRow(
			button(ButtonSuccess, "⭐ Add to Watchlist", "wa:"+symbol),
		),
	}

	h.respond(interaction, "", []Embed{embed}, buttons)
}

func (h *Handler) handleBalance(ctx context.Context, interaction *Interaction) {
	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	apiKey, apiSecret, err := h.userSvc.GetDecryptedCredentials(ctx, userID)
	if err != nil {
		h.respondEphemeral(interaction, "❌ failed to retrieve your api keys. try /setup to reconfigure.")
		return
	}

	balances, err := h.exchange.GetBalance(ctx, apiKey, apiSecret)
	if err != nil {
		h.respondEphemeral(interaction, fmt.Sprintf("❌ failed to get balance: %s", err.Error()))
		return
	}

	if len(balances) == 0 {
		h.respondEphemeral(interaction, "💼 your account has no balances.")
		return
	}

	embed := buildBalanceEmbed(balances)
	buttons := []Component{
		actionRow(
			button(ButtonPrimary, "🔄 Refresh", "refresh_balance"),
			button(ButtonSecondary, "📊 Portfolio", "portfolio"),
		),
	}

	h.respondEphemeralEmbed(interaction, []Embed{embed}, buttons)
}

func (h *Handler) handlePortfolio(ctx context.Context, interaction *Interaction) {
	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	apiKey, apiSecret, err := h.userSvc.GetDecryptedCredentials(ctx, userID)
	if err != nil {
		h.respond(interaction, "❌ failed to retrieve your api keys. try /setup to reconfigure.", nil, nil)
		return
	}

	balances, err := h.exchange.GetBalance(ctx, apiKey, apiSecret)
	if err != nil {
		h.respond(interaction, fmt.Sprintf("❌ failed to get balance: %s", err.Error()), nil, nil)
		return
	}

	if len(balances) == 0 {
		h.respond(interaction, "💼 your portfolio is empty.", nil, nil)
		return
	}

	embed, _ := h.buildPortfolioEmbed(ctx, balances)
	buttons := []Component{
		actionRow(
			button(ButtonPrimary, "🔄 Refresh", "portfolio"),
			button(ButtonSecondary, "💼 Balances", "refresh_balance"),
		),
	}

	h.respond(interaction, "", []Embed{embed}, buttons)
}

func (h *Handler) handleOrderBook(ctx context.Context, interaction *Interaction) {
	symbolInput := getOption(interaction, "symbol")
	if symbolInput == "" {
		h.respond(interaction, "please provide a symbol. example: `/orderbook BTC`", nil, nil)
		return
	}

	symbol := normalizeSymbol(symbolInput)
	depth := 5
	if d := getOptionInt(interaction, "depth"); d > 0 && d <= 20 {
		depth = d
	}

	book, err := h.exchange.GetOrderBook(ctx, symbol, depth)
	if err != nil {
		h.respond(interaction, fmt.Sprintf("❌ failed to get order book: %s", err.Error()), nil, nil)
		return
	}

	embed := buildOrderBookEmbed(book)
	buttons := []Component{
		actionRow(
			button(ButtonPrimary, "🔄 Refresh", fmt.Sprintf("ob:%s:%d", symbol, depth)),
			button(ButtonSecondary, "💰 Price", "price:"+symbol),
		),
	}

	h.respond(interaction, "", []Embed{embed}, buttons)
}

func (h *Handler) handleWatchlist(ctx context.Context, interaction *Interaction) {
	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	items, err := h.watchSvc.List(ctx, userID)
	if err != nil {
		log.Printf("error listing watchlist for user %d: %v", userID, err)
		h.respond(interaction, "failed to load watchlist. please try again.", nil, nil)
		return
	}

	if len(items) == 0 {
		h.respond(interaction, "your watchlist is empty.\n\nuse `/watchadd` to add one, or `/watchreset` for the default top-10.", nil, nil)
		return
	}

	desc := ""
	var buttonRows []Component

	for i, item := range items {
		ticker, err := h.exchange.GetPrice(ctx, item.Symbol)
		if err == nil && ticker != nil {
			changeEmoji := "📈"
			if ticker.ChangePct < 0 {
				changeEmoji = "📉"
			}
			desc += fmt.Sprintf("%d. **%s** — $%s %s %.2f%%\n", i+1, item.Symbol, formatPrice(ticker.Price), changeEmoji, ticker.ChangePct)
		} else {
			desc += fmt.Sprintf("%d. **%s**\n", i+1, item.Symbol)
		}

		// add button rows (3 per row)
		if i%3 == 0 {
			buttonRows = append(buttonRows, Component{Type: ComponentActionRow})
		}
		row := len(buttonRows) - 1
		buttonRows[row].Components = append(buttonRows[row].Components, Component{
			Type:     ComponentButton,
			Style:    ButtonSecondary,
			Label:    item.Symbol,
			CustomID: "wl_price:" + item.Symbol,
		})
	}

	embed := Embed{
		Title:       fmt.Sprintf("📋 Your Watchlist (%d symbols)", len(items)),
		Description: desc,
		Color:       ColorBlue,
		Footer:      &EmbedFooter{Text: "use /watchadd, /watchremove, or /watchreset to manage"},
	}

	h.respond(interaction, "", []Embed{embed}, buttonRows)
}

func (h *Handler) handleWatchAdd(ctx context.Context, interaction *Interaction) {
	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	symbol := getOption(interaction, "symbol")
	if symbol == "" {
		h.respond(interaction, "please provide a symbol. example: `/watchadd BTCUSDT`", nil, nil)
		return
	}

	symbol = strings.TrimSpace(symbol)
	if err := h.watchSvc.Add(ctx, userID, symbol); err != nil {
		h.respond(interaction, fmt.Sprintf("❌ %s", err.Error()), nil, nil)
		return
	}

	h.respond(interaction, "✅ added to watchlist. use `/watchlist` to see your list.", nil, nil)
}

func (h *Handler) handleWatchRemove(ctx context.Context, interaction *Interaction) {
	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	symbol := getOption(interaction, "symbol")
	if symbol == "" {
		h.respond(interaction, "please provide a symbol. example: `/watchremove BTCUSDT`", nil, nil)
		return
	}

	symbol = strings.TrimSpace(symbol)
	if err := h.watchSvc.Remove(ctx, userID, symbol); err != nil {
		h.respond(interaction, fmt.Sprintf("❌ %s", err.Error()), nil, nil)
		return
	}

	h.respond(interaction, "✅ removed from watchlist.", nil, nil)
}

func (h *Handler) handleWatchReset(interaction *Interaction) {
	buttons := []Component{
		actionRow(
			button(ButtonSuccess, "✅ Yes, reset", "watchreset_confirm"),
			button(ButtonDanger, "❌ Cancel", "watchreset_cancel"),
		),
	}

	h.respond(interaction, "⚠️ **are you sure?**\n\nthis will replace your current watchlist with the default top-10 symbols.", nil, buttons)
}

func (h *Handler) handleSettings(ctx context.Context, interaction *Interaction) {
	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	scan, err := h.prefsSvc.GetScanning(ctx, userID)
	if err != nil {
		log.Printf("error getting scanning prefs for user %d: %v", userID, err)
		h.respond(interaction, "failed to load preferences. please try again.", nil, nil)
		return
	}

	notif, err := h.prefsSvc.GetNotification(ctx, userID)
	if err != nil {
		log.Printf("error getting notification prefs for user %d: %v", userID, err)
		h.respond(interaction, "failed to load preferences. please try again.", nil, nil)
		return
	}

	trade, err := h.prefsSvc.GetTrading(ctx, userID)
	if err != nil {
		log.Printf("error getting trading prefs for user %d: %v", userID, err)
		h.respond(interaction, "failed to load preferences. please try again.", nil, nil)
		return
	}

	scanStatus := "disabled"
	if scan.IsScanningEnabled {
		scanStatus = "enabled"
	}

	embed := Embed{
		Title: "⚙️ Your Preferences",
		Color: ColorBlue,
		Fields: []EmbedField{
			{Name: "Scanning Status", Value: scanStatus, Inline: true},
			{Name: "Min Confidence", Value: fmt.Sprintf("%d%%", scan.MinConfidence), Inline: true},
			{Name: "Scan Interval", Value: fmt.Sprintf("%d min", scan.ScanIntervalMins), Inline: true},
			{Name: "Timeframes", Value: strings.Join(scan.EnabledTimeframes, ", "), Inline: true},
			{Name: "Max Daily Notifications", Value: fmt.Sprintf("%d", notif.MaxDailyNotifications), Inline: true},
			{Name: "Timezone", Value: notif.Timezone, Inline: true},
			{Name: "Daily Summary Hour", Value: fmt.Sprintf("%d", notif.DailySummaryHour), Inline: true},
			{Name: "Position Size", Value: fmt.Sprintf("$%.2f (max $%.2f)", trade.DefaultPositionSize, trade.MaxPositionSize), Inline: true},
			{Name: "Stop Loss", Value: fmt.Sprintf("%.1f%%", trade.DefaultStopLossPct), Inline: true},
			{Name: "Take Profit", Value: fmt.Sprintf("%.1f%%", trade.DefaultTakeProfitPct), Inline: true},
			{Name: "Max Leverage", Value: fmt.Sprintf("%dx", trade.MaxLeverage), Inline: true},
			{Name: "Risk Per Trade", Value: fmt.Sprintf("%.1f%%", trade.RiskPerTradePct), Inline: true},
		},
		Footer: &EmbedFooter{Text: "use /set <key> <value> to change"},
	}

	h.respond(interaction, "", []Embed{embed}, nil)
}

func (h *Handler) handleSet(ctx context.Context, interaction *Interaction) {
	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	key := strings.ToLower(getOption(interaction, "key"))
	value := getOption(interaction, "value")
	if key == "" || value == "" {
		h.respond(interaction, "usage: `/set <key> <value>`\n\nkeys: confidence, interval, maxnotifs, timezone, summaryhour, positionsize, stoploss, takeprofit, leverage, risk, scanning", nil, nil)
		return
	}

	var setErr error
	switch key {
	case "confidence":
		v, err := strconv.Atoi(value)
		if err != nil {
			h.respond(interaction, "confidence must be a number (0-100)", nil, nil)
			return
		}
		setErr = h.prefsSvc.SetMinConfidence(ctx, userID, v)

	case "interval":
		v, err := strconv.Atoi(value)
		if err != nil {
			h.respond(interaction, "interval must be a number (1-60 minutes)", nil, nil)
			return
		}
		setErr = h.prefsSvc.SetScanInterval(ctx, userID, v)

	case "maxnotifs":
		v, err := strconv.Atoi(value)
		if err != nil {
			h.respond(interaction, "maxnotifs must be a number (1-100)", nil, nil)
			return
		}
		setErr = h.prefsSvc.SetMaxDailyNotifications(ctx, userID, v)

	case "timezone":
		setErr = h.prefsSvc.SetTimezone(ctx, userID, value)

	case "summaryhour":
		v, err := strconv.Atoi(value)
		if err != nil {
			h.respond(interaction, "summaryhour must be a number (0-23)", nil, nil)
			return
		}
		setErr = h.prefsSvc.SetDailySummaryHour(ctx, userID, v)

	case "positionsize":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			h.respond(interaction, "positionsize must be a number", nil, nil)
			return
		}
		setErr = h.prefsSvc.SetPositionSize(ctx, userID, v, v*3)

	case "stoploss":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			h.respond(interaction, "stoploss must be a number", nil, nil)
			return
		}
		setErr = h.prefsSvc.SetStopLoss(ctx, userID, v)

	case "takeprofit":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			h.respond(interaction, "takeprofit must be a number", nil, nil)
			return
		}
		setErr = h.prefsSvc.SetTakeProfit(ctx, userID, v)

	case "leverage":
		v, err := strconv.Atoi(value)
		if err != nil {
			h.respond(interaction, "leverage must be a number (1-125)", nil, nil)
			return
		}
		setErr = h.prefsSvc.SetMaxLeverage(ctx, userID, v)

	case "risk":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			h.respond(interaction, "risk must be a number", nil, nil)
			return
		}
		setErr = h.prefsSvc.SetRiskPerTrade(ctx, userID, v)

	case "scanning":
		v := strings.ToLower(value)
		if v != "on" && v != "off" {
			h.respond(interaction, "scanning must be 'on' or 'off'", nil, nil)
			return
		}
		setErr = h.prefsSvc.ToggleScanning(ctx, userID, v == "on")

	default:
		h.respond(interaction, fmt.Sprintf("unknown setting: %s\n\nkeys: confidence, interval, maxnotifs, timezone, summaryhour, positionsize, stoploss, takeprofit, leverage, risk, scanning", key), nil, nil)
		return
	}

	if setErr != nil {
		h.respond(interaction, fmt.Sprintf("❌ %s", setErr.Error()), nil, nil)
		return
	}

	h.respond(interaction, fmt.Sprintf("✅ %s updated. use `/settings` to see all preferences.", key), nil, nil)
}

func (h *Handler) handleLink(ctx context.Context, interaction *Interaction) {
	discordUser := getInteractionUser(interaction)
	if discordUser == nil {
		h.respondEphemeral(interaction, "could not identify your account.")
		return
	}

	discordID, err := strconv.ParseInt(discordUser.ID, 10, 64)
	if err != nil {
		h.respondEphemeral(interaction, "invalid discord user id.")
		return
	}

	telegramIDStr := getOption(interaction, "telegram_id")
	if telegramIDStr == "" {
		h.respondEphemeral(interaction, "please provide your telegram id. example: `/link telegram_id:99999`")
		return
	}

	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil {
		h.respondEphemeral(interaction, "invalid telegram id. it should be a number.")
		return
	}

	linked, err := h.userSvc.LinkDiscordToTelegram(ctx, telegramID, discordID)
	if err != nil {
		h.respondEphemeral(interaction, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

	embed := Embed{
		Title:       "🔗 Accounts Linked",
		Description: fmt.Sprintf("your discord account has been linked to telegram user (id: %d).\n\nyour watchlist, preferences, and api keys are now shared across both platforms.", telegramID),
		Color:       ColorGreen,
		Fields: []EmbedField{
			{Name: "User ID", Value: fmt.Sprintf("%d", linked.ID), Inline: true},
			{Name: "Status", Value: "✅ Linked", Inline: true},
		},
	}

	h.respond(interaction, "", []Embed{embed}, nil)
}

// --- component (button) handlers ---

func (h *Handler) componentPrice(ctx context.Context, interaction *Interaction, symbol string) {
	ticker, err := h.exchange.GetPrice(ctx, symbol)
	if err != nil {
		h.updateMessage(interaction, "failed to refresh price.", nil, nil)
		return
	}

	embed := buildTickerEmbed(ticker)
	buttons := []Component{
		actionRow(
			button(ButtonPrimary, "🔄 Refresh", "price:"+symbol),
			button(ButtonSecondary, "📊 Order Book", "ob:"+symbol),
		),
		actionRow(
			button(ButtonSuccess, "⭐ Add to Watchlist", "wa:"+symbol),
		),
	}

	h.updateMessage(interaction, "", []Embed{embed}, buttons)
}

func (h *Handler) componentOrderBook(ctx context.Context, interaction *Interaction, data string) {
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
		h.updateMessage(interaction, "failed to refresh order book.", nil, nil)
		return
	}

	embed := buildOrderBookEmbed(book)
	buttons := []Component{
		actionRow(
			button(ButtonPrimary, "🔄 Refresh", fmt.Sprintf("ob:%s:%d", symbol, depth)),
			button(ButtonSecondary, "💰 Price", "price:"+symbol),
		),
	}

	h.updateMessage(interaction, "", []Embed{embed}, buttons)
}

func (h *Handler) componentWatchAdd(ctx context.Context, interaction *Interaction, symbol string) {
	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	if err := h.watchSvc.Add(ctx, userID, symbol); err != nil {
		h.respondEphemeral(interaction, err.Error())
		return
	}

	h.respondEphemeral(interaction, fmt.Sprintf("✅ %s added to watchlist", symbol))
}

func (h *Handler) componentBalance(ctx context.Context, interaction *Interaction) {
	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	apiKey, apiSecret, err := h.userSvc.GetDecryptedCredentials(ctx, userID)
	if err != nil {
		h.updateMessage(interaction, "❌ failed to get credentials.", nil, nil)
		return
	}

	balances, err := h.exchange.GetBalance(ctx, apiKey, apiSecret)
	if err != nil {
		h.updateMessage(interaction, "❌ failed to refresh balance.", nil, nil)
		return
	}

	if len(balances) == 0 {
		h.updateMessage(interaction, "💼 your account has no balances.", nil, nil)
		return
	}

	embed := buildBalanceEmbed(balances)
	buttons := []Component{
		actionRow(
			button(ButtonPrimary, "🔄 Refresh", "refresh_balance"),
			button(ButtonSecondary, "📊 Portfolio", "portfolio"),
		),
	}

	h.updateMessage(interaction, "", []Embed{embed}, buttons)
}

func (h *Handler) componentPortfolio(ctx context.Context, interaction *Interaction) {
	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	apiKey, apiSecret, err := h.userSvc.GetDecryptedCredentials(ctx, userID)
	if err != nil {
		h.updateMessage(interaction, "❌ failed to get credentials.", nil, nil)
		return
	}

	balances, err := h.exchange.GetBalance(ctx, apiKey, apiSecret)
	if err != nil {
		h.updateMessage(interaction, "❌ failed to refresh portfolio.", nil, nil)
		return
	}

	embed, _ := h.buildPortfolioEmbed(ctx, balances)
	buttons := []Component{
		actionRow(
			button(ButtonPrimary, "🔄 Refresh", "portfolio"),
			button(ButtonSecondary, "💼 Balances", "refresh_balance"),
		),
	}

	h.updateMessage(interaction, "", []Embed{embed}, buttons)
}

func (h *Handler) componentWatchResetConfirm(ctx context.Context, interaction *Interaction) {
	userID, ok := h.resolveUser(ctx, interaction)
	if !ok {
		return
	}

	if err := h.watchSvc.Reset(ctx, userID); err != nil {
		h.updateMessage(interaction, "❌ failed to reset watchlist.", nil, nil)
		return
	}

	h.updateMessage(interaction, "✅ watchlist reset to default top-10 symbols.\n\nuse `/watchlist` to see them.", nil, nil)
}

// --- response helpers ---

func (h *Handler) respond(interaction *Interaction, content string, embeds []Embed, components []Component) {
	resp := &InteractionResponse{
		Type: ResponseMessage,
		Data: &InteractionCallbackData{
			Content:    content,
			Embeds:     embeds,
			Components: components,
		},
	}
	if err := h.bot.RespondInteraction(interaction.ID, interaction.Token, resp); err != nil {
		log.Printf("error responding to interaction %s: %v", interaction.ID, err)
	}
}

func (h *Handler) respondEphemeral(interaction *Interaction, content string) {
	resp := &InteractionResponse{
		Type: ResponseMessage,
		Data: &InteractionCallbackData{
			Content: content,
			Flags:   FlagEphemeral,
		},
	}
	if err := h.bot.RespondInteraction(interaction.ID, interaction.Token, resp); err != nil {
		log.Printf("error responding ephemeral to interaction %s: %v", interaction.ID, err)
	}
}

func (h *Handler) respondEphemeralEmbed(interaction *Interaction, embeds []Embed, components []Component) {
	resp := &InteractionResponse{
		Type: ResponseMessage,
		Data: &InteractionCallbackData{
			Embeds:     embeds,
			Components: components,
			Flags:      FlagEphemeral,
		},
	}
	if err := h.bot.RespondInteraction(interaction.ID, interaction.Token, resp); err != nil {
		log.Printf("error responding ephemeral embed to interaction %s: %v", interaction.ID, err)
	}
}

func (h *Handler) updateMessage(interaction *Interaction, content string, embeds []Embed, components []Component) {
	resp := &InteractionResponse{
		Type: ResponseUpdateMessage,
		Data: &InteractionCallbackData{
			Content:    content,
			Embeds:     embeds,
			Components: components,
		},
	}
	if err := h.bot.RespondInteraction(interaction.ID, interaction.Token, resp); err != nil {
		log.Printf("error updating message for interaction %s: %v", interaction.ID, err)
	}
}

// --- user resolution ---

// resolves a discord interaction user to an internal user id
func (h *Handler) resolveUser(ctx context.Context, interaction *Interaction) (int, bool) {
	result, ok := h.resolveUserResult(ctx, interaction)
	if !ok {
		return 0, false
	}

	activated, hasKeys, _ := h.userSvc.GetStatus(ctx, result.User.ID)
	if !activated || !hasKeys {
		h.respondEphemeral(interaction, "you need to complete setup first. use `/setup` to connect your binance api keys.")
		return 0, false
	}

	return result.User.ID, true
}

func (h *Handler) resolveUserResult(ctx context.Context, interaction *Interaction) (*user.RegisterResult, bool) {
	discordUser := getInteractionUser(interaction)
	if discordUser == nil {
		h.respondEphemeral(interaction, "could not identify your account.")
		return nil, false
	}

	discordID, err := strconv.ParseInt(discordUser.ID, 10, 64)
	if err != nil {
		h.respondEphemeral(interaction, "invalid discord user id.")
		return nil, false
	}

	result, err := h.userSvc.RegisterDiscord(ctx, discordID, discordUser.Username)
	if err != nil {
		h.respondEphemeral(interaction, "something went wrong. try /start first.")
		return nil, false
	}

	return result, true
}

// extracts the discord user from an interaction (guild or dm)
func getInteractionUser(interaction *Interaction) *DiscordUser {
	if interaction.User != nil {
		return interaction.User
	}
	if interaction.Member != nil && interaction.Member.User != nil {
		return interaction.Member.User
	}
	return nil
}

// extracts a string option value from interaction data
func getOption(interaction *Interaction, name string) string {
	if interaction.Data == nil {
		return ""
	}
	for _, opt := range interaction.Data.Options {
		if opt.Name == name {
			switch v := opt.Value.(type) {
			case string:
				return v
			case float64:
				return fmt.Sprintf("%.0f", v)
			}
		}
	}
	return ""
}

// extracts an integer option value from interaction data
func getOptionInt(interaction *Interaction, name string) int {
	if interaction.Data == nil {
		return 0
	}
	for _, opt := range interaction.Data.Options {
		if opt.Name == name {
			switch v := opt.Value.(type) {
			case float64:
				return int(v)
			case string:
				n, _ := strconv.Atoi(v)
				return n
			}
		}
	}
	return 0
}

// --- embed builders ---

func buildTickerEmbed(ticker *exchange.Ticker) Embed {
	color := ColorGreen
	changeEmoji := "📈"
	if ticker.ChangePct < 0 {
		color = ColorRed
		changeEmoji = "📉"
	}

	return Embed{
		Title: fmt.Sprintf("💰 %s", ticker.Symbol),
		Color: color,
		Fields: []EmbedField{
			{Name: "Price", Value: fmt.Sprintf("$%s", formatPrice(ticker.Price)), Inline: true},
			{Name: "24h Change", Value: fmt.Sprintf("%s %s (%.2f%%)", changeEmoji, formatPriceChange(ticker.PriceChange), ticker.ChangePct), Inline: true},
			{Name: "24h Volume", Value: formatVolume(ticker.QuoteVolume), Inline: true},
		},
	}
}

func buildBalanceEmbed(balances []exchange.Balance) Embed {
	desc := ""
	for _, b := range balances {
		line := fmt.Sprintf("• **%s**: %s", b.Asset, formatBalance(b.Free))
		if b.Locked > 0 {
			line += fmt.Sprintf(" (locked: %s)", formatBalance(b.Locked))
		}
		desc += line + "\n"
	}

	return Embed{
		Title:       "💼 Your Balances",
		Description: desc,
		Color:       ColorBlue,
	}
}

func buildOrderBookEmbed(book *exchange.OrderBook) Embed {
	asks := ""
	for i := len(book.Asks) - 1; i >= 0; i-- {
		a := book.Asks[i]
		asks += fmt.Sprintf("$%s × %s\n", formatPrice(a.Price), formatBalance(a.Quantity))
	}

	bids := ""
	for _, b := range book.Bids {
		bids += fmt.Sprintf("$%s × %s\n", formatPrice(b.Price), formatBalance(b.Quantity))
	}

	return Embed{
		Title: fmt.Sprintf("📊 Order Book — %s", book.Symbol),
		Color: ColorBlue,
		Fields: []EmbedField{
			{Name: "Asks (Sell)", Value: asks},
			{Name: "Bids (Buy)", Value: bids},
		},
	}
}

func (h *Handler) buildPortfolioEmbed(ctx context.Context, balances []exchange.Balance) (Embed, float64) {
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

		if isStablecoin(b.Asset) {
			line.usd = total
			line.priced = true
		} else {
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

	var fields []EmbedField
	for _, l := range lines {
		value := formatBalance(l.amount)
		if l.priced {
			value += fmt.Sprintf(" ≈ $%s", formatPrice(l.usd))
		}
		fields = append(fields, EmbedField{
			Name:   l.asset,
			Value:  value,
			Inline: true,
		})
	}

	return Embed{
		Title:  "📊 Portfolio Overview",
		Color:  ColorGold,
		Fields: fields,
		Footer: &EmbedFooter{Text: fmt.Sprintf("Estimated total: $%s", formatPrice(totalUSD))},
	}, totalUSD
}

// --- formatting helpers ---

func normalizeSymbol(input string) string {
	input = strings.ToUpper(strings.TrimSpace(input))
	if strings.Contains(input, "/") {
		return input
	}
	for _, quote := range []string{"USDT", "BUSD", "USDC", "BTC", "ETH", "BNB"} {
		if strings.HasSuffix(input, quote) {
			base := strings.TrimSuffix(input, quote)
			if len(base) > 0 {
				return base + "/" + quote
			}
		}
	}
	return input + "/USDT"
}

func formatPrice(price float64) string {
	if price >= 1 {
		return fmt.Sprintf("%.2f", price)
	}
	return strconv.FormatFloat(price, 'f', -1, 64)
}

func formatPriceChange(change float64) string {
	if change >= 0 {
		return "+" + formatPrice(change)
	}
	return formatPrice(change)
}

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

func formatBalance(val float64) string {
	if val == 0 {
		return "0"
	}
	return strconv.FormatFloat(val, 'f', -1, 64)
}

func isStablecoin(asset string) bool {
	switch asset {
	case "USDT", "USDC", "BUSD", "DAI", "TUSD", "USDP", "FDUSD":
		return true
	}
	return false
}

// component builder helpers

func actionRow(buttons ...Component) Component {
	return Component{
		Type:       ComponentActionRow,
		Components: buttons,
	}
}

func button(style int, label, customID string) Component {
	return Component{
		Type:     ComponentButton,
		Style:    style,
		Label:    label,
		CustomID: customID,
	}
}
