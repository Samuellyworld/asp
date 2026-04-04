package livetrading

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/opportunity"
	"github.com/trading-bot/go-bot/internal/pipeline"
)

// --- mocks ---

type mockOrders struct {
	mu           sync.Mutex
	nextID       int64
	orders       map[int64]*exchange.Order
	placeErr     error
	cancelErr    error
	placedCount  int
	cancelCount  int
	lastQuantity float64
}

func newMockOrders() *mockOrders {
	return &mockOrders{orders: make(map[int64]*exchange.Order)}
}

func (m *mockOrders) PlaceOrder(symbol string, side exchange.OrderSide, orderType exchange.OrderType, quantity, price float64, apiKey, apiSecret string) (*exchange.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.placeErr != nil {
		return nil, m.placeErr
	}

	m.nextID++
	m.placedCount++
	m.lastQuantity = quantity

	avgPrice := price
	if avgPrice == 0 {
		avgPrice = 42450 // default fill price
	}

	order := &exchange.Order{
		OrderID:     m.nextID,
		Symbol:      symbol,
		Side:        side,
		Type:        orderType,
		Status:      exchange.OrderStatusFilled,
		Quantity:    quantity,
		ExecutedQty: quantity,
		AvgPrice:    avgPrice,
		CreatedAt:   time.Now(),
	}
	m.orders[order.OrderID] = order
	return order, nil
}

func (m *mockOrders) PlaceStopLoss(symbol string, side exchange.OrderSide, quantity, stopPrice, price float64, apiKey, apiSecret string) (*exchange.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.placeErr != nil {
		return nil, m.placeErr
	}

	m.nextID++
	m.placedCount++

	order := &exchange.Order{
		OrderID:   m.nextID,
		Symbol:    symbol,
		Side:      side,
		Type:      exchange.OrderTypeStopLoss,
		Status:    exchange.OrderStatusNew,
		StopPrice: stopPrice,
		Price:     price,
		Quantity:  quantity,
	}
	m.orders[order.OrderID] = order
	return order, nil
}

func (m *mockOrders) PlaceTakeProfit(symbol string, side exchange.OrderSide, quantity, stopPrice, price float64, apiKey, apiSecret string) (*exchange.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.placeErr != nil {
		return nil, m.placeErr
	}

	m.nextID++
	m.placedCount++

	order := &exchange.Order{
		OrderID:   m.nextID,
		Symbol:    symbol,
		Side:      side,
		Type:      exchange.OrderTypeTakeProfit,
		Status:    exchange.OrderStatusNew,
		StopPrice: stopPrice,
		Price:     price,
		Quantity:  quantity,
	}
	m.orders[order.OrderID] = order
	return order, nil
}

func (m *mockOrders) CancelOrder(symbol string, orderID int64, apiKey, apiSecret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelCount++
	if m.cancelErr != nil {
		return m.cancelErr
	}
	delete(m.orders, orderID)
	return nil
}

func (m *mockOrders) GetOrder(symbol string, orderID int64, apiKey, apiSecret string) (*exchange.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if o, ok := m.orders[orderID]; ok {
		return o, nil
	}
	return nil, fmt.Errorf("order not found")
}

func (m *mockOrders) GetOpenOrders(symbol string, apiKey, apiSecret string) ([]exchange.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []exchange.Order
	for _, o := range m.orders {
		if o.Symbol == symbol && o.Status == exchange.OrderStatusNew {
			result = append(result, *o)
		}
	}
	return result, nil
}

type mockKeys struct {
	keys map[int][2]string // userID -> [key, secret]
	err  error
}

func newMockKeys() *mockKeys {
	return &mockKeys{keys: map[int][2]string{
		1: {"test_key_1", "test_secret_1"},
		2: {"test_key_2", "test_secret_2"},
	}}
}

func (m *mockKeys) DecryptKeys(userID int) (string, string, error) {
	if m.err != nil {
		return "", "", m.err
	}
	k, ok := m.keys[userID]
	if !ok {
		return "", "", fmt.Errorf("no keys for user %d", userID)
	}
	return k[0], k[1], nil
}

type mockBalance struct {
	balances map[int]float64
	err      error
}

func (m *mockBalance) GetAvailableBalance(userID int, asset string) (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.balances[userID], nil
}

type mockPositionCounter struct {
	counts map[int]int
}

func (m *mockPositionCounter) OpenPositionCount(userID int) int {
	return m.counts[userID]
}

