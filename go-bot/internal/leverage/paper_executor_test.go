package leverage

import (
	"fmt"
	"strings"
	"testing"
)

// mock price provider for testing
type mockPrices struct {
	prices map[string]float64
	err    error
}

func (m *mockPrices) GetPrice(symbol string) (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	p, ok := m.prices[symbol]
	if !ok {
		return 0, fmt.Errorf("no price for %s", symbol)
	}
	return p, nil
}

// mock balance provider for safety checker
type mockBalance struct {
	balance float64
	err     error
}

func (m *mockBalance) GetFuturesBalance(_ int, _ string) (float64, error) {
	return m.balance, m.err
}

// mock leverage status provider
type mockLeverageStatus struct {
	enabled bool
}

func (m *mockLeverageStatus) IsLeverageEnabled(_ int) bool {
	return m.enabled
}

// helper to create a standard executor for tests
func newTestExecutor(price float64) *PaperExecutor {
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": price}}
	return NewPaperExecutor(prices, nil, NewFundingTracker())
}

// helper to create an executor with safety checks enabled
func newTestExecutorWithSafety(price float64, cfg SafetyConfig) *PaperExecutor {
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": price}}
	balance := &mockBalance{balance: 10000}
	status := &mockLeverageStatus{enabled: true}
	safety := NewSafetyChecker(cfg, balance, status)
	return NewPaperExecutor(prices, safety, NewFundingTracker())
}

func TestPaperExecutor_NewPaperExecutor(t *testing.T) {
	prices := &mockPrices{prices: map[string]float64{}}
	funding := NewFundingTracker()
	exec := NewPaperExecutor(prices, nil, funding)

	if exec == nil {
		t.Fatal("NewPaperExecutor returned nil")
	}
	if exec.positions == nil {
		t.Fatal("positions map should be initialized")
	}
	if exec.prices != prices {
		t.Error("prices provider not set correctly")
	}
	if exec.funding != funding {
		t.Error("funding tracker not set correctly")
	}
	if exec.safety != nil {
		t.Error("safety should be nil when not provided")
	}
	if exec.Count() != 0 {
		t.Errorf("new executor should have 0 positions, got %d", exec.Count())
	}
}

func TestPaperExecutor_OpenLongPosition(t *testing.T) {
	exec := newTestExecutor(50000)

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 48000, 55000, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}

	if pos.ID == "" {
		t.Error("position ID should not be empty")
	}
	if pos.UserID != 1 {
		t.Errorf("UserID = %d, want 1", pos.UserID)
	}
	if pos.Symbol != "BTCUSDT" {
		t.Errorf("Symbol = %q, want BTCUSDT", pos.Symbol)
	}
	if pos.Side != SideLong {
		t.Errorf("Side = %q, want LONG", pos.Side)
	}
	if pos.Leverage != 10 {
		t.Errorf("Leverage = %d, want 10", pos.Leverage)
	}
	if pos.EntryPrice != 50000 {
		t.Errorf("EntryPrice = %.2f, want 50000", pos.EntryPrice)
	}
	if pos.MarkPrice != 50000 {
		t.Errorf("MarkPrice = %.2f, want 50000 (should match entry)", pos.MarkPrice)
	}
	if pos.Margin != 500 {
		t.Errorf("Margin = %.2f, want 500", pos.Margin)
	}

	// notional = margin * leverage = 500 * 10 = 5000
	wantNotional := 5000.0
	if !almostEqual(pos.NotionalValue, wantNotional, floatTolerance) {
		t.Errorf("NotionalValue = %.6f, want %.6f", pos.NotionalValue, wantNotional)
	}

	// quantity = notional / price = 5000 / 50000 = 0.1
	wantQty := 0.1
	if !almostEqual(pos.Quantity, wantQty, floatTolerance) {
		t.Errorf("Quantity = %.6f, want %.6f", pos.Quantity, wantQty)
	}

	// liquidation price for long 10x: entry * (1 - 1/10 + 0.004) = 50000 * 0.904 = 45200
	wantLiq := CalculateLiquidationPrice(50000, 10, string(SideLong), DefaultMaintenanceMarginRate)
	if !almostEqual(pos.LiquidationPrice, wantLiq, floatTolerance) {
		t.Errorf("LiquidationPrice = %.6f, want %.6f", pos.LiquidationPrice, wantLiq)
	}

	if pos.StopLoss != 48000 {
		t.Errorf("StopLoss = %.2f, want 48000", pos.StopLoss)
	}
	if pos.TakeProfit != 55000 {
		t.Errorf("TakeProfit = %.2f, want 55000", pos.TakeProfit)
	}
	if !pos.IsPaper {
		t.Error("IsPaper should be true")
	}
	if pos.Status != "open" {
		t.Errorf("Status = %q, want open", pos.Status)
	}
	if pos.MarginType != "isolated" {
		t.Errorf("MarginType = %q, want isolated", pos.MarginType)
	}
	if pos.Platform != "telegram" {
		t.Errorf("Platform = %q, want telegram", pos.Platform)
	}
	if pos.OpenedAt.IsZero() {
		t.Error("OpenedAt should not be zero")
	}
	if pos.MainOrderID != 0 {
		t.Errorf("MainOrderID = %d, want 0 for paper", pos.MainOrderID)
	}
}

