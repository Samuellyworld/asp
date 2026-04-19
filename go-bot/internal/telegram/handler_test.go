package telegram

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

type sentMessage struct {
	chatID int64
	text   string
}

type editedMessage struct {
	chatID    int64
	messageID int
	text      string
	keyboard  *InlineKeyboardMarkup
}

type keyboardMessage struct {
	chatID   int64
	text     string
	keyboard *InlineKeyboardMarkup
}

type answeredCallback struct {
	queryID string
	text    string
}

type mockBot struct {
	messages  []sentMessage
	keyboards []keyboardMessage
	edits     []editedMessage
	callbacks []answeredCallback
	deletes   []int
	sendErr   error
}

func (m *mockBot) SendMessage(chatID int64, text string) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.messages = append(m.messages, sentMessage{chatID, text})
	return nil
}

func (m *mockBot) SendMessageWithKeyboard(chatID int64, text string, keyboard *InlineKeyboardMarkup) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.keyboards = append(m.keyboards, keyboardMessage{chatID, text, keyboard})
	return nil
}

func (m *mockBot) EditMessageText(chatID int64, messageID int, text string, keyboard *InlineKeyboardMarkup) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.edits = append(m.edits, editedMessage{chatID, messageID, text, keyboard})
	return nil
}

func (m *mockBot) AnswerCallbackQuery(queryID string, text string) error {
	m.callbacks = append(m.callbacks, answeredCallback{queryID, text})
	return nil
}

func (m *mockBot) DeleteMessage(_ int64, messageID int) error {
	m.deletes = append(m.deletes, messageID)
	return nil
}

func (m *mockBot) lastMessage() string {
	if len(m.messages) == 0 {
		return ""
	}
	return m.messages[len(m.messages)-1].text
}

func (m *mockBot) lastKeyboardMessage() string {
	if len(m.keyboards) == 0 {
		return ""
	}
	return m.keyboards[len(m.keyboards)-1].text
}

func (m *mockBot) lastKeyboard() *InlineKeyboardMarkup {
	if len(m.keyboards) == 0 {
		return nil
	}
	return m.keyboards[len(m.keyboards)-1].keyboard
}

func (m *mockBot) lastEdit() *editedMessage {
	if len(m.edits) == 0 {
		return nil
	}
	return &m.edits[len(m.edits)-1]
}

func (m *mockBot) reset() {
	m.messages = nil
	m.keyboards = nil
	m.edits = nil
	m.callbacks = nil
	m.deletes = nil
}

// --- mock user repo (satisfies user.userRepository) ---

type mockUserRepo struct {
	users       map[int64]*user.User
	credentials map[int]bool
	nextID      int
	findErr     error
	createErr   error
	activateErr error
	saveErr     error
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		users:       make(map[int64]*user.User),
		credentials: make(map[int]bool),
		nextID:      1,
	}
}

