package opportunity

import (
	"strings"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/pipeline"
	"github.com/trading-bot/go-bot/internal/user"
)

// --- test helpers ---

func ptr[T any](v T) *T { return &v }

func testResult(symbol string, action claude.Action, confidence float64) *pipeline.Result {
	return &pipeline.Result{
		Symbol: symbol,
		Ticker: &exchange.Ticker{Symbol: symbol, Price: 42000},
		Decision: &claude.Decision{
			Action:     action,
			Confidence: confidence,
			Plan: claude.TradePlan{
				Entry:        42000,
				StopLoss:     41500,
				TakeProfit:   43500,
				PositionSize: 500,
				RiskReward:   3.0,
			},
			Reasoning: "strong technical setup",
		},
	}
}

func testManager() *Manager {
	return NewManager(DefaultConfig())
}

// --- manager tests ---

func TestCreateOpportunity(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)

	id := m.Create(1, "BTC/USDT", result, "telegram")

	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if !strings.HasPrefix(id, "opp_1_") {
		t.Errorf("expected id prefix opp_1_, got %s", id)
	}

	opp := m.Get(id)
	if opp == nil {
		t.Fatal("expected opportunity")
	}
	if opp.Status != StatusPending {
		t.Errorf("expected pending, got %s", opp.Status)
	}
	if opp.UserID != 1 {
		t.Errorf("expected user 1, got %d", opp.UserID)
	}
	if opp.Symbol != "BTC/USDT" {
		t.Errorf("expected BTC/USDT, got %s", opp.Symbol)
	}
	if opp.Action != claude.ActionBuy {
		t.Errorf("expected BUY, got %s", opp.Action)
	}
	if opp.Platform != "telegram" {
		t.Errorf("expected telegram, got %s", opp.Platform)
	}
}

func TestCreateMultipleOpportunities(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)

	id1 := m.Create(1, "BTC/USDT", result, "telegram")
	id2 := m.Create(1, "ETH/USDT", result, "telegram")
	id3 := m.Create(2, "BTC/USDT", result, "discord")

	if id1 == id2 || id2 == id3 {
		t.Error("ids should be unique")
	}
	if m.Count() != 3 {
		t.Errorf("expected 3 opportunities, got %d", m.Count())
	}
}

func TestGetForUser(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)

	id := m.Create(1, "BTC/USDT", result, "telegram")

	// correct user
	opp := m.GetForUser(id, 1)
	if opp == nil {
		t.Fatal("expected opportunity for user 1")
	}

	// wrong user
	opp = m.GetForUser(id, 2)
	if opp != nil {
		t.Error("should not return opportunity for wrong user")
	}

	// nonexistent id
	opp = m.GetForUser("nonexistent", 1)
	if opp != nil {
		t.Error("should not return opportunity for bad id")
	}
}

func TestPendingForUser(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)

	m.Create(1, "BTC/USDT", result, "telegram")
	m.Create(1, "ETH/USDT", result, "telegram")
	m.Create(2, "SOL/USDT", result, "discord")

	pending := m.PendingForUser(1)
	if len(pending) != 2 {
		t.Errorf("expected 2 pending for user 1, got %d", len(pending))
	}

	pending = m.PendingForUser(2)
	if len(pending) != 1 {
		t.Errorf("expected 1 pending for user 2, got %d", len(pending))
	}

	pending = m.PendingForUser(99)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending for user 99, got %d", len(pending))
	}
}

func TestApprove(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")

	ok := m.Approve(id, 1)
	if !ok {
		t.Fatal("approve should succeed")
	}

	opp := m.Get(id)
	if opp.Status != StatusApproved {
		t.Errorf("expected approved, got %s", opp.Status)
	}
	if opp.ResolvedAt == nil {
		t.Error("resolved at should be set")
	}
}

func TestApproveWrongUser(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")

	ok := m.Approve(id, 2)
	if ok {
		t.Error("approve should fail for wrong user")
	}

	opp := m.Get(id)
	if opp.Status != StatusPending {
		t.Errorf("should still be pending, got %s", opp.Status)
	}
}

func TestApproveAlreadyResolved(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")

	m.Approve(id, 1)
	ok := m.Approve(id, 1) // second attempt
	if ok {
		t.Error("should not approve already resolved opportunity")
	}
}

