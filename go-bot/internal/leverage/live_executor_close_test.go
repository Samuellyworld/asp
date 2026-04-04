package leverage

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/exchange"
)

// --- close position tests ---

func TestLiveExecutor_CloseWithProfit(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")

	futures.placeOrderResp = &binance.FuturesOrder{
		OrderID: 400, Symbol: "BTCUSDT", Side: exchange.SideSell,
		Status: exchange.OrderStatusFilled, Quantity: pos.Quantity,
		ExecutedQty: pos.Quantity, AvgPrice: 52000,
	}

	closed, err := exec.Close(pos.ID, "take_profit")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	if closed.Status != "closed" {
		t.Errorf("Status = %q, want closed", closed.Status)
	}
	if closed.CloseReason != "take_profit" {
		t.Errorf("CloseReason = %q, want take_profit", closed.CloseReason)
	}
	if closed.ClosePrice != 52000 {
		t.Errorf("ClosePrice = %f, want 52000", closed.ClosePrice)
	}
	if closed.ClosedAt == nil {
		t.Fatal("ClosedAt should not be nil")
	}

	// pnl: (52000 - 50000) * 0.02 = 40
	wantPnL := (52000.0 - 50000.0) * pos.Quantity
	if math.Abs(closed.PnL-wantPnL) > 1e-6 {
		t.Errorf("PnL = %f, want %f", closed.PnL, wantPnL)
	}

	if exec.Count() != 0 {
		t.Errorf("Count() = %d, want 0", exec.Count())
	}

	// should be accessible via Get (from closed list)
	got := exec.Get(pos.ID)
	if got == nil {
		t.Fatal("Get() should return closed position")
	}
	if got.Status != "closed" {
		t.Errorf("Get() status = %q, want closed", got.Status)
	}
}

func TestLiveExecutor_CloseWithLoss(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")

	futures.placeOrderResp = &binance.FuturesOrder{
		OrderID: 400, Symbol: "BTCUSDT", Side: exchange.SideSell,
		Status: exchange.OrderStatusFilled, Quantity: pos.Quantity,
		ExecutedQty: pos.Quantity, AvgPrice: 49000,
	}

	closed, err := exec.Close(pos.ID, "stop_loss")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// pnl: (49000 - 50000) * 0.02 = -20
	wantPnL := (49000.0 - 50000.0) * pos.Quantity
	if math.Abs(closed.PnL-wantPnL) > 1e-6 {
		t.Errorf("PnL = %f, want %f", closed.PnL, wantPnL)
	}
}

func TestLiveExecutor_CloseShortWithProfit(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideShort, 10, 100, 0, 0, "telegram")

	futures.placeOrderResp = &binance.FuturesOrder{
		OrderID: 400, Symbol: "BTCUSDT", Side: exchange.SideBuy,
		Status: exchange.OrderStatusFilled, Quantity: pos.Quantity,
		ExecutedQty: pos.Quantity, AvgPrice: 48000,
	}

	closed, err := exec.Close(pos.ID, "manual")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// short pnl: (50000 - 48000) * 0.02 = 40
	wantPnL := (50000.0 - 48000.0) * pos.Quantity
	if math.Abs(closed.PnL-wantPnL) > 1e-6 {
		t.Errorf("PnL = %f, want %f", closed.PnL, wantPnL)
	}
}

func TestLiveExecutor_CloseWithFundingFees(t *testing.T) {
	exec, futures, _, funding, _ := newTestLiveExecutor()

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")

	// record some funding fees
	funding.RecordPayment(pos.ID, 0.0001, pos.NotionalValue)
	funding.RecordPayment(pos.ID, 0.0002, pos.NotionalValue)
	expectedFunding := funding.CumulativeFees(pos.ID)

	futures.placeOrderResp = &binance.FuturesOrder{
		OrderID: 400, Symbol: "BTCUSDT", Side: exchange.SideSell,
		Status: exchange.OrderStatusFilled, Quantity: pos.Quantity,
		ExecutedQty: pos.Quantity, AvgPrice: 52000,
	}

	closed, err := exec.Close(pos.ID, "manual")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	rawPnL := (52000.0 - 50000.0) * pos.Quantity
	wantPnL := rawPnL - expectedFunding

	if math.Abs(closed.PnL-wantPnL) > 1e-6 {
		t.Errorf("PnL = %f, want %f (raw %f - funding %f)", closed.PnL, wantPnL, rawPnL, expectedFunding)
	}
	if math.Abs(closed.FundingPaid-expectedFunding) > 1e-6 {
		t.Errorf("FundingPaid = %f, want %f", closed.FundingPaid, expectedFunding)
	}

	// funding tracker should be cleaned up
	if fees := funding.CumulativeFees(pos.ID); fees != 0 {
		t.Errorf("funding fees should be cleaned up after close, got %f", fees)
	}
}

