package scanner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/pipeline"
	"github.com/trading-bot/go-bot/internal/preferences"
	"github.com/trading-bot/go-bot/internal/user"
	"github.com/trading-bot/go-bot/internal/watchlist"
)

// --- mocks ---

type mockUserProvider struct {
	users []*user.User
	err   error
}

func (m *mockUserProvider) ListActive(_ context.Context) ([]*user.User, error) {
	return m.users, m.err
}

type mockWatchlistProvider struct {
	items map[int][]watchlist.Item
}

func (m *mockWatchlistProvider) List(_ context.Context, userID int) ([]watchlist.Item, error) {
	return m.items[userID], nil
}

type mockPrefsProvider struct {
	scanning     map[int]*preferences.Scanning
	notification map[int]*preferences.Notification
}

func (m *mockPrefsProvider) GetScanning(_ context.Context, userID int) (*preferences.Scanning, error) {
	if s, ok := m.scanning[userID]; ok {
		return s, nil
	}
	return nil, nil
}

func (m *mockPrefsProvider) GetNotification(_ context.Context, userID int) (*preferences.Notification, error) {
	if n, ok := m.notification[userID]; ok {
		return n, nil
	}
	return &preferences.Notification{MaxDailyNotifications: 10, OpportunityNotifications: true}, nil
}

type mockAnalyzer struct {
	results map[string]*pipeline.Result
	err     error
	mu      sync.Mutex
	calls   []string
}

func (m *mockAnalyzer) Analyze(_ context.Context, symbol string) (*pipeline.Result, error) {
	m.mu.Lock()
	m.calls = append(m.calls, symbol)
	m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}
	if r, ok := m.results[symbol]; ok {
		return r, nil
	}
	return &pipeline.Result{Symbol: symbol}, nil
}

func (m *mockAnalyzer) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

type notification struct {
	platform string
	target   string
	message  string
}

type mockNotifier struct {
	mu            sync.Mutex
	notifications []notification
	telegramErr   error
	discordErr    error
}

func (m *mockNotifier) NotifyTelegram(chatID int64, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, notification{
		platform: "telegram",
		target:   fmt.Sprintf("%d", chatID),
		message:  message,
	})
	return m.telegramErr
}

func (m *mockNotifier) NotifyDiscord(channelID string, title, description string, fields []pipeline.DiscordField, color int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, notification{
		platform: "discord",
		target:   channelID,
		message:  fmt.Sprintf("%s: %s", title, description),
	})
	return m.discordErr
}

func (m *mockNotifier) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.notifications)
}

func (m *mockNotifier) telegramCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, n := range m.notifications {
		if n.platform == "telegram" {
			count++
		}
	}
	return count
}

func (m *mockNotifier) discordCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, n := range m.notifications {
		if n.platform == "discord" {
			count++
		}
	}
	return count
}

// --- test helpers ---

func ptr[T any](v T) *T { return &v }

func testUser(id int, telegramID int64) *user.User {
	return &user.User{
		ID:          id,
		TelegramID:  ptr(telegramID),
		IsActivated: true,
	}
}

func testUserBoth(id int, telegramID int64, discordID int64) *user.User {
	return &user.User{
		ID:          id,
		TelegramID:  ptr(telegramID),
		DiscordID:   ptr(discordID),
		IsActivated: true,
	}
}

func testScanPrefs(userID int) *preferences.Scanning {
	return &preferences.Scanning{
		UserID:            userID,
		MinConfidence:     80,
		IsScanningEnabled: true,
		UseMLPredictions:  true,
		UseSentiment:      true,
	}
}

func testNotifPrefs(userID int) *preferences.Notification {
	return &preferences.Notification{
		UserID:                   userID,
		MaxDailyNotifications:    10,
		OpportunityNotifications: true,
	}
}

func buyResult(symbol string, confidence float64) *pipeline.Result {
	return &pipeline.Result{
		Symbol: symbol,
		Ticker: &exchange.Ticker{Symbol: symbol, Price: 42000},
		Decision: &claude.Decision{
			Action:     claude.ActionBuy,
			Confidence: confidence,
			Plan: claude.TradePlan{
				Entry:      42000,
				StopLoss:   41500,
				TakeProfit: 43000,
				RiskReward: 2.0,
			},
			Reasoning: "test buy signal",
		},
	}
}