func TestReject(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")

	ok := m.Reject(id, 1)
	if !ok {
		t.Fatal("reject should succeed")
	}

	opp := m.Get(id)
	if opp.Status != StatusRejected {
		t.Errorf("expected rejected, got %s", opp.Status)
	}
}

func TestRejectWrongUser(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")

	ok := m.Reject(id, 999)
	if ok {
		t.Error("reject should fail for wrong user")
	}
}

func TestRejectNonexistent(t *testing.T) {
	m := testManager()
	ok := m.Reject("opp_fake", 1)
	if ok {
		t.Error("reject should fail for nonexistent opportunity")
	}
}

func TestModify(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")

	plan := &claude.TradePlan{
		Entry:        42100,
		StopLoss:     41700,
		TakeProfit:   43800,
		PositionSize: 300,
		RiskReward:   4.25,
	}

	ok := m.Modify(id, 1, plan)
	if !ok {
		t.Fatal("modify should succeed")
	}

	opp := m.Get(id)
	if opp.Status != StatusModified {
		t.Errorf("expected modified, got %s", opp.Status)
	}
	if opp.ModifiedPlan == nil {
		t.Fatal("modified plan should be set")
	}
	if opp.ModifiedPlan.Entry != 42100 {
		t.Errorf("expected entry 42100, got %.2f", opp.ModifiedPlan.Entry)
	}
	if opp.ModifiedPlan.PositionSize != 300 {
		t.Errorf("expected position 300, got %.2f", opp.ModifiedPlan.PositionSize)
	}
}

func TestModifyWrongUser(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")

	ok := m.Modify(id, 2, &claude.TradePlan{})
	if ok {
		t.Error("modify should fail for wrong user")
	}
}

func TestModifyAlreadyResolved(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")

	m.Reject(id, 1)
	ok := m.Modify(id, 1, &claude.TradePlan{})
	if ok {
		t.Error("should not modify already resolved opportunity")
	}
}

func TestSetMessageID(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")

	m.SetMessageID(id, 12345)

	opp := m.Get(id)
	if opp.MessageID != 12345 {
		t.Errorf("expected message id 12345, got %d", opp.MessageID)
	}
}

func TestSetChannelID(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "discord")

	m.SetChannelID(id, "channel_abc")

	opp := m.Get(id)
	if opp.ChannelID != "channel_abc" {
		t.Errorf("expected channel_abc, got %s", opp.ChannelID)
	}
}

// --- expiry tests ---

func TestExpireOld(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)

	now := time.Now()
	m.nowFunc = func() time.Time { return now }

	id := m.Create(1, "BTC/USDT", result, "telegram")

	// advance past expiry (15 min default)
	now = now.Add(16 * time.Minute)
	m.expireOld()

	opp := m.Get(id)
	if opp.Status != StatusExpired {
		t.Errorf("expected expired, got %s", opp.Status)
	}
	if opp.ResolvedAt == nil {
		t.Error("resolved at should be set")
	}
}

func TestExpireOldDoesNotExpireFresh(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)

	now := time.Now()
	m.nowFunc = func() time.Time { return now }

	id := m.Create(1, "BTC/USDT", result, "telegram")

	// only 5 min passed — should not expire
	now = now.Add(5 * time.Minute)
	m.expireOld()

	opp := m.Get(id)
	if opp.Status != StatusPending {
		t.Errorf("expected still pending after 5 min, got %s", opp.Status)
	}
}

func TestExpireOldDoesNotExpireResolved(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)

	now := time.Now()
	m.nowFunc = func() time.Time { return now }

	id := m.Create(1, "BTC/USDT", result, "telegram")
	m.Approve(id, 1)

	// advance past expiry
	now = now.Add(20 * time.Minute)
	m.expireOld()

	opp := m.Get(id)
	if opp.Status != StatusApproved {
		t.Errorf("approved opportunity should not be expired, got %s", opp.Status)
	}
}

func TestExpireCallback(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)

	now := time.Now()
	m.nowFunc = func() time.Time { return now }

	m.Create(1, "BTC/USDT", result, "telegram")

	var expiredOpp *Opportunity
	m.OnExpire(func(opp *Opportunity) {
		expiredOpp = opp
	})

	now = now.Add(16 * time.Minute)
	m.expireOld()

	if expiredOpp == nil {
		t.Fatal("expire callback should have fired")
	}
	if expiredOpp.Symbol != "BTC/USDT" {
		t.Errorf("expected BTC/USDT, got %s", expiredOpp.Symbol)
	}
}

