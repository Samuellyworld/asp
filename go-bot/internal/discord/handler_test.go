package discord

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/preferences"
	"github.com/trading-bot/go-bot/internal/user"
	"github.com/trading-bot/go-bot/internal/watchlist"
)

// --- mock bot ---

type interactionResp struct {
	interactionID string
	token         string
	resp          *InteractionResponse
}

type mockBot struct {
	messages   []string
	embeds     []Embed
	responses  []interactionResp
	sendErr    error
	respondErr error
}

func (m *mockBot) SendMessage(_ string, content string) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.messages = append(m.messages, content)
	return nil
}

func (m *mockBot) SendEmbed(_ string, content string, embeds []Embed, _ []Component) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.messages = append(m.messages, content)
	m.embeds = append(m.embeds, embeds...)
	return nil
}

func (m *mockBot) RespondInteraction(interactionID, token string, resp *InteractionResponse) error {
	if m.respondErr != nil {
		return m.respondErr
	}
	m.responses = append(m.responses, interactionResp{interactionID, token, resp})
	if resp.Data != nil {
		if resp.Data.Content != "" {
			m.messages = append(m.messages, resp.Data.Content)
		}
		m.embeds = append(m.embeds, resp.Data.Embeds...)
	}
	return nil
}

func (m *mockBot) EditInteractionResponse(_ string, content string, embeds []Embed, _ []Component) error {
	m.messages = append(m.messages, content)
	m.embeds = append(m.embeds, embeds...)
	return nil
}

func (m *mockBot) lastMessage() string {
	if len(m.messages) == 0 {
		return ""
	}
	return m.messages[len(m.messages)-1]
}

func (m *mockBot) lastResponse() *InteractionResponse {
	if len(m.responses) == 0 {
		return nil
	}
	return m.responses[len(m.responses)-1].resp
}

// --- mock user repo ---

type mockUserRepo struct {
	users          map[int64]*user.User
	discordUsers   map[int64]*user.User
	credentials    map[int]*user.Credentials
	nextID         int
	findErr        error
	findDiscordErr error
	createErr      error
	createDiscErr  error
	prefsErr       error
	activeErr      error
	activateErr    error
	saveCredErr    error
	hasCredsErr    error
	hasCredsVal    bool
	getCredErr     error
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		users:        make(map[int64]*user.User),
		discordUsers: make(map[int64]*user.User),
		credentials:  make(map[int]*user.Credentials),
		nextID:       1,
	}
}

func (m *mockUserRepo) FindByTelegramID(_ context.Context, telegramID int64) (*user.User, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	return m.users[telegramID], nil
}

func (m *mockUserRepo) FindByDiscordID(_ context.Context, discordID int64) (*user.User, error) {
	if m.findDiscordErr != nil {
		return nil, m.findDiscordErr
	}
	return m.discordUsers[discordID], nil
}