// helper to create a test opportunity
func testOpp(symbol string, action claude.Action, sl, tp, size float64) *opportunity.Opportunity {
	return &opportunity.Opportunity{
		ID:     "opp_1",
		UserID: 1,
		Symbol: symbol,
		Action: action,
		Status: opportunity.StatusApproved,
		Result: &pipeline.Result{
			Symbol: symbol,
			Decision: &claude.Decision{
				Action:     action,
				Confidence: 85,
				Plan: claude.TradePlan{
					Entry:        42450,
					StopLoss:     sl,
					TakeProfit:   tp,
					PositionSize: size,
					RiskReward:   2.5,
				},
			},
		},
		Platform: "telegram",
	}
}

// --- confirmation manager tests ---

func TestConfirmation_Confirm(t *testing.T) {
	cm := NewConfirmationManager()

	if cm.IsConfirmed(1) {
		t.Fatal("should not be confirmed initially")
	}

	if cm.Confirm(1, "wrong phrase") {
		t.Fatal("wrong phrase should not confirm")
	}

	if cm.Confirm(1, DefaultConfirmPhrase) != true {
		t.Fatal("correct phrase should confirm")
	}

	if !cm.IsConfirmed(1) {
		t.Fatal("should be confirmed after correct phrase")
	}
}

func TestConfirmation_Revoke(t *testing.T) {
	cm := NewConfirmationManager()
	cm.Confirm(1, DefaultConfirmPhrase)

	cm.Revoke(1)
	if cm.IsConfirmed(1) {
		t.Fatal("should not be confirmed after revoke")
	}
}

func TestConfirmation_Phrase(t *testing.T) {
	cm := NewConfirmationManager()
	if cm.Phrase() != DefaultConfirmPhrase {
		t.Fatalf("expected '%s', got '%s'", DefaultConfirmPhrase, cm.Phrase())
	}
}

func TestConfirmation_GetConfirmation(t *testing.T) {
	cm := NewConfirmationManager()

	if cm.GetConfirmation(1) != nil {
		t.Fatal("should return nil for unconfirmed user")
	}

	cm.Confirm(1, DefaultConfirmPhrase)
	conf := cm.GetConfirmation(1)
	if conf == nil {
		t.Fatal("should return confirmation")
	}
	if !conf.Confirmed {
		t.Fatal("should be confirmed")
	}
	if conf.UserID != 1 {
		t.Fatalf("expected userID 1, got %d", conf.UserID)
	}
}

func TestConfirmation_MultipleUsers(t *testing.T) {
	cm := NewConfirmationManager()

	cm.Confirm(1, DefaultConfirmPhrase)
	cm.Confirm(2, DefaultConfirmPhrase)

	if !cm.IsConfirmed(1) || !cm.IsConfirmed(2) {
		t.Fatal("both users should be confirmed")
	}

	cm.Revoke(1)
	if cm.IsConfirmed(1) {
		t.Fatal("user 1 should not be confirmed")
	}
	if !cm.IsConfirmed(2) {
		t.Fatal("user 2 should still be confirmed")
	}
}

// --- safety checker tests ---

func TestSafety_AllPassBasic(t *testing.T) {
	cm := NewConfirmationManager()
	cm.Confirm(1, DefaultConfirmPhrase)

	config := DefaultSafetyConfig()
	balance := &mockBalance{balances: map[int]float64{1: 1000}}
	positions := &mockPositionCounter{counts: map[int]int{1: 0}}
	losses := NewLossTracker()

	checker := NewSafetyChecker(config, balance, positions, losses, cm)
	result := checker.Check(1, "BTCUSDT", 50, "USDT")

	if !result.Passed {
		t.Fatalf("expected all checks to pass, blocked: %s", result.Blocked)
	}
}

func TestSafety_NotConfirmed(t *testing.T) {
	cm := NewConfirmationManager()
	config := DefaultSafetyConfig()

	checker := NewSafetyChecker(config, nil, nil, nil, cm)
	result := checker.Check(1, "BTCUSDT", 50, "USDT")

	if result.Passed {
		t.Fatal("should fail when user not confirmed")
	}
	if !strings.Contains(result.Blocked, "not confirmed") {
		t.Fatalf("expected confirmation error, got: %s", result.Blocked)
	}
}

func TestSafety_OrderSizeTooSmall(t *testing.T) {
	cm := NewConfirmationManager()
	cm.Confirm(1, DefaultConfirmPhrase)
	config := DefaultSafetyConfig()

	checker := NewSafetyChecker(config, nil, nil, nil, cm)
	result := checker.Check(1, "BTCUSDT", 5, "USDT") // below $10 min

	if result.Passed {
		t.Fatal("should fail for small order")
	}
	if !strings.Contains(result.Blocked, "below minimum") {
		t.Fatalf("expected min size error, got: %s", result.Blocked)
	}
}