func TestLiveExecutor_CloseCancelsSLTP(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 48000, 55000, "telegram")

	if pos.SLOrderID == 0 || pos.TPOrderID == 0 {
		t.Fatal("expected SL and TP orders to be placed")
	}

	futures.placeOrderResp = &binance.FuturesOrder{
		OrderID: 400, Symbol: "BTCUSDT", Side: exchange.SideSell,
		Status: exchange.OrderStatusFilled, Quantity: pos.Quantity,
		ExecutedQty: pos.Quantity, AvgPrice: 51000,
	}

	_, err := exec.Close(pos.ID, "manual")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	if futures.cancelCalls != 2 {
		t.Errorf("CancelOrder called %d times, want 2", futures.cancelCalls)
	}
}

func TestLiveExecutor_CloseNonExistent(t *testing.T) {
	exec, _, _, _, _ := newTestLiveExecutor()

	_, err := exec.Close("lev_999", "manual")
	if err == nil {
		t.Fatal("expected error for non-existent position")
	}
	if !strings.Contains(err.Error(), "position not found") {
		t.Errorf("error = %q, want it to contain 'position not found'", err.Error())
	}
}

func TestLiveExecutor_CloseAlreadyClosed(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")

	futures.placeOrderResp = &binance.FuturesOrder{
		OrderID: 400, Symbol: "BTCUSDT", Side: exchange.SideSell,
		Status: exchange.OrderStatusFilled, Quantity: pos.Quantity,
		ExecutedQty: pos.Quantity, AvgPrice: 51000,
	}

	_, err := exec.Close(pos.ID, "manual")
	if err != nil {
		t.Fatalf("first Close() error: %v", err)
	}

	// second close should fail
	_, err = exec.Close(pos.ID, "manual")
	if err == nil {
		t.Fatal("expected error for already closed position")
	}
	if !strings.Contains(err.Error(), "position not found") {
		t.Errorf("error = %q, want it to contain 'position not found'", err.Error())
	}
}

func TestLiveExecutor_CloseKeyDecryptionError(t *testing.T) {
	exec, _, keys, _, _ := newTestLiveExecutor()

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")

	keys.err = fmt.Errorf("decryption failed")

	_, err := exec.Close(pos.ID, "manual")
	if err == nil {
		t.Fatal("expected key decryption error on close")
	}
	if !strings.Contains(err.Error(), "failed to decrypt keys") {
		t.Errorf("error = %q, want it to contain 'failed to decrypt keys'", err.Error())
	}
	if exec.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (position should remain open)", exec.Count())
	}
}

func TestLiveExecutor_CloseOrderError(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")

	futures.placeOrderErr = fmt.Errorf("close order rejected")

	_, err := exec.Close(pos.ID, "manual")
	if err == nil {
		t.Fatal("expected close order error")
	}
	if !strings.Contains(err.Error(), "failed to close position") {
		t.Errorf("error = %q, want it to contain 'failed to close position'", err.Error())
	}
	if exec.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (position should remain open)", exec.Count())
	}
}

func TestLiveExecutor_CloseFallbackClosePrice(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")

	futures.placeOrderResp = &binance.FuturesOrder{
		OrderID: 400, Symbol: "BTCUSDT", Side: exchange.SideSell,
		Status: exchange.OrderStatusFilled, Quantity: pos.Quantity,
		ExecutedQty: pos.Quantity, AvgPrice: 0,
	}

	closed, err := exec.Close(pos.ID, "manual")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if closed.ClosePrice != pos.MarkPrice {
		t.Errorf("ClosePrice = %f, want %f (mark price fallback)", closed.ClosePrice, pos.MarkPrice)
	}
}

func TestLiveExecutor_NilFundingTracker(t *testing.T) {
	futures := defaultMockFutures()
	keys := defaultMockKeys()
	prices := defaultMockPrices()

	exec := NewLiveExecutor(futures, keys, nil, nil, prices)

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}

	futures.placeOrderResp = &binance.FuturesOrder{
		OrderID: 400, Symbol: "BTCUSDT", Side: exchange.SideSell,
		Status: exchange.OrderStatusFilled, Quantity: pos.Quantity,
		ExecutedQty: pos.Quantity, AvgPrice: 52000,
	}

	closed, err := exec.Close(pos.ID, "manual")
	if err != nil {
		t.Fatalf("Close() with nil funding tracker error: %v", err)
	}

	wantPnL := (52000.0 - 50000.0) * pos.Quantity
	if math.Abs(closed.PnL-wantPnL) > 1e-6 {
		t.Errorf("PnL = %f, want %f", closed.PnL, wantPnL)
	}
	if closed.FundingPaid != 0 {
		t.Errorf("FundingPaid = %f, want 0 (nil tracker)", closed.FundingPaid)
	}
}

