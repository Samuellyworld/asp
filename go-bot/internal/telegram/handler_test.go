package telegram

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/preferences"
	"github.com/trading-bot/go-bot/internal/user"
	"github.com/trading-bot/go-bot/internal/watchlist"
)

// --- mock bot ---

type sentMessage struct {
	chatID int64
	text   string
}

type mockBot struct {
	messages []sentMessage
	deletes  []int
	sendErr  error
}

func (m *mockBot) SendMessage(chatID int64, text string) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.messages = append(m.messages, sentMessage{chatID, text})
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

func (m *mockBot) reset() {
	m.messages = nil
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

	userSvc := user.NewService(userRepo, &mockEncryptor{}, nil, validator, false)
	wizard := user.NewSetupWizard()
	watchSvc := watchlist.NewService(watchRepo)
	prefsSvc := preferences.NewService(prefsRepo)

	handler := NewHandler(bot, userSvc, wizard, watchSvc, prefsSvc)

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

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "BTC/USDT") {
		t.Errorf("expected BTC/USDT in watchlist, got: %s", msg)
	}
	if !strings.Contains(msg, "ETH/USDT") {
		t.Errorf("expected ETH/USDT in watchlist, got: %s", msg)
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

func TestWatchReset_Success(t *testing.T) {
	env := newTestEnv()
	env.seedActivatedUser(12345)

	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "/watchreset"))

	msg := env.bot.lastMessage()
	if !strings.Contains(msg, "reset") {
		t.Errorf("expected reset message, got: %s", msg)
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

	// send api key
	env.handler.HandleUpdate(context.Background(), makeUpdate(12345, 100, "my-api-key-12345"))
	if len(env.bot.deletes) == 0 {
		t.Error("expected api key message to be deleted")
	}
	msg := env.bot.lastMessage()
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