func TestPaperExecutor_OpenShortPosition(t *testing.T) {
	exec := newTestExecutor(50000)

	pos, err := exec.OpenPosition(2, "BTCUSDT", SideShort, 5, 1000, 52000, 45000, "discord")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}

	if pos.Side != SideShort {
		t.Errorf("Side = %q, want SHORT", pos.Side)
	}

	// notional = 1000 * 5 = 5000, quantity = 5000 / 50000 = 0.1
	wantNotional := 5000.0
	if !almostEqual(pos.NotionalValue, wantNotional, floatTolerance) {
		t.Errorf("NotionalValue = %.6f, want %.6f", pos.NotionalValue, wantNotional)
	}

	wantQty := 0.1
	if !almostEqual(pos.Quantity, wantQty, floatTolerance) {
		t.Errorf("Quantity = %.6f, want %.6f", pos.Quantity, wantQty)
	}

	// liquidation price for short 5x: entry * (1 + 1/5 - 0.004) = 50000 * 1.196 = 59800
	wantLiq := CalculateLiquidationPrice(50000, 5, string(SideShort), DefaultMaintenanceMarginRate)
	if !almostEqual(pos.LiquidationPrice, wantLiq, floatTolerance) {
		t.Errorf("LiquidationPrice = %.6f, want %.6f", pos.LiquidationPrice, wantLiq)
	}

	if pos.StopLoss != 52000 {
		t.Errorf("StopLoss = %.2f, want 52000", pos.StopLoss)
	}
	if pos.TakeProfit != 45000 {
		t.Errorf("TakeProfit = %.2f, want 45000", pos.TakeProfit)
	}
	if pos.UserID != 2 {
		t.Errorf("UserID = %d, want 2", pos.UserID)
	}
	if pos.Platform != "discord" {
		t.Errorf("Platform = %q, want discord", pos.Platform)
	}
}

func TestPaperExecutor_SafetyCheckBlocks(t *testing.T) {
	cfg := SafetyConfig{
		HardMaxLeverage:        20,
		UserMaxLeverage:        10,
		MaxMarginPerTrade:      500,
		MinLiquidationDistance:  10,
		RequireLeverageEnabled: true,
	}
	exec := newTestExecutorWithSafety(50000, cfg)

	// try opening with leverage exceeding hard cap
	_, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 25, 100, 0, 0, "telegram")
	if err == nil {
		t.Fatal("expected safety check error for excessive leverage")
	}
	if !strings.Contains(err.Error(), "safety check failed") {
		t.Errorf("error should mention safety check, got: %v", err)
	}
}