func TestSafety_OrderSizeTooLarge(t *testing.T) {
	cm := NewConfirmationManager()
	cm.Confirm(1, DefaultConfirmPhrase)
	config := DefaultSafetyConfig()

	checker := NewSafetyChecker(config, nil, nil, nil, cm)
	result := checker.Check(1, "BTCUSDT", 200, "USDT") // above $100 max

	if result.Passed {
		t.Fatal("should fail for large order")
	}
	if !strings.Contains(result.Blocked, "exceeds maximum") {
		t.Fatalf("expected max size error, got: %s", result.Blocked)
	}
}

func TestSafety_PositionSizeExceeded(t *testing.T) {
	cm := NewConfirmationManager()
	cm.Confirm(1, DefaultConfirmPhrase)
	config := DefaultSafetyConfig()
	config.MaxOrderSize = 200 // allow large orders but position limit still $100

	checker := NewSafetyChecker(config, nil, nil, nil, cm)
	result := checker.Check(1, "BTCUSDT", 150, "USDT")

	if result.Passed {
		t.Fatal("should fail when position exceeds max")
	}
}

func TestSafety_TooManyPositions(t *testing.T) {
	cm := NewConfirmationManager()
	cm.Confirm(1, DefaultConfirmPhrase)
	config := DefaultSafetyConfig()
	positions := &mockPositionCounter{counts: map[int]int{1: 3}} // at limit

	checker := NewSafetyChecker(config, nil, positions, nil, cm)
	result := checker.Check(1, "BTCUSDT", 50, "USDT")

	if result.Passed {
		t.Fatal("should fail when at position limit")
	}
	if !strings.Contains(result.Blocked, "open positions") {
		t.Fatalf("expected position count error, got: %s", result.Blocked)
	}
}

func TestSafety_InsufficientBalance(t *testing.T) {
	cm := NewConfirmationManager()
	cm.Confirm(1, DefaultConfirmPhrase)
	config := DefaultSafetyConfig()
	balance := &mockBalance{balances: map[int]float64{1: 30}}
	positions := &mockPositionCounter{counts: map[int]int{1: 0}}
	losses := NewLossTracker()

	checker := NewSafetyChecker(config, balance, positions, losses, cm)
	result := checker.Check(1, "BTCUSDT", 50, "USDT")

	if result.Passed {
		t.Fatal("should fail with insufficient balance")
	}
	if !strings.Contains(result.Blocked, "insufficient balance") {
		t.Fatalf("expected balance error, got: %s", result.Blocked)
	}
}

func TestSafety_BalanceError(t *testing.T) {
	cm := NewConfirmationManager()
	cm.Confirm(1, DefaultConfirmPhrase)
	config := DefaultSafetyConfig()
	balance := &mockBalance{err: fmt.Errorf("api error")}
	positions := &mockPositionCounter{counts: map[int]int{1: 0}}
	losses := NewLossTracker()

	checker := NewSafetyChecker(config, balance, positions, losses, cm)
	result := checker.Check(1, "BTCUSDT", 50, "USDT")

	if result.Passed {
		t.Fatal("should fail when balance check errors")
	}
}

func TestSafety_DailyLossLimitReached(t *testing.T) {
	cm := NewConfirmationManager()
	cm.Confirm(1, DefaultConfirmPhrase)
	config := DefaultSafetyConfig()
	balance := &mockBalance{balances: map[int]float64{1: 1000}}
	positions := &mockPositionCounter{counts: map[int]int{1: 0}}
	losses := NewLossTracker()
	losses.RecordLoss(1, -50) // full daily limit

	checker := NewSafetyChecker(config, balance, positions, losses, cm)
	result := checker.Check(1, "BTCUSDT", 50, "USDT")

	if result.Passed {
		t.Fatal("should fail when daily loss limit reached")
	}
	if !strings.Contains(result.Blocked, "daily loss limit") {
		t.Fatalf("expected loss limit error, got: %s", result.Blocked)
	}
}

func TestSafety_NoConfirmationRequired(t *testing.T) {
	config := DefaultSafetyConfig()
	config.RequireConfirmation = false
	balance := &mockBalance{balances: map[int]float64{1: 1000}}
	positions := &mockPositionCounter{counts: map[int]int{1: 0}}
	losses := NewLossTracker()

	checker := NewSafetyChecker(config, balance, positions, losses, nil)
	result := checker.Check(1, "BTCUSDT", 50, "USDT")

	if !result.Passed {
		t.Fatalf("should pass when confirmation not required, blocked: %s", result.Blocked)
	}
}

// --- loss tracker tests ---

