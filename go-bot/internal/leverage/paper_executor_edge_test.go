// edge case and utility tests for the paper leverage executor.
package leverage

import (
	"fmt"
	"strings"
	"testing"
)

func TestPaperExecutor_AllOpen(t *testing.T) {
	exec := newTestExecutor(50000)

	exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	exec.OpenPosition(2, "BTCUSDT", SideShort, 5, 300, 0, 0, "discord")
	exec.OpenPosition(3, "BTCUSDT", SideLong, 20, 100, 0, 0, "telegram")

	all := exec.AllOpen()
	if len(all) != 3 {
		t.Errorf("AllOpen() count = %d, want 3", len(all))
	}
}

func TestPaperExecutor_AllOpenEmpty(t *testing.T) {
	exec := newTestExecutor(50000)

	all := exec.AllOpen()
	if len(all) != 0 {
		t.Errorf("AllOpen() on empty executor = %d, want 0", len(all))
	}
}

func TestPaperExecutor_UpdateMarkPrice(t *testing.T) {
	exec := newTestExecutor(50000)

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")

	exec.UpdateMarkPrice(pos.ID, 51000)

	updated := exec.Get(pos.ID)
	if updated.MarkPrice != 51000 {
		t.Errorf("MarkPrice = %.2f, want 51000", updated.MarkPrice)
	}
	// entry price should remain unchanged
	if updated.EntryPrice != 50000 {
		t.Errorf("EntryPrice = %.2f, want 50000 (unchanged)", updated.EntryPrice)
	}
}

func TestPaperExecutor_UpdateMarkPriceNonExistent(t *testing.T) {
	exec := newTestExecutor(50000)

	// should not panic
	exec.UpdateMarkPrice("nonexistent", 51000)
}

func TestPaperExecutor_CloseNonExistent(t *testing.T) {
	exec := newTestExecutor(50000)

	_, err := exec.Close("nonexistent", "manual")
	if err == nil {
		t.Fatal("expected error when closing non-existent position")
	}
	if !strings.Contains(err.Error(), "position not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestPaperExecutor_CloseAlreadyClosed(t *testing.T) {
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 50000}}
	exec := NewPaperExecutor(prices, nil, NewFundingTracker())

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	posID := pos.ID

	_, err := exec.Close(posID, "manual")
	if err != nil {
		t.Fatalf("first Close() error: %v", err)
	}

	_, err = exec.Close(posID, "manual")
	if err == nil {
		t.Fatal("expected error when closing already closed position")
	}
	if !strings.Contains(err.Error(), "already closed") {
		t.Errorf("error should mention already closed, got: %v", err)
	}
}

func TestPaperExecutor_AdjustNonExistent(t *testing.T) {
	exec := newTestExecutor(50000)

	err := exec.Adjust("nonexistent", "sl", 48000)
	if err == nil {
		t.Fatal("expected error when adjusting non-existent position")
	}
	if !strings.Contains(err.Error(), "position not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestPaperExecutor_OpenZeroMargin(t *testing.T) {
	exec := newTestExecutor(50000)

	_, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 0, 0, 0, "telegram")
	if err == nil {
		t.Fatal("expected error for zero margin")
	}
	if !strings.Contains(err.Error(), "margin must be positive") {
		t.Errorf("error should mention margin, got: %v", err)
	}
}

func TestPaperExecutor_OpenNegativeMargin(t *testing.T) {
	exec := newTestExecutor(50000)

	_, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, -100, 0, 0, "telegram")
	if err == nil {
		t.Fatal("expected error for negative margin")
	}
	if !strings.Contains(err.Error(), "margin must be positive") {
		t.Errorf("error should mention margin, got: %v", err)
	}
}

func TestPaperExecutor_OpenZeroLeverage(t *testing.T) {
	exec := newTestExecutor(50000)

	_, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 0, 500, 0, 0, "telegram")
	if err == nil {
		t.Fatal("expected error for zero leverage")
	}
	if !strings.Contains(err.Error(), "leverage must be positive") {
		t.Errorf("error should mention leverage, got: %v", err)
	}
}

func TestPaperExecutor_PriceProviderError(t *testing.T) {
	prices := &mockPrices{err: fmt.Errorf("exchange down")}
	exec := NewPaperExecutor(prices, nil, NewFundingTracker())

	_, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	if err == nil {
		t.Fatal("expected error when price provider fails")
	}
	if !strings.Contains(err.Error(), "failed to get price") {
		t.Errorf("error should mention price failure, got: %v", err)
	}
}

func TestPaperExecutor_PriceProviderSymbolNotFound(t *testing.T) {
	exec := newTestExecutor(50000) // only has BTCUSDT

	_, err := exec.OpenPosition(1, "ETHUSDT", SideLong, 10, 500, 0, 0, "telegram")
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
	if !strings.Contains(err.Error(), "failed to get price") {
		t.Errorf("error should mention price failure, got: %v", err)
	}
}