func (m *mockUserRepo) Create(_ context.Context, telegramID int64, username string) (*user.User, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	u := &user.User{
		ID:          m.nextID,
		UUID:        fmt.Sprintf("uuid-%d", m.nextID),
		TelegramID:  &telegramID,
		Username:    &username,
		TradingMode: "paper",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.nextID++
	m.users[telegramID] = u
	return u, nil
}

func (m *mockUserRepo) CreateFromDiscord(_ context.Context, discordID int64, username string) (*user.User, error) {
	if m.createDiscErr != nil {
		return nil, m.createDiscErr
	}
	u := &user.User{
		ID:          m.nextID,
		UUID:        fmt.Sprintf("uuid-%d", m.nextID),
		DiscordID:   &discordID,
		Username:    &username,
		TradingMode: "paper",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.nextID++
	m.discordUsers[discordID] = u
	return u, nil
}

func (m *mockUserRepo) CreateDefaultPreferences(_ context.Context, _ int) error {
	return m.prefsErr
}

func (m *mockUserRepo) UpdateLastActive(_ context.Context, _ int, _ string) error {
	return m.activeErr
}

func (m *mockUserRepo) LinkDiscordToTelegram(_ context.Context, telegramID, discordID int64) (*user.User, error) {
	// look in users map (telegram users)
	for _, u := range m.users {
		if u.TelegramID != nil && *u.TelegramID == telegramID {
			u.DiscordID = &discordID
			m.discordUsers[discordID] = u
			return u, nil
		}
	}
	return nil, nil
}

func (m *mockUserRepo) Activate(_ context.Context, userID int) error {
	if m.activateErr != nil {
		return m.activateErr
	}
	for _, u := range m.discordUsers {
		if u.ID == userID {
			u.IsActivated = true
		}
	}
	for _, u := range m.users {
		if u.ID == userID {
			u.IsActivated = true
		}
	}
	return nil
}

func (m *mockUserRepo) SaveCredentials(_ context.Context, cred *user.Credentials) (*user.Credentials, error) {
	if m.saveCredErr != nil {
		return nil, m.saveCredErr
	}
	cred.ID = m.nextID
	m.nextID++
	cred.CreatedAt = time.Now()
	m.credentials[cred.UserID] = cred
	return cred, nil
}

func (m *mockUserRepo) HasValidCredentials(_ context.Context, userID int) (bool, error) {
	if m.hasCredsErr != nil {
		return false, m.hasCredsErr
	}
	if m.hasCredsVal {
		return true, nil
	}
	_, ok := m.credentials[userID]
	return ok, nil
}

func (m *mockUserRepo) GetCredentials(_ context.Context, userID int, _ string) (*user.Credentials, error) {
	if m.getCredErr != nil {
		return nil, m.getCredErr
	}
	return m.credentials[userID], nil
}

func (m *mockUserRepo) seedDiscord(discordID int64, username string) *user.User {
	u := &user.User{
		ID:          m.nextID,
		UUID:        fmt.Sprintf("uuid-%d", m.nextID),
		DiscordID:   &discordID,
		Username:    &username,
		TradingMode: "paper",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.nextID++
	m.discordUsers[discordID] = u
	return u
}

func (m *mockUserRepo) seedActivatedDiscord(discordID int64, username string) *user.User {
	u := m.seedDiscord(discordID, username)
	u.IsActivated = true
	m.hasCredsVal = true
	return u
}

func (m *mockUserRepo) seedTelegram(telegramID int64, username string) *user.User {
	u := &user.User{
		ID:          m.nextID,
		UUID:        fmt.Sprintf("uuid-%d", m.nextID),
		TelegramID:  &telegramID,
		Username:    &username,
		TradingMode: "paper",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.nextID++
	m.users[telegramID] = u
	return u
}

func (m *mockUserRepo) seedActivatedTelegram(telegramID int64, username string) *user.User {
	u := m.seedTelegram(telegramID, username)
	u.IsActivated = true
	m.hasCredsVal = true
	return u
}

// --- mock validator ---

type mockValidator struct {
	perms *binance.APIPermissions
	err   error
}

func (m *mockValidator) ValidateKeys(_ context.Context, _, _ string) (*binance.APIPermissions, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.perms, nil
}

// --- mock encryptor ---

type mockEncryptor struct {
	err    error
	decErr error
}

func (m *mockEncryptor) Encrypt(plaintext []byte, _ []byte) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	enc := make([]byte, len(plaintext))
	for i, b := range plaintext {
		enc[len(plaintext)-1-i] = b
	}
	return enc, nil
}

func (m *mockEncryptor) Decrypt(ciphertext []byte, _ []byte) ([]byte, error) {
	if m.decErr != nil {
		return nil, m.decErr
	}
	dec := make([]byte, len(ciphertext))
	for i, b := range ciphertext {
		dec[len(ciphertext)-1-i] = b
	}
	return dec, nil
}

// --- mock exchange ---

type mockExchange struct {
	ticker    *exchange.Ticker
	orderBook *exchange.OrderBook
	balances  []exchange.Balance
	priceErr  error
	bookErr   error
	balErr    error
}

func (m *mockExchange) GetPrice(_ context.Context, symbol string) (*exchange.Ticker, error) {
	if m.priceErr != nil {
		return nil, m.priceErr
	}
	if m.ticker != nil {
		return m.ticker, nil
	}
	return &exchange.Ticker{
		Symbol:      symbol,
		Price:       42000.0,
		PriceChange: -850.0,
		ChangePct:   -1.98,
		QuoteVolume: 1_200_000_000,
	}, nil
}

func (m *mockExchange) GetOrderBook(_ context.Context, symbol string, depth int) (*exchange.OrderBook, error) {
	if m.bookErr != nil {
		return nil, m.bookErr
	}
	if m.orderBook != nil {
		return m.orderBook, nil
	}
	return &exchange.OrderBook{
		Symbol: symbol,
		Asks: []exchange.OrderBookEntry{
			{Price: 42100, Quantity: 1.5},
			{Price: 42200, Quantity: 2.0},
		},
		Bids: []exchange.OrderBookEntry{
			{Price: 41900, Quantity: 1.8},
			{Price: 41800, Quantity: 3.0},
		},
	}, nil
}

func (m *mockExchange) GetBalance(_ context.Context, _, _ string) ([]exchange.Balance, error) {
	if m.balErr != nil {
		return nil, m.balErr
	}
	if m.balances != nil {
		return m.balances, nil
	}
	return []exchange.Balance{
		{Asset: "USDT", Free: 1000.0, Locked: 0},
		{Asset: "BTC", Free: 0.05, Locked: 0},
	}, nil
}

// --- mock watchlist repo ---

type mockWatchlistRepo struct {
	items     []watchlist.Item
	listErr   error
	addErr    error
	removeErr error
	resetErr  error
}

func (m *mockWatchlistRepo) GetByUserID(_ context.Context, _ int) ([]watchlist.Item, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.items, nil
}

func (m *mockWatchlistRepo) Add(_ context.Context, userID int, symbol string) error {
	if m.addErr != nil {
		return m.addErr
	}
	m.items = append(m.items, watchlist.Item{UserID: userID, Symbol: symbol})
	return nil
}

func (m *mockWatchlistRepo) Remove(_ context.Context, _ int, _ string) error {
	return m.removeErr
}

func (m *mockWatchlistRepo) Count(_ context.Context, _ int) (int, error) {
	return len(m.items), nil
}

func (m *mockWatchlistRepo) Exists(_ context.Context, _ int, _ string) (bool, error) {
	return false, nil
}

func (m *mockWatchlistRepo) Reset(_ context.Context, _ int) error {
	return m.resetErr
}

// --- mock prefs repo ---

type mockPrefsRepo struct {
	scanPrefs  *preferences.Scanning
	notifPrefs *preferences.Notification
	tradePrefs *preferences.Trading
	getErr     error
	setErr     error
}

func newMockPrefsRepo() *mockPrefsRepo {
	return &mockPrefsRepo{
		scanPrefs: &preferences.Scanning{
			IsScanningEnabled: true,
			MinConfidence:     65,
			ScanIntervalMins:  5,
			EnabledTimeframes: []string{"5m", "15m", "1h"},
		},
		notifPrefs: &preferences.Notification{
			MaxDailyNotifications: 50,
			Timezone:              "UTC",
			DailySummaryHour:      9,
		},
		tradePrefs: &preferences.Trading{
			DefaultPositionSize:  100.0,
			MaxPositionSize:      500.0,
			DefaultStopLossPct:   2.0,
			DefaultTakeProfitPct: 4.0,
			MaxLeverage:          5,
			RiskPerTradePct:      1.0,
		},
	}
}

func (m *mockPrefsRepo) GetScanning(_ context.Context, _ int) (*preferences.Scanning, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.scanPrefs, nil
}

func (m *mockPrefsRepo) GetNotification(_ context.Context, _ int) (*preferences.Notification, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.notifPrefs, nil
}

func (m *mockPrefsRepo) GetTrading(_ context.Context, _ int) (*preferences.Trading, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.tradePrefs, nil
}

func (m *mockPrefsRepo) UpdateScanning(_ context.Context, _ *preferences.Scanning) error {
	return m.setErr
}

func (m *mockPrefsRepo) UpdateNotification(_ context.Context, _ *preferences.Notification) error {
	return m.setErr
}

func (m *mockPrefsRepo) UpdateTrading(_ context.Context, _ *preferences.Trading) error {
	return m.setErr
}

// --- test helpers ---

func newTestHandler(repo *mockUserRepo, exch *mockExchange, watchRepo *mockWatchlistRepo, prefsRepo *mockPrefsRepo) (*Handler, *mockBot) {
	bot := &mockBot{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	enc := &mockEncryptor{}
	userSvc := user.NewService(repo, enc, nil, validator, false)
	watchSvc := watchlist.NewService(watchRepo)
	prefsSvc := preferences.NewService(prefsRepo)
	handler := NewHandler(bot, userSvc, watchSvc, prefsSvc, exch)
	return handler, bot
}

func makeInteraction(discordID, name string, options ...ApplicationCommandOption) *Interaction {
	return &Interaction{
		ID:        "int-123",
		Type:      InteractionCommand,
		ChannelID: "ch-1",
		User:      &DiscordUser{ID: discordID, Username: "testuser"},
		Token:     "test-token",
		Data:      &InteractionData{Name: name, Options: options},
	}
}

func makeGuildInteraction(discordID, name string, options ...ApplicationCommandOption) *Interaction {
	return &Interaction{
		ID:        "int-123",
		Type:      InteractionCommand,
		ChannelID: "ch-1",
		Member:    &GuildMember{User: &DiscordUser{ID: discordID, Username: "guilduser"}},
		Token:     "test-token",
		Data:      &InteractionData{Name: name, Options: options},
	}
}

func makeComponentInteraction(discordID, customID string) *Interaction {
	return &Interaction{
		ID:        "int-456",
		Type:      InteractionComponent,
		ChannelID: "ch-1",
		User:      &DiscordUser{ID: discordID, Username: "testuser"},
		Token:     "test-token",
		Data:      &InteractionData{CustomID: customID},
	}
}

func strOpt(name, value string) ApplicationCommandOption {
	return ApplicationCommandOption{Name: name, Type: OptionString, Value: value}
}

func intOpt(name string, value float64) ApplicationCommandOption {
	return ApplicationCommandOption{Name: name, Type: OptionInteger, Value: value}
}

// --- tests ---

func TestHandleInteraction_Ping(t *testing.T) {
	handler, bot := newTestHandler(newMockUserRepo(), &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	interaction := &Interaction{
		ID:    "ping-1",
		Type:  InteractionPing,
		Token: "tok",
	}
	handler.HandleInteraction(context.Background(), interaction)

	if len(bot.responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(bot.responses))
	}
	if bot.responses[0].resp.Type != ResponsePong {
		t.Errorf("expected pong response, got type %d", bot.responses[0].resp.Type)
	}
}

func TestHandleStart_NewUser(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "start"))

	if len(bot.messages) == 0 {
		t.Fatal("expected a response message")
	}
	if !strings.Contains(bot.lastMessage(), "welcome") {
		t.Errorf("expected welcome message, got: %s", bot.lastMessage())
	}
	if !strings.Contains(bot.lastMessage(), "created") {
		t.Errorf("expected 'created' in welcome, got: %s", bot.lastMessage())
	}
}

func TestHandleStart_ExistingUser_Active(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "start"))

	if !strings.Contains(bot.lastMessage(), "welcome back") {
		t.Errorf("expected 'welcome back', got: %s", bot.lastMessage())
	}
	if !strings.Contains(bot.lastMessage(), "active") {
		t.Errorf("expected 'active' in message, got: %s", bot.lastMessage())
	}
}

