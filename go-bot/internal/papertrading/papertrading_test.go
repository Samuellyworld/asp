package papertrading

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/circuitbreaker"
	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/opportunity"
	"github.com/trading-bot/go-bot/internal/pipeline"
)

// mock price provider for testing
type mockPrices struct {
	mu     sync.Mutex
	prices map[string]float64
	err    error
}

func newMockPrices() *mockPrices {
	return &mockPrices{prices: make(map[string]float64)}
}

func (m *mockPrices) GetPrice(symbol string) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return 0, m.err
	}
	p, ok := m.prices[symbol]
	if !ok {
		return 0, fmt.Errorf("no price for %s", symbol)
	}
	return p, nil
}

func (m *mockPrices) set(symbol string, price float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prices[symbol] = price
}

// helper to create a test opportunity
func testOpp(symbol string, action claude.Action, entry, sl, tp, size float64) *opportunity.Opportunity {
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
					Entry:        entry,
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

// --- position p&l tests ---

func TestPosition_PnL_BuyProfit(t *testing.T) {
	pos := &Position{
		Action:       claude.ActionBuy,
		EntryPrice:   100,
		CurrentPrice: 110,
		Quantity:     2,
	}
	if pnl := pos.PnL(); pnl != 20 {
		t.Fatalf("expected pnl 20, got %.2f", pnl)
	}
	if pct := pos.PnLPercent(); pct != 10 {
		t.Fatalf("expected 10%%, got %.2f%%", pct)
	}
}

func TestPosition_PnL_BuyLoss(t *testing.T) {
	pos := &Position{
		Action:       claude.ActionBuy,
		EntryPrice:   100,
		CurrentPrice: 95,
		Quantity:     2,
	}
	if pnl := pos.PnL(); pnl != -10 {
		t.Fatalf("expected pnl -10, got %.2f", pnl)
	}
}

func TestPosition_PnL_SellProfit(t *testing.T) {
	pos := &Position{
		Action:       claude.ActionSell,
		EntryPrice:   100,
		CurrentPrice: 90,
		Quantity:     2,
	}
	if pnl := pos.PnL(); pnl != 20 {
		t.Fatalf("expected pnl 20, got %.2f", pnl)
	}
}

func TestPosition_PnL_SellLoss(t *testing.T) {
	pos := &Position{
		Action:       claude.ActionSell,
		EntryPrice:   100,
		CurrentPrice: 105,
		Quantity:     2,
	}
	if pnl := pos.PnL(); pnl != -10 {
		t.Fatalf("expected pnl -10, got %.2f", pnl)
	}
}

func TestPosition_PnL_ZeroEntry(t *testing.T) {
	pos := &Position{EntryPrice: 0, CurrentPrice: 100, Quantity: 1}
	if pct := pos.PnLPercent(); pct != 0 {
		t.Fatalf("expected 0%% for zero entry, got %.2f%%", pct)
	}
}

func TestPosition_ClosedPnL(t *testing.T) {
	pos := &Position{
		Action:     claude.ActionBuy,
		EntryPrice: 42450,
		ClosePrice: 44200,
		Quantity:   0.01177,
	}
	pnl := pos.ClosedPnL()
	if pnl < 20 || pnl > 21 {
		t.Fatalf("expected pnl ~20.6, got %.2f", pnl)
	}
}

func TestPosition_ClosedPnLPercent(t *testing.T) {
	pos := &Position{
		Action:     claude.ActionBuy,
		EntryPrice: 100,
		ClosePrice: 104,
		Quantity:   1,
	}
	pct := pos.ClosedPnLPercent()
	if pct != 4 {
		t.Fatalf("expected 4%%, got %.2f%%", pct)
	}
}

// --- tp/sl detection ---

func TestPosition_IsTPHit_Buy(t *testing.T) {
	pos := &Position{Action: claude.ActionBuy, TakeProfit: 110, CurrentPrice: 110}
	if !pos.IsTPHit() {
		t.Fatal("expected tp hit")
	}
	pos.CurrentPrice = 109
	if pos.IsTPHit() {
		t.Fatal("should not hit tp at 109")
	}
}

func TestPosition_IsTPHit_Sell(t *testing.T) {
	pos := &Position{Action: claude.ActionSell, TakeProfit: 90, CurrentPrice: 90}
	if !pos.IsTPHit() {
		t.Fatal("expected tp hit")
	}
	pos.CurrentPrice = 91
	if pos.IsTPHit() {
		t.Fatal("should not hit tp at 91")
	}
}

func TestPosition_IsTPHit_ZeroTP(t *testing.T) {
	pos := &Position{Action: claude.ActionBuy, TakeProfit: 0, CurrentPrice: 999}
	if pos.IsTPHit() {
		t.Fatal("zero tp should never hit")
	}
}

func TestPosition_IsSLHit_Buy(t *testing.T) {
	pos := &Position{Action: claude.ActionBuy, StopLoss: 90, CurrentPrice: 90}
	if !pos.IsSLHit() {
		t.Fatal("expected sl hit")
	}
	pos.CurrentPrice = 91
	if pos.IsSLHit() {
		t.Fatal("should not hit sl at 91")
	}
}

func TestPosition_IsSLHit_Sell(t *testing.T) {
	pos := &Position{Action: claude.ActionSell, StopLoss: 110, CurrentPrice: 110}
	if !pos.IsSLHit() {
		t.Fatal("expected sl hit")
	}
	pos.CurrentPrice = 109
	if pos.IsSLHit() {
		t.Fatal("should not hit sl at 109")
	}
}

func TestPosition_IsSLHit_ZeroSL(t *testing.T) {
	pos := &Position{Action: claude.ActionBuy, StopLoss: 0, CurrentPrice: 0.01}
	if pos.IsSLHit() {
		t.Fatal("zero sl should never hit")
	}
}

// --- milestone detection ---

func TestPosition_NewMilestones_ProfitSide(t *testing.T) {
	pos := &Position{
		Action:        claude.ActionBuy,
		EntryPrice:    100,
		CurrentPrice:  103,
		Quantity:      1,
		HitMilestones: make(map[float64]bool),
	}
	ms := pos.NewMilestones()
	if len(ms) != 3 {
		t.Fatalf("expected 3 milestones (+1, +2, +3), got %d: %v", len(ms), ms)
	}
}

func TestPosition_NewMilestones_LossSide(t *testing.T) {
	pos := &Position{
		Action:        claude.ActionBuy,
		EntryPrice:    100,
		CurrentPrice:  98.5,
		Quantity:      1,
		HitMilestones: make(map[float64]bool),
	}
	ms := pos.NewMilestones()
	// -1.5% hits all three: -0.5, -1.0, -1.5
	if len(ms) != 3 {
		t.Fatalf("expected 3 loss milestones (-0.5, -1.0, -1.5), got %d: %v", len(ms), ms)
	}
}

func TestPosition_NewMilestones_AlreadyHit(t *testing.T) {
	pos := &Position{
		Action:        claude.ActionBuy,
		EntryPrice:    100,
		CurrentPrice:  103,
		Quantity:      1,
		HitMilestones: map[float64]bool{1.0: true, 2.0: true},
	}
	ms := pos.NewMilestones()
	if len(ms) != 1 {
		t.Fatalf("expected 1 new milestone (+3), got %d: %v", len(ms), ms)
	}
	if ms[0] != 3.0 {
		t.Fatalf("expected +3.0, got %+.1f", ms[0])
	}
}

func TestPosition_NewMilestones_NoMilestones(t *testing.T) {
	pos := &Position{
		Action:        claude.ActionBuy,
		EntryPrice:    100,
		CurrentPrice:  100.4,
		Quantity:      1,
		HitMilestones: make(map[float64]bool),
	}
	ms := pos.NewMilestones()
	if len(ms) != 0 {
		t.Fatalf("expected no milestones, got %d", len(ms))
	}
}

func TestPosition_NewMilestones_SellDirection(t *testing.T) {
	pos := &Position{
		Action:        claude.ActionSell,
		EntryPrice:    100,
		CurrentPrice:  98, // price dropped = profit for short
		Quantity:      1,
		HitMilestones: make(map[float64]bool),
	}
	ms := pos.NewMilestones()
	if len(ms) != 2 { // +1% and +2%
		t.Fatalf("expected 2 profit milestones for short, got %d: %v", len(ms), ms)
	}
}

// --- executor tests ---

func TestExecutor_Execute(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if pos.Symbol != "BTCUSDT" {
		t.Fatalf("expected BTCUSDT, got %s", pos.Symbol)
	}
	if pos.EntryPrice != 42450 {
		t.Fatalf("expected entry 42450, got %.2f", pos.EntryPrice)
	}
	if pos.Status != PositionOpen {
		t.Fatalf("expected open status, got %s", pos.Status)
	}
	if pos.StopLoss != 41800 {
		t.Fatalf("expected sl 41800, got %.2f", pos.StopLoss)
	}
	if pos.TakeProfit != 44200 {
		t.Fatalf("expected tp 44200, got %.2f", pos.TakeProfit)
	}
	expectedQty := 500.0 / 42450.0
	if pos.Quantity < expectedQty*0.99 || pos.Quantity > expectedQty*1.01 {
		t.Fatalf("expected qty ~%.6f, got %.6f", expectedQty, pos.Quantity)
	}
	if exec.Count() != 1 {
		t.Fatalf("expected 1 position, got %d", exec.Count())
	}
}

func TestExecutor_Execute_NotApproved(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	opp.Status = opportunity.StatusPending
	_, err := exec.Execute(opp)
	if err == nil {
		t.Fatal("expected error for pending opp")
	}
}

func TestExecutor_Execute_ModifiedPlan(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
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
	if pos.PositionSize != 300 {
		t.Fatalf("expected modified size 300, got %.2f", pos.PositionSize)
	}
}

func TestExecutor_Execute_PriceError(t *testing.T) {
	prices := newMockPrices()
	prices.err = fmt.Errorf("api down")
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	_, err := exec.Execute(opp)
	if err == nil {
		t.Fatal("expected error when price fails")
	}
}

func TestExecutor_Execute_InvalidSize(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 0)
	_, err := exec.Execute(opp)
	if err == nil {
		t.Fatal("expected error for zero size")
	}
}