func (m *mockUserRepo) FindByTelegramID(_ context.Context, telegramID int64) (*user.User, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	return m.users[telegramID], nil
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

func (m *mockUserRepo) CreateDefaultPreferences(_ context.Context, _ int) error { return nil }
func (m *mockUserRepo) UpdateLastActive(_ context.Context, _ int, _ string) error { return nil }
func (m *mockUserRepo) FindByDiscordID(_ context.Context, _ int64) (*user.User, error) { return nil, nil }
func (m *mockUserRepo) CreateFromDiscord(_ context.Context, _ int64, _ string) (*user.User, error) { return nil, nil }
func (m *mockUserRepo) LinkDiscordToTelegram(_ context.Context, _ int64, _ int64) (*user.User, error) { return nil, nil }

func (m *mockUserRepo) Activate(_ context.Context, userID int) error {
	if m.activateErr != nil {
		return m.activateErr
	}
	for _, u := range m.users {
		if u.ID == userID {
			u.IsActivated = true
		}
	}
	return nil
}

func (m *mockUserRepo) SaveCredentials(_ context.Context, cred *user.Credentials) (*user.Credentials, error) {
	if m.saveErr != nil {
		return nil, m.saveErr
	}
	cred.ID = m.nextID
	m.nextID++
	m.credentials[cred.UserID] = true
	return cred, nil
}

func (m *mockUserRepo) HasValidCredentials(_ context.Context, userID int) (bool, error) {
	return m.credentials[userID], nil
}

func (m *mockUserRepo) GetCredentials(_ context.Context, userID int, _ string) (*user.Credentials, error) {
	for _, u := range m.users {
		if u.ID == userID {
			if m.credentials[userID] {
				return &user.Credentials{
					ID:                 1,
					UserID:             userID,
					Exchange:           "binance",
					APIKeyEncrypted:    []byte("test-key"),
					APISecretEncrypted: []byte("test-secret"),
					Salt:               []byte("salt"),
					IsValid:            true,
				}, nil
			}
		}
	}
	return nil, nil
}

func (m *mockUserRepo) ListActive(_ context.Context) ([]*user.User, error) {
	return nil, nil
}

func (m *mockUserRepo) SetLeverageEnabled(_ context.Context, _ int, _ bool) error {
	return nil
}

func (m *mockUserRepo) IsLeverageEnabled(_ context.Context, _ int) (bool, error) {
	return false, nil
}

func (m *mockUserRepo) seed(telegramID int64, username string, activated bool) *user.User {
	u := &user.User{
		ID:          m.nextID,
		UUID:        fmt.Sprintf("uuid-%d", m.nextID),
		TelegramID:  &telegramID,
		Username:    &username,
		IsActivated: activated,
		TradingMode: "paper",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.nextID++
	m.users[telegramID] = u
	if activated {
		m.credentials[u.ID] = true
	}
	return u
}

// --- mock watchlist repo (satisfies watchlist.repository) ---

type mockWatchRepo struct {
	items  map[int][]watchlist.Item
	addErr error
	rmErr  error
}

func newMockWatchRepo() *mockWatchRepo {
	return &mockWatchRepo{items: make(map[int][]watchlist.Item)}
}

func (m *mockWatchRepo) GetByUserID(_ context.Context, userID int) ([]watchlist.Item, error) {
	return m.items[userID], nil
}

func (m *mockWatchRepo) Add(_ context.Context, userID int, symbol string) error {
	if m.addErr != nil {
		return m.addErr
	}
	m.items[userID] = append(m.items[userID], watchlist.Item{Symbol: symbol, IsActive: true})
	return nil
}

func (m *mockWatchRepo) Remove(_ context.Context, userID int, symbol string) error {
	if m.rmErr != nil {
		return m.rmErr
	}
	items := m.items[userID]
	for i, item := range items {
		if item.Symbol == symbol {
			m.items[userID] = append(items[:i], items[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("symbol %s not found", symbol)
}

func (m *mockWatchRepo) Exists(_ context.Context, userID int, symbol string) (bool, error) {
	for _, item := range m.items[userID] {
		if item.Symbol == symbol {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockWatchRepo) Count(_ context.Context, userID int) (int, error) {
	return len(m.items[userID]), nil
}

func (m *mockWatchRepo) Reset(_ context.Context, userID int) error {
	m.items[userID] = []watchlist.Item{
		{Symbol: "BTC/USDT", IsActive: true},
		{Symbol: "ETH/USDT", IsActive: true},
	}
	return nil
}

func (m *mockWatchRepo) seed(userID int, symbols ...string) {
	for _, s := range symbols {
		m.items[userID] = append(m.items[userID], watchlist.Item{Symbol: s, IsActive: true})
	}
}

// --- mock preferences repo (satisfies preferences.repository) ---

type mockPrefsRepo struct {
	scanning     map[int]*preferences.Scanning
	notification map[int]*preferences.Notification
	trading      map[int]*preferences.Trading
}

func newMockPrefsRepo() *mockPrefsRepo {
	return &mockPrefsRepo{
		scanning:     make(map[int]*preferences.Scanning),
		notification: make(map[int]*preferences.Notification),
		trading:      make(map[int]*preferences.Trading),
	}
}

func (m *mockPrefsRepo) GetScanning(_ context.Context, userID int) (*preferences.Scanning, error) {
	return m.scanning[userID], nil
}

func (m *mockPrefsRepo) UpdateScanning(_ context.Context, s *preferences.Scanning) error {
	m.scanning[s.UserID] = s
	return nil
}

func (m *mockPrefsRepo) GetNotification(_ context.Context, userID int) (*preferences.Notification, error) {
	return m.notification[userID], nil
}

func (m *mockPrefsRepo) UpdateNotification(_ context.Context, n *preferences.Notification) error {
	m.notification[n.UserID] = n
	return nil
}

func (m *mockPrefsRepo) GetTrading(_ context.Context, userID int) (*preferences.Trading, error) {
	return m.trading[userID], nil
}

func (m *mockPrefsRepo) UpdateTrading(_ context.Context, t *preferences.Trading) error {
	m.trading[t.UserID] = t
	return nil
}

func (m *mockPrefsRepo) seedAll(userID int) {
	m.scanning[userID] = &preferences.Scanning{
		UserID:            userID,
		MinConfidence:     60,
		ScanIntervalMins:  5,
		EnabledTimeframes: []string{"1h", "4h"},
		EnabledIndicators: []string{"rsi", "macd"},
		IsScanningEnabled: true,
	}
	m.notification[userID] = &preferences.Notification{
		UserID:                userID,
		MaxDailyNotifications: 50,
		Timezone:              "UTC",
		DailySummaryHour:      9,
		DailySummaryEnabled:   true,
	}
	m.trading[userID] = &preferences.Trading{
		UserID:               userID,
		DefaultPositionSize:  100,
		MaxPositionSize:      300,
		DefaultStopLossPct:   2.0,
		DefaultTakeProfitPct: 6.0,
		MaxLeverage:          10,
		RiskPerTradePct:      1.0,
	}
}

// --- mock validator (satisfies user.keyValidator) ---

type mockKeyValidator struct {
	perms *binance.APIPermissions
	err   error
}

func (m *mockKeyValidator) ValidateKeys(_ context.Context, _, _ string) (*binance.APIPermissions, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.perms, nil
}

// --- mock encryptor (satisfies user.encryptor) ---

type mockEncryptor struct{}

func (m *mockEncryptor) Encrypt(plaintext []byte, _ []byte) ([]byte, error) {
	return plaintext, nil
}

func (m *mockEncryptor) Decrypt(ciphertext []byte, _ []byte) ([]byte, error) {
	return ciphertext, nil
}

// --- mock exchange (satisfies exchangeClient) ---

type mockExchange struct {
	priceErr   error
	balanceErr error
	bookErr    error
}

func (m *mockExchange) GetPrice(_ context.Context, symbol string) (*exchange.Ticker, error) {
	if m.priceErr != nil {
		return nil, m.priceErr
	}
	prices := map[string]*exchange.Ticker{
		"BTC/USDT":  {Symbol: "BTC/USDT", Price: 42000.00, PriceChange: -850.00, ChangePct: -1.98, Volume: 28500.5, QuoteVolume: 1197021000},
		"ETH/USDT":  {Symbol: "ETH/USDT", Price: 2280.00, PriceChange: 45.00, ChangePct: 2.01, Volume: 185000.0, QuoteVolume: 421800000},
		"DOGE/USDT": {Symbol: "DOGE/USDT", Price: 0.0825, PriceChange: 0.0015, ChangePct: 1.85, Volume: 980000000.0, QuoteVolume: 80850000},
	}
	t, ok := prices[symbol]
	if !ok {
		return nil, fmt.Errorf("symbol not found: %s", symbol)
	}
	return t, nil
}

func (m *mockExchange) GetBalance(_ context.Context, apiKey, apiSecret string) ([]exchange.Balance, error) {
	if m.balanceErr != nil {
		return nil, m.balanceErr
	}
	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("missing credentials")
	}
	return []exchange.Balance{
		{Asset: "USDT", Free: 1000.00, Locked: 0},
		{Asset: "BTC", Free: 0.05, Locked: 0},
		{Asset: "ETH", Free: 1.5, Locked: 0.5},
	}, nil
}

func (m *mockExchange) GetOrderBook(_ context.Context, symbol string, depth int) (*exchange.OrderBook, error) {
	if m.bookErr != nil {
		return nil, m.bookErr
	}
	books := map[string]*exchange.OrderBook{
		"BTC/USDT": {
			Symbol: "BTC/USDT",
			Bids: []exchange.OrderBookEntry{
				{Price: 41999.00, Quantity: 0.500},
				{Price: 41998.00, Quantity: 1.200},
				{Price: 41995.00, Quantity: 0.350},
			},
			Asks: []exchange.OrderBookEntry{
				{Price: 42001.00, Quantity: 0.800},
				{Price: 42002.00, Quantity: 0.450},
				{Price: 42005.00, Quantity: 1.500},
			},
		},
	}
	ob, ok := books[symbol]
	if !ok {
		return nil, fmt.Errorf("symbol not found: %s", symbol)
	}
	result := &exchange.OrderBook{Symbol: ob.Symbol}
	bids := ob.Bids
	asks := ob.Asks
	if depth > 0 && depth < len(bids) {
		bids = bids[:depth]
	}
	if depth > 0 && depth < len(asks) {
		asks = asks[:depth]
	}
	result.Bids = bids
	result.Asks = asks
	return result, nil
}

// --- test helpers ---

func makeUpdate(tid, cid int64, text string) Update {
	return Update{
		UpdateID: 1,
		Message: &Message{
			MessageID: 1,
			From:      &From{ID: tid, Username: "testuser", FirstName: "Test"},
			Chat:      &Chat{ID: cid, Type: "private"},
			Text:      text,
		},
	}
}

type testEnv struct {
	bot       *mockBot
	userRepo  *mockUserRepo
	watchRepo *mockWatchRepo
	prefsRepo *mockPrefsRepo
	validator *mockKeyValidator
	handler   *Handler
	wizard    *user.SetupWizard
}

func newTestEnv() *testEnv {
	bot := &mockBot{}
	userRepo := newMockUserRepo()
	watchRepo := newMockWatchRepo()
	prefsRepo := newMockPrefsRepo()
	validator := &mockKeyValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}

	exch := &mockExchange{}

	userSvc := user.NewService(userRepo, &mockEncryptor{}, nil, validator, false)
	wizard := user.NewSetupWizard()
	watchSvc := watchlist.NewService(watchRepo)
	prefsSvc := preferences.NewService(prefsRepo)

	handler := NewHandler(bot, userSvc, wizard, watchSvc, prefsSvc, exch)

	return &testEnv{
		bot:       bot,
		userRepo:  userRepo,
		watchRepo: watchRepo,
		prefsRepo: prefsRepo,
		validator: validator,
		handler:   handler,
		wizard:    wizard,
	}
}

// seed an activated user with all preferences
func (e *testEnv) seedActivatedUser(tid int64) *user.User {
	u := e.userRepo.seed(tid, "testuser", true)
	e.prefsRepo.seedAll(u.ID)
	return u
}

// --- HandleUpdate routing tests ---

func TestHandleUpdate_NilMessage(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), Update{})
	if len(env.bot.messages) != 0 {
		t.Error("expected no messages for nil message")
	}
}

func TestHandleUpdate_BotMessage(t *testing.T) {
	env := newTestEnv()
	update := Update{
		Message: &Message{
			From: &From{ID: 1, IsBot: true},
			Chat: &Chat{ID: 1},
			Text: "/start",
		},
	}
	env.handler.HandleUpdate(context.Background(), update)
	if len(env.bot.messages) != 0 {
		t.Error("expected no response to bot messages")
	}
}

func TestHandleUpdate_NilFrom(t *testing.T) {
	env := newTestEnv()
	update := Update{
		Message: &Message{
			Chat: &Chat{ID: 1},
			Text: "/start",
		},
	}
	env.handler.HandleUpdate(context.Background(), update)
	if len(env.bot.messages) != 0 {
		t.Error("expected no response when From is nil")
	}
}

func TestHandleUpdate_UnknownCommand(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(1, 1, "/foobar"))

	if len(env.bot.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(env.bot.messages))
	}
	if !strings.Contains(env.bot.lastMessage(), "unknown command") {
		t.Errorf("expected unknown command message, got: %s", env.bot.lastMessage())
	}
}

func TestHandleUpdate_PlainText_NoResponse(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(1, 1, "hello there"))

	// plain text without / prefix should not trigger any response
	if len(env.bot.messages) != 0 {
		t.Errorf("expected no response for plain text, got %d messages", len(env.bot.messages))
	}
}

// --- /start tests ---

func TestStart_NewUser(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/start"))

	if len(env.bot.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(env.bot.messages))
	}
	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "welcome") {
		t.Errorf("expected welcome message, got: %s", msg)
	}
	if !strings.Contains(msg, "/setup") {
		t.Errorf("expected setup prompt in message, got: %s", msg)
	}
}