func TestHandleStart_ExistingUser_Inactive(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedDiscord(12345, "testuser")
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "start"))

	if !strings.Contains(bot.lastMessage(), "set up") {
		t.Errorf("expected setup prompt, got: %s", bot.lastMessage())
	}
}

func TestHandleStart_GuildUser(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeGuildInteraction("12345", "start"))

	if !strings.Contains(bot.lastMessage(), "welcome") {
		t.Errorf("expected welcome message for guild user, got: %s", bot.lastMessage())
	}
}

func TestHandleSetup_Success(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	interaction := makeInteraction("12345", "setup",
		strOpt("api_key", "test-key"),
		strOpt("api_secret", "test-secret"),
	)
	handler.HandleInteraction(context.Background(), interaction)

	if !strings.Contains(bot.lastMessage(), "setup complete") {
		t.Errorf("expected setup complete, got: %s", bot.lastMessage())
	}

	// should be ephemeral
	resp := bot.lastResponse()
	if resp.Data.Flags != FlagEphemeral {
		t.Error("setup response should be ephemeral")
	}
}

func TestHandleSetup_MissingKey(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	interaction := makeInteraction("12345", "setup",
		strOpt("api_key", "test-key"),
	)
	handler.HandleInteraction(context.Background(), interaction)

	if !strings.Contains(bot.lastMessage(), "required") {
		t.Errorf("expected required message, got: %s", bot.lastMessage())
	}
}