func TestPaperExecutor_SafetyCheckMarginLimit(t *testing.T) {
	cfg := SafetyConfig{
		HardMaxLeverage:        20,
		UserMaxLeverage:        10,
		MaxMarginPerTrade:      500,
		MinLiquidationDistance:  10,
		RequireLeverageEnabled: true,
	}
	exec := newTestExecutorWithSafety(50000, cfg)

	// margin exceeds limit
	_, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 5, 600, 0, 0, "telegram")
	if err == nil {
		t.Fatal("expected safety check error for excessive margin")
	}
	if !strings.Contains(err.Error(), "safety check failed") {
		t.Errorf("error should mention safety check, got: %v", err)
	}
}

func TestPaperExecutor_CloseLongWithProfit(t *testing.T) {
	// open at 50000, close at 52000 => profit
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 50000}}
	exec := NewPaperExecutor(prices, nil, NewFundingTracker())

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}

	// price moves up
	prices.prices["BTCUSDT"] = 52000

	closed, err := exec.Close(pos.ID, "take_profit")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// pnl = (52000 - 50000) * 0.1 = 200 (quantity = 5000/50000 = 0.1)
	wantPnL := 200.0
	if !almostEqual(closed.PnL, wantPnL, floatTolerance) {
		t.Errorf("PnL = %.6f, want %.6f", closed.PnL, wantPnL)
	}
	if closed.Status != "closed" {
		t.Errorf("Status = %q, want closed", closed.Status)
	}
	if closed.CloseReason != "take_profit" {
		t.Errorf("CloseReason = %q, want take_profit", closed.CloseReason)
	}
	if closed.ClosePrice != 52000 {
		t.Errorf("ClosePrice = %.2f, want 52000", closed.ClosePrice)
	}
	if closed.ClosedAt == nil {
		t.Error("ClosedAt should not be nil")
	}

	// verify removed from open and added to closed
	if exec.Count() != 0 {
		t.Errorf("Count() = %d, want 0 after close", exec.Count())
	}
}

func TestPaperExecutor_CloseShortWithLoss(t *testing.T) {
	// open short at 50000, close at 52000 => loss
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 50000}}
	exec := NewPaperExecutor(prices, nil, NewFundingTracker())

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideShort, 10, 500, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}

	// price moves up (bad for short)
	prices.prices["BTCUSDT"] = 52000

	closed, err := exec.Close(pos.ID, "stop_loss")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// pnl = (50000 - 52000) * 0.1 = -200
	wantPnL := -200.0
	if !almostEqual(closed.PnL, wantPnL, floatTolerance) {
		t.Errorf("PnL = %.6f, want %.6f", closed.PnL, wantPnL)
	}
	if closed.CloseReason != "stop_loss" {
		t.Errorf("CloseReason = %q, want stop_loss", closed.CloseReason)
	}
}

func TestPaperExecutor_CloseIncludesFundingFees(t *testing.T) {
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 50000}}
	funding := NewFundingTracker()
	exec := NewPaperExecutor(prices, nil, funding)

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}

	// simulate funding payments: 0.0001 * 5000 = 0.5, and 0.0002 * 5000 = 1.0
	// total funding = 1.5
	funding.RecordPayment(pos.ID, 0.0001, pos.NotionalValue)
	funding.RecordPayment(pos.ID, 0.0002, pos.NotionalValue)
	pos.FundingPaid = funding.CumulativeFees(pos.ID) // 1.5

	// price moves up: raw pnl = (52000 - 50000) * 0.1 = 200
	prices.prices["BTCUSDT"] = 52000

	closed, err := exec.Close(pos.ID, "manual")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// pnl = raw_pnl - funding_paid = 200 - 1.5 = 198.5
	wantPnL := 198.5
	if !almostEqual(closed.PnL, wantPnL, floatTolerance) {
		t.Errorf("PnL = %.6f, want %.6f (raw 200 - funding 1.5)", closed.PnL, wantPnL)
	}
}