func TestExecutor_Execute_NilResult(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	opp.Result = nil
	_, err := exec.Execute(opp)
	if err == nil {
		t.Fatal("expected error for nil result")
	}
}

func TestExecutor_Close(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	closed, err := exec.Close(pos.ID, CloseTP, 44200)
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if closed.Status != PositionClosed {
		t.Fatalf("expected closed status, got %s", closed.Status)
	}
	if closed.CloseReason != CloseTP {
		t.Fatalf("expected tp reason, got %s", closed.CloseReason)
	}
	if closed.ClosePrice != 44200 {
		t.Fatalf("expected close price 44200, got %.2f", closed.ClosePrice)
	}
	if closed.ClosedAt == nil {
		t.Fatal("expected ClosedAt to be set")
	}
	if exec.Count() != 0 {
		t.Fatalf("expected 0 open positions, got %d", exec.Count())
	}
}

func TestExecutor_Close_NotFound(t *testing.T) {
	exec := NewExecutor(newMockPrices())
	_, err := exec.Close("nonexistent", CloseManual, 100)
	if err == nil {
		t.Fatal("expected error for missing position")
	}
}

func TestExecutor_Close_AlreadyClosed(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)
	exec.Close(pos.ID, CloseManual, 43000)
	_, err := exec.Close(pos.ID, CloseManual, 43000)
	if err == nil {
		t.Fatal("expected error for double close")
	}
}