func TestStart_ReturningActivatedUser(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/start"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "welcome back") {
		t.Errorf("expected welcome back message, got: %s", msg)
	}
	if !strings.Contains(msg, "active") {
		t.Errorf("expected active status in message, got: %s", msg)
	}
}

func TestStart_ReturningNonActivatedUser(t *testing.T) {
	env := newTestEnv()
	env.userRepo.seed(12345, "testuser", false)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/start"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "welcome back") {
		t.Errorf("expected welcome back message, got: %s", msg)
	}
	if !strings.Contains(msg, "/setup") {
		t.Errorf("expected setup prompt, got: %s", msg)
	}
}

func TestStart_RegistrationError(t *testing.T) {
	env := newTestEnv()
	env.userRepo.findErr = fmt.Errorf("db down")

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/start"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "went wrong") {
		t.Errorf("expected error message, got: %s", msg)
	}
}

// --- /status tests ---

func TestStatus_Activated(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/status"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "active") {
		t.Errorf("expected active status, got: %s", msg)
	}
	if !strings.Contains(msg, "connected") {
		t.Errorf("expected connected keys, got: %s", msg)
	}
}

func TestStatus_NotActivated(t *testing.T) {
	env := newTestEnv()
	env.userRepo.seed(12345, "testuser", false)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/status"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "pending") {
		t.Errorf("expected pending status, got: %s", msg)
	}
}