func TestHandleSetup_Ephemeral(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	interaction := makeInteraction("12345", "setup",
		strOpt("api_key", "key"),
		strOpt("api_secret", "secret"),
	)
	handler.HandleInteraction(context.Background(), interaction)

	resp := bot.lastResponse()
	if resp == nil || resp.Data == nil {
		t.Fatal("expected response data")
	}
	if resp.Data.Flags != FlagEphemeral {
		t.Error("setup response should be ephemeral")
	}
}

func TestHandleStatus_Active(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "status"))

	if len(bot.embeds) == 0 {
		t.Fatal("expected embed in status response")
	}

	found := false
	for _, f := range bot.embeds[0].Fields {
		if f.Name == "Account" && strings.Contains(f.Value, "Active") {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Active' in status embed fields")
	}
}

func TestHandleStatus_Inactive(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedDiscord(12345, "testuser")
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "status"))

	if len(bot.embeds) == 0 {
		t.Fatal("expected embed in status response")
	}

	found := false
	for _, f := range bot.embeds[0].Fields {
		if f.Name == "Account" && strings.Contains(f.Value, "Pending") {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Pending' in status embed fields")
	}
}

func TestHandleHelp(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "help"))

	if len(bot.embeds) == 0 {
		t.Fatal("expected embed in help response")
	}
	if bot.embeds[0].Title != "Available Commands" {
		t.Errorf("expected 'Available Commands' title, got: %s", bot.embeds[0].Title)
	}
	if len(bot.embeds[0].Fields) < 4 {
		t.Errorf("expected at least 4 fields in help embed, got %d", len(bot.embeds[0].Fields))
	}
}

func TestHandlePrice_Success(t *testing.T) {
	repo := newMockUserRepo()
	exch := &mockExchange{}
	handler, bot := newTestHandler(repo, exch, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "price", strOpt("symbol", "BTC")))

	if len(bot.embeds) == 0 {
		t.Fatal("expected price embed")
	}
	if !strings.Contains(bot.embeds[0].Title, "BTC") {
		t.Errorf("expected BTC in title, got: %s", bot.embeds[0].Title)
	}

	// should have buttons
	resp := bot.lastResponse()
	if resp.Data == nil || len(resp.Data.Components) == 0 {
		t.Error("expected buttons in price response")
	}
}

func TestHandlePrice_NoSymbol(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "price"))

	if !strings.Contains(bot.lastMessage(), "provide a symbol") {
		t.Errorf("expected usage message, got: %s", bot.lastMessage())
	}
}

func TestHandlePrice_ExchangeError(t *testing.T) {
	repo := newMockUserRepo()
	exch := &mockExchange{priceErr: fmt.Errorf("api error")}
	handler, bot := newTestHandler(repo, exch, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "price", strOpt("symbol", "BTC")))

	if !strings.Contains(bot.lastMessage(), "failed to get price") {
		t.Errorf("expected error message, got: %s", bot.lastMessage())
	}
}

func TestHandlePrice_SymbolNormalization(t *testing.T) {
	repo := newMockUserRepo()
	exch := &mockExchange{}
	handler, bot := newTestHandler(repo, exch, &mockWatchlistRepo{}, newMockPrefsRepo())

	// bare symbol should normalize to /USDT
	handler.HandleInteraction(context.Background(), makeInteraction("12345", "price", strOpt("symbol", "btc")))

	if len(bot.embeds) == 0 {
		t.Fatal("expected price embed")
	}
	if !strings.Contains(bot.embeds[0].Title, "BTC/USDT") {
		t.Errorf("expected BTC/USDT in title, got: %s", bot.embeds[0].Title)
	}
}

func TestHandleBalance_Success(t *testing.T) {
	repo := newMockUserRepo()
	u := repo.seedActivatedDiscord(12345, "testuser")

	enc := &mockEncryptor{}
	encKey, _ := enc.Encrypt([]byte("key"), []byte("salt"))
	encSec, _ := enc.Encrypt([]byte("secret"), []byte("salt"))
	repo.credentials[u.ID] = &user.Credentials{
		ID: 1, UserID: u.ID, Exchange: "binance",
		APIKeyEncrypted: encKey, APISecretEncrypted: encSec,
		Salt: []byte("salt"), IsValid: true,
	}

	exch := &mockExchange{}
	handler, bot := newTestHandler(repo, exch, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "balance"))

	if len(bot.embeds) == 0 {
		t.Fatal("expected balance embed")
	}
	if !strings.Contains(bot.embeds[0].Title, "Balances") {
		t.Errorf("expected 'Balances' in title, got: %s", bot.embeds[0].Title)
	}

	// should be ephemeral
	resp := bot.lastResponse()
	if resp.Data.Flags != FlagEphemeral {
		t.Error("balance response should be ephemeral")
	}
}