func TestExpireMultiple(t *testing.T) {
	m := testManager()
	now := time.Now()
	m.nowFunc = func() time.Time { return now }

	r := testResult("BTC/USDT", claude.ActionBuy, 85)
	m.Create(1, "BTC/USDT", r, "telegram") // will expire
	m.Create(1, "ETH/USDT", r, "telegram") // will expire

	// create one that's "newer"
	now = now.Add(10 * time.Minute)
	m.Create(1, "SOL/USDT", r, "telegram") // 10 min newer

	// advance to 16 min from start — first two expire, third doesn't
	now = now.Add(6 * time.Minute) // total: 16 min from start

	count := 0
	m.OnExpire(func(opp *Opportunity) { count++ })
	m.expireOld()

	if count != 2 {
		t.Errorf("expected 2 expired, got %d", count)
	}

	pending := m.PendingForUser(1)
	if len(pending) != 1 {
		t.Errorf("expected 1 still pending, got %d", len(pending))
	}
}

// --- cleanup tests ---

func TestCleanup(t *testing.T) {
	m := testManager()
	now := time.Now()
	m.nowFunc = func() time.Time { return now }

	r := testResult("BTC/USDT", claude.ActionBuy, 85)
	id1 := m.Create(1, "BTC/USDT", r, "telegram")
	id2 := m.Create(1, "ETH/USDT", r, "telegram")
	m.Create(1, "SOL/USDT", r, "telegram") // stays pending

	m.Approve(id1, 1)
	m.Reject(id2, 1)

	// advance 2 hours
	now = now.Add(2 * time.Hour)
	removed := m.Cleanup(1 * time.Hour)

	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}
	if m.Count() != 1 {
		t.Errorf("expected 1 remaining, got %d", m.Count())
	}
}

func TestCleanupDoesNotRemoveRecent(t *testing.T) {
	m := testManager()
	r := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", r, "telegram")
	m.Approve(id, 1)

	removed := m.Cleanup(1 * time.Hour)
	if removed != 0 {
		t.Errorf("should not remove recently resolved, got %d", removed)
	}
}

// --- stats tests ---

func TestStats(t *testing.T) {
	m := testManager()
	r := testResult("BTC/USDT", claude.ActionBuy, 85)

	id1 := m.Create(1, "BTC/USDT", r, "telegram")
	m.Create(1, "ETH/USDT", r, "telegram")
	id3 := m.Create(1, "SOL/USDT", r, "telegram")

	m.Approve(id1, 1)
	m.Reject(id3, 1)

	stats := m.Stats()
	if stats[StatusPending] != 1 {
		t.Errorf("expected 1 pending, got %d", stats[StatusPending])
	}
	if stats[StatusApproved] != 1 {
		t.Errorf("expected 1 approved, got %d", stats[StatusApproved])
	}
	if stats[StatusRejected] != 1 {
		t.Errorf("expected 1 rejected, got %d", stats[StatusRejected])
	}
}

// --- start/stop tests ---

func TestStartStopExpiry(t *testing.T) {
	m := NewManager(Config{
		ExpiryDuration:  100 * time.Millisecond,
		CleanupInterval: 50 * time.Millisecond,
	})

	m.StartExpiry()

	// double start should be idempotent
	m.StartExpiry()

	time.Sleep(30 * time.Millisecond)
	m.StopExpiry()
}

// --- default config tests ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ExpiryDuration != 15*time.Minute {
		t.Errorf("expected 15m expiry, got %v", cfg.ExpiryDuration)
	}
	if cfg.CleanupInterval != 1*time.Minute {
		t.Errorf("expected 1m cleanup interval, got %v", cfg.CleanupInterval)
	}
}

// --- channel routing tests ---

func TestResolveChannelTelegramOnly(t *testing.T) {
	u := &user.User{TelegramID: ptr(int64(100))}
	ch := ResolveChannel(u)
	if ch != "telegram" {
		t.Errorf("expected telegram, got %s", ch)
	}
}

func TestResolveChannelDiscordOnly(t *testing.T) {
	u := &user.User{DiscordID: ptr(int64(200))}
	ch := ResolveChannel(u)
	if ch != "discord" {
		t.Errorf("expected discord, got %s", ch)
	}
}