func TestLossTracker_RecordAndQuery(t *testing.T) {
	tracker := NewLossTracker()

	tracker.RecordLoss(1, -10)
	tracker.RecordLoss(1, -5)

	loss := tracker.DailyLoss(1, time.Now())
	if loss != 15 {
		t.Fatalf("expected daily loss 15, got %.2f", loss)
	}
}

func TestLossTracker_IgnoresProfit(t *testing.T) {
	tracker := NewLossTracker()

	tracker.RecordLoss(1, 10) // positive - should be ignored
	tracker.RecordLoss(1, 0)  // zero - should be ignored

	loss := tracker.DailyLoss(1, time.Now())
	if loss != 0 {
		t.Fatalf("expected 0 loss, got %.2f", loss)
	}
}

func TestLossTracker_SeparateUsers(t *testing.T) {
	tracker := NewLossTracker()

	tracker.RecordLoss(1, -10)
	tracker.RecordLoss(2, -20)

	if tracker.DailyLoss(1, time.Now()) != 10 {
		t.Fatal("user 1 should have 10 loss")
	}
	if tracker.DailyLoss(2, time.Now()) != 20 {
		t.Fatal("user 2 should have 20 loss")
	}
}

func TestLossTracker_ResetDaily(t *testing.T) {
	tracker := NewLossTracker()
	tracker.RecordLoss(1, -10)
	// today's loss should survive reset
	tracker.ResetDaily()
	if tracker.DailyLoss(1, time.Now()) != 10 {
		t.Fatal("today's loss should survive reset")
	}
}

// --- executor tests ---

func TestExecutor_Execute(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()

	exec := NewExecutor(orders, keys, nil, nil)
	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)

	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if pos.Symbol != "BTCUSDT" {
		t.Fatalf("expected BTCUSDT, got %s", pos.Symbol)
	}
	if pos.Side != exchange.SideBuy {
		t.Fatalf("expected BUY, got %s", pos.Side)
	}
	if pos.MainOrderID == 0 {
		t.Fatal("main order id should be set")
	}
	if pos.SLOrderID == 0 {
		t.Fatal("sl order id should be set")
	}
	if pos.TPOrderID == 0 {
		t.Fatal("tp order id should be set")
	}
	if pos.Status != "open" {
		t.Fatalf("expected open, got %s", pos.Status)
	}

	// 3 orders: market + sl + tp
	if orders.placedCount != 3 {
		t.Fatalf("expected 3 orders placed, got %d", orders.placedCount)
	}
}

func TestExecutor_Execute_NotApproved(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	opp.Status = opportunity.StatusPending

	_, err := exec.Execute(opp)
	if err == nil {
		t.Fatal("expected error for pending opp")
	}
}

func TestExecutor_Execute_ModifiedPlan(t *testing.T) {
	orders := newMockOrders()
	exec := NewExecutor(orders, newMockKeys(), nil, nil)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	opp.Status = opportunity.StatusModified
	opp.ModifiedPlan = &claude.TradePlan{
		Entry: 42500, StopLoss: 42000, TakeProfit: 45000, PositionSize: 300,
	}

	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if pos.StopLoss != 42000 {
		t.Fatalf("expected modified sl 42000, got %.2f", pos.StopLoss)
	}
	if pos.TakeProfit != 45000 {
		t.Fatalf("expected modified tp 45000, got %.2f", pos.TakeProfit)
	}
}

func TestExecutor_Execute_SellSide(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	opp := testOpp("BTCUSDT", claude.ActionSell, 43000, 41000, 500)

	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if pos.Side != exchange.SideSell {
		t.Fatalf("expected SELL, got %s", pos.Side)
	}
}

func TestExecutor_Execute_KeyDecryptError(t *testing.T) {
	keys := newMockKeys()
	keys.err = fmt.Errorf("decryption failed")
	exec := NewExecutor(newMockOrders(), keys, nil, nil)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	_, err := exec.Execute(opp)
	if err == nil || !strings.Contains(err.Error(), "decrypt") {
		t.Fatalf("expected decrypt error, got: %v", err)
	}
}

func TestExecutor_Execute_OrderPlaceError(t *testing.T) {
	orders := newMockOrders()
	orders.placeErr = fmt.Errorf("exchange down")
	exec := NewExecutor(orders, newMockKeys(), nil, nil)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	_, err := exec.Execute(opp)
	if err == nil || !strings.Contains(err.Error(), "failed to place") {
		t.Fatalf("expected order error, got: %v", err)
	}
}

func TestExecutor_Execute_SafetyFails(t *testing.T) {
	cm := NewConfirmationManager()
	// not confirmed -> safety fails
	config := DefaultSafetyConfig()
	checker := NewSafetyChecker(config, nil, nil, nil, cm)

	exec := NewExecutor(newMockOrders(), newMockKeys(), checker, nil)
	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 50)

	_, err := exec.Execute(opp)
	if err == nil || !strings.Contains(err.Error(), "safety check failed") {
		t.Fatalf("expected safety failure, got: %v", err)
	}
}