func TestPaperExecutor_AdjustStopLoss(t *testing.T) {
	exec := newTestExecutor(50000)

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 48000, 55000, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}

	// adjust stop loss
	err = exec.Adjust(pos.ID, "sl", 49000)
	if err != nil {
		t.Fatalf("Adjust(sl) error: %v", err)
	}

	updated := exec.Get(pos.ID)
	if updated.StopLoss != 49000 {
		t.Errorf("StopLoss = %.2f, want 49000", updated.StopLoss)
	}
	// take profit unchanged
	if updated.TakeProfit != 55000 {
		t.Errorf("TakeProfit = %.2f, want 55000 (unchanged)", updated.TakeProfit)
	}
}

func TestPaperExecutor_AdjustTakeProfit(t *testing.T) {
	exec := newTestExecutor(50000)

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 48000, 55000, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}

	err = exec.Adjust(pos.ID, "tp", 56000)
	if err != nil {
		t.Fatalf("Adjust(tp) error: %v", err)
	}

	updated := exec.Get(pos.ID)
	if updated.TakeProfit != 56000 {
		t.Errorf("TakeProfit = %.2f, want 56000", updated.TakeProfit)
	}
}

func TestPaperExecutor_AdjustAlternateFieldNames(t *testing.T) {
	exec := newTestExecutor(50000)

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 48000, 55000, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}

	err = exec.Adjust(pos.ID, "stop_loss", 49500)
	if err != nil {
		t.Fatalf("Adjust(stop_loss) error: %v", err)
	}
	if exec.Get(pos.ID).StopLoss != 49500 {
		t.Errorf("StopLoss = %.2f, want 49500", exec.Get(pos.ID).StopLoss)
	}

	err = exec.Adjust(pos.ID, "take_profit", 56500)
	if err != nil {
		t.Fatalf("Adjust(take_profit) error: %v", err)
	}
	if exec.Get(pos.ID).TakeProfit != 56500 {
		t.Errorf("TakeProfit = %.2f, want 56500", exec.Get(pos.ID).TakeProfit)
	}
}

func TestPaperExecutor_AdjustUnknownField(t *testing.T) {
	exec := newTestExecutor(50000)

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}

	err = exec.Adjust(pos.ID, "leverage", 20)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("error should mention unknown field, got: %v", err)
	}
}

func TestPaperExecutor_GetOpenPosition(t *testing.T) {
	exec := newTestExecutor(50000)

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")

	got := exec.Get(pos.ID)
	if got == nil {
		t.Fatal("Get() returned nil for open position")
	}
	if got.ID != pos.ID {
		t.Errorf("ID = %q, want %q", got.ID, pos.ID)
	}
}

func TestPaperExecutor_GetClosedPosition(t *testing.T) {
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 50000}}
	exec := NewPaperExecutor(prices, nil, NewFundingTracker())

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	posID := pos.ID

	_, err := exec.Close(posID, "manual")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	got := exec.Get(posID)
	if got == nil {
		t.Fatal("Get() returned nil for closed position")
	}
	if got.Status != "closed" {
		t.Errorf("Status = %q, want closed", got.Status)
	}
}

func TestPaperExecutor_GetNonExistent(t *testing.T) {
	exec := newTestExecutor(50000)

	got := exec.Get("nonexistent")
	if got != nil {
		t.Errorf("Get() for nonexistent position should return nil, got %v", got)
	}
}

func TestPaperExecutor_OpenPositionsFiltering(t *testing.T) {
	exec := newTestExecutor(50000)

	exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	exec.OpenPosition(1, "BTCUSDT", SideShort, 5, 300, 0, 0, "telegram")
	exec.OpenPosition(2, "BTCUSDT", SideLong, 10, 500, 0, 0, "discord")

	user1 := exec.OpenPositions(1)
	if len(user1) != 2 {
		t.Errorf("OpenPositions(1) count = %d, want 2", len(user1))
	}
	for _, pos := range user1 {
		if pos.UserID != 1 {
			t.Errorf("position in user1 results has UserID %d", pos.UserID)
		}
	}

	user2 := exec.OpenPositions(2)
	if len(user2) != 1 {
		t.Errorf("OpenPositions(2) count = %d, want 1", len(user2))
	}

	user99 := exec.OpenPositions(99)
	if len(user99) != 0 {
		t.Errorf("OpenPositions(99) count = %d, want 0", len(user99))
	}
}