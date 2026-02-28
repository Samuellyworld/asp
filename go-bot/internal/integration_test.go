// cross-platform integration tests for phase 1
// verifies: register on telegram, link discord, watchlist sync, price parity
package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/discord"
	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/preferences"
	"github.com/trading-bot/go-bot/internal/user"
	"github.com/trading-bot/go-bot/internal/watchlist"
)

// --- mock user repository ---

type mockUserRepo struct {
	users        map[int64]*user.User
	discordUsers map[int64]*user.User
	credentials  map[int]*user.Credentials
	nextID       int
	prefsErr     error
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
	return m.users[telegramID], nil
}

func (m *mockUserRepo) FindByDiscordID(_ context.Context, discordID int64) (*user.User, error) {
	return m.discordUsers[discordID], nil
}

func (m *mockUserRepo) Create(_ context.Context, telegramID int64, username string) (*user.User, error) {
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
	return nil
}

func (m *mockUserRepo) LinkDiscordToTelegram(_ context.Context, telegramID, discordID int64) (*user.User, error) {
	u, ok := m.users[telegramID]
	if !ok {
		return nil, nil
	}
	u.DiscordID = &discordID
	m.discordUsers[discordID] = u
	return u, nil
}

func (m *mockUserRepo) Activate(_ context.Context, userID int) error {
	for _, u := range m.users {
		if u.ID == userID {
			u.IsActivated = true
		}
	}
	for _, u := range m.discordUsers {
		if u.ID == userID {
			u.IsActivated = true
		}
	}
	return nil
}

func (m *mockUserRepo) SaveCredentials(_ context.Context, cred *user.Credentials) (*user.Credentials, error) {
	cred.ID = m.nextID
	m.nextID++
	cred.CreatedAt = time.Now()
	m.credentials[cred.UserID] = cred
	return cred, nil
}

func (m *mockUserRepo) HasValidCredentials(_ context.Context, userID int) (bool, error) {
	_, ok := m.credentials[userID]
	return ok, nil
}

func (m *mockUserRepo) GetCredentials(_ context.Context, userID int, _ string) (*user.Credentials, error) {
	return m.credentials[userID], nil
}

// --- mock encryptor ---

type mockEncryptor struct{}

func (m *mockEncryptor) Encrypt(plaintext []byte, _ []byte) ([]byte, error) {
	return append([]byte("enc:"), plaintext...), nil
}

func (m *mockEncryptor) Decrypt(ciphertext []byte, _ []byte) ([]byte, error) {
	if len(ciphertext) < 4 {
		return nil, fmt.Errorf("invalid ciphertext")
	}
	return ciphertext[4:], nil
}

// --- mock key validator ---

type mockValidator struct {
	perms *binance.APIPermissions
}

func (m *mockValidator) ValidateKeys(_ context.Context, _, _ string) (*binance.APIPermissions, error) {
	return m.perms, nil
}

// --- mock watchlist repository ---

type mockWatchlistRepo struct {
	items map[int][]watchlist.Item
}

func newMockWatchlistRepo() *mockWatchlistRepo {
	return &mockWatchlistRepo{items: make(map[int][]watchlist.Item)}
}

func (m *mockWatchlistRepo) GetByUserID(_ context.Context, userID int) ([]watchlist.Item, error) {
	return m.items[userID], nil
}

func (m *mockWatchlistRepo) Add(_ context.Context, userID int, symbol string) error {
	m.items[userID] = append(m.items[userID], watchlist.Item{UserID: userID, Symbol: symbol})
	return nil
}

func (m *mockWatchlistRepo) Remove(_ context.Context, userID int, symbol string) error {
	items := m.items[userID]
	for i, item := range items {
		if item.Symbol == symbol {
			m.items[userID] = append(items[:i], items[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockWatchlistRepo) Exists(_ context.Context, userID int, symbol string) (bool, error) {
	for _, item := range m.items[userID] {
		if item.Symbol == symbol {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockWatchlistRepo) Count(_ context.Context, userID int) (int, error) {
	return len(m.items[userID]), nil
}

func (m *mockWatchlistRepo) Reset(_ context.Context, userID int) error {
	m.items[userID] = []watchlist.Item{
		{UserID: userID, Symbol: "BTC/USDT"},
		{UserID: userID, Symbol: "ETH/USDT"},
		{UserID: userID, Symbol: "BNB/USDT"},
		{UserID: userID, Symbol: "SOL/USDT"},
		{UserID: userID, Symbol: "XRP/USDT"},
		{UserID: userID, Symbol: "ADA/USDT"},
		{UserID: userID, Symbol: "DOGE/USDT"},
		{UserID: userID, Symbol: "AVAX/USDT"},
		{UserID: userID, Symbol: "DOT/USDT"},
		{UserID: userID, Symbol: "MATIC/USDT"},
	}
	return nil
}

// --- mock preferences repository ---

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
	p := m.scanning[userID]
	if p == nil {
		return &preferences.Scanning{
			UserID:            userID,
			IsScanningEnabled: true,
			MinConfidence:     65,
			ScanIntervalMins:  5,
			EnabledTimeframes: []string{"5m", "15m", "1h"},
		}, nil
	}
	return p, nil
}

func (m *mockPrefsRepo) UpdateScanning(_ context.Context, s *preferences.Scanning) error {
	m.scanning[s.UserID] = s
	return nil
}

func (m *mockPrefsRepo) GetNotification(_ context.Context, userID int) (*preferences.Notification, error) {
	p := m.notification[userID]
	if p == nil {
		return &preferences.Notification{
			UserID:                userID,
			MaxDailyNotifications: 10,
			Timezone:              "UTC",
			DailySummaryHour:      9,
		}, nil
	}
	return p, nil
}

func (m *mockPrefsRepo) UpdateNotification(_ context.Context, n *preferences.Notification) error {
	m.notification[n.UserID] = n
	return nil
}

func (m *mockPrefsRepo) GetTrading(_ context.Context, userID int) (*preferences.Trading, error) {
	p := m.trading[userID]
	if p == nil {
		return &preferences.Trading{
			UserID:               userID,
			DefaultPositionSize:  100.0,
			MaxPositionSize:      500.0,
			DefaultStopLossPct:   2.0,
			DefaultTakeProfitPct: 4.0,
			MaxLeverage:          5,
			RiskPerTradePct:      1.0,
		}, nil
	}
	return p, nil
}

func (m *mockPrefsRepo) UpdateTrading(_ context.Context, t *preferences.Trading) error {
	m.trading[t.UserID] = t
	return nil
}

// --- mock exchange client ---

type mockExchange struct{}

func (m *mockExchange) GetPrice(_ context.Context, symbol string) (*exchange.Ticker, error) {
	prices := map[string]float64{
		"BTC/USDT":  42450.0,
		"ETH/USDT":  2380.0,
		"SOL/USDT":  98.50,
		"DOGE/USDT": 0.082,
	}
	price, ok := prices[symbol]
	if !ok {
		return nil, fmt.Errorf("unknown symbol: %s", symbol)
	}
	return &exchange.Ticker{
		Symbol:    symbol,
		Price:     price,
		Volume:    1000000,
		ChangePct: -1.5,
	}, nil
}

func (m *mockExchange) GetOrderBook(_ context.Context, symbol string, depth int) (*exchange.OrderBook, error) {
	return &exchange.OrderBook{
		Symbol: symbol,
		Bids:   []exchange.OrderBookEntry{{Price: 42449, Quantity: 0.5}},
		Asks:   []exchange.OrderBookEntry{{Price: 42451, Quantity: 0.8}},
	}, nil
}

func (m *mockExchange) GetBalance(_ context.Context, _, _ string) ([]exchange.Balance, error) {
	return []exchange.Balance{
		{Asset: "USDT", Free: 1000.0, Locked: 0},
		{Asset: "BTC", Free: 0.05, Locked: 0},
	}, nil
}

// --- mock discord bot ---

type mockDiscordBot struct {
	responses []*discord.InteractionResponse
	messages  []string
	embeds    []discord.Embed
}

func (m *mockDiscordBot) SendMessage(_ string, content string) error {
	m.messages = append(m.messages, content)
	return nil
}

func (m *mockDiscordBot) SendEmbed(_ string, _ string, embeds []discord.Embed, _ []discord.Component) error {
	m.embeds = append(m.embeds, embeds...)
	return nil
}

func (m *mockDiscordBot) RespondInteraction(_, _ string, resp *discord.InteractionResponse) error {
	m.responses = append(m.responses, resp)
	if resp.Data != nil {
		if resp.Data.Content != "" {
			m.messages = append(m.messages, resp.Data.Content)
		}
		m.embeds = append(m.embeds, resp.Data.Embeds...)
	}
	return nil
}

func (m *mockDiscordBot) EditInteractionResponse(_ string, content string, embeds []discord.Embed, _ []discord.Component) error {
	m.messages = append(m.messages, content)
	m.embeds = append(m.embeds, embeds...)
	return nil
}

// --- test helper ---

func makeDiscordInteraction(discordID, name string, options ...discord.ApplicationCommandOption) *discord.Interaction {
	return &discord.Interaction{
		ID:    "intx-" + name,
		Token: "token-" + name,
		Type:  discord.InteractionCommand,
		User:  &discord.DiscordUser{ID: discordID, Username: "discord_user"},
		Data: &discord.InteractionData{
			Name:    name,
			Options: options,
		},
	}
}

func strOpt(name, value string) discord.ApplicationCommandOption {
	return discord.ApplicationCommandOption{Name: name, Value: value}
}

// === INTEGRATION TESTS ===

// test 1: register on telegram, setup keys, verify user is created and activated
func TestIntegration_RegisterTelegram_SetupKeys(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)

	ctx := context.Background()
	telegramID := int64(99999)

	// step 1: register
	result, err := userSvc.Register(ctx, telegramID, "telegram_user")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if !result.IsNewUser {
		t.Error("expected new user")
	}
	if result.User.TelegramID == nil || *result.User.TelegramID != telegramID {
		t.Error("telegram id not set")
	}

	userID := result.User.ID

	// step 2: setup api keys
	setupResult, err := userSvc.SetupAPIKeys(ctx, userID, "TEST_KEY", "TEST_SECRET")
	if err != nil {
		t.Fatalf("setup keys failed: %v", err)
	}
	if !setupResult.Activated {
		t.Error("expected user to be activated")
	}
	if !setupResult.Permissions.Spot {
		t.Error("expected spot permission")
	}

	// verify user is activated
	activated, hasKeys, err := userSvc.GetStatus(ctx, userID)
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	if !activated || !hasKeys {
		t.Error("expected user to be activated with keys")
	}

	// verify credentials are encrypted (not plain text)
	cred := repo.credentials[userID]
	if cred == nil {
		t.Fatal("credentials not stored")
	}
	if string(cred.APIKeyEncrypted) == "TEST_KEY" {
		t.Error("api key stored in plain text")
	}
}

// test 2: register on telegram, add watchlist symbols, verify list
func TestIntegration_TelegramWatchlist(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)
	watchRepo := newMockWatchlistRepo()
	watchSvc := watchlist.NewService(watchRepo)

	ctx := context.Background()

	// register + setup
	result, _ := userSvc.Register(ctx, 99999, "telegram_user")
	userSvc.SetupAPIKeys(ctx, result.User.ID, "KEY", "SECRET")
	userID := result.User.ID

	// reset to default watchlist (simulates initial setup)
	if err := watchSvc.Reset(ctx, userID); err != nil {
		t.Fatalf("reset failed: %v", err)
	}

	items, _ := watchSvc.List(ctx, userID)
	if len(items) != 10 {
		t.Fatalf("expected 10 default symbols, got %d", len(items))
	}

	// add LINK/USDT (not in default 10)
	if err := watchSvc.Add(ctx, userID, "LINK/USDT"); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	items, _ = watchSvc.List(ctx, userID)
	if len(items) != 11 {
		t.Fatalf("expected 11 symbols after add, got %d", len(items))
	}

	// verify LINK/USDT is in the list
	found := false
	for _, item := range items {
		if item.Symbol == "LINK/USDT" {
			found = true
			break
		}
	}
	if !found {
		t.Error("LINK/USDT not found in watchlist")
	}
}

// test 3: register on telegram, link discord, verify single user row with both platform ids
func TestIntegration_LinkDiscord_SingleUserRow(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)

	ctx := context.Background()
	telegramID := int64(99999)
	discordID := int64(88888)

	// register via telegram
	result, err := userSvc.Register(ctx, telegramID, "shared_user")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	userID := result.User.ID

	// link discord
	linked, err := userSvc.LinkDiscordToTelegram(ctx, telegramID, discordID)
	if err != nil {
		t.Fatalf("link discord failed: %v", err)
	}

	// verify same user id
	if linked.ID != userID {
		t.Errorf("expected same user id %d, got %d", userID, linked.ID)
	}

	// verify both platform ids set
	if linked.TelegramID == nil || *linked.TelegramID != telegramID {
		t.Error("telegram id not set on linked user")
	}
	if linked.DiscordID == nil || *linked.DiscordID != discordID {
		t.Error("discord id not set on linked user")
	}

	// verify only one user exists (not two separate rows)
	telegramUser, _ := userSvc.Register(ctx, telegramID, "shared_user")
	discordUser, _ := userSvc.RegisterDiscord(ctx, discordID, "shared_user")
	if telegramUser.User.ID != discordUser.User.ID {
		t.Errorf("telegram user id (%d) != discord user id (%d) — should be same user",
			telegramUser.User.ID, discordUser.User.ID)
	}
}

// test 4: register on telegram, add symbols, link discord, verify watchlist synced
func TestIntegration_WatchlistSyncAcrossPlatforms(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)
	watchRepo := newMockWatchlistRepo()
	watchSvc := watchlist.NewService(watchRepo)

	ctx := context.Background()
	telegramID := int64(99999)
	discordID := int64(88888)

	// step 1: register on telegram + setup keys
	result, _ := userSvc.Register(ctx, telegramID, "sync_user")
	userSvc.SetupAPIKeys(ctx, result.User.ID, "KEY", "SECRET")
	userID := result.User.ID

	// step 2: reset watchlist to defaults + add LINK/USDT from telegram
	watchSvc.Reset(ctx, userID)
	watchSvc.Add(ctx, userID, "LINK/USDT")

	telegramItems, _ := watchSvc.List(ctx, userID)
	if len(telegramItems) != 11 {
		t.Fatalf("expected 11 symbols from telegram, got %d", len(telegramItems))
	}

	// step 3: link discord
	linked, err := userSvc.LinkDiscordToTelegram(ctx, telegramID, discordID)
	if err != nil {
		t.Fatalf("link failed: %v", err)
	}

	// step 4: discord should see the same watchlist (same user id)
	discordItems, _ := watchSvc.List(ctx, linked.ID)
	if len(discordItems) != len(telegramItems) {
		t.Errorf("watchlist not synced: telegram has %d, discord has %d",
			len(telegramItems), len(discordItems))
	}

	// verify LINK/USDT visible on discord side
	linkFound := false
	for _, item := range discordItems {
		if item.Symbol == "LINK/USDT" {
			linkFound = true
			break
		}
	}
	if !linkFound {
		t.Error("LINK/USDT added on telegram not visible on discord")
	}

	// step 5: add another symbol from discord side
	if err := watchSvc.Add(ctx, linked.ID, "UNI/USDT"); err != nil {
		t.Fatalf("add from discord failed: %v", err)
	}

	// now telegram should also see it (same user id)
	telegramItems2, _ := watchSvc.List(ctx, userID)
	uniFound := false
	for _, item := range telegramItems2 {
		if item.Symbol == "UNI/USDT" {
			uniFound = true
			break
		}
	}
	if !uniFound {
		t.Error("UNI/USDT added on discord not visible on telegram")
	}

	if len(telegramItems2) != 12 {
		t.Errorf("expected 12 symbols total, got %d", len(telegramItems2))
	}
}

// test 5: prices match on both platforms (same exchange client)
func TestIntegration_PriceParityAcrossPlatforms(t *testing.T) {
	exch := &mockExchange{}
	ctx := context.Background()

	// same exchange returns same prices regardless of caller
	telegramPrice, err := exch.GetPrice(ctx, "BTC/USDT")
	if err != nil {
		t.Fatalf("telegram price failed: %v", err)
	}

	discordPrice, err := exch.GetPrice(ctx, "BTC/USDT")
	if err != nil {
		t.Fatalf("discord price failed: %v", err)
	}

	if telegramPrice.Price != discordPrice.Price {
		t.Errorf("price mismatch: telegram=%.2f, discord=%.2f",
			telegramPrice.Price, discordPrice.Price)
	}

	if telegramPrice.Symbol != discordPrice.Symbol {
		t.Error("symbol mismatch between platforms")
	}

	// verify multiple symbols
	for _, symbol := range []string{"BTC/USDT", "ETH/USDT", "SOL/USDT", "DOGE/USDT"} {
		p1, _ := exch.GetPrice(ctx, symbol)
		p2, _ := exch.GetPrice(ctx, symbol)
		if p1.Price != p2.Price {
			t.Errorf("price mismatch for %s: %.6f vs %.6f", symbol, p1.Price, p2.Price)
		}
	}
}

// test 6: full end-to-end via discord handler: /link + /watchlist
func TestIntegration_DiscordLink_ThenWatchlist(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)
	watchRepo := newMockWatchlistRepo()
	watchSvc := watchlist.NewService(watchRepo)
	prefsSvc := preferences.NewService(newMockPrefsRepo())
	exch := &mockExchange{}

	ctx := context.Background()
	telegramID := int64(99999)
	discordID := int64(88888)

	// step 1: register on telegram (via service directly, simulating telegram bot)
	result, _ := userSvc.Register(ctx, telegramID, "multi_platform_user")
	userSvc.SetupAPIKeys(ctx, result.User.ID, "KEY", "SECRET")
	userID := result.User.ID

	// step 2: add watchlist symbols on telegram side
	watchSvc.Reset(ctx, userID)
	watchSvc.Add(ctx, userID, "LINK/USDT")

	// step 3: discord user calls /link
	bot := &mockDiscordBot{}
	discordHandler := discord.NewHandler(bot, userSvc, watchSvc, prefsSvc, exch)

	linkInteraction := makeDiscordInteraction(
		fmt.Sprintf("%d", discordID),
		"link",
		strOpt("telegram_id", fmt.Sprintf("%d", telegramID)),
	)
	discordHandler.HandleInteraction(ctx, linkInteraction)

	// verify link response has embed
	if len(bot.embeds) == 0 {
		t.Fatal("expected link confirmation embed")
	}
	if bot.embeds[0].Title != "🔗 Accounts Linked" {
		t.Errorf("unexpected embed title: %s", bot.embeds[0].Title)
	}

	// step 4: discord user calls /watchlist — should see all 11 symbols
	bot2 := &mockDiscordBot{}
	discordHandler2 := discord.NewHandler(bot2, userSvc, watchSvc, prefsSvc, exch)

	// user is now linked so discord resolves to same user
	watchInteraction := makeDiscordInteraction(fmt.Sprintf("%d", discordID), "watchlist")
	discordHandler2.HandleInteraction(ctx, watchInteraction)

	// check that watchlist response contains data
	if len(bot2.embeds) == 0 {
		t.Fatal("expected watchlist embed after link")
	}

	// verify the user id is the same by checking watchlist count
	items, _ := watchSvc.List(ctx, userID)
	if len(items) != 11 {
		t.Errorf("expected 11 watchlist items, got %d", len(items))
	}

	// verify LINK/USDT visible in discord-side watchlist
	linkSeen := false
	for _, item := range items {
		if item.Symbol == "LINK/USDT" {
			linkSeen = true
			break
		}
	}
	if !linkSeen {
		t.Error("LINK/USDT added on telegram not visible after link")
	}
}