func sellResult(symbol string, confidence float64) *pipeline.Result {
	return &pipeline.Result{
		Symbol: symbol,
		Ticker: &exchange.Ticker{Symbol: symbol, Price: 42000},
		Decision: &claude.Decision{
			Action:     claude.ActionSell,
			Confidence: confidence,
			Reasoning:  "test sell signal",
		},
	}
}

func holdResult(symbol string) *pipeline.Result {
	return &pipeline.Result{
		Symbol: symbol,
		Ticker: &exchange.Ticker{Symbol: symbol, Price: 42000},
		Decision: &claude.Decision{
			Action:     claude.ActionHold,
			Confidence: 50,
			Reasoning:  "mixed signals",
		},
	}
}

func testScanner(users []*user.User, items map[int][]watchlist.Item, results map[string]*pipeline.Result) (*Scanner, *mockNotifier, *mockAnalyzer) {
	userIDs := make(map[int]*preferences.Scanning)
	notifIDs := make(map[int]*preferences.Notification)
	for _, u := range users {
		userIDs[u.ID] = testScanPrefs(u.ID)
		notifIDs[u.ID] = testNotifPrefs(u.ID)
	}

	analyzer := &mockAnalyzer{results: results}
	notifier := &mockNotifier{}

	s := New(
		&mockUserProvider{users: users},
		&mockWatchlistProvider{items: items},
		&mockPrefsProvider{scanning: userIDs, notification: notifIDs},
		analyzer,
		notifier,
		DefaultConfig(),
	)

	return s, notifier, analyzer
}

// --- tests ---

func TestScanCycleNotifiesBuySignal(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 85),
	}

	s, notifier, _ := testScanner(users, items, results)
	s.runCycle(context.Background())

	if notifier.count() != 1 {
		t.Fatalf("expected 1 notification, got %d", notifier.count())
	}
	if notifier.telegramCount() != 1 {
		t.Error("expected telegram notification")
	}
	if s.DailyCount(1) != 1 {
		t.Errorf("expected daily count 1, got %d", s.DailyCount(1))
	}
}

func TestScanCycleNotifiesBothPlatforms(t *testing.T) {
	users := []*user.User{testUserBoth(1, 100, 200)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 90),
	}

	s, notifier, _ := testScanner(users, items, results)
	s.runCycle(context.Background())

	if notifier.telegramCount() != 1 {
		t.Error("expected telegram notification")
	}
	if notifier.discordCount() != 1 {
		t.Error("expected discord notification")
	}
}

func TestScanCycleSkipsHold(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": holdResult("BTC/USDT"),
	}

	s, notifier, _ := testScanner(users, items, results)
	s.runCycle(context.Background())

	if notifier.count() != 0 {
		t.Errorf("expected no notifications for hold, got %d", notifier.count())
	}
}

func TestScanCycleSkipsLowConfidence(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 60), // below 80% threshold
	}

	s, notifier, _ := testScanner(users, items, results)
	s.runCycle(context.Background())

	if notifier.count() != 0 {
		t.Errorf("expected no notifications for low confidence, got %d", notifier.count())
	}
}

func TestScanCycleRespectsCustomConfidence(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 70),
	}

	analyzer := &mockAnalyzer{results: results}
	notifier := &mockNotifier{}
	s := New(
		&mockUserProvider{users: users},
		&mockWatchlistProvider{items: items},
		&mockPrefsProvider{
			scanning: map[int]*preferences.Scanning{
				1: {UserID: 1, MinConfidence: 60, IsScanningEnabled: true},
			},
			notification: map[int]*preferences.Notification{
				1: testNotifPrefs(1),
			},
		},
		analyzer,
		notifier,
		DefaultConfig(),
	)
	s.runCycle(context.Background())

	if notifier.count() != 1 {
		t.Errorf("expected notification with custom threshold 60, got %d notifs", notifier.count())
	}
}

func TestDailyLimitEnforced(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	symbols := make([]watchlist.Item, 15)
	results := make(map[string]*pipeline.Result)
	for i := 0; i < 15; i++ {
		sym := fmt.Sprintf("SYM%d/USDT", i)
		symbols[i] = watchlist.Item{Symbol: sym, IsActive: true}
		results[sym] = buyResult(sym, 90)
	}
	items := map[int][]watchlist.Item{1: symbols}

	s, notifier, _ := testScanner(users, items, results)
	s.runCycle(context.Background())

	if s.DailyCount(1) != 10 {
		t.Errorf("expected daily count capped at 10, got %d", s.DailyCount(1))
	}
	if notifier.count() != 10 {
		t.Errorf("expected 10 notifications (daily limit), got %d", notifier.count())
	}
}

func TestDailyLimitCustom(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	symbols := make([]watchlist.Item, 8)
	results := make(map[string]*pipeline.Result)
	for i := 0; i < 8; i++ {
		sym := fmt.Sprintf("SYM%d/USDT", i)
		symbols[i] = watchlist.Item{Symbol: sym, IsActive: true}
		results[sym] = buyResult(sym, 90)
	}
	items := map[int][]watchlist.Item{1: symbols}

	analyzer := &mockAnalyzer{results: results}
	notifier := &mockNotifier{}
	s := New(
		&mockUserProvider{users: users},
		&mockWatchlistProvider{items: items},
		&mockPrefsProvider{
			scanning:     map[int]*preferences.Scanning{1: testScanPrefs(1)},
			notification: map[int]*preferences.Notification{1: {UserID: 1, MaxDailyNotifications: 3, OpportunityNotifications: true}},
		},
		analyzer,
		notifier,
		DefaultConfig(),
	)
	s.runCycle(context.Background())

	if s.DailyCount(1) != 3 {
		t.Errorf("expected daily count capped at 3, got %d", s.DailyCount(1))
	}
}

func TestDuplicateSuppression(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 85),
	}

	s, notifier, _ := testScanner(users, items, results)

	// first cycle: should notify
	s.runCycle(context.Background())
	if notifier.count() != 1 {
		t.Fatalf("expected 1 notification on first cycle, got %d", notifier.count())
	}

	// second cycle: same symbol+direction within 1h, should suppress
	s.runCycle(context.Background())
	if notifier.count() != 1 {
		t.Errorf("expected still 1 notification (duplicate suppressed), got %d", notifier.count())
	}
	if s.DailyCount(1) != 1 {
		t.Errorf("expected daily count still 1, got %d", s.DailyCount(1))
	}
}

func TestDuplicateAllowsDifferentDirection(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}

	// first cycle: buy
	results1 := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 85),
	}
	s, notifier, analyzer := testScanner(users, items, results1)
	s.runCycle(context.Background())

	// second cycle: sell (different direction)
	analyzer.results = map[string]*pipeline.Result{
		"BTC/USDT": sellResult("BTC/USDT", 85),
	}
	s.runCycle(context.Background())

	if notifier.count() != 2 {
		t.Errorf("expected 2 notifications (different directions), got %d", notifier.count())
	}
}

func TestDuplicateExpiresAfterWindow(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 85),
	}

	s, notifier, _ := testScanner(users, items, results)

	// mock time
	now := time.Now()
	s.nowFunc = func() time.Time { return now }

	s.runCycle(context.Background())
	if notifier.count() != 1 {
		t.Fatal("expected 1 notification")
	}

	// advance time past the duplicate window (60 min default)
	now = now.Add(61 * time.Minute)
	s.runCycle(context.Background())

	if notifier.count() != 2 {
		t.Errorf("expected 2 notifications after window expired, got %d", notifier.count())
	}
}

func TestMidnightReset(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 85),
	}

	s, _, _ := testScanner(users, items, results)
	s.runCycle(context.Background())

	if s.DailyCount(1) != 1 {
		t.Fatal("expected daily count 1")
	}

	s.ResetDaily()

	if s.DailyCount(1) != 0 {
		t.Errorf("expected daily count 0 after reset, got %d", s.DailyCount(1))
	}
}

func TestResetDailyPrunesOldNotifications(t *testing.T) {
	s := &Scanner{
		states: map[int]*userState{
			1: {
				dailyCount: 5,
				recentNotifs: []recentNotification{
					{symbol: "OLD", direction: "BUY", sentAt: time.Now().Add(-2 * time.Hour)},
					{symbol: "RECENT", direction: "BUY", sentAt: time.Now()},
				},
			},
		},
		config:  DefaultConfig(),
		nowFunc: time.Now,
	}

	s.ResetDaily()

	if s.states[1].dailyCount != 0 {
		t.Error("daily count should be 0")
	}
	if len(s.states[1].recentNotifs) != 1 {
		t.Errorf("expected 1 recent notif after pruning, got %d", len(s.states[1].recentNotifs))
	}
	if s.states[1].recentNotifs[0].symbol != "RECENT" {
		t.Error("should keep the recent notification")
	}
}

func TestScanCycleSkipsBannedUser(t *testing.T) {
	users := []*user.User{
		{ID: 1, TelegramID: ptr(int64(100)), IsActivated: true, IsBanned: true},
	}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 90),
	}

	s, notifier, analyzer := testScanner(users, items, results)
	s.runCycle(context.Background())

	if notifier.count() != 0 {
		t.Error("banned user should not receive notifications")
	}
	if analyzer.callCount() != 0 {
		t.Error("should not analyze for banned user")
	}
}

func TestScanCycleSkipsInactiveUser(t *testing.T) {
	users := []*user.User{
		{ID: 1, TelegramID: ptr(int64(100)), IsActivated: false},
	}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 90),
	}

	s, notifier, _ := testScanner(users, items, results)
	s.runCycle(context.Background())

	if notifier.count() != 0 {
		t.Error("inactive user should not receive notifications")
	}
}

func TestScanCycleSkipsScanningDisabled(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 90),
	}

	analyzer := &mockAnalyzer{results: results}
	notifier := &mockNotifier{}
	s := New(
		&mockUserProvider{users: users},
		&mockWatchlistProvider{items: items},
		&mockPrefsProvider{
			scanning: map[int]*preferences.Scanning{
				1: {UserID: 1, IsScanningEnabled: false},
			},
			notification: map[int]*preferences.Notification{
				1: testNotifPrefs(1),
			},
		},
		analyzer,
		notifier,
		DefaultConfig(),
	)
	s.runCycle(context.Background())

	if notifier.count() != 0 {
		t.Error("should not notify when scanning disabled")
	}
}

func TestScanCycleSkipsInactiveSymbol(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: false}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 90),
	}

	s, notifier, analyzer := testScanner(users, items, results)
	s.runCycle(context.Background())

	if notifier.count() != 0 {
		t.Error("inactive symbol should not trigger notification")
	}
	if analyzer.callCount() != 0 {
		t.Error("inactive symbol should not be analyzed")
	}
}

func TestScanCycleSkipsOpportunityNotificationsDisabled(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 90),
	}

	analyzer := &mockAnalyzer{results: results}
	notifier := &mockNotifier{}
	s := New(
		&mockUserProvider{users: users},
		&mockWatchlistProvider{items: items},
		&mockPrefsProvider{
			scanning: map[int]*preferences.Scanning{1: testScanPrefs(1)},
			notification: map[int]*preferences.Notification{
				1: {UserID: 1, MaxDailyNotifications: 10, OpportunityNotifications: false},
			},
		},
		analyzer,
		notifier,
		DefaultConfig(),
	)
	s.runCycle(context.Background())

	if notifier.count() != 0 {
		t.Error("should not notify when opportunity notifications disabled")
	}
}