func TestExecutor_Adjust(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	if err := exec.Adjust(pos.ID, "sl", 42450); err != nil {
		t.Fatalf("adjust sl failed: %v", err)
	}
	if pos.StopLoss != 42450 {
		t.Fatalf("expected sl 42450, got %.2f", pos.StopLoss)
	}

	if err := exec.Adjust(pos.ID, "tp", 46000); err != nil {
		t.Fatalf("adjust tp failed: %v", err)
	}
	if pos.TakeProfit != 46000 {
		t.Fatalf("expected tp 46000, got %.2f", pos.TakeProfit)
	}
}

func TestExecutor_Adjust_StopLossAlias(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	if err := exec.Adjust(pos.ID, "stop_loss", 42000); err != nil {
		t.Fatalf("adjust stop_loss failed: %v", err)
	}
	if pos.StopLoss != 42000 {
		t.Fatalf("expected sl 42000, got %.2f", pos.StopLoss)
	}
}

func TestExecutor_Adjust_TakeProfitAlias(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	if err := exec.Adjust(pos.ID, "take_profit", 47000); err != nil {
		t.Fatalf("adjust take_profit failed: %v", err)
	}
	if pos.TakeProfit != 47000 {
		t.Fatalf("expected tp 47000, got %.2f", pos.TakeProfit)
	}
}

func TestExecutor_Adjust_UnknownField(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	if err := exec.Adjust(pos.ID, "entry", 42500); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestExecutor_Adjust_NotFound(t *testing.T) {
	exec := NewExecutor(newMockPrices())
	if err := exec.Adjust("nonexistent", "sl", 100); err == nil {
		t.Fatal("expected error for missing position")
	}
}

func TestExecutor_Get(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	got := exec.Get(pos.ID)
	if got == nil {
		t.Fatal("expected to find position")
	}
	if got.ID != pos.ID {
		t.Fatalf("expected %s, got %s", pos.ID, got.ID)
	}

	// close and verify still findable
	exec.Close(pos.ID, CloseManual, 43000)
	got = exec.Get(pos.ID)
	if got == nil {
		t.Fatal("expected to find closed position")
	}
}

func TestExecutor_Get_NotFound(t *testing.T) {
	exec := NewExecutor(newMockPrices())
	if exec.Get("nonexistent") != nil {
		t.Fatal("expected nil for missing position")
	}
}

func TestExecutor_OpenPositions(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	prices.set("ETHUSDT", 2200)
	exec := NewExecutor(prices)

	opp1 := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	opp1.UserID = 1
	exec.Execute(opp1)

	opp2 := testOpp("ETHUSDT", claude.ActionBuy, 2200, 2100, 2400, 200)
	opp2.UserID = 1
	exec.Execute(opp2)

	opp3 := testOpp("BTCUSDT", claude.ActionSell, 42450, 43000, 41000, 300)
	opp3.UserID = 2
	exec.Execute(opp3)

	u1 := exec.OpenPositions(1)
	if len(u1) != 2 {
		t.Fatalf("expected 2 positions for user 1, got %d", len(u1))
	}

	u2 := exec.OpenPositions(2)
	if len(u2) != 1 {
		t.Fatalf("expected 1 position for user 2, got %d", len(u2))
	}
}

func TestExecutor_AllOpen(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	prices.set("ETHUSDT", 2200)
	exec := NewExecutor(prices)

	exec.Execute(testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500))
	exec.Execute(testOpp("ETHUSDT", claude.ActionBuy, 2200, 2100, 2400, 200))

	all := exec.AllOpen()
	if len(all) != 2 {
		t.Fatalf("expected 2 open positions, got %d", len(all))
	}
}

func TestExecutor_ClosedPositions(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)
	exec.Close(pos.ID, CloseTP, 44200)

	closed := exec.ClosedPositions(1)
	if len(closed) != 1 {
		t.Fatalf("expected 1 closed position, got %d", len(closed))
	}
}

func TestExecutor_UpdatePrice(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	exec.UpdatePrice(pos.ID, 43000)
	if pos.CurrentPrice != 43000 {
		t.Fatalf("expected current price 43000, got %.2f", pos.CurrentPrice)
	}
}

// --- summary tests ---

func TestExecutor_Summary(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	prices.set("ETHUSDT", 2200)
	exec := NewExecutor(prices)

	// open and close a winning trade
	opp1 := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos1, _ := exec.Execute(opp1)
	exec.Close(pos1.ID, CloseTP, 44200)

	// open and close a losing trade
	opp2 := testOpp("ETHUSDT", claude.ActionBuy, 2200, 2100, 2400, 200)
	pos2, _ := exec.Execute(opp2)
	exec.Close(pos2.ID, CloseSL, 2100)

	// open position (still active)
	opp3 := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 300)
	exec.Execute(opp3)

	summary := exec.Summary(1, time.Now())
	if summary.ClosedCount != 2 {
		t.Fatalf("expected 2 closed, got %d", summary.ClosedCount)
	}
	if summary.Wins != 1 {
		t.Fatalf("expected 1 win, got %d", summary.Wins)
	}
	if summary.Losses != 1 {
		t.Fatalf("expected 1 loss, got %d", summary.Losses)
	}
	if summary.OpenCount != 1 {
		t.Fatalf("expected 1 open, got %d", summary.OpenCount)
	}
	if summary.BestTrade == nil {
		t.Fatal("expected best trade to be set")
	}
	if summary.WorstTrade == nil {
		t.Fatal("expected worst trade to be set")
	}
	if summary.BestTrade.ClosedPnL() <= summary.WorstTrade.ClosedPnL() {
		t.Fatal("best trade pnl should exceed worst trade pnl")
	}
}

func TestExecutor_Summary_NoTrades(t *testing.T) {
	exec := NewExecutor(newMockPrices())
	summary := exec.Summary(1, time.Now())
	if summary.ClosedCount != 0 || summary.OpenCount != 0 {
		t.Fatal("expected empty summary")
	}
}