func TestResolveChannelBothLastTelegram(t *testing.T) {
	u := &user.User{
		TelegramID:        ptr(int64(100)),
		DiscordID:         ptr(int64(200)),
		LastActiveChannel: ptr("telegram"),
	}
	ch := ResolveChannel(u)
	if ch != "telegram" {
		t.Errorf("expected telegram, got %s", ch)
	}
}

func TestResolveChannelBothLastDiscord(t *testing.T) {
	u := &user.User{
		TelegramID:        ptr(int64(100)),
		DiscordID:         ptr(int64(200)),
		LastActiveChannel: ptr("discord"),
	}
	ch := ResolveChannel(u)
	if ch != "discord" {
		t.Errorf("expected discord, got %s", ch)
	}
}

func TestResolveChannelBothNoLastActive(t *testing.T) {
	u := &user.User{
		TelegramID: ptr(int64(100)),
		DiscordID:  ptr(int64(200)),
	}
	ch := ResolveChannel(u)
	if ch != "telegram" {
		t.Errorf("expected telegram as default, got %s", ch)
	}
}

func TestResolveChannelNeither(t *testing.T) {
	u := &user.User{}
	ch := ResolveChannel(u)
	if ch != "" {
		t.Errorf("expected empty, got %s", ch)
	}
}

func TestResolveChannelCaseInsensitive(t *testing.T) {
	u := &user.User{
		TelegramID:        ptr(int64(100)),
		DiscordID:         ptr(int64(200)),
		LastActiveChannel: ptr("Discord"),
	}
	ch := ResolveChannel(u)
	if ch != "discord" {
		t.Errorf("expected discord (case insensitive), got %s", ch)
	}
}

// --- notification format tests ---

func TestFormatTelegramOpportunity(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")
	opp := m.Get(id)

	msg := FormatTelegramOpportunity(opp)
	checks := []string{"Opportunity Detected", "BTC/USDT", "BUY", "15 minutes"}
	for _, check := range checks {
		if !strings.Contains(msg, check) {
			t.Errorf("expected %q in message, got:\n%s", check, msg)
		}
	}
}

func TestTelegramButtons(t *testing.T) {
	buttons := TelegramButtons("opp_1_1")
	if len(buttons) != 1 {
		t.Fatalf("expected 1 row, got %d", len(buttons))
	}
	if len(buttons[0]) != 3 {
		t.Fatalf("expected 3 buttons, got %d", len(buttons[0]))
	}

	if !strings.Contains(buttons[0][0].Data, "opp_approve:opp_1_1") {
		t.Errorf("approve button data wrong: %s", buttons[0][0].Data)
	}
	if !strings.Contains(buttons[0][1].Data, "opp_reject:opp_1_1") {
		t.Errorf("reject button data wrong: %s", buttons[0][1].Data)
	}
	if !strings.Contains(buttons[0][2].Data, "opp_modify:opp_1_1") {
		t.Errorf("modify button data wrong: %s", buttons[0][2].Data)
	}
}

func TestDiscordButtons(t *testing.T) {
	buttons := DiscordButtons("opp_1_1")
	if len(buttons) != 3 {
		t.Fatalf("expected 3 buttons, got %d", len(buttons))
	}

	if buttons[0].Style != ButtonStyleSuccess {
		t.Errorf("approve should be success style, got %d", buttons[0].Style)
	}
	if buttons[1].Style != ButtonStyleDanger {
		t.Errorf("reject should be danger style, got %d", buttons[1].Style)
	}
	if buttons[2].Style != ButtonStyleSecondary {
		t.Errorf("modify should be secondary style, got %d", buttons[2].Style)
	}
}

func TestModifyButtons(t *testing.T) {
	buttons := ModifyButtons("opp_1_1")
	if len(buttons) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(buttons))
	}
	if len(buttons[0]) != 2 {
		t.Errorf("first row should have 2 buttons, got %d", len(buttons[0]))
	}
	if len(buttons[2]) != 2 {
		t.Errorf("third row should have 2 buttons, got %d", len(buttons[2]))
	}
	if !strings.Contains(buttons[2][0].Data, "opp_mod_confirm") {
		t.Error("should have confirm button")
	}
}