func TestScanCycleMultipleUsers(t *testing.T) {
	users := []*user.User{
		testUser(1, 100),
		testUser(2, 200),
		testUser(3, 300),
	}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
		2: {{Symbol: "ETH/USDT", IsActive: true}},
		3: {{Symbol: "BTC/USDT", IsActive: true}, {Symbol: "SOL/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 85),
		"ETH/USDT": buyResult("ETH/USDT", 90),
		"SOL/USDT": buyResult("SOL/USDT", 82),
	}

	s, notifier, analyzer := testScanner(users, items, results)
	s.runCycle(context.Background())

	// 4 total: user1(BTC) + user2(ETH) + user3(BTC, SOL)
	if analyzer.callCount() != 4 {
		t.Errorf("expected 4 analyses, got %d", analyzer.callCount())
	}
	if notifier.count() != 4 {
		t.Errorf("expected 4 notifications, got %d", notifier.count())
	}
	if s.DailyCount(1) != 1 {
		t.Errorf("user 1 daily count: expected 1, got %d", s.DailyCount(1))
	}
	if s.DailyCount(3) != 2 {
		t.Errorf("user 3 daily count: expected 2, got %d", s.DailyCount(3))
	}
}

func TestScanCycleAnalyzerError(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}

	analyzer := &mockAnalyzer{err: fmt.Errorf("rust engine down")}
	notifier := &mockNotifier{}
	s := New(
		&mockUserProvider{users: users},
		&mockWatchlistProvider{items: items},
		&mockPrefsProvider{
			scanning:     map[int]*preferences.Scanning{1: testScanPrefs(1)},
			notification: map[int]*preferences.Notification{1: testNotifPrefs(1)},
		},
		analyzer,
		notifier,
		DefaultConfig(),
	)
	s.runCycle(context.Background())

	if notifier.count() != 0 {
		t.Error("should not notify on analyzer error")
	}
}

func TestScanCycleUserProviderError(t *testing.T) {
	analyzer := &mockAnalyzer{}
	notifier := &mockNotifier{}
	s := New(
		&mockUserProvider{err: fmt.Errorf("db down")},
		&mockWatchlistProvider{items: map[int][]watchlist.Item{}},
		&mockPrefsProvider{
			scanning:     map[int]*preferences.Scanning{},
			notification: map[int]*preferences.Notification{},
		},
		analyzer,
		notifier,
		DefaultConfig(),
	)
	s.runCycle(context.Background())

	if notifier.count() != 0 {
		t.Error("should not notify on user provider error")
	}
	if s.CycleCount() != 0 {
		t.Error("cycle count should not increment on error")
	}
}

func TestScanCycleSellSignal(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": sellResult("BTC/USDT", 88),
	}

	s, notifier, _ := testScanner(users, items, results)
	s.runCycle(context.Background())

	if notifier.count() != 1 {
		t.Errorf("expected 1 notification for sell signal, got %d", notifier.count())
	}
}

func TestCycleCountIncrements(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": holdResult("BTC/USDT"),
	}

	s, _, _ := testScanner(users, items, results)

	s.runCycle(context.Background())
	if s.CycleCount() != 1 {
		t.Errorf("expected cycle count 1, got %d", s.CycleCount())
	}

	s.runCycle(context.Background())
	if s.CycleCount() != 2 {
		t.Errorf("expected cycle count 2, got %d", s.CycleCount())
	}
}

func TestStartStop(t *testing.T) {
	users := []*user.User{}
	s, _, _ := testScanner(users, map[int][]watchlist.Item{}, map[string]*pipeline.Result{})
	s.config.Interval = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	if !s.IsRunning() {
		t.Error("scanner should be running")
	}

	// double start should be idempotent
	s.Start(ctx)

	time.Sleep(50 * time.Millisecond)
	s.Stop()

	if s.IsRunning() {
		t.Error("scanner should be stopped")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Interval != 5*time.Minute {
		t.Errorf("expected 5m interval, got %v", cfg.Interval)
	}
	if cfg.DefaultMaxDaily != 10 {
		t.Errorf("expected max daily 10, got %d", cfg.DefaultMaxDaily)
	}
	if cfg.DefaultMinConfidence != 80 {
		t.Errorf("expected min confidence 80, got %d", cfg.DefaultMinConfidence)
	}
	if cfg.DuplicateWindowMins != 60 {
		t.Errorf("expected duplicate window 60, got %d", cfg.DuplicateWindowMins)
	}
}

func TestNotificationContainsOpportunityHeader(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 85),
	}

	s, notifier, _ := testScanner(users, items, results)
	s.runCycle(context.Background())

	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if len(notifier.notifications) == 0 {
		t.Fatal("expected at least 1 notification")
	}
	msg := notifier.notifications[0].message
	if !strings.Contains(msg, "Opportunity") {
		t.Error("telegram message should contain opportunity header")
	}
}