func TestExecutor_Summary_FiltersByDate(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)
	exec.Close(pos.ID, CloseTP, 44200)

	yesterday := time.Now().Add(-24 * time.Hour)
	summary := exec.Summary(1, yesterday)
	if summary.ClosedCount != 0 {
		t.Fatal("should not include today's trade in yesterday's summary")
	}
}

// --- monitor tests ---

func TestMonitor_StartStop(t *testing.T) {
	exec := NewExecutor(newMockPrices())
	config := DefaultMonitorConfig()
	config.CheckInterval = 10 * time.Millisecond

	mon := NewMonitor(exec, newMockPrices(), config)
	if mon.IsRunning() {
		t.Fatal("should not be running before start")
	}

	mon.Start(context.Background())
	if !mon.IsRunning() {
		t.Fatal("should be running after start")
	}

	// double start should be safe
	mon.Start(context.Background())

	mon.Stop()
	if mon.IsRunning() {
		t.Fatal("should not be running after stop")
	}

	// double stop should be safe
	mon.Stop()
}

func TestMonitor_DetectsTPHit(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	exec.Execute(opp)

	// update price to hit tp
	prices.set("BTCUSDT", 44200)

	var events []Event
	var mu sync.Mutex

	config := DefaultMonitorConfig()
	config.CheckInterval = 10 * time.Millisecond
	mon := NewMonitor(exec, prices, config)
	mon.OnEvent = func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	// manually trigger a check
	mon.CheckPositions()

	mu.Lock()
	defer mu.Unlock()

	if len(events) == 0 {
		t.Fatal("expected tp hit event")
	}
	found := false
	for _, e := range events {
		if e.Type == EventTPHit {
			found = true
			if !e.IsUrgent {
				t.Fatal("tp hit should be urgent")
			}
			if e.Position.ClosePrice != 44200 {
				t.Fatalf("expected close at 44200, got %.2f", e.Position.ClosePrice)
			}
		}
	}
	if !found {
		t.Fatal("expected EventTPHit event")
	}
	if exec.Count() != 0 {
		t.Fatalf("position should be closed, got %d open", exec.Count())
	}
}

func TestMonitor_DetectsSLHit(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	exec.Execute(opp)

	prices.set("BTCUSDT", 41800)

	var events []Event
	var mu sync.Mutex

	config := DefaultMonitorConfig()
	mon := NewMonitor(exec, prices, config)
	mon.OnEvent = func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	mon.CheckPositions()

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, e := range events {
		if e.Type == EventSLHit {
			found = true
			if !e.IsUrgent {
				t.Fatal("sl hit should be urgent")
			}
		}
	}
	if !found {
		t.Fatal("expected EventSLHit event")
	}
}

func TestMonitor_DetectsMilestones(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	exec.Execute(opp)

	// +2% = 42450 * 1.02 = 43299
	prices.set("BTCUSDT", 43299)

	var events []Event
	var mu sync.Mutex

	config := DefaultMonitorConfig()
	mon := NewMonitor(exec, prices, config)
	mon.OnEvent = func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	mon.CheckPositions()

	mu.Lock()
	defer mu.Unlock()

	milestoneCount := 0
	for _, e := range events {
		if e.Type == EventMilestone {
			milestoneCount++
		}
	}
	if milestoneCount != 2 { // +1% and +2%
		t.Fatalf("expected 2 milestone events, got %d", milestoneCount)
	}
}

func TestMonitor_MilestonesFireOnceEach(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	exec.Execute(opp)

	prices.set("BTCUSDT", 43299) // +2%

	var events []Event
	var mu sync.Mutex

	config := DefaultMonitorConfig()
	mon := NewMonitor(exec, prices, config)
	mon.OnEvent = func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	mon.CheckPositions()
	mon.CheckPositions() // second check should not re-fire same milestones

	mu.Lock()
	defer mu.Unlock()

	milestoneCount := 0
	for _, e := range events {
		if e.Type == EventMilestone {
			milestoneCount++
		}
	}
	if milestoneCount != 2 { // still only +1% and +2%
		t.Fatalf("expected 2 total milestone events (no duplicates), got %d", milestoneCount)
	}
}

func TestMonitor_PeriodicUpdate(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	exec.Execute(opp)

	prices.set("BTCUSDT", 42500) // small move, no milestones

	var events []Event
	var mu sync.Mutex

	config := DefaultMonitorConfig()
	config.CooldownPeriod = 0 // disable cooldown for test
	mon := NewMonitor(exec, prices, config)
	mon.OnEvent = func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	mon.CheckPositions()

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, e := range events {
		if e.Type == EventPeriodicUpdate {
			found = true
		}
	}
	if !found {
		t.Fatal("expected periodic update event")
	}
}

func TestMonitor_CooldownPreventsSpam(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	exec.Execute(opp)

	prices.set("BTCUSDT", 42500)

	var events []Event
	var mu sync.Mutex

	config := DefaultMonitorConfig()
	config.CooldownPeriod = 1 * time.Hour // long cooldown
	config.SmallPositionInterval = 1 * time.Hour
	config.MediumPositionInterval = 1 * time.Hour
	config.LargePositionInterval = 1 * time.Hour
	mon := NewMonitor(exec, prices, config)
	mon.OnEvent = func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	// first check should send periodic update (no prior notification)
	mon.CheckPositions()
	// second check immediately should be suppressed by cooldown
	mon.CheckPositions()

	mu.Lock()
	defer mu.Unlock()

	periodicCount := 0
	for _, e := range events {
		if e.Type == EventPeriodicUpdate {
			periodicCount++
		}
	}
	if periodicCount != 1 {
		t.Fatalf("expected 1 periodic update (cooldown should prevent second), got %d", periodicCount)
	}
}