// --- /cancel tests ---

func TestCancel_InWizard(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)
	env.wizard.Start(12345, 1)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/cancel"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "cancelled") {
		t.Errorf("expected cancelled message, got: %s", msg)
	}
	if env.wizard.IsInSetup(12345) {
		t.Error("expected wizard session to be cleared")
	}
}

func TestCancel_NotInWizard(t *testing.T) {
	env := newTestEnv()

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/cancel"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "nothing to cancel") {
		t.Errorf("expected nothing to cancel, got: %s", msg)
	}
}

// --- /help ---

func TestHelp(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/help"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "available commands") {
		t.Errorf("expected help message, got: %s", msg)
	}
	// should mention key commands
	for _, cmd := range []string{"/start", "/setup", "/watchlist", "/settings"} {
		if !strings.Contains(msg, cmd) {
			t.Errorf("expected %s in help message", cmd)
		}
	}
}

// --- /watchlist tests ---

func TestWatchlist_Empty(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/watchlist"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "empty") {
		t.Errorf("expected empty watchlist message, got: %s", msg)
	}
}

func TestWatchlist_WithItems(t *testing.T) {
	env := newTestEnv()
	u := env.seedActivatedUser(12345)
	env.watchRepo.seed(u.ID, "BTC/USDT", "ETH/USDT")

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/watchlist"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "BTC/USDT") {
		t.Errorf("expected BTC/USDT in watchlist, got: %s", msg)
	}
	if !strings.Contains(msg, "ETH/USDT") {
		t.Errorf("expected ETH/USDT in watchlist, got: %s", msg)
	}
	kb := env.bot.lastKeyboard()
	if kb == nil {
		t.Fatal("expected inline keyboard on watchlist")
	}
	if len(kb.InlineKeyboard) == 0 {
		t.Error("expected at least one row of buttons")
	}
}

func TestWatchlist_Alias(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/wl"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "empty") {
		t.Errorf("expected /wl to work as alias, got: %s", msg)
	}
}

func TestWatchlist_NotSetup(t *testing.T) {
	env := newTestEnv()
	env.userRepo.seed(12345, "testuser", false)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/watchlist"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "setup") {
		t.Errorf("expected setup prompt, got: %s", msg)
	}
}

// --- /watchadd tests ---

func TestWatchAdd_Success(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/watchadd BTCUSDT"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "added") {
		t.Errorf("expected added message, got: %s", msg)
	}
}

func TestWatchAdd_NoArgs(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/watchadd"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "usage") {
		t.Errorf("expected usage message, got: %s", msg)
	}
}

func TestWatchAdd_Alias(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/wa ETHUSDT"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "added") {
		t.Errorf("expected /wa to work as alias, got: %s", msg)
	}
}

func TestWatchAdd_InvalidSymbol(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/watchadd X"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "❌") {
		t.Errorf("expected error message for invalid symbol, got: %s", msg)
	}
}

// --- /watchremove tests ---

func TestWatchRemove_Success(t *testing.T) {
	env := newTestEnv()
	u := env.seedActivatedUser(12345)
	env.watchRepo.seed(u.ID, "BTC/USDT")

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/watchremove BTCUSDT"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "removed") {
		t.Errorf("expected removed message, got: %s", msg)
	}
}

func TestWatchRemove_NoArgs(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/watchremove"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "usage") {
		t.Errorf("expected usage message, got: %s", msg)
	}
}

// --- /watchreset ---

func TestWatchReset_ShowsConfirmation(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/watchreset"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "are you sure") {
		t.Errorf("expected confirmation prompt, got: %s", msg)
	}
	kb := env.bot.lastKeyboard()
	if kb == nil {
		t.Fatal("expected inline keyboard for confirmation")
	}
	found := false
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == "watchreset_confirm" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected watchreset_confirm callback data in buttons")
	}
}

// --- /settings ---

func TestSettings_Success(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/settings"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "preferences") {
		t.Errorf("expected preferences message, got: %s", msg)
	}
	if !strings.Contains(msg, "60%") {
		t.Errorf("expected confidence value, got: %s", msg)
	}
	if !strings.Contains(msg, "stop loss") {
		t.Errorf("expected stop loss in settings, got: %s", msg)
	}
}

func TestSettings_NotSetup(t *testing.T) {
	env := newTestEnv()
	env.userRepo.seed(12345, "testuser", false)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/settings"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "setup") {
		t.Errorf("expected setup prompt, got: %s", msg)
	}
}

// --- /set tests ---

func TestSet_Confidence(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set confidence 80"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "updated") {
		t.Errorf("expected updated message, got: %s", msg)
	}
}

func TestSet_Interval(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set interval 15"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "updated") {
		t.Errorf("expected updated message, got: %s", msg)
	}
}

func TestSet_InvalidConfidenceValue(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set confidence 200"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "❌") {
		t.Errorf("expected error for invalid confidence, got: %s", msg)
	}
}

func TestSet_NonNumericConfidence(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set confidence abc"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "number") {
		t.Errorf("expected number error, got: %s", msg)
	}
}

func TestSet_PositionSizeWithMax(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set positionsize 100 500"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "updated") {
		t.Errorf("expected updated message, got: %s", msg)
	}
}

func TestSet_PositionSizeWithoutMax(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set positionsize 100"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "updated") {
		t.Errorf("expected updated message (max defaults to 3x), got: %s", msg)
	}
}

func TestSet_StopLoss(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set stoploss 3.5"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "updated") {
		t.Errorf("expected updated message, got: %s", msg)
	}
}

func TestSet_TakeProfit(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set takeprofit 10"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "updated") {
		t.Errorf("expected updated message, got: %s", msg)
	}
}

func TestSet_Leverage(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set leverage 20"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "updated") {
		t.Errorf("expected updated message, got: %s", msg)
	}
}

func TestSet_Risk(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set risk 2.5"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "updated") {
		t.Errorf("expected updated message, got: %s", msg)
	}
}

func TestSet_ScanningOn(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set scanning off"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "updated") {
		t.Errorf("expected updated message, got: %s", msg)
	}
}

func TestSet_ScanningInvalid(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set scanning maybe"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "on") && !strings.Contains(msg, "off") {
		t.Errorf("expected on/off hint, got: %s", msg)
	}
}

func TestSet_Timezone(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set timezone America/New_York"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "updated") {
		t.Errorf("expected updated message, got: %s", msg)
	}
}

func TestSet_MaxNotifs(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set maxnotifs 25"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "updated") {
		t.Errorf("expected updated message, got: %s", msg)
	}
}

func TestSet_SummaryHour(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set summaryhour 18"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "updated") {
		t.Errorf("expected updated message, got: %s", msg)
	}
}

func TestSet_UnknownKey(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set foobar 123"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "unknown setting") {
		t.Errorf("expected unknown setting message, got: %s", msg)
	}
}

func TestSet_NoArgs(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "usage") {
		t.Errorf("expected usage message, got: %s", msg)
	}
}

func TestSet_OneArg(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/set confidence"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "usage") {
		t.Errorf("expected usage message, got: %s", msg)
	}
}

// --- wizard flow tests ---

func TestWizardFlow_Complete(t *testing.T) {
	env := newTestEnv()
	env.userRepo.seed(12345, "testuser", false)

	// start setup
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/setup"))
	if !env.wizard.IsInSetup(12345) {
		t.Fatal("expected wizard to be active")
	}

	env.bot.reset()

	// select exchange
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "binance"))
	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "exchange set to") {
		t.Errorf("expected exchange confirmation, got: %s", msg)
	}

	env.bot.reset()

	// send api key
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "my-api-key-12345"))
	if len(env.bot.deletes) == 0 {
		t.Error("expected api key message to be deleted")
	}
	msg = env.bot.lastMessage()
	if !strings.Contains(msg, "api key received") {
		t.Errorf("expected key confirmation, got: %s", msg)
	}

	env.bot.reset()

	// send api secret
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "my-api-secret-12345"))
	if len(env.bot.deletes) == 0 {
		t.Error("expected api secret message to be deleted")
	}

	// should get validation + setup complete messages
	found := false
	for _, m := range env.bot.messages {
		if strings.Contains(m.text, "setup complete") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected setup complete message")
	}

	// wizard should be done
	if env.wizard.IsInSetup(12345) {
		t.Error("expected wizard to be inactive after completion")
	}
}

func TestWizardFlow_CancelDuringSetup(t *testing.T) {
	env := newTestEnv()
	env.userRepo.seed(12345, "testuser", false)

	// start setup
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/setup"))
	if !env.wizard.IsInSetup(12345) {
		t.Fatal("expected wizard to be active")
	}

	env.bot.reset()

	// cancel during wizard
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/cancel"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "cancelled") {
		t.Errorf("expected cancelled message, got: %s", msg)
	}
	if env.wizard.IsInSetup(12345) {
		t.Error("expected wizard to be inactive after cancel")
	}
}

func TestWizardFlow_ValidationFails(t *testing.T) {
	env := newTestEnv()
	env.userRepo.seed(12345, "testuser", false)
	env.validator.err = fmt.Errorf("invalid api key")

	// start setup
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/setup"))
	env.bot.reset()

	// select exchange
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "binance"))
	env.bot.reset()

	// send api key
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "bad-key"))
	env.bot.reset()

	// send api secret
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "bad-secret"))

	found := false
	for _, m := range env.bot.messages {
		if strings.Contains(m.text, "setup failed") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected setup failed message")
	}
}

// --- bot send error handling ---

func TestSendError_DoesNotPanic(t *testing.T) {
	env := newTestEnv()
	env.bot.sendErr = fmt.Errorf("network error")

	// should not panic even if send fails
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/help"))
}

// --- /price tests ---

func TestPrice_BTC(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/price BTC"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "BTC/USDT") {
		t.Errorf("expected BTC/USDT in message, got: %s", msg)
	}
	if !strings.Contains(msg, "42000") {
		t.Errorf("expected price in message, got: %s", msg)
	}
	kb := env.bot.lastKeyboard()
	if kb == nil {
		t.Fatal("expected inline keyboard on price")
	}
	// should have refresh and order book buttons
	foundRefresh := false
	foundOB := false
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == "price:BTC/USDT" {
				foundRefresh = true
			}
			if btn.CallbackData == "ob:BTC/USDT" {
				foundOB = true
			}
		}
	}
	if !foundRefresh {
		t.Error("expected refresh button")
	}
	if !foundOB {
		t.Error("expected order book button")
	}
}

func TestPrice_WithUSDTSuffix(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/price ETHUSDT"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "ETH/USDT") {
		t.Errorf("expected ETH/USDT in message, got: %s", msg)
	}
	if !strings.Contains(msg, "2280") {
		t.Errorf("expected price in message, got: %s", msg)
	}
}

func TestPrice_WithSlash(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/price ETH/USDT"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "ETH/USDT") {
		t.Errorf("expected ETH/USDT, got: %s", msg)
	}
}

func TestPrice_Alias(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/p BTC"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "BTC/USDT") {
		t.Errorf("expected /p to work as alias, got: %s", msg)
	}
}

func TestPrice_NoArgs(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/price"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "usage") {
		t.Errorf("expected usage message, got: %s", msg)
	}
}

func TestPrice_UnknownSymbol(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/price ZZZZZ"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "❌") {
		t.Errorf("expected error for unknown symbol, got: %s", msg)
	}
}