func TestIsDuplicateDirectly(t *testing.T) {
	s := &Scanner{
		states:  make(map[int]*userState),
		config:  DefaultConfig(),
		nowFunc: time.Now,
	}

	// no state yet — not duplicate
	if s.isDuplicate(1, "BTC/USDT", claude.ActionBuy) {
		t.Error("should not be duplicate with no history")
	}

	// record one
	s.recordNotification(1, "BTC/USDT", claude.ActionBuy)

	// same symbol+direction — duplicate
	if !s.isDuplicate(1, "BTC/USDT", claude.ActionBuy) {
		t.Error("should be duplicate")
	}

	// different symbol — not duplicate
	if s.isDuplicate(1, "ETH/USDT", claude.ActionBuy) {
		t.Error("different symbol should not be duplicate")
	}

	// same symbol, different direction — not duplicate
	if s.isDuplicate(1, "BTC/USDT", claude.ActionSell) {
		t.Error("different direction should not be duplicate")
	}

	// different user — not duplicate
	if s.isDuplicate(2, "BTC/USDT", claude.ActionBuy) {
		t.Error("different user should not be duplicate")
	}
}

func TestDailyLimitReachedDirectly(t *testing.T) {
	s := &Scanner{
		states: map[int]*userState{
			1: {dailyCount: 9},
			2: {dailyCount: 10},
		},
		config:  DefaultConfig(),
		nowFunc: time.Now,
	}

	if s.dailyLimitReached(1, 10) {
		t.Error("9/10 should not be at limit")
	}
	if !s.dailyLimitReached(2, 10) {
		t.Error("10/10 should be at limit")
	}
	if s.dailyLimitReached(3, 10) {
		t.Error("unknown user should not be at limit")
	}
}

func TestNilDecisionSkipped(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": {Symbol: "BTC/USDT"}, // nil decision
	}

	s, notifier, _ := testScanner(users, items, results)
	s.runCycle(context.Background())

	if notifier.count() != 0 {
		t.Error("nil decision should not trigger notification")
	}
}

func TestTelegramNotifyError(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {{Symbol: "BTC/USDT", IsActive: true}},
	}
	results := map[string]*pipeline.Result{
		"BTC/USDT": buyResult("BTC/USDT", 85),
	}

	analyzer := &mockAnalyzer{results: results}
	notifier := &mockNotifier{telegramErr: fmt.Errorf("telegram api down")}
	s := New(
		&mockUserProvider{users: users},
		&mockWatchlistProvider{items: items},
		&mockPrefsProvider{
			scanning:     map[int]*preferences.Scanning{1: testScanPrefs(1)},
			notification: map[int]*preferences.Notification{1: testNotifPrefs(1)},
		},
		analyzer,
		notifier,
		DefaultConfig(),
	)
	s.runCycle(context.Background())

	// notification still counted even if delivery failed
	if s.DailyCount(1) != 1 {
		t.Errorf("expected daily count 1 even on telegram error, got %d", s.DailyCount(1))
	}
}

func TestEmptyWatchlist(t *testing.T) {
	users := []*user.User{testUser(1, 100)}
	items := map[int][]watchlist.Item{
		1: {},
	}

	s, notifier, analyzer := testScanner(users, items, map[string]*pipeline.Result{})
	s.runCycle(context.Background())

	if analyzer.callCount() != 0 {
		t.Error("should not analyze with empty watchlist")
	}
	if notifier.count() != 0 {
		t.Error("should not notify with empty watchlist")
	}
	if s.CycleCount() != 1 {
		t.Error("cycle should still complete")
	}
}