// test 7: link to non-existent telegram id fails
func TestIntegration_LinkDiscord_NoTelegramUser(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)

	ctx := context.Background()

	_, err := userSvc.LinkDiscordToTelegram(ctx, 99999, 88888)
	if err == nil {
		t.Error("expected error when linking to non-existent telegram user")
	}
}

// test 8: link same discord to two telegram users fails
func TestIntegration_LinkDiscord_AlreadyLinked(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)

	ctx := context.Background()

	// register two telegram users
	userSvc.Register(ctx, 11111, "user_one")
	userSvc.Register(ctx, 22222, "user_two")

	// link discord to first user
	_, err := userSvc.LinkDiscordToTelegram(ctx, 11111, 88888)
	if err != nil {
		t.Fatalf("first link failed: %v", err)
	}

	// try to link same discord to second user
	_, err = userSvc.LinkDiscordToTelegram(ctx, 22222, 88888)
	if err == nil {
		t.Error("expected error when discord already linked to another user")
	}
}

// test 9: link already linked discord to same user is idempotent
func TestIntegration_LinkDiscord_Idempotent(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)

	ctx := context.Background()
	telegramID := int64(99999)
	discordID := int64(88888)

	userSvc.Register(ctx, telegramID, "idem_user")

	// link once
	u1, err := userSvc.LinkDiscordToTelegram(ctx, telegramID, discordID)
	if err != nil {
		t.Fatalf("first link failed: %v", err)
	}

	// link again — should succeed and return same user
	u2, err := userSvc.LinkDiscordToTelegram(ctx, telegramID, discordID)
	if err != nil {
		t.Fatalf("second link failed: %v", err)
	}

	if u1.ID != u2.ID {
		t.Errorf("idempotent link returned different user ids: %d vs %d", u1.ID, u2.ID)
	}
}