func TestFormatExpiredMessage(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")
	opp := m.Get(id)

	msg := FormatExpiredMessage(opp)
	if !strings.Contains(msg, "Expired") {
		t.Error("should contain Expired")
	}
	if !strings.Contains(msg, "BTC/USDT") {
		t.Error("should contain symbol")
	}
	if !strings.Contains(msg, "BUY") {
		t.Error("should contain action")
	}
}

func TestFormatApprovedMessage(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")
	m.Approve(id, 1)
	opp := m.Get(id)

	msg := FormatApprovedMessage(opp)
	if !strings.Contains(msg, "Approved") {
		t.Error("should contain Approved")
	}
	if !strings.Contains(msg, "42000") {
		t.Error("should contain entry price")
	}
	if !strings.Contains(msg, "R/R") {
		t.Error("should contain risk-reward")
	}
}

func TestFormatApprovedMessageWithModifiedPlan(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")
	m.Modify(id, 1, &claude.TradePlan{
		Entry: 42500, StopLoss: 42000, TakeProfit: 44000, PositionSize: 200, RiskReward: 3.0,
	})
	opp := m.Get(id)

	msg := FormatApprovedMessage(opp)
	if !strings.Contains(msg, "42500") {
		t.Error("should use modified entry price")
	}
}

func TestFormatRejectedMessage(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionSell, 90)
	id := m.Create(1, "BTC/USDT", result, "telegram")
	m.Reject(id, 1)
	opp := m.Get(id)

	msg := FormatRejectedMessage(opp)
	if !strings.Contains(msg, "Rejected") {
		t.Error("should contain Rejected")
	}
	if !strings.Contains(msg, "SELL") {
		t.Error("should contain action")
	}
	if !strings.Contains(msg, "🔴") {
		t.Error("should contain sell emoji")
	}
}

func TestFormatModifiedMessage(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")
	m.Modify(id, 1, &claude.TradePlan{
		Entry: 42100, StopLoss: 41700, TakeProfit: 43800, PositionSize: 300, RiskReward: 4.25,
	})
	opp := m.Get(id)

	msg := FormatModifiedMessage(opp)
	if !strings.Contains(msg, "Modified") {
		t.Error("should contain Modified")
	}
	if !strings.Contains(msg, "42100") {
		t.Error("should contain modified entry")
	}
}

func TestFormatModifiedMessageNilPlan(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")
	opp := m.Get(id)
	opp.Status = StatusModified

	msg := FormatModifiedMessage(opp)
	// should fall back to FormatApprovedMessage
	if !strings.Contains(msg, "Approved") {
		t.Error("nil modified plan should fall back to approved format")
	}
}

func TestStatusEmoji(t *testing.T) {
	if statusEmoji(claude.ActionBuy) != "🟢" {
		t.Error("buy should be green")
	}
	if statusEmoji(claude.ActionSell) != "🔴" {
		t.Error("sell should be red")
	}
	if statusEmoji(claude.ActionHold) != "⏸️" {
		t.Error("hold should be pause")
	}
}

// --- edge case tests ---

func TestApproveNonexistent(t *testing.T) {
	m := testManager()
	ok := m.Approve("fake_id", 1)
	if ok {
		t.Error("should not approve nonexistent")
	}
}

func TestGetNonexistent(t *testing.T) {
	m := testManager()
	opp := m.Get("fake_id")
	if opp != nil {
		t.Error("should return nil for nonexistent")
	}
}

func TestSetMessageIDNonexistent(t *testing.T) {
	m := testManager()
	// should not panic
	m.SetMessageID("fake_id", 123)
}

func TestSetChannelIDNonexistent(t *testing.T) {
	m := testManager()
	// should not panic
	m.SetChannelID("fake_id", "ch_123")
}

func TestRejectThenApprove(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)
	id := m.Create(1, "BTC/USDT", result, "telegram")

	m.Reject(id, 1)
	ok := m.Approve(id, 1)
	if ok {
		t.Error("should not approve after reject")
	}
}

func TestExpireThenApprove(t *testing.T) {
	m := testManager()
	result := testResult("BTC/USDT", claude.ActionBuy, 85)

	now := time.Now()
	m.nowFunc = func() time.Time { return now }

	id := m.Create(1, "BTC/USDT", result, "telegram")

	now = now.Add(16 * time.Minute)
	m.expireOld()

	ok := m.Approve(id, 1)
	if ok {
		t.Error("should not approve expired opportunity")
	}
}