func TestHandleBalance_NotSetup(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedDiscord(12345, "testuser")
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "balance"))

	if !strings.Contains(bot.lastMessage(), "setup") {
		t.Errorf("expected setup prompt, got: %s", bot.lastMessage())
	}
}

func TestHandleBalance_EmptyBalance(t *testing.T) {
	repo := newMockUserRepo()
	u := repo.seedActivatedDiscord(12345, "testuser")
	enc := &mockEncryptor{}
	encKey, _ := enc.Encrypt([]byte("key"), []byte("salt"))
	encSec, _ := enc.Encrypt([]byte("secret"), []byte("salt"))
	repo.credentials[u.ID] = &user.Credentials{
		ID: 1, UserID: u.ID, Exchange: "binance",
		APIKeyEncrypted: encKey, APISecretEncrypted: encSec,
		Salt: []byte("salt"), IsValid: true,
	}

	exch := &mockExchange{balances: []exchange.Balance{}}
	handler, bot := newTestHandler(repo, exch, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "balance"))

	if !strings.Contains(bot.lastMessage(), "no balances") {
		t.Errorf("expected no balances message, got: %s", bot.lastMessage())
	}
}

func TestHandlePortfolio_Success(t *testing.T) {
	repo := newMockUserRepo()
	u := repo.seedActivatedDiscord(12345, "testuser")
	enc := &mockEncryptor{}
	encKey, _ := enc.Encrypt([]byte("key"), []byte("salt"))
	encSec, _ := enc.Encrypt([]byte("secret"), []byte("salt"))
	repo.credentials[u.ID] = &user.Credentials{
		ID: 1, UserID: u.ID, Exchange: "binance",
		APIKeyEncrypted: encKey, APISecretEncrypted: encSec,
		Salt: []byte("salt"), IsValid: true,
	}

	exch := &mockExchange{
		balances: []exchange.Balance{
			{Asset: "USDT", Free: 1000},
			{Asset: "BTC", Free: 0.05},
		},
	}
	handler, bot := newTestHandler(repo, exch, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "portfolio"))

	if len(bot.embeds) == 0 {
		t.Fatal("expected portfolio embed")
	}
	if !strings.Contains(bot.embeds[0].Title, "Portfolio") {
		t.Errorf("expected 'Portfolio' in title, got: %s", bot.embeds[0].Title)
	}
	if bot.embeds[0].Footer == nil {
		t.Error("expected footer with total estimate")
	}
}

func TestHandleOrderBook_Success(t *testing.T) {
	repo := newMockUserRepo()
	exch := &mockExchange{}
	handler, bot := newTestHandler(repo, exch, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "orderbook",
		strOpt("symbol", "BTC"),
		intOpt("depth", 5),
	))

	if len(bot.embeds) == 0 {
		t.Fatal("expected orderbook embed")
	}
	if !strings.Contains(bot.embeds[0].Title, "Order Book") {
		t.Errorf("expected 'Order Book' in title, got: %s", bot.embeds[0].Title)
	}
}

func TestHandleOrderBook_NoSymbol(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "orderbook"))

	if !strings.Contains(bot.lastMessage(), "provide a symbol") {
		t.Errorf("expected usage message, got: %s", bot.lastMessage())
	}
}

func TestHandleWatchlist_WithItems(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")

	watchRepo := &mockWatchlistRepo{
		items: []watchlist.Item{
			{UserID: 1, Symbol: "BTCUSDT", Priority: 1},
			{UserID: 1, Symbol: "ETHUSDT", Priority: 2},
		},
	}
	handler, bot := newTestHandler(repo, &mockExchange{}, watchRepo, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "watchlist"))

	if len(bot.embeds) == 0 {
		t.Fatal("expected watchlist embed")
	}
	if !strings.Contains(bot.embeds[0].Title, "Watchlist") {
		t.Errorf("expected 'Watchlist' in title, got: %s", bot.embeds[0].Title)
	}
	if !strings.Contains(bot.embeds[0].Description, "BTCUSDT") {
		t.Error("expected BTCUSDT in watchlist")
	}
}

func TestHandleWatchlist_Empty(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")

	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "watchlist"))

	if !strings.Contains(bot.lastMessage(), "empty") {
		t.Errorf("expected empty watchlist message, got: %s", bot.lastMessage())
	}
}

func TestHandleWatchAdd_Success(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")

	watchRepo := &mockWatchlistRepo{}
	handler, bot := newTestHandler(repo, &mockExchange{}, watchRepo, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "watchadd", strOpt("symbol", "BTCUSDT")))

	if !strings.Contains(bot.lastMessage(), "added") {
		t.Errorf("expected 'added' message, got: %s", bot.lastMessage())
	}
}

func TestHandleWatchAdd_NoSymbol(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")

	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "watchadd"))

	if !strings.Contains(bot.lastMessage(), "provide a symbol") {
		t.Errorf("expected usage message, got: %s", bot.lastMessage())
	}
}