func TestExecutor_Execute_InvalidSize(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 0)

	_, err := exec.Execute(opp)
	if err == nil {
		t.Fatal("expected error for zero size")
	}
}

func TestExecutor_Execute_NilResult(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	opp.Result = nil

	_, err := exec.Execute(opp)
	if err == nil {
		t.Fatal("expected error for nil result")
	}
}

func TestExecutor_Execute_NoSLTP(t *testing.T) {
	orders := newMockOrders()
	exec := NewExecutor(orders, newMockKeys(), nil, nil)
	opp := testOpp("BTCUSDT", claude.ActionBuy, 0, 0, 500) // no sl/tp

	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if pos.SLOrderID != 0 {
		t.Fatal("sl order should not be placed when sl=0")
	}
	if pos.TPOrderID != 0 {
		t.Fatal("tp order should not be placed when tp=0")
	}
	// only 1 market order
	if orders.placedCount != 1 {
		t.Fatalf("expected 1 order (market only), got %d", orders.placedCount)
	}
}

func TestExecutor_Close(t *testing.T) {
	orders := newMockOrders()
	exec := NewExecutor(orders, newMockKeys(), nil, nil)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	closed, err := exec.Close(pos.ID, "manual")
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if closed.Status != "closed" {
		t.Fatalf("expected closed, got %s", closed.Status)
	}
	if closed.CloseReason != "manual" {
		t.Fatalf("expected manual reason, got %s", closed.CloseReason)
	}
	if closed.ClosedAt == nil {
		t.Fatal("ClosedAt should be set")
	}
	// market + sl + tp placed, then sl + tp cancelled + close market = 3 cancels not counted separately
	if exec.Count() != 0 {
		t.Fatalf("expected 0 open, got %d", exec.Count())
	}
}

func TestExecutor_Close_NotFound(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	_, err := exec.Close("nonexistent", "manual")
	if err == nil {
		t.Fatal("expected error for missing position")
	}
}

func TestExecutor_Close_AlreadyClosed(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)
	exec.Close(pos.ID, "manual")

	_, err := exec.Close(pos.ID, "manual")
	if err == nil {
		t.Fatal("expected error for double close")
	}
}

func TestExecutor_Close_TracksLoss(t *testing.T) {
	orders := newMockOrders()
	losses := NewLossTracker()
	exec := NewExecutor(orders, newMockKeys(), nil, losses)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	// close at lower price = loss (mock returns 42450 avg price for closes too)
	exec.Close(pos.ID, "stop_loss")

	// since mock always fills at 42450 (same as entry), pnl = 0
	// the loss tracker should not record zero
	loss := losses.DailyLoss(1, time.Now())
	if loss != 0 {
		t.Fatalf("expected 0 loss (entry = close price), got %.2f", loss)
	}
}

func TestExecutor_Close_PnLCalculation_Buy(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	// entry is at 42450 (mock), close at 42450 (mock) -> pnl = 0
	closed, _ := exec.Close(pos.ID, "manual")
	if closed.PnL != 0 {
		t.Fatalf("expected 0 pnl (same price), got %.2f", closed.PnL)
	}
}

func TestExecutor_Get(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	got := exec.Get(pos.ID)
	if got == nil || got.ID != pos.ID {
		t.Fatal("should find open position")
	}

	exec.Close(pos.ID, "manual")
	got = exec.Get(pos.ID)
	if got == nil {
		t.Fatal("should find closed position")
	}
}

func TestExecutor_Get_NotFound(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	if exec.Get("nonexistent") != nil {
		t.Fatal("expected nil")
	}
}

func TestExecutor_OpenPositions(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)

	opp1 := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	opp1.UserID = 1
	exec.Execute(opp1)

	opp2 := testOpp("ETHUSDT", claude.ActionBuy, 2100, 2400, 200)
	opp2.UserID = 2
	exec.Execute(opp2)

	u1 := exec.OpenPositions(1)
	if len(u1) != 1 {
		t.Fatalf("expected 1 for user 1, got %d", len(u1))
	}
}

func TestExecutor_AllOpen(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	exec.Execute(testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500))
	exec.Execute(testOpp("ETHUSDT", claude.ActionBuy, 2100, 2400, 200))

	if len(exec.AllOpen()) != 2 {
		t.Fatalf("expected 2 open, got %d", len(exec.AllOpen()))
	}
}