func TestPrice_LowercaseNormalized(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/price btc"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "BTC/USDT") {
		t.Errorf("expected lowercase to be normalized, got: %s", msg)
	}
}

func TestPrice_SubDollarPrice(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/price DOGE"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "DOGE/USDT") {
		t.Errorf("expected DOGE/USDT, got: %s", msg)
	}
	if !strings.Contains(msg, "0.0825") {
		t.Errorf("expected sub-dollar price display, got: %s", msg)
	}
}

func TestPrice_ShowsChangeDirection(t *testing.T) {
	env := newTestEnv()

	// BTC has negative change
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/price BTC"))
	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "📉") {
		t.Errorf("expected down emoji for negative change, got: %s", msg)
	}

	env.bot.reset()

	// ETH has positive change
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/price ETH"))
	msg = env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "📈") {
		t.Errorf("expected up emoji for positive change, got: %s", msg)
	}
}

// --- /balance tests ---

func TestBalance_Success(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/balance"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "balances") {
		t.Errorf("expected balances header, got: %s", msg)
	}
	if !strings.Contains(msg, "USDT") {
		t.Errorf("expected USDT in balances, got: %s", msg)
	}
	if !strings.Contains(msg, "BTC") {
		t.Errorf("expected BTC in balances, got: %s", msg)
	}
	if !strings.Contains(msg, "ETH") {
		t.Errorf("expected ETH in balances, got: %s", msg)
	}
	kb := env.bot.lastKeyboard()
	if kb == nil {
		t.Fatal("expected inline keyboard on balance")
	}
	foundRefresh := false
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == "refresh_balance" {
				foundRefresh = true
			}
		}
	}
	if !foundRefresh {
		t.Error("expected refresh_balance callback button")
	}
}

func TestBalance_Alias(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/bal"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "balances") {
		t.Errorf("expected /bal to work as alias, got: %s", msg)
	}
}

func TestBalance_ShowsLockedAmount(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/balance"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "locked") {
		t.Errorf("expected locked amount display for ETH, got: %s", msg)
	}
}

func TestBalance_NotSetup(t *testing.T) {
	env := newTestEnv()
	env.userRepo.seed(12345, "testuser", false)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/balance"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "setup") {
		t.Errorf("expected setup prompt, got: %s", msg)
	}
}

// --- /orderbook tests ---

func TestOrderBook_Success(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/orderbook BTC"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "order book") {
		t.Errorf("expected order book header, got: %s", msg)
	}
	if !strings.Contains(msg, "BTC/USDT") {
		t.Errorf("expected BTC/USDT in order book, got: %s", msg)
	}
	if !strings.Contains(msg, "bids") {
		t.Errorf("expected bids section, got: %s", msg)
	}
	if !strings.Contains(msg, "asks") {
		t.Errorf("expected asks section, got: %s", msg)
	}
	kb := env.bot.lastKeyboard()
	if kb == nil {
		t.Fatal("expected inline keyboard on order book")
	}
}

func TestOrderBook_Alias(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/ob BTC"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "order book") {
		t.Errorf("expected /ob to work as alias, got: %s", msg)
	}
}

func TestOrderBook_NoArgs(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/orderbook"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "usage") {
		t.Errorf("expected usage message, got: %s", msg)
	}
}

func TestOrderBook_UnknownSymbol(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/orderbook ZZZZZ"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "❌") {
		t.Errorf("expected error for unknown symbol, got: %s", msg)
	}
}

func TestOrderBook_WithDepth(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/ob BTC 2"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "BTC/USDT") {
		t.Errorf("expected BTC/USDT with depth, got: %s", msg)
	}
}

// --- format helpers tests ---