func TestHandleWatchRemove_Success(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")

	watchRepo := &mockWatchlistRepo{}
	handler, bot := newTestHandler(repo, &mockExchange{}, watchRepo, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "watchremove", strOpt("symbol", "BTCUSDT")))

	if !strings.Contains(bot.lastMessage(), "removed") {
		t.Errorf("expected 'removed' message, got: %s", bot.lastMessage())
	}
}

func TestHandleWatchReset_Confirmation(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "watchreset"))

	if !strings.Contains(bot.lastMessage(), "are you sure") {
		t.Errorf("expected confirmation prompt, got: %s", bot.lastMessage())
	}

	// should have confirm/cancel buttons
	resp := bot.lastResponse()
	if resp.Data == nil || len(resp.Data.Components) == 0 {
		t.Error("expected buttons in watchreset response")
	}
}

func TestHandleSettings_Success(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")

	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "settings"))

	if len(bot.embeds) == 0 {
		t.Fatal("expected settings embed")
	}
	if !strings.Contains(bot.embeds[0].Title, "Preferences") {
		t.Errorf("expected 'Preferences' in title, got: %s", bot.embeds[0].Title)
	}
	if len(bot.embeds[0].Fields) < 10 {
		t.Errorf("expected at least 10 settings fields, got %d", len(bot.embeds[0].Fields))
	}
}

func TestHandleSet_Confidence(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")

	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "set",
		strOpt("key", "confidence"),
		strOpt("value", "75"),
	))

	if !strings.Contains(bot.lastMessage(), "updated") {
		t.Errorf("expected 'updated' message, got: %s", bot.lastMessage())
	}
}

func TestHandleSet_InvalidKey(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")

	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "set",
		strOpt("key", "nonexistent"),
		strOpt("value", "123"),
	))

	if !strings.Contains(bot.lastMessage(), "unknown setting") {
		t.Errorf("expected 'unknown setting' message, got: %s", bot.lastMessage())
	}
}

func TestHandleSet_Scanning(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")

	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "set",
		strOpt("key", "scanning"),
		strOpt("value", "off"),
	))

	if !strings.Contains(bot.lastMessage(), "updated") {
		t.Errorf("expected 'updated' message, got: %s", bot.lastMessage())
	}
}

func TestHandleSet_ScanningInvalid(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")

	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "set",
		strOpt("key", "scanning"),
		strOpt("value", "maybe"),
	))

	if !strings.Contains(bot.lastMessage(), "'on' or 'off'") {
		t.Errorf("expected on/off message, got: %s", bot.lastMessage())
	}
}

func TestHandleSet_AllKeys(t *testing.T) {
	keys := []struct {
		key   string
		value string
	}{
		{"confidence", "70"},
		{"interval", "10"},
		{"maxnotifs", "25"},
		{"timezone", "US/Eastern"},
		{"summaryhour", "8"},
		{"positionsize", "200"},
		{"stoploss", "3.5"},
		{"takeprofit", "7.0"},
		{"leverage", "10"},
		{"risk", "2.0"},
		{"scanning", "on"},
	}

	for _, tc := range keys {
		t.Run(tc.key, func(t *testing.T) {
			repo := newMockUserRepo()
			repo.seedActivatedDiscord(12345, "testuser")
			handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

			handler.HandleInteraction(context.Background(), makeInteraction("12345", "set",
				strOpt("key", tc.key),
				strOpt("value", tc.value),
			))

			if !strings.Contains(bot.lastMessage(), "updated") {
				t.Errorf("key=%s: expected 'updated', got: %s", tc.key, bot.lastMessage())
			}
		})
	}
}

// --- component (button) tests ---

func TestComponentPrice_Refresh(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeComponentInteraction("12345", "price:BTC/USDT"))

	if len(bot.embeds) == 0 {
		t.Fatal("expected price embed from refresh")
	}
	resp := bot.lastResponse()
	if resp.Type != ResponseUpdateMessage {
		t.Errorf("expected update message type, got %d", resp.Type)
	}
}

func TestComponentOrderBook_Refresh(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeComponentInteraction("12345", "ob:BTC/USDT:5"))

	if len(bot.embeds) == 0 {
		t.Fatal("expected orderbook embed from refresh")
	}
}

func TestComponentWatchAdd(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")
	watchRepo := &mockWatchlistRepo{}
	handler, bot := newTestHandler(repo, &mockExchange{}, watchRepo, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeComponentInteraction("12345", "wa:BTCUSDT"))

	if !strings.Contains(bot.lastMessage(), "added to watchlist") {
		t.Errorf("expected 'added to watchlist', got: %s", bot.lastMessage())
	}
}

func TestComponentWatchResetConfirm(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedDiscord(12345, "testuser")
	watchRepo := &mockWatchlistRepo{}
	handler, bot := newTestHandler(repo, &mockExchange{}, watchRepo, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeComponentInteraction("12345", "watchreset_confirm"))

	if !strings.Contains(bot.lastMessage(), "reset to default") {
		t.Errorf("expected reset confirmation, got: %s", bot.lastMessage())
	}
}

func TestComponentWatchResetCancel(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeComponentInteraction("12345", "watchreset_cancel"))

	if !strings.Contains(bot.lastMessage(), "cancelled") {
		t.Errorf("expected cancelled message, got: %s", bot.lastMessage())
	}
}