func TestExecutor_OpenPositionCount(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	exec.Execute(testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500))

	if exec.OpenPositionCount(1) != 1 {
		t.Fatalf("expected 1, got %d", exec.OpenPositionCount(1))
	}
}

// --- emergency stop tests ---

func TestEmergencyStop_ClosesAllPositions(t *testing.T) {
	orders := newMockOrders()
	exec := NewExecutor(orders, newMockKeys(), nil, nil)

	exec.Execute(testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500))
	exec.Execute(testOpp("ETHUSDT", claude.ActionBuy, 2100, 2400, 200))

	if exec.Count() != 2 {
		t.Fatalf("expected 2 open, got %d", exec.Count())
	}

	es := NewEmergencyStop(exec)
	closed, errors := es.Execute(1)

	if len(errors) != 0 {
		t.Fatalf("expected no errors, got %v", errors)
	}
	if len(closed) != 2 {
		t.Fatalf("expected 2 closed, got %d", len(closed))
	}
	if exec.Count() != 0 {
		t.Fatalf("expected 0 open after emergency stop, got %d", exec.Count())
	}

	for _, p := range closed {
		if p.CloseReason != "emergency_stop" {
			t.Fatalf("expected emergency_stop reason, got %s", p.CloseReason)
		}
	}
}

func TestEmergencyStop_NoPositions(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	es := NewEmergencyStop(exec)

	closed, errors := es.Execute(1)
	if len(closed) != 0 || len(errors) != 0 {
		t.Fatal("should return empty for no positions")
	}
}

func TestEmergencyStop_OnlyUserPositions(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)

	opp1 := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	opp1.UserID = 1
	exec.Execute(opp1)

	opp2 := testOpp("ETHUSDT", claude.ActionBuy, 2100, 2400, 200)
	opp2.UserID = 2
	exec.Execute(opp2)

	es := NewEmergencyStop(exec)
	closed, _ := es.Execute(1) // only user 1

	if len(closed) != 1 {
		t.Fatalf("expected 1 closed (only user 1), got %d", len(closed))
	}
	if exec.Count() != 1 {
		t.Fatalf("user 2 position should remain, got %d open", exec.Count())
	}
}

func TestEmergencyStop_CallsOnStop(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	exec.Execute(testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500))

	es := NewEmergencyStop(exec)
	var stopUserID int
	var stopPositions []*LivePosition
	es.OnStop(func(uid int, closed []*LivePosition) {
		stopUserID = uid
		stopPositions = closed
	})

	es.Execute(1)

	if stopUserID != 1 {
		t.Fatalf("expected callback with userID 1, got %d", stopUserID)
	}
	if len(stopPositions) != 1 {
		t.Fatalf("expected 1 position in callback, got %d", len(stopPositions))
	}
}

func TestEmergencyStop_RevokesConfirmation(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)
	exec.Execute(testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500))

	cm := NewConfirmationManager()
	cm.Confirm(1, DefaultConfirmPhrase)

	es := NewEmergencyStop(exec)
	es.OnStop(func(uid int, _ []*LivePosition) {
		cm.Revoke(uid)
	})

	es.Execute(1)
	if cm.IsConfirmed(1) {
		t.Fatal("confirmation should be revoked after emergency stop")
	}
}

// --- notification formatting tests ---

func TestFormatTradeExecuted_Buy(t *testing.T) {
	pos := &LivePosition{
		Symbol:      "BTCUSDT",
		Side:        exchange.SideBuy,
		Quantity:    0.01177,
		EntryPrice:  42450,
		StopLoss:    41800,
		TakeProfit:  44200,
		MainOrderID: 1234567890,
		SLOrderID:   1234567891,
		TPOrderID:   1234567892,
	}

	msg := FormatTradeExecuted(pos)
	if !strings.Contains(msg, "LIVE:") {
		t.Fatal("should say LIVE")
	}
	if !strings.Contains(msg, "Bought") {
		t.Fatal("should say Bought")
	}
	if !strings.Contains(msg, "1234567890") {
		t.Fatal("should include order ID")
	}
	if !strings.Contains(msg, "SL order placed ✓") {
		t.Fatal("should show SL placed")
	}
	if !strings.Contains(msg, "TP order placed ✓") {
		t.Fatal("should show TP placed")
	}
}

func TestFormatTradeExecuted_Sell(t *testing.T) {
	pos := &LivePosition{
		Side:        exchange.SideSell,
		Symbol:      "BTCUSDT",
		Quantity:    0.01177,
		EntryPrice:  42450,
		MainOrderID: 123,
	}
	msg := FormatTradeExecuted(pos)
	if !strings.Contains(msg, "Sold") {
		t.Fatal("should say Sold")
	}
}