func TestNormalizeSymbolForExchange(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"BTC", "BTC/USDT"},
		{"btc", "BTC/USDT"},
		{"BTCUSDT", "BTC/USDT"},
		{"BTC/USDT", "BTC/USDT"},
		{"ethusdt", "ETH/USDT"},
		{"ETHBTC", "ETH/BTC"},
		{"SOL", "SOL/USDT"},
		{"DOGEUSDT", "DOGE/USDT"},
	}

	for _, tt := range tests {
		got := normalizeSymbolForExchange(tt.input)
		if got != tt.want {
			t.Errorf("normalizeSymbolForExchange(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatPrice(t *testing.T) {
	tests := []struct {
		price float64
		want  string
	}{
		{42000.00, "42000.00"},
		{2280.50, "2280.50"},
		{0.0825, "0.0825"},
		{0.00001234, "0.00001234"},
	}

	for _, tt := range tests {
		got := formatPrice(tt.price)
		if got != tt.want {
			t.Errorf("formatPrice(%v) = %q, want %q", tt.price, got, tt.want)
		}
	}
}

func TestFormatVolume(t *testing.T) {
	tests := []struct {
		vol  float64
		want string
	}{
		{1197021000, "$1.2B"},
		{421800000, "$421.8M"},
		{50000, "$50.0K"},
		{500, "$500.00"},
	}

	for _, tt := range tests {
		got := formatVolume(tt.vol)
		if got != tt.want {
			t.Errorf("formatVolume(%v) = %q, want %q", tt.vol, got, tt.want)
		}
	}
}

func TestFormatBalance(t *testing.T) {
	tests := []struct {
		val  float64
		want string
	}{
		{0, "0"},
		{1000.00, "1000"},
		{0.05, "0.05"},
		{1.5, "1.5"},
	}

	for _, tt := range tests {
		got := formatBalance(tt.val)
		if got != tt.want {
			t.Errorf("formatBalance(%v) = %q, want %q", tt.val, got, tt.want)
		}
	}
}

func TestFormatTickerMessage(t *testing.T) {
	ticker := &exchange.Ticker{
		Symbol:      "BTC/USDT",
		Price:       42000.00,
		PriceChange: -850.00,
		ChangePct:   -1.98,
		QuoteVolume: 1197021000,
	}
	msg := formatTickerMessage(ticker)
	if !strings.Contains(msg, "BTC/USDT") {
		t.Error("expected symbol in ticker message")
	}
	if !strings.Contains(msg, "42000") {
		t.Error("expected price in ticker message")
	}
	if !strings.Contains(msg, "📉") {
		t.Error("expected down emoji for negative change")
	}
}

func TestFormatOrderBookMessage(t *testing.T) {
	book := &exchange.OrderBook{
		Symbol: "BTC/USDT",
		Bids: []exchange.OrderBookEntry{
			{Price: 41999.00, Quantity: 0.5},
		},
		Asks: []exchange.OrderBookEntry{
			{Price: 42001.00, Quantity: 0.8},
		},
	}
	msg := formatOrderBookMessage(book)
	if !strings.Contains(msg, "BTC/USDT") {
		t.Error("expected symbol in order book message")
	}
	if !strings.Contains(msg, "bids") {
		t.Error("expected bids section")
	}
	if !strings.Contains(msg, "asks") {
		t.Error("expected asks section")
	}
}

func TestIsStablecoin(t *testing.T) {
	stables := []string{"USDT", "USDC", "BUSD", "DAI", "TUSD", "USDP", "FDUSD"}
	for _, s := range stables {
		if !isStablecoin(s) {
			t.Errorf("expected %s to be a stablecoin", s)
		}
	}
	nonStables := []string{"BTC", "ETH", "SOL", "DOGE"}
	for _, s := range nonStables {
		if isStablecoin(s) {
			t.Errorf("expected %s to NOT be a stablecoin", s)
		}
	}
}

// --- callback query helpers ---

func makeCallbackQuery(tid, cid int64, msgID int, data string) Update {
	return Update{
		UpdateID: 1,
		CallbackQuery: &CallbackQuery{
			ID:   "cb-123",
			From: &From{ID: tid, Username: "testuser"},
			Message: &Message{
				MessageID: msgID,
				Chat:      &Chat{ID: cid},
			},
			Data: data,
		},
	}
}

// --- callback query tests ---

func TestCallback_RefreshPrice(t *testing.T) {
	env := newTestEnv()

	env.handler.HandleUpdate(context.Background(), makeCallbackQuery(12345, 100, 42, "price:BTC/USDT"))

	if len(env.bot.callbacks) == 0 {
		t.Fatal("expected callback to be answered")
	}
	if env.bot.callbacks[0].text != "price updated" {
		t.Errorf("expected 'price updated' callback, got: %s", env.bot.callbacks[0].text)
	}

	edit := env.bot.lastEdit()
	if edit == nil {
		t.Fatal("expected message to be edited")
	}
	if edit.messageID != 42 {
		t.Errorf("expected message ID 42, got: %d", edit.messageID)
	}
	if !strings.Contains(edit.text, "BTC/USDT") {
		t.Errorf("expected BTC/USDT in edited message, got: %s", edit.text)
	}
	if edit.keyboard == nil {
		t.Error("expected keyboard in edited message")
	}
}

func TestCallback_RefreshPrice_Error(t *testing.T) {
	env := newTestEnv()

	env.handler.HandleUpdate(context.Background(), makeCallbackQuery(12345, 100, 42, "price:ZZZZZ/USDT"))

	if len(env.bot.callbacks) == 0 {
		t.Fatal("expected callback to be answered")
	}
	if !strings.Contains(env.bot.callbacks[0].text, "failed") {
		t.Errorf("expected failure callback, got: %s", env.bot.callbacks[0].text)
	}
	if len(env.bot.edits) != 0 {
		t.Error("expected no edit on error")
	}
}

func TestCallback_OrderBook(t *testing.T) {
	env := newTestEnv()

	env.handler.HandleUpdate(context.Background(), makeCallbackQuery(12345, 100, 42, "ob:BTC/USDT:5"))

	if len(env.bot.callbacks) == 0 {
		t.Fatal("expected callback to be answered")
	}
	edit := env.bot.lastEdit()
	if edit == nil {
		t.Fatal("expected message to be edited")
	}
	if !strings.Contains(edit.text, "order book") {
		t.Error("expected order book in edited message")
	}
}

func TestCallback_OrderBook_NoDepth(t *testing.T) {
	env := newTestEnv()

	env.handler.HandleUpdate(context.Background(), makeCallbackQuery(12345, 100, 42, "ob:BTC/USDT"))

	edit := env.bot.lastEdit()
	if edit == nil {
		t.Fatal("expected message to be edited")
	}
	if !strings.Contains(edit.text, "BTC/USDT") {
		t.Error("expected BTC/USDT in edited message")
	}
}

func TestCallback_WatchAdd(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeCallbackQuery(12345, 100, 42, "wa:BTC/USDT"))

	if len(env.bot.callbacks) == 0 {
		t.Fatal("expected callback to be answered")
	}
	if !strings.Contains(env.bot.callbacks[0].text, "added to watchlist") {
		t.Errorf("expected 'added to watchlist' callback, got: %s", env.bot.callbacks[0].text)
	}
}

func TestCallback_WatchAdd_NotSetup(t *testing.T) {
	env := newTestEnv()
	env.userRepo.seed(12345, "testuser", false)

	env.handler.HandleUpdate(context.Background(), makeCallbackQuery(12345, 100, 42, "wa:BTC/USDT"))

	if len(env.bot.callbacks) == 0 {
		t.Fatal("expected callback to be answered")
	}
	if !strings.Contains(env.bot.callbacks[0].text, "complete setup") {
		t.Errorf("expected 'complete setup' callback, got: %s", env.bot.callbacks[0].text)
	}
}

func TestCallback_RefreshBalance(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeCallbackQuery(12345, 100, 42, "refresh_balance"))

	if len(env.bot.callbacks) == 0 {
		t.Fatal("expected callback to be answered")
	}
	edit := env.bot.lastEdit()
	if edit == nil {
		t.Fatal("expected message to be edited")
	}
	if !strings.Contains(edit.text, "balances") {
		t.Error("expected balances in edited message")
	}
}