func TestComponentBalance_Refresh(t *testing.T) {
	repo := newMockUserRepo()
	u := repo.seedActivatedDiscord(12345, "testuser")
	enc := &mockEncryptor{}
	encKey, _ := enc.Encrypt([]byte("key"), []byte("salt"))
	encSec, _ := enc.Encrypt([]byte("secret"), []byte("salt"))
	repo.credentials[u.ID] = &user.Credentials{
		ID: 1, UserID: u.ID, Exchange: "binance",
		APIKeyEncrypted: encKey, APISecretEncrypted: encSec,
		Salt: []byte("salt"), IsValid: true,
	}

	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeComponentInteraction("12345", "refresh_balance"))

	if len(bot.embeds) == 0 {
		t.Fatal("expected balance embed from refresh")
	}
}

func TestComponentPortfolio_Refresh(t *testing.T) {
	repo := newMockUserRepo()
	u := repo.seedActivatedDiscord(12345, "testuser")
	enc := &mockEncryptor{}
	encKey, _ := enc.Encrypt([]byte("key"), []byte("salt"))
	encSec, _ := enc.Encrypt([]byte("secret"), []byte("salt"))
	repo.credentials[u.ID] = &user.Credentials{
		ID: 1, UserID: u.ID, Exchange: "binance",
		APIKeyEncrypted: encKey, APISecretEncrypted: encSec,
		Salt: []byte("salt"), IsValid: true,
	}

	exch := &mockExchange{
		balances: []exchange.Balance{
			{Asset: "USDT", Free: 1000},
		},
	}
	handler, bot := newTestHandler(repo, exch, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeComponentInteraction("12345", "portfolio"))

	if len(bot.embeds) == 0 {
		t.Fatal("expected portfolio embed from refresh")
	}
}

// --- edge cases ---

func TestHandleInteraction_NoUser(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	// interaction with no user or member
	interaction := &Interaction{
		ID:    "int-1",
		Type:  InteractionCommand,
		Token: "tok",
		Data:  &InteractionData{Name: "start"},
	}
	handler.HandleInteraction(context.Background(), interaction)

	if !strings.Contains(bot.lastMessage(), "could not identify") {
		t.Errorf("expected identification error, got: %s", bot.lastMessage())
	}
}

func TestHandleInteraction_UnknownCommand(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("12345", "nonexistent"))

	if !strings.Contains(bot.lastMessage(), "unknown command") {
		t.Errorf("expected unknown command message, got: %s", bot.lastMessage())
	}
}

// --- link command tests ---

func TestHandleLink_Success(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedTelegram(99999, "telegram_user")
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("88888", "link", strOpt("telegram_id", "99999")))

	if len(bot.embeds) == 0 {
		t.Fatal("expected embed response")
	}
	if bot.embeds[0].Title != "🔗 Accounts Linked" {
		t.Errorf("expected linked title, got: %s", bot.embeds[0].Title)
	}
}

func TestHandleLink_InvalidTelegramID(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("88888", "link", strOpt("telegram_id", "not_a_number")))

	if len(bot.messages) == 0 {
		t.Fatal("expected error message")
	}
	if bot.lastMessage() != "invalid telegram id. it should be a number." {
		t.Errorf("expected invalid id message, got: %s", bot.lastMessage())
	}
}

func TestHandleLink_TelegramUserNotFound(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeInteraction("88888", "link", strOpt("telegram_id", "99999")))

	if len(bot.messages) == 0 {
		t.Fatal("expected error message")
	}
	if !strings.Contains(bot.lastMessage(), "❌") {
		t.Errorf("expected error with ❌, got: %s", bot.lastMessage())
	}
}

func TestHandleLink_AlreadyLinkedToDifferentUser(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedTelegram(11111, "user_one")
	repo.seedActivatedTelegram(22222, "user_two")

	// link discord to first user
	discordID := int64(88888)
	u := repo.users[int64(11111)]
	u.DiscordID = &discordID
	repo.discordUsers[discordID] = u

	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	// try to link same discord to second user
	handler.HandleInteraction(context.Background(), makeInteraction("88888", "link", strOpt("telegram_id", "22222")))

	if len(bot.messages) == 0 {
		t.Fatal("expected error message")
	}
	if !strings.Contains(bot.lastMessage(), "❌") {
		t.Errorf("expected error with ❌, got: %s", bot.lastMessage())
	}
}

func TestHandleLink_Idempotent(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedTelegram(99999, "idem_user")
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	// link once
	handler.HandleInteraction(context.Background(), makeInteraction("88888", "link", strOpt("telegram_id", "99999")))
	if len(bot.embeds) == 0 {
		t.Fatal("expected embed on first link")
	}

	// link again — should succeed
	handler.HandleInteraction(context.Background(), makeInteraction("88888", "link", strOpt("telegram_id", "99999")))
	if len(bot.embeds) < 2 {
		t.Fatal("expected embed on second link")
	}
	if bot.embeds[1].Title != "🔗 Accounts Linked" {
		t.Errorf("expected linked title on retry, got: %s", bot.embeds[1].Title)
	}
}

func TestHandleLink_ViaGuild(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedActivatedTelegram(99999, "guild_user")
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	handler.HandleInteraction(context.Background(), makeGuildInteraction("88888", "link", strOpt("telegram_id", "99999")))

	if len(bot.embeds) == 0 {
		t.Fatal("expected embed on guild link")
	}
	if bot.embeds[0].Title != "🔗 Accounts Linked" {
		t.Errorf("expected linked title, got: %s", bot.embeds[0].Title)
	}
}