// test 10: preferences shared across platforms after link
func TestIntegration_PreferencesSyncAfterLink(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)
	prefsRepo := newMockPrefsRepo()
	prefsSvc := preferences.NewService(prefsRepo)

	ctx := context.Background()
	telegramID := int64(99999)
	discordID := int64(88888)

	// register on telegram
	result, _ := userSvc.Register(ctx, telegramID, "prefs_user")
	userID := result.User.ID

	// set preferences on telegram side
	prefsSvc.SetMinConfidence(ctx, userID, 90)

	// link discord
	linked, err := userSvc.LinkDiscordToTelegram(ctx, telegramID, discordID)
	if err != nil {
		t.Fatalf("link failed: %v", err)
	}

	// read preferences from discord side (same user id)
	scanning, err := prefsSvc.GetScanning(ctx, linked.ID)
	if err != nil {
		t.Fatalf("get scanning failed: %v", err)
	}

	if scanning.MinConfidence != 90 {
		t.Errorf("expected confidence 90 on discord, got %d", scanning.MinConfidence)
	}
}

// test 11: api keys shared across platforms after link
func TestIntegration_APIKeysSharedAfterLink(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)

	ctx := context.Background()
	telegramID := int64(99999)
	discordID := int64(88888)

	// register + setup on telegram
	result, _ := userSvc.Register(ctx, telegramID, "keys_user")
	userSvc.SetupAPIKeys(ctx, result.User.ID, "MY_KEY", "MY_SECRET")

	// link discord
	linked, _ := userSvc.LinkDiscordToTelegram(ctx, telegramID, discordID)

	// discord side should see the same credentials
	activated, hasKeys, err := userSvc.GetStatus(ctx, linked.ID)
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	if !activated || !hasKeys {
		t.Error("expected linked discord user to have active status and api keys")
	}

	// decrypt from discord side
	key, secret, err := userSvc.GetDecryptedCredentials(ctx, linked.ID)
	if err != nil {
		t.Fatalf("decrypt from discord side failed: %v", err)
	}
	if key != "MY_KEY" || secret != "MY_SECRET" {
		t.Errorf("credential mismatch on discord: key=%s, secret=%s", key, secret)
	}
}