func TestFormatTradeExecuted_NoSLTP(t *testing.T) {
	pos := &LivePosition{
		Side:        exchange.SideBuy,
		Symbol:      "BTCUSDT",
		Quantity:    0.01,
		EntryPrice:  42450,
		MainOrderID: 123,
	}
	msg := FormatTradeExecuted(pos)
	if !strings.Contains(msg, "SL order placed ✗") {
		t.Fatal("should show SL not placed")
	}
}

func TestFormatPositionClosed_Profit(t *testing.T) {
	pos := &LivePosition{
		Symbol:      "BTCUSDT",
		Side:        exchange.SideBuy,
		EntryPrice:  42450,
		ClosePrice:  44200,
		Quantity:    0.01177,
		PnL:         20.60,
		CloseReason: "take_profit",
	}
	msg := FormatPositionClosed(pos)
	if !strings.Contains(msg, "Take Profit Hit") {
		t.Fatal("should say Take Profit Hit")
	}
	if !strings.Contains(msg, "🎉") {
		t.Fatal("should have celebration emoji")
	}
}

func TestFormatPositionClosed_Loss(t *testing.T) {
	pos := &LivePosition{
		Symbol:      "BTCUSDT",
		Side:        exchange.SideBuy,
		EntryPrice:  42450,
		ClosePrice:  41800,
		Quantity:    0.01177,
		PnL:         -7.65,
		CloseReason: "stop_loss",
	}
	msg := FormatPositionClosed(pos)
	if !strings.Contains(msg, "Stop Loss Hit") {
		t.Fatal("should say Stop Loss Hit")
	}
}

func TestFormatPositionClosed_EmergencyStop(t *testing.T) {
	pos := &LivePosition{
		Symbol:      "BTCUSDT",
		Side:        exchange.SideBuy,
		EntryPrice:  42450,
		ClosePrice:  42000,
		Quantity:    0.01,
		PnL:         -4.50,
		CloseReason: "emergency_stop",
	}
	msg := FormatPositionClosed(pos)
	if !strings.Contains(msg, "Emergency Stop") {
		t.Fatal("should say Emergency Stop")
	}
	if !strings.Contains(msg, "🚨") {
		t.Fatal("should have emergency emoji")
	}
}

func TestFormatPositionClosed_Manual(t *testing.T) {
	pos := &LivePosition{
		Symbol:      "BTCUSDT",
		Side:        exchange.SideBuy,
		EntryPrice:  42450,
		ClosePrice:  43000,
		Quantity:    0.01,
		PnL:         5.50,
		CloseReason: "manual",
	}
	msg := FormatPositionClosed(pos)
	if !strings.Contains(msg, "Manually Closed") {
		t.Fatal("should say Manually Closed")
	}
}

func TestFormatEmergencyStop_WithPositions(t *testing.T) {
	closed := []*LivePosition{
		{Symbol: "BTCUSDT", PnL: -5, ClosePrice: 42000},
		{Symbol: "ETHUSDT", PnL: 3, ClosePrice: 2300},
	}
	msg := FormatEmergencyStop(1, closed, nil)
	if !strings.Contains(msg, "Emergency Stop Executed") {
		t.Fatal("should mention Emergency Stop")
	}
	if !strings.Contains(msg, "BTCUSDT") || !strings.Contains(msg, "ETHUSDT") {
		t.Fatal("should list all symbols")
	}
	if !strings.Contains(msg, "Trading disabled") {
		t.Fatal("should say trading disabled")
	}
}

func TestFormatEmergencyStop_NoPositions(t *testing.T) {
	msg := FormatEmergencyStop(1, nil, nil)
	if !strings.Contains(msg, "No open positions") {
		t.Fatal("should say no positions")
	}
}

func TestFormatEmergencyStop_WithErrors(t *testing.T) {
	closed := []*LivePosition{{Symbol: "BTCUSDT", PnL: 0, ClosePrice: 42000}}
	errs := []error{fmt.Errorf("failed")}
	msg := FormatEmergencyStop(1, closed, errs)
	if !strings.Contains(msg, "failed to close") {
		t.Fatal("should mention failures")
	}
}

func TestFormatConfirmPrompt(t *testing.T) {
	config := DefaultSafetyConfig()
	msg := FormatConfirmPrompt(config)
	if !strings.Contains(msg, "Live Trading Mode") {
		t.Fatal("should mention live mode")
	}
	if !strings.Contains(msg, DefaultConfirmPhrase) {
		t.Fatal("should include phrase")
	}
	if !strings.Contains(msg, "Max position:") {
		t.Fatal("should show max position")
	}
}

func TestFormatConfirmSuccess(t *testing.T) {
	config := DefaultSafetyConfig()
	msg := FormatConfirmSuccess(config)
	if !strings.Contains(msg, "Live mode ON") {
		t.Fatal("should confirm live mode")
	}
}