func TestMonitor_UrgentBypassesCooldown(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	exec.Execute(opp)

	var events []Event
	var mu sync.Mutex

	config := DefaultMonitorConfig()
	config.CooldownPeriod = 1 * time.Hour
	mon := NewMonitor(exec, prices, config)
	mon.OnEvent = func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	// first check triggers a periodic
	mon.CheckPositions()

	// move price to hit SL — should fire regardless of cooldown
	prices.set("BTCUSDT", 41800)
	mon.CheckPositions()

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, e := range events {
		if e.Type == EventSLHit {
			found = true
		}
	}
	if !found {
		t.Fatal("urgent SL hit should bypass cooldown")
	}
}

func TestMonitor_PeriodicInterval_SmallPosition(t *testing.T) {
	config := DefaultMonitorConfig()
	mon := NewMonitor(nil, nil, config)
	interval := mon.periodicInterval(50)
	if interval != 1*time.Hour {
		t.Fatalf("expected 1h for small position, got %v", interval)
	}
}

func TestMonitor_PeriodicInterval_MediumPosition(t *testing.T) {
	config := DefaultMonitorConfig()
	mon := NewMonitor(nil, nil, config)
	interval := mon.periodicInterval(250)
	if interval != 30*time.Minute {
		t.Fatalf("expected 30m for medium position, got %v", interval)
	}
}

func TestMonitor_PeriodicInterval_LargePosition(t *testing.T) {
	config := DefaultMonitorConfig()
	mon := NewMonitor(nil, nil, config)
	interval := mon.periodicInterval(1000)
	if interval != 15*time.Minute {
		t.Fatalf("expected 15m for large position, got %v", interval)
	}
}

func TestMonitor_Cleanup(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)
	config := DefaultMonitorConfig()
	config.CooldownPeriod = 0
	mon := NewMonitor(exec, prices, config)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	// record a notification
	mon.recordNotification(pos.ID, EventPeriodicUpdate)

	// close the position
	exec.Close(pos.ID, CloseManual, 43000)

	// cleanup should remove tracking for closed positions
	mon.Cleanup()

	mon.mu.Lock()
	_, exists := mon.lastNotified[pos.ID]
	mon.mu.Unlock()

	if exists {
		t.Fatal("expected notification record to be cleaned up")
	}
}

func TestMonitor_PriceErrorSkipsPosition(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	exec.Execute(opp)

	// make price provider return error
	errPrices := newMockPrices()
	errPrices.err = fmt.Errorf("api timeout")

	var events []Event
	config := DefaultMonitorConfig()
	mon := NewMonitor(exec, errPrices, config)
	mon.OnEvent = func(e Event) {
		events = append(events, e)
	}

	mon.CheckPositions()
	if len(events) != 0 {
		t.Fatalf("expected no events when price fails, got %d", len(events))
	}
}

func TestMonitor_BackgroundLoop(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	exec.Execute(opp)

	// set price to hit tp
	prices.set("BTCUSDT", 44200)

	var events []Event
	var mu sync.Mutex

	config := DefaultMonitorConfig()
	config.CheckInterval = 10 * time.Millisecond
	mon := NewMonitor(exec, prices, config)
	mon.OnEvent = func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	mon.Start(context.Background())
	time.Sleep(50 * time.Millisecond)
	mon.Stop()

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, e := range events {
		if e.Type == EventTPHit {
			found = true
		}
	}
	if !found {
		t.Fatal("background loop should have detected tp hit")
	}
}

// --- notification formatting tests ---

func TestFormatTradeExecuted_Buy(t *testing.T) {
	pos := &Position{
		Action:       claude.ActionBuy,
		Symbol:       "BTCUSDT",
		EntryPrice:   42450,
		Quantity:     0.01177,
		StopLoss:     41800,
		TakeProfit:   44200,
		PositionSize: 500,
	}
	msg := FormatTradeExecuted(pos)
	if !strings.Contains(msg, "Paper Trade") {
		t.Fatal("should contain Paper Trade")
	}
	if !strings.Contains(msg, "Bought") {
		t.Fatal("should say Bought")
	}
	if !strings.Contains(msg, "BTCUSDT") {
		t.Fatal("should include symbol")
	}
	if !strings.Contains(msg, "SL:") || !strings.Contains(msg, "TP:") {
		t.Fatal("should include SL and TP")
	}
}

func TestFormatTradeExecuted_Sell(t *testing.T) {
	pos := &Position{
		Action:       claude.ActionSell,
		Symbol:       "BTCUSDT",
		EntryPrice:   42450,
		Quantity:     0.01177,
		StopLoss:     43000,
		TakeProfit:   41000,
		PositionSize: 500,
	}
	msg := FormatTradeExecuted(pos)
	if !strings.Contains(msg, "Sold") {
		t.Fatal("should say Sold")
	}
}

func TestFormatMilestone_Profit(t *testing.T) {
	pos := &Position{
		Action:       claude.ActionBuy,
		Symbol:       "BTCUSDT",
		EntryPrice:   42450,
		CurrentPrice: 43299,
		Quantity:     0.01177,
	}
	msg := FormatMilestone(pos, 2.0)
	if !strings.Contains(msg, "📈") {
		t.Fatal("should use up emoji for positive milestone")
	}
	if !strings.Contains(msg, "+2.0%") {
		t.Fatal("should show +2.0%")
	}
	if !strings.Contains(msg, "P&L:") {
		t.Fatal("should show P&L")
	}
}

func TestFormatMilestone_Loss(t *testing.T) {
	pos := &Position{
		Action:       claude.ActionBuy,
		Symbol:       "BTCUSDT",
		EntryPrice:   42450,
		CurrentPrice: 42025,
		Quantity:     0.01177,
	}
	msg := FormatMilestone(pos, -1.0)
	if !strings.Contains(msg, "📉") {
		t.Fatal("should use down emoji for negative milestone")
	}
	if !strings.Contains(msg, "-1.0%") {
		t.Fatal("should show -1.0%")
	}
}