// --- accessor tests ---

func TestLiveExecutor_Get(t *testing.T) {
	exec, _, _, _, _ := newTestLiveExecutor()

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")

	got := exec.Get(pos.ID)
	if got == nil {
		t.Fatal("Get() returned nil for open position")
	}
	if got.ID != pos.ID {
		t.Errorf("Get() ID = %q, want %q", got.ID, pos.ID)
	}

	got = exec.Get("lev_999")
	if got != nil {
		t.Errorf("Get() should return nil for non-existent position, got %v", got)
	}
}

func TestLiveExecutor_OpenPositions(t *testing.T) {
	exec, _, _, _, _ := newTestLiveExecutor()

	exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")
	exec.OpenPosition(1, "ETHUSDT", SideShort, 5, 50, 0, 0, "telegram")
	exec.OpenPosition(2, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")

	user1Pos := exec.OpenPositions(1)
	if len(user1Pos) != 2 {
		t.Errorf("OpenPositions(1) count = %d, want 2", len(user1Pos))
	}

	user2Pos := exec.OpenPositions(2)
	if len(user2Pos) != 1 {
		t.Errorf("OpenPositions(2) count = %d, want 1", len(user2Pos))
	}

	user3Pos := exec.OpenPositions(3)
	if len(user3Pos) != 0 {
		t.Errorf("OpenPositions(3) count = %d, want 0", len(user3Pos))
	}
}

func TestLiveExecutor_AllOpen(t *testing.T) {
	exec, _, _, _, _ := newTestLiveExecutor()

	exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")
	exec.OpenPosition(2, "ETHUSDT", SideShort, 5, 50, 0, 0, "telegram")

	all := exec.AllOpen()
	if len(all) != 2 {
		t.Errorf("AllOpen() count = %d, want 2", len(all))
	}
}

func TestLiveExecutor_Count(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()

	if exec.Count() != 0 {
		t.Errorf("Count() = %d, want 0", exec.Count())
	}

	exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")
	if exec.Count() != 1 {
		t.Errorf("Count() = %d, want 1", exec.Count())
	}

	exec.OpenPosition(1, "ETHUSDT", SideShort, 5, 50, 0, 0, "telegram")
	if exec.Count() != 2 {
		t.Errorf("Count() = %d, want 2", exec.Count())
	}

	// close first position
	pos := exec.AllOpen()[0]
	futures.placeOrderResp = &binance.FuturesOrder{
		OrderID: 400, Symbol: pos.Symbol, Side: exchange.SideSell,
		Status: exchange.OrderStatusFilled, Quantity: pos.Quantity,
		ExecutedQty: pos.Quantity, AvgPrice: 51000,
	}
	exec.Close(pos.ID, "manual")

	if exec.Count() != 1 {
		t.Errorf("Count() = %d, want 1 after close", exec.Count())
	}
}

func TestLiveExecutor_NilSafetyChecker(t *testing.T) {
	exec := NewLiveExecutor(defaultMockFutures(), defaultMockKeys(), nil, NewFundingTracker(), defaultMockPrices())

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() should succeed with nil safety checker: %v", err)
	}
	if pos == nil {
		t.Fatal("position should not be nil")
	}
}

func TestLiveExecutor_MultiplePositionIDs(t *testing.T) {
	exec, _, _, _, _ := newTestLiveExecutor()

	pos1, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")
	pos2, _ := exec.OpenPosition(1, "ETHUSDT", SideLong, 5, 50, 0, 0, "telegram")

	if pos1.ID == pos2.ID {
		t.Errorf("position IDs should be unique, got %q and %q", pos1.ID, pos2.ID)
	}
	if pos1.ID != "lev_1" {
		t.Errorf("first ID = %q, want lev_1", pos1.ID)
	}
	if pos2.ID != "lev_2" {
		t.Errorf("second ID = %q, want lev_2", pos2.ID)
	}
}

func TestLiveExecutor_UserKeysMocked(t *testing.T) {
	futures := defaultMockFutures()
	keys := &mockLiveKeys{apiKey: "custom_key", apiSecret: "custom_secret"}
	prices := defaultMockPrices()
	funding := NewFundingTracker()

	exec := NewLiveExecutor(futures, keys, nil, funding, prices)

	_, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() with custom keys error: %v", err)
	}
}