func TestFormatSafetyFailure(t *testing.T) {
	result := SafetyResult{
		Passed: false,
		Checks: []CheckResult{
			{Name: "balance", Passed: false, Message: "insufficient"},
			{Name: "position_count", Passed: true, Message: "1/3"},
		},
	}
	msg := FormatSafetyFailure(result)
	if !strings.Contains(msg, "Trade Blocked") {
		t.Fatal("should say blocked")
	}
	if !strings.Contains(msg, "❌") {
		t.Fatal("should show failed check")
	}
	if !strings.Contains(msg, "✅") {
		t.Fatal("should show passed check")
	}
}

// --- full lifecycle integration test ---

func TestFullLifecycle_LiveTrade(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	cm := NewConfirmationManager()
	losses := NewLossTracker()

	config := DefaultSafetyConfig()
	config.MaxPositionSize = 1000
	config.MaxOrderSize = 1000
	balance := &mockBalance{balances: map[int]float64{1: 5000}}
	positions := &mockPositionCounter{counts: map[int]int{1: 0}}

	checker := NewSafetyChecker(config, balance, positions, losses, cm)
	exec := NewExecutor(orders, keys, checker, losses)

	// 1. must confirm before trading
	opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 500)
	_, err := exec.Execute(opp)
	if err == nil || !strings.Contains(err.Error(), "safety check") {
		t.Fatalf("should fail without confirmation, got: %v", err)
	}

	// 2. confirm risk
	if !cm.Confirm(1, DefaultConfirmPhrase) {
		t.Fatal("confirmation should succeed")
	}
	confirmMsg := FormatConfirmSuccess(config)
	if !strings.Contains(confirmMsg, "Live mode ON") {
		t.Fatal("confirm message wrong")
	}

	// 3. execute live trade
	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	execMsg := FormatTradeExecuted(pos)
	if !strings.Contains(execMsg, "LIVE:") {
		t.Fatal("should show LIVE indicator")
	}
	if !strings.Contains(execMsg, fmt.Sprintf("%d", pos.MainOrderID)) {
		t.Fatal("should show order ID")
	}

	// 4. close position
	closed, err := exec.Close(pos.ID, "take_profit")
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}

	closeMsg := FormatPositionClosed(closed)
	if !strings.Contains(closeMsg, "Take Profit Hit") {
		t.Fatal("should show tp hit")
	}

	// 5. emergency stop (open another position first)
	exec.Execute(testOpp("ETHUSDT", claude.ActionBuy, 2100, 2400, 200))
	es := NewEmergencyStop(exec)
	es.OnStop(func(uid int, _ []*LivePosition) {
		cm.Revoke(uid)
	})

	esClosed, esErrs := es.Execute(1)
	if len(esErrs) != 0 {
		t.Fatalf("emergency stop errors: %v", esErrs)
	}

	esMsg := FormatEmergencyStop(1, esClosed, esErrs)
	if !strings.Contains(esMsg, "Emergency Stop Executed") {
		t.Fatal("should show emergency stop")
	}

	// 6. confirmation should be revoked
	if cm.IsConfirmed(1) {
		t.Fatal("confirmation should be revoked after emergency stop")
	}
}

func TestConcurrent_Execution(t *testing.T) {
	exec := NewExecutor(newMockOrders(), newMockKeys(), nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			opp := testOpp("BTCUSDT", claude.ActionBuy, 41800, 44200, 100)
			_, err := exec.Execute(opp)
			if err != nil {
				t.Errorf("concurrent execute failed: %v", err)
			}
		}()
	}
	wg.Wait()

	if exec.Count() != 5 {
		t.Fatalf("expected 5 positions, got %d", exec.Count())
	}
}

// --- order type tests ---

func TestOrderTypes(t *testing.T) {
	if exchange.SideBuy != "BUY" {
		t.Fatal("SideBuy should be BUY")
	}
	if exchange.SideSell != "SELL" {
		t.Fatal("SideSell should be SELL")
	}
	if exchange.OrderTypeMarket != "MARKET" {
		t.Fatal("OrderTypeMarket should be MARKET")
	}
	if exchange.OrderTypeLimit != "LIMIT" {
		t.Fatal("OrderTypeLimit should be LIMIT")
	}
	if exchange.OrderTypeStopLoss != "STOP_LOSS_LIMIT" {
		t.Fatal("OrderTypeStopLoss should be STOP_LOSS_LIMIT")
	}
	if exchange.OrderStatusFilled != "FILLED" {
		t.Fatal("OrderStatusFilled should be FILLED")
	}
}