func TestFormatAISuggestion(t *testing.T) {
	msg := FormatAISuggestion("Move SL to entry ($42,450)?")
	if !strings.Contains(msg, "Claude suggests") {
		t.Fatal("should mention Claude")
	}
	if !strings.Contains(msg, "Move SL to entry") {
		t.Fatal("should include suggestion text")
	}
}

func TestFormatTPHit(t *testing.T) {
	pos := &Position{
		Action:     claude.ActionBuy,
		Symbol:     "BTCUSDT",
		EntryPrice: 42450,
		ClosePrice: 44200,
		Quantity:   0.01177,
	}
	msg := FormatTPHit(pos)
	if !strings.Contains(msg, "Take Profit Hit") {
		t.Fatal("should mention Take Profit Hit")
	}
	if !strings.Contains(msg, "Profit:") {
		t.Fatal("should show profit")
	}
}

func TestFormatSLHit(t *testing.T) {
	pos := &Position{
		Action:     claude.ActionBuy,
		Symbol:     "BTCUSDT",
		EntryPrice: 42450,
		ClosePrice: 41800,
		Quantity:   0.01177,
	}
	msg := FormatSLHit(pos)
	if !strings.Contains(msg, "Stop Loss Hit") {
		t.Fatal("should mention Stop Loss Hit")
	}
	if !strings.Contains(msg, "Loss:") {
		t.Fatal("should show loss")
	}
}

func TestFormatManualClose(t *testing.T) {
	pos := &Position{
		Action:     claude.ActionBuy,
		Symbol:     "BTCUSDT",
		EntryPrice: 42450,
		ClosePrice: 43000,
		Quantity:   0.01177,
	}
	msg := FormatManualClose(pos)
	if !strings.Contains(msg, "Position Closed") {
		t.Fatal("should mention Position Closed")
	}
}

func TestFormatPeriodicUpdate(t *testing.T) {
	pos := &Position{
		Action:       claude.ActionBuy,
		Symbol:       "BTCUSDT",
		EntryPrice:   42450,
		CurrentPrice: 42800,
		Quantity:     0.01177,
	}
	msg := FormatPeriodicUpdate(pos)
	if !strings.Contains(msg, "Update:") {
		t.Fatal("should say Update")
	}
	if !strings.Contains(msg, "P&L:") {
		t.Fatal("should show P&L")
	}
}

func TestFormatDailySummary_WithTrades(t *testing.T) {
	btcWin := &Position{
		Symbol:     "BTCUSDT",
		Action:     claude.ActionBuy,
		EntryPrice: 42450,
		ClosePrice: 44200,
		Quantity:   0.01177,
	}
	ethLoss := &Position{
		Symbol:     "ETHUSDT",
		Action:     claude.ActionBuy,
		EntryPrice: 2200,
		ClosePrice: 2100,
		Quantity:   0.1,
	}
	openPos := &Position{
		Symbol:       "BTCUSDT",
		Action:       claude.ActionBuy,
		EntryPrice:   42450,
		CurrentPrice: 42800,
		Quantity:     0.01,
	}

	summary := &DailySummary{
		UserID:        1,
		Date:          time.Now(),
		ClosedCount:   2,
		Wins:          1,
		Losses:        1,
		TotalPnL:      btcWin.ClosedPnL() + ethLoss.ClosedPnL(),
		BestTrade:     btcWin,
		WorstTrade:    ethLoss,
		OpenCount:     1,
		OpenPositions: []*Position{openPos},
	}

	msg := FormatDailySummary(summary)
	if !strings.Contains(msg, "Daily Summary") {
		t.Fatal("should say Daily Summary")
	}
	if !strings.Contains(msg, "2 trades") {
		t.Fatal("should show trade count")
	}
	if !strings.Contains(msg, "1W 1L") {
		t.Fatal("should show wins/losses")
	}
	if !strings.Contains(msg, "Best:") {
		t.Fatal("should show best trade")
	}
	if !strings.Contains(msg, "Worst:") {
		t.Fatal("should show worst trade")
	}
	if !strings.Contains(msg, "Open: 1") {
		t.Fatal("should show open positions")
	}
}

func TestFormatDailySummary_NoTrades(t *testing.T) {
	summary := &DailySummary{
		UserID: 1,
		Date:   time.Now(),
	}
	msg := FormatDailySummary(summary)
	if !strings.Contains(msg, "No closed trades today") {
		t.Fatal("should say no trades")
	}
}

// --- button tests ---

func TestAdjustButtons(t *testing.T) {
	buttons := AdjustButtons("pt_1")
	if len(buttons) != 2 {
		t.Fatalf("expected 2 buttons, got %d", len(buttons))
	}
	if !strings.Contains(buttons[0].Data, "pt_adjust_yes:pt_1") {
		t.Fatalf("expected yes button data, got %s", buttons[0].Data)
	}
	if !strings.Contains(buttons[1].Data, "pt_adjust_no:pt_1") {
		t.Fatalf("expected no button data, got %s", buttons[1].Data)
	}
	if buttons[0].Style != ButtonStyleSuccess {
		t.Fatal("yes button should be success style")
	}
}

func TestCloseButton(t *testing.T) {
	btn := CloseButton("pt_1")
	if !strings.Contains(btn.Data, "pt_close:pt_1") {
		t.Fatalf("expected close button data, got %s", btn.Data)
	}
	if btn.Style != ButtonStyleDanger {
		t.Fatal("close button should be danger style")
	}
}

func TestPositionButtons(t *testing.T) {
	buttons := PositionButtons("pt_1")
	if len(buttons) != 3 {
		t.Fatalf("expected 3 buttons, got %d", len(buttons))
	}
	if !strings.Contains(buttons[0].Data, "pt_adj_sl:pt_1") {
		t.Fatalf("expected sl button, got %s", buttons[0].Data)
	}
	if !strings.Contains(buttons[1].Data, "pt_adj_tp:pt_1") {
		t.Fatalf("expected tp button, got %s", buttons[1].Data)
	}
	if !strings.Contains(buttons[2].Data, "pt_close:pt_1") {
		t.Fatalf("expected close button, got %s", buttons[2].Data)
	}
}