func TestPaperExecutor_ClosePriceProviderError(t *testing.T) {
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 50000}}
	exec := NewPaperExecutor(prices, nil, NewFundingTracker())

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")

	// make price provider fail before close
	prices.err = fmt.Errorf("exchange down")

	_, err := exec.Close(pos.ID, "manual")
	if err == nil {
		t.Fatal("expected error when price provider fails on close")
	}
	if !strings.Contains(err.Error(), "failed to get close price") {
		t.Errorf("error should mention close price failure, got: %v", err)
	}

	// position should still be open
	if exec.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (position should remain open)", exec.Count())
	}
}

func TestPaperExecutor_CountAfterOperations(t *testing.T) {
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 50000}}
	exec := NewPaperExecutor(prices, nil, NewFundingTracker())

	if exec.Count() != 0 {
		t.Errorf("initial Count() = %d, want 0", exec.Count())
	}

	pos1, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	if exec.Count() != 1 {
		t.Errorf("after 1 open Count() = %d, want 1", exec.Count())
	}

	exec.OpenPosition(2, "BTCUSDT", SideShort, 5, 300, 0, 0, "discord")
	if exec.Count() != 2 {
		t.Errorf("after 2 opens Count() = %d, want 2", exec.Count())
	}

	exec.Close(pos1.ID, "manual")
	if exec.Count() != 1 {
		t.Errorf("after 1 close Count() = %d, want 1", exec.Count())
	}
}

func TestPaperExecutor_IDsAreUnique(t *testing.T) {
	exec := newTestExecutor(50000)

	pos1, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	pos2, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	pos3, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")

	ids := map[string]bool{pos1.ID: true, pos2.ID: true, pos3.ID: true}
	if len(ids) != 3 {
		t.Errorf("expected 3 unique IDs, got %d (ids: %s, %s, %s)", len(ids), pos1.ID, pos2.ID, pos3.ID)
	}
}

func TestPaperExecutor_IDPrefix(t *testing.T) {
	exec := newTestExecutor(50000)

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	if !strings.HasPrefix(pos.ID, "lp_") {
		t.Errorf("ID = %q, want prefix lp_", pos.ID)
	}
}

func TestPaperExecutor_NilSafetyAllowed(t *testing.T) {
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 50000}}
	exec := NewPaperExecutor(prices, nil, NewFundingTracker())

	// should succeed without safety checker
	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() with nil safety should succeed, got: %v", err)
	}
	if pos == nil {
		t.Fatal("position should not be nil")
	}
}

func TestPaperExecutor_CloseShortWithProfit(t *testing.T) {
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 50000}}
	exec := NewPaperExecutor(prices, nil, NewFundingTracker())

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideShort, 10, 500, 0, 0, "telegram")

	// price drops (good for short)
	prices.prices["BTCUSDT"] = 48000

	closed, err := exec.Close(pos.ID, "take_profit")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// pnl = (50000 - 48000) * 0.1 = 200
	wantPnL := 200.0
	if !almostEqual(closed.PnL, wantPnL, floatTolerance) {
		t.Errorf("PnL = %.6f, want %.6f", closed.PnL, wantPnL)
	}
}

func TestPaperExecutor_CloseLongWithLoss(t *testing.T) {
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 50000}}
	exec := NewPaperExecutor(prices, nil, NewFundingTracker())

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")

	// price drops (bad for long)
	prices.prices["BTCUSDT"] = 48000

	closed, err := exec.Close(pos.ID, "stop_loss")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// pnl = (48000 - 50000) * 0.1 = -200
	wantPnL := -200.0
	if !almostEqual(closed.PnL, wantPnL, floatTolerance) {
		t.Errorf("PnL = %.6f, want %.6f", closed.PnL, wantPnL)
	}
}

func TestPaperExecutor_CloseFlatPnL(t *testing.T) {
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 50000}}
	exec := NewPaperExecutor(prices, nil, NewFundingTracker())

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")

	// price unchanged
	closed, err := exec.Close(pos.ID, "manual")
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	if !almostEqual(closed.PnL, 0, floatTolerance) {
		t.Errorf("PnL = %.6f, want 0 (flat)", closed.PnL)
	}
}

func TestPaperExecutor_OpenPositionNoSLTP(t *testing.T) {
	exec := newTestExecutor(50000)

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}
	if pos.StopLoss != 0 {
		t.Errorf("StopLoss = %.2f, want 0", pos.StopLoss)
	}
	if pos.TakeProfit != 0 {
		t.Errorf("TakeProfit = %.2f, want 0", pos.TakeProfit)
	}
}

func TestPaperExecutor_FundingFeesReduceProfit(t *testing.T) {
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 50000}}
	funding := NewFundingTracker()
	exec := NewPaperExecutor(prices, nil, funding)

	pos, _ := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 500, 0, 0, "telegram")

	// accumulate significant funding fees
	funding.RecordPayment(pos.ID, 0.001, pos.NotionalValue) // 0.001 * 5000 = 5.0
	pos.FundingPaid = funding.CumulativeFees(pos.ID)

	// close at breakeven price: raw pnl = 0, but funding makes it negative
	closed, _ := exec.Close(pos.ID, "manual")

	// pnl = 0 - 5.0 = -5.0
	wantPnL := -5.0
	if !almostEqual(closed.PnL, wantPnL, floatTolerance) {
		t.Errorf("PnL = %.6f, want %.6f (funding fees should reduce profit)", closed.PnL, wantPnL)
	}
}