func TestCallback_Portfolio(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeCallbackQuery(12345, 100, 42, "portfolio"))

	if len(env.bot.callbacks) == 0 {
		t.Fatal("expected callback to be answered")
	}
	edit := env.bot.lastEdit()
	if edit == nil {
		t.Fatal("expected message to be edited")
	}
	if !strings.Contains(edit.text, "portfolio") {
		t.Error("expected portfolio in edited message")
	}
}

func TestCallback_WatchResetConfirm(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeCallbackQuery(12345, 100, 42, "watchreset_confirm"))

	if len(env.bot.callbacks) == 0 {
		t.Fatal("expected callback to be answered")
	}
	edit := env.bot.lastEdit()
	if edit == nil {
		t.Fatal("expected message to be edited")
	}
	if !strings.Contains(edit.text, "reset") {
		t.Error("expected reset confirmation in edited message")
	}
}

func TestCallback_WatchResetCancel(t *testing.T) {
	env := newTestEnv()

	env.handler.HandleUpdate(context.Background(), makeCallbackQuery(12345, 100, 42, "watchreset_cancel"))

	if len(env.bot.callbacks) == 0 {
		t.Fatal("expected callback to be answered")
	}
	edit := env.bot.lastEdit()
	if edit == nil {
		t.Fatal("expected message to be edited")
	}
	if !strings.Contains(edit.text, "cancelled") {
		t.Error("expected cancelled message in edited text")
	}
}

func TestCallback_WatchlistPrice(t *testing.T) {
	env := newTestEnv()

	env.handler.HandleUpdate(context.Background(), makeCallbackQuery(12345, 100, 42, "wl_price:BTC/USDT"))

	edit := env.bot.lastEdit()
	if edit == nil {
		t.Fatal("expected message to be edited with price")
	}
	if !strings.Contains(edit.text, "BTC/USDT") {
		t.Error("expected BTC/USDT in edited message")
	}
}

func TestCallback_NilFrom(t *testing.T) {
	env := newTestEnv()
	update := Update{
		CallbackQuery: &CallbackQuery{
			ID:   "cb-123",
			From: nil,
			Message: &Message{
				MessageID: 42,
				Chat:      &Chat{ID: 100},
			},
			Data: "price:BTC/USDT",
		},
	}
	env.handler.HandleUpdate(context.Background(), update)
	if len(env.bot.edits) != 0 {
		t.Error("expected no action when From is nil")
	}
}

func TestCallback_NilMessage(t *testing.T) {
	env := newTestEnv()
	update := Update{
		CallbackQuery: &CallbackQuery{
			ID:      "cb-123",
			From:    &From{ID: 12345},
			Message: nil,
			Data:    "price:BTC/USDT",
		},
	}
	env.handler.HandleUpdate(context.Background(), update)
	if len(env.bot.edits) != 0 {
		t.Error("expected no action when Message is nil")
	}
}

func TestCallback_UnknownData(t *testing.T) {
	env := newTestEnv()

	env.handler.HandleUpdate(context.Background(), makeCallbackQuery(12345, 100, 42, "unknown_action"))

	if len(env.bot.callbacks) == 0 {
		t.Fatal("expected callback to be answered even for unknown data")
	}
}

// --- /portfolio tests ---

func TestPortfolio_Success(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/portfolio"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "portfolio") {
		t.Errorf("expected portfolio header, got: %s", msg)
	}
	if !strings.Contains(msg, "estimated total") {
		t.Errorf("expected estimated total, got: %s", msg)
	}
}

func TestPortfolio_Alias(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/pf"))

	msg := env.bot.lastKeyboardMessage()
	if !strings.Contains(msg, "portfolio") {
		t.Errorf("expected /pf to work as alias, got: %s", msg)
	}
}

func TestPortfolio_NotSetup(t *testing.T) {
	env := newTestEnv()
	env.userRepo.seed(12345, "testuser", false)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/portfolio"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "setup") {
		t.Errorf("expected setup prompt, got: %s", msg)
	}
}

func TestPortfolio_ShowsUSDValues(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/portfolio"))

	msg := env.bot.lastKeyboardMessage()
	// USDT is a stablecoin, should show 1:1 usd value
	if !strings.Contains(msg, "USDT") {
		t.Errorf("expected USDT in portfolio, got: %s", msg)
	}
	// should contain dollar sign for estimated values
	if !strings.Contains(msg, "$") {
		t.Errorf("expected dollar values in portfolio, got: %s", msg)
	}
}

func TestPortfolio_HasRefreshButton(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/portfolio"))

	kb := env.bot.lastKeyboard()
	if kb == nil {
		t.Fatal("expected inline keyboard on portfolio")
	}
	found := false
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == "portfolio" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected portfolio refresh button")
	}
}

// --- help includes portfolio ---

func TestHelp_IncludesPortfolio(t *testing.T) {
	env := newTestEnv()
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/help"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "portfolio") {
		t.Errorf("expected portfolio in help text, got: %s", msg)
	}
}

// --- callback query takes priority over message ---

func TestCallbackQuery_TakesPriority(t *testing.T) {
	env := newTestEnv()

	// update has both a message and a callback query
	update := Update{
		UpdateID: 1,
		Message:  &Message{MessageID: 1, From: &From{ID: 12345}, Chat: &Chat{ID: 100}, Text: "/help"},
		CallbackQuery: &CallbackQuery{
			ID:      "cb-456",
			From:    &From{ID: 12345},
			Message: &Message{MessageID: 42, Chat: &Chat{ID: 100}},
			Data:    "price:BTC/USDT",
		},
	}
	env.handler.HandleUpdate(context.Background(), update)

	// callback should be processed, not the /help message
	if len(env.bot.edits) == 0 {
		t.Error("expected callback to be processed (edit message)")
	}
	// /help would use SendMessage, should not have triggered
	helpFound := false
	for _, m := range env.bot.messages {
		if strings.Contains(m.text, "available commands") {
			helpFound = true
		}
	}
	if helpFound {
		t.Error("expected /help NOT to be processed when callback query is present")
	}
}