// test 12: discord /link via handler with invalid telegram id
func TestIntegration_DiscordLinkHandler_InvalidID(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)
	watchSvc := watchlist.NewService(newMockWatchlistRepo())
	prefsSvc := preferences.NewService(newMockPrefsRepo())
	exch := &mockExchange{}

	bot := &mockDiscordBot{}
	handler := discord.NewHandler(bot, userSvc, watchSvc, prefsSvc, exch)

	ctx := context.Background()

	// try to link with non-numeric telegram id
	interaction := makeDiscordInteraction("88888", "link", strOpt("telegram_id", "not_a_number"))
	handler.HandleInteraction(ctx, interaction)

	if len(bot.messages) == 0 {
		t.Fatal("expected error response")
	}

	found := false
	for _, msg := range bot.messages {
		if msg == "invalid telegram id. it should be a number." {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected invalid telegram id error, got: %v", bot.messages)
	}
}

// test 13: discord /link with non-existent telegram user via handler
func TestIntegration_DiscordLinkHandler_NoUser(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)
	watchSvc := watchlist.NewService(newMockWatchlistRepo())
	prefsSvc := preferences.NewService(newMockPrefsRepo())
	exch := &mockExchange{}

	bot := &mockDiscordBot{}
	handler := discord.NewHandler(bot, userSvc, watchSvc, prefsSvc, exch)

	ctx := context.Background()

	// try to link to non-existent user
	interaction := makeDiscordInteraction("88888", "link", strOpt("telegram_id", "99999"))
	handler.HandleInteraction(ctx, interaction)

	if len(bot.messages) == 0 {
		t.Fatal("expected error response")
	}

	errorFound := false
	for _, msg := range bot.messages {
		if len(msg) > 0 && msg[0] == 0xe2 { // starts with ❌
			errorFound = true
			break
		}
	}
	if !errorFound {
		t.Errorf("expected error message with ❌, got: %v", bot.messages)
	}
}