// --- full lifecycle integration test ---

func TestFullLifecycle_BuyTPHit(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	// 1. execute paper trade
	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	msg := FormatTradeExecuted(pos)
	if !strings.Contains(msg, "Bought") {
		t.Fatal("trade executed message missing")
	}

	// 2. price moves up to +2% — milestones fire
	prices.set("BTCUSDT", 43299)

	var events []Event
	var mu sync.Mutex
	config := DefaultMonitorConfig()
	config.CooldownPeriod = 0
	mon := NewMonitor(exec, prices, config)
	mon.OnEvent = func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	mon.CheckPositions()

	mu.Lock()
	milestones := 0
	for _, e := range events {
		if e.Type == EventMilestone {
			milestones++
			milestoneMsg := FormatMilestone(e.Position, e.Milestone)
			if e.Milestone >= AITriggerThreshold {
				aiMsg := FormatAISuggestion("Move SL to entry ($42,450)?")
				if !strings.Contains(aiMsg, "Claude suggests") {
					t.Fatal("ai suggestion should be generated at +2%")
				}
			}
			if milestoneMsg == "" {
				t.Fatal("milestone message should not be empty")
			}
		}
	}
	mu.Unlock()

	if milestones != 2 {
		t.Fatalf("expected 2 milestones (+1%%, +2%%), got %d", milestones)
	}

	// 3. adjust SL to entry after AI suggestion
	if err := exec.Adjust(pos.ID, "sl", 42450); err != nil {
		t.Fatalf("adjust failed: %v", err)
	}

	// 4. price hits TP
	prices.set("BTCUSDT", 44200)
	events = nil
	mon.CheckPositions()

	mu.Lock()
	tpHit := false
	for _, e := range events {
		if e.Type == EventTPHit {
			tpHit = true
			tpMsg := FormatTPHit(e.Position)
			if !strings.Contains(tpMsg, "Take Profit Hit") {
				t.Fatal("tp message incorrect")
			}
		}
	}
	mu.Unlock()

	if !tpHit {
		t.Fatal("expected tp hit event")
	}

	// 5. daily summary
	summary := exec.Summary(1, time.Now())
	summaryMsg := FormatDailySummary(summary)
	if !strings.Contains(summaryMsg, "1 trades") {
		t.Fatalf("summary should show 1 trade, got: %s", summaryMsg)
	}
}

func TestFullLifecycle_SellSLHit(t *testing.T) {
	prices := newMockPrices()
	prices.set("ETHUSDT", 2200)
	exec := NewExecutor(prices)

	// 1. short sell
	opp := testOpp("ETHUSDT", claude.ActionSell, 2200, 2300, 2000, 200)
	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	msg := FormatTradeExecuted(pos)
	if !strings.Contains(msg, "Sold") {
		t.Fatal("should say Sold for short position")
	}

	// 2. price goes against us — hits SL
	prices.set("ETHUSDT", 2300)

	var events []Event
	var mu sync.Mutex
	config := DefaultMonitorConfig()
	mon := NewMonitor(exec, prices, config)
	mon.OnEvent = func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	mon.CheckPositions()

	mu.Lock()
	defer mu.Unlock()

	slHit := false
	for _, e := range events {
		if e.Type == EventSLHit {
			slHit = true
			slMsg := FormatSLHit(e.Position)
			if !strings.Contains(slMsg, "Stop Loss Hit") {
				t.Fatal("sl message incorrect")
			}
		}
	}
	if !slHit {
		t.Fatal("expected sl hit for short position going up")
	}
}

func TestFullLifecycle_ManualClose(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	exec := NewExecutor(prices)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42450, 41800, 44200, 500)
	pos, _ := exec.Execute(opp)

	closed, err := exec.Close(pos.ID, CloseManual, 43000)
	if err != nil {
		t.Fatalf("manual close failed: %v", err)
	}

	msg := FormatManualClose(closed)
	if !strings.Contains(msg, "Position Closed") {
		t.Fatal("should show position closed")
	}
	if closed.ClosedPnL() <= 0 {
		t.Fatal("closing above entry should be profitable")
	}
}

func TestConcurrent_MultiplePositions(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42450)
	prices.set("ETHUSDT", 2200)
	prices.set("SOLUSDT", 100)
	exec := NewExecutor(prices)

	var wg sync.WaitGroup
	symbols := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}
	for _, sym := range symbols {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			opp := testOpp(s, claude.ActionBuy, 100, 90, 110, 200)
			_, err := exec.Execute(opp)
			if err != nil {
				t.Errorf("concurrent execute failed: %v", err)
			}
		}(sym)
	}
	wg.Wait()

	if exec.Count() != 3 {
		t.Fatalf("expected 3 positions, got %d", exec.Count())
	}
}

func TestFormatHelpers_PnlSign(t *testing.T) {
	if pnlSign(10) != "+" {
		t.Fatal("positive should be +")
	}
	if pnlSign(-5) != "-" {
		t.Fatal("negative should be -")
	}
	if pnlSign(0) != "+" {
		t.Fatal("zero should be +")
	}
}

func TestFormatHelpers_AbsVal(t *testing.T) {
	if absVal(-5) != 5 {
		t.Fatal("abs(-5) should be 5")
	}
	if absVal(5) != 5 {
		t.Fatal("abs(5) should be 5")
	}
	if absVal(0) != 0 {
		t.Fatal("abs(0) should be 0")
	}
}

func TestFormatHelpers_FormatPrice(t *testing.T) {
	p := formatPrice(42450)
	if p != "42450.00" {
		t.Fatalf("expected 42450.00, got %s", p)
	}
	p = formatPrice(2.5)
	if p != "2.5000" {
		t.Fatalf("expected 2.5000, got %s", p)
	}
	p = formatPrice(0.0035)
	if p != "0.003500" {
		t.Fatalf("expected 0.003500, got %s", p)
	}
}