func TestHandleInteraction_NilData(t *testing.T) {
	repo := newMockUserRepo()
	handler, bot := newTestHandler(repo, &mockExchange{}, &mockWatchlistRepo{}, newMockPrefsRepo())

	interaction := &Interaction{
		ID:    "int-1",
		Type:  InteractionCommand,
		Token: "tok",
		Data:  nil,
	}
	handler.HandleInteraction(context.Background(), interaction)

	if len(bot.responses) != 0 {
		t.Error("expected no response for nil data")
	}
}

func TestSlashCommands_AllDefined(t *testing.T) {
	commands := SlashCommands()
	expectedNames := []string{
		"start", "setup", "status", "help", "price", "balance",
		"portfolio", "orderbook", "watchlist", "watchadd", "watchremove",
		"watchreset", "settings", "set", "link",
	}

	if len(commands) != len(expectedNames) {
		t.Fatalf("expected %d commands, got %d", len(expectedNames), len(commands))
	}

	nameMap := make(map[string]bool)
	for _, cmd := range commands {
		nameMap[cmd.Name] = true
	}

	for _, name := range expectedNames {
		if !nameMap[name] {
			t.Errorf("missing command: %s", name)
		}
	}
}

func TestGetOption_StringValue(t *testing.T) {
	interaction := makeInteraction("1", "test", strOpt("key", "hello"))
	if v := getOption(interaction, "key"); v != "hello" {
		t.Errorf("expected 'hello', got %q", v)
	}
}

func TestGetOption_NumberValue(t *testing.T) {
	interaction := makeInteraction("1", "test", intOpt("depth", 10))
	if v := getOption(interaction, "depth"); v != "10" {
		t.Errorf("expected '10', got %q", v)
	}
}

func TestGetOption_Missing(t *testing.T) {
	interaction := makeInteraction("1", "test")
	if v := getOption(interaction, "missing"); v != "" {
		t.Errorf("expected empty string, got %q", v)
	}
}

func TestGetOptionInt(t *testing.T) {
	interaction := makeInteraction("1", "test", intOpt("depth", 7))
	if v := getOptionInt(interaction, "depth"); v != 7 {
		t.Errorf("expected 7, got %d", v)
	}
}

func TestGetInteractionUser_DM(t *testing.T) {
	interaction := &Interaction{
		User: &DiscordUser{ID: "123", Username: "dm-user"},
	}
	u := getInteractionUser(interaction)
	if u == nil || u.ID != "123" {
		t.Error("expected dm user")
	}
}

func TestGetInteractionUser_Guild(t *testing.T) {
	interaction := &Interaction{
		Member: &GuildMember{User: &DiscordUser{ID: "456", Username: "guild-user"}},
	}
	u := getInteractionUser(interaction)
	if u == nil || u.ID != "456" {
		t.Error("expected guild user")
	}
}

func TestGetInteractionUser_None(t *testing.T) {
	interaction := &Interaction{}
	u := getInteractionUser(interaction)
	if u != nil {
		t.Error("expected nil user")
	}
}

func TestNormalizeSymbol(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"BTC", "BTC/USDT"},
		{"btc", "BTC/USDT"},
		{"BTCUSDT", "BTC/USDT"},
		{"ETHBTC", "ETH/BTC"},
		{"BTC/USDT", "BTC/USDT"},
		{"SOL", "SOL/USDT"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeSymbol(tt.input)
			if got != tt.want {
				t.Errorf("normalizeSymbol(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatPrice(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{42000.5, "42000.50"},
		{0.00001234, "0.00001234"},
		{1.5, "1.50"},
	}
	for _, tt := range tests {
		got := formatPrice(tt.input)
		if got != tt.want {
			t.Errorf("formatPrice(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatVolume(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{1_500_000_000, "$1.5B"},
		{250_000_000, "$250.0M"},
		{5_000, "$5.0K"},
		{500, "$500.00"},
	}
	for _, tt := range tests {
		got := formatVolume(tt.input)
		if got != tt.want {
			t.Errorf("formatVolume(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsStablecoin(t *testing.T) {
	stables := []string{"USDT", "USDC", "BUSD", "DAI", "TUSD", "USDP", "FDUSD"}
	for _, s := range stables {
		if !isStablecoin(s) {
			t.Errorf("expected %s to be stablecoin", s)
		}
	}
	if isStablecoin("BTC") {
		t.Error("BTC should not be stablecoin")
	}
}

func TestActionRow(t *testing.T) {
	row := actionRow(
		button(ButtonPrimary, "Click", "id1"),
		button(ButtonDanger, "Delete", "id2"),
	)
	if row.Type != ComponentActionRow {
		t.Errorf("expected action row type, got %d", row.Type)
	}
	if len(row.Components) != 2 {
		t.Errorf("expected 2 buttons, got %d", len(row.Components))
	}
	if row.Components[0].Label != "Click" {
		t.Errorf("expected 'Click' label, got %s", row.Components[0].Label)
	}
}

func TestButton(t *testing.T) {
	b := button(ButtonSuccess, "OK", "btn-ok")
	if b.Type != ComponentButton {
		t.Errorf("expected button type, got %d", b.Type)
	}
	if b.Style != ButtonSuccess {
		t.Errorf("expected success style, got %d", b.Style)
	}
	if b.CustomID != "btn-ok" {
		t.Errorf("expected custom id 'btn-ok', got %s", b.CustomID)
	}
}