// test 14: no duplicate users after link — verify user count
func TestIntegration_NoDuplicateUsers(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	userSvc := user.NewService(repo, enc, nil, validator, false)

	ctx := context.Background()
	telegramID := int64(99999)
	discordID := int64(88888)

	// register on telegram
	userSvc.Register(ctx, telegramID, "dup_user")

	// count users in repo before link
	usersBefore := len(repo.users)

	// link discord
	userSvc.LinkDiscordToTelegram(ctx, telegramID, discordID)

	// users map should not grow (same user object is shared)
	usersAfter := len(repo.users)
	if usersAfter != usersBefore {
		t.Errorf("user count changed after link: before=%d, after=%d", usersBefore, usersAfter)
	}

	// discord user lookup should find the same user as telegram
	tgUser, _ := repo.FindByTelegramID(ctx, telegramID)
	dcUser, _ := repo.FindByDiscordID(ctx, discordID)
	if tgUser == nil || dcUser == nil {
		t.Fatal("expected both lookups to succeed")
	}
	if tgUser.ID != dcUser.ID {
		t.Errorf("different user ids: telegram=%d, discord=%d", tgUser.ID, dcUser.ID)
	}
}

// test 15: discord slash commands definition includes /link
func TestIntegration_LinkCommand_Registered(t *testing.T) {
	commands := discord.SlashCommands()
	found := false
	for _, cmd := range commands {
		if cmd.Name == "link" {
			found = true
			if len(cmd.Options) == 0 {
				t.Error("/link command has no options")
			}
			if cmd.Options[0].Name != "telegram_id" {
				t.Errorf("expected first option 'telegram_id', got '%s'", cmd.Options[0].Name)
			}
			break
		}
	}
	if !found {
		t.Error("/link command not found in slash commands")
	}
}