func TestFormatHelpers_FormatQty(t *testing.T) {
	q := formatQty(1.5)
	if q != "1.5000" {
		t.Fatalf("expected 1.5000, got %s", q)
	}
	q = formatQty(0.01177)
	if q != "0.01177" {
		t.Fatalf("expected 0.01177, got %s", q)
	}
}

// --- trade logger tests ---

type tradeLogEntry struct {
	isOpen bool
	pos    *Position
}

type mockTradeLogger struct {
	mu      sync.Mutex
	entries []tradeLogEntry
	err     error
}

func (m *mockTradeLogger) LogOpen(_ context.Context, pos *Position) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, tradeLogEntry{isOpen: true, pos: pos})
	return m.err
}

func (m *mockTradeLogger) LogClose(_ context.Context, pos *Position) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, tradeLogEntry{isOpen: false, pos: pos})
	return m.err
}

func (m *mockTradeLogger) opens() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, e := range m.entries {
		if e.isOpen {
			n++
		}
	}
	return n
}

func (m *mockTradeLogger) closes() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, e := range m.entries {
		if !e.isOpen {
			n++
		}
	}
	return n
}

func TestTradeLoggerCalledOnExecute(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTC/USDT", 42000)

	exec := NewExecutor(prices)
	logger := &mockTradeLogger{}
	exec.SetTradeLogger(logger)

	opp := testOpp("BTC/USDT", claude.ActionBuy, 42000, 41500, 43000, 500)
	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if logger.opens() != 1 {
		t.Errorf("expected 1 open log, got %d", logger.opens())
	}
	if logger.closes() != 0 {
		t.Errorf("expected 0 close logs, got %d", logger.closes())
	}

	// verify logged position matches
	entry := logger.entries[0]
	if entry.pos.ID != pos.ID {
		t.Errorf("expected pos ID %s, got %s", pos.ID, entry.pos.ID)
	}
	if entry.pos.Symbol != "BTC/USDT" {
		t.Errorf("expected BTC/USDT, got %s", entry.pos.Symbol)
	}
}

func TestTradeLoggerCalledOnClose(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTC/USDT", 42000)

	exec := NewExecutor(prices)
	logger := &mockTradeLogger{}
	exec.SetTradeLogger(logger)

	opp := testOpp("BTC/USDT", claude.ActionBuy, 42000, 41500, 43000, 500)
	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	prices.set("BTC/USDT", 43500)
	_, err = exec.Close(pos.ID, CloseTP, 43500)
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}

	if logger.opens() != 1 {
		t.Errorf("expected 1 open log, got %d", logger.opens())
	}
	if logger.closes() != 1 {
		t.Errorf("expected 1 close log, got %d", logger.closes())
	}

	// verify close entry
	closeEntry := logger.entries[1]
	if closeEntry.pos.ClosePrice != 43500 {
		t.Errorf("expected close price 43500, got %f", closeEntry.pos.ClosePrice)
	}
}

func TestTradeLoggerNilSafe(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTC/USDT", 42000)

	exec := NewExecutor(prices)
	// deliberately NOT setting logger

	opp := testOpp("BTC/USDT", claude.ActionBuy, 42000, 41500, 43000, 500)
	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("execute should succeed without logger: %v", err)
	}

	_, err = exec.Close(pos.ID, CloseTP, 43500)
	if err != nil {
		t.Fatalf("close should succeed without logger: %v", err)
	}
}

func TestTradeLoggerErrorDoesNotFailTrade(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTC/USDT", 42000)

	exec := NewExecutor(prices)
	logger := &mockTradeLogger{err: fmt.Errorf("db connection lost")}
	exec.SetTradeLogger(logger)

	opp := testOpp("BTC/USDT", claude.ActionBuy, 42000, 41500, 43000, 500)
	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("execute should succeed despite logger error: %v", err)
	}
	if pos == nil {
		t.Fatal("position should not be nil")
	}
}

// --- circuit breaker integration tests ---

func TestExecutor_CircuitBreakerBlocks(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42000)
	exec := NewExecutor(prices)

	// configure breaker with low threshold
	cb := circuitbreaker.New(circuitbreaker.Config{
		MaxDailyLoss:         20,
		MaxConsecutiveLosses: 0,
		CooldownDuration:     time.Hour,
	})
	exec.SetCircuitBreaker(cb)

	// trip the breaker externally (simulating losses from other executors)
	cb.RecordTrade(1, -25)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42000, 41500, 43000, 500)
	_, err := exec.Execute(opp)
	if err == nil {
		t.Fatal("expected circuit breaker to block trade")
	}
	if !strings.Contains(err.Error(), "circuit breaker") {
		t.Errorf("error should mention circuit breaker, got: %v", err)
	}
}

func TestExecutor_CircuitBreakerAllows(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42000)
	exec := NewExecutor(prices)

	cb := circuitbreaker.New(circuitbreaker.Config{
		MaxDailyLoss:         100,
		MaxConsecutiveLosses: 10,
		CooldownDuration:     time.Hour,
	})
	exec.SetCircuitBreaker(cb)

	// small loss — shouldn't trip
	cb.RecordTrade(1, -10)

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42000, 41500, 43000, 500)
	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("trade should be allowed: %v", err)
	}
	if pos == nil {
		t.Fatal("position should not be nil")
	}
}

func TestExecutor_NilCircuitBreakerAllows(t *testing.T) {
	prices := newMockPrices()
	prices.set("BTCUSDT", 42000)
	exec := NewExecutor(prices)
	// no breaker set — should work normally

	opp := testOpp("BTCUSDT", claude.ActionBuy, 42000, 41500, 43000, 500)
	pos, err := exec.Execute(opp)
	if err != nil {
		t.Fatalf("trade should work without breaker: %v", err)
	}
	if pos == nil {
		t.Fatal("position should not be nil")
	}
}
