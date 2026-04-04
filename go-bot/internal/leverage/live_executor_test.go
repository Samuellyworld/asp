package leverage

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/exchange"
)

// --- mocks for live executor tests ---

type mockFutures struct {
	setLeverageCalls  int
	setLeverageErr    error
	setMarginCalls    int
	setMarginTypeErr  error
	placeOrderResp    *binance.FuturesOrder
	placeOrderErr     error
	placeStopResp     *binance.FuturesOrder
	placeStopErr      error
	placeTPResp       *binance.FuturesOrder
	placeTPErr        error
	cancelCalls       int
	cancelErr         error
	positions         []binance.FuturesPosition
	positionsErr      error
	lastOrderSide     exchange.OrderSide
	lastOrderQty      float64
	lastStopPrice     float64
	lastTPPrice       float64
}

func (m *mockFutures) SetLeverage(symbol string, leverage int, apiKey, apiSecret string) error {
	m.setLeverageCalls++
	return m.setLeverageErr
}

func (m *mockFutures) SetMarginType(symbol string, marginType string, apiKey, apiSecret string) error {
	m.setMarginCalls++
	return m.setMarginTypeErr
}

func (m *mockFutures) PlaceOrder(symbol string, side exchange.OrderSide, orderType exchange.OrderType, quantity, price float64, apiKey, apiSecret string) (*binance.FuturesOrder, error) {
	m.lastOrderSide = side
	m.lastOrderQty = quantity
	if m.placeOrderErr != nil {
		return nil, m.placeOrderErr
	}
	if m.placeOrderResp != nil {
		return m.placeOrderResp, nil
	}
	return &binance.FuturesOrder{
		OrderID:     100,
		Symbol:      symbol,
		Side:        side,
		Status:      exchange.OrderStatusFilled,
		Quantity:    quantity,
		ExecutedQty: quantity,
		AvgPrice:    50000,
	}, nil
}

func (m *mockFutures) PlaceStopMarket(symbol string, side exchange.OrderSide, quantity, stopPrice float64, apiKey, apiSecret string) (*binance.FuturesOrder, error) {
	m.lastStopPrice = stopPrice
	if m.placeStopErr != nil {
		return nil, m.placeStopErr
	}
	if m.placeStopResp != nil {
		return m.placeStopResp, nil
	}
	return &binance.FuturesOrder{
		OrderID:   200,
		Symbol:    symbol,
		Side:      side,
		Status:    exchange.OrderStatusNew,
		StopPrice: stopPrice,
		Quantity:  quantity,
	}, nil
}

func (m *mockFutures) PlaceTakeProfitMarket(symbol string, side exchange.OrderSide, quantity, stopPrice float64, apiKey, apiSecret string) (*binance.FuturesOrder, error) {
	m.lastTPPrice = stopPrice
	if m.placeTPErr != nil {
		return nil, m.placeTPErr
	}
	if m.placeTPResp != nil {
		return m.placeTPResp, nil
	}
	return &binance.FuturesOrder{
		OrderID:   300,
		Symbol:    symbol,
		Side:      side,
		Status:    exchange.OrderStatusNew,
		StopPrice: stopPrice,
		Quantity:  quantity,
	}, nil
}

func (m *mockFutures) CancelOrder(symbol string, orderID int64, apiKey, apiSecret string) error {
	m.cancelCalls++
	return m.cancelErr
}

func (m *mockFutures) GetPositions(apiKey, apiSecret string) ([]binance.FuturesPosition, error) {
	if m.positionsErr != nil {
		return nil, m.positionsErr
	}
	return m.positions, nil
}

type mockLiveKeys struct {
	apiKey    string
	apiSecret string
	err       error
}

func (m *mockLiveKeys) DecryptKeys(userID int) (string, string, error) {
	if m.err != nil {
		return "", "", m.err
	}
	return m.apiKey, m.apiSecret, nil
}

type mockMarkPrice struct {
	prices map[string]float64
	err    error
}

func (m *mockMarkPrice) GetMarkPrice(symbol string) (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	if p, ok := m.prices[symbol]; ok {
		return p, nil
	}
	return 0, fmt.Errorf("no price for %s", symbol)
}

// mock balance provider for live safety checker
type mockLiveBalance struct {
	balance float64
	err     error
}

func (m *mockLiveBalance) GetFuturesBalance(userID int, asset string) (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.balance, nil
}

// mock leverage status provider for live safety checker
type mockLiveLeverageStatus struct {
	enabled bool
}

func (m *mockLiveLeverageStatus) IsLeverageEnabled(userID int) bool {
	return m.enabled
}

// --- helpers ---
func defaultMockFutures() *mockFutures {
	return &mockFutures{
		positions: []binance.FuturesPosition{
			{
				Symbol:           "BTCUSDT",
				LiquidationPrice: 45000,
				EntryPrice:       50000,
				PositionAmt:      0.02,
				Leverage:         10,
				MarkPrice:        50000,
			},
		},
	}
}

func defaultMockKeys() *mockLiveKeys {
	return &mockLiveKeys{apiKey: "test_key", apiSecret: "test_secret"}
}

func defaultMockPrices() *mockMarkPrice {
	return &mockMarkPrice{
		prices: map[string]float64{
			"BTCUSDT": 50000,
			"ETHUSDT": 3000,
		},
	}
}

func defaultSafetyChecker() *SafetyChecker {
	cfg := SafetyConfig{
		HardMaxLeverage:        20,
		UserMaxLeverage:        10,
		MaxMarginPerTrade:      500,
		MinLiquidationDistance:  5,
		RequireLeverageEnabled: true,
	}
	return NewSafetyChecker(
		cfg,
		&mockLiveBalance{balance: 10000},
		&mockLiveLeverageStatus{enabled: true},
	)
}

func newTestLiveExecutor() (*LiveExecutor, *mockFutures, *mockLiveKeys, *FundingTracker, *mockMarkPrice) {
	futures := defaultMockFutures()
	keys := defaultMockKeys()
	funding := NewFundingTracker()
	prices := defaultMockPrices()
	exec := NewLiveExecutor(futures, keys, nil, funding, prices)
	return exec, futures, keys, funding, prices
}

// --- open position tests ---

func TestLiveExecutor_NewLiveExecutor(t *testing.T) {
	exec, _, _, _, _ := newTestLiveExecutor()
	if exec == nil {
		t.Fatal("NewLiveExecutor() returned nil")
	}
	if exec.positions == nil {
		t.Fatal("positions map should be initialized")
	}
	if exec.Count() != 0 {
		t.Errorf("expected 0 positions, got %d", exec.Count())
	}
}

func TestLiveExecutor_OpenLong(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 48000, 55000, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}

	// verify all position fields
	checks := []struct{ name, got, want string }{
		{"ID", pos.ID, "lev_1"},
		{"Symbol", pos.Symbol, "BTCUSDT"},
		{"Side", string(pos.Side), string(SideLong)},
		{"Status", pos.Status, "open"},
		{"Platform", pos.Platform, "telegram"},
		{"MarginType", pos.MarginType, "isolated"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if pos.UserID != 1 {
		t.Errorf("UserID = %d, want 1", pos.UserID)
	}
	if pos.Leverage != 10 {
		t.Errorf("Leverage = %d, want 10", pos.Leverage)
	}
	if pos.EntryPrice != 50000 {
		t.Errorf("EntryPrice = %f, want 50000", pos.EntryPrice)
	}
	if pos.Margin != 100 {
		t.Errorf("Margin = %f, want 100", pos.Margin)
	}
	if pos.IsPaper {
		t.Error("IsPaper should be false for live positions")
	}
	if pos.MainOrderID != 100 || pos.SLOrderID != 200 || pos.TPOrderID != 300 {
		t.Errorf("order IDs = (%d,%d,%d), want (100,200,300)", pos.MainOrderID, pos.SLOrderID, pos.TPOrderID)
	}
	if pos.StopLoss != 48000 || pos.TakeProfit != 55000 {
		t.Errorf("SL/TP = (%f,%f), want (48000,55000)", pos.StopLoss, pos.TakeProfit)
	}
	if pos.OpenedAt.IsZero() {
		t.Error("OpenedAt should not be zero")
	}
	// quantity: notional = 100 * 10 = 1000, qty = 1000 / 50000 = 0.02
	if math.Abs(pos.Quantity-0.02) > 1e-10 {
		t.Errorf("Quantity = %f, want 0.02", pos.Quantity)
	}
	if math.Abs(pos.NotionalValue-pos.EntryPrice*pos.Quantity) > 1e-6 {
		t.Errorf("NotionalValue = %f, want %f", pos.NotionalValue, pos.EntryPrice*pos.Quantity)
	}
	// exchange liquidation price should be used (from mock positions)
	if pos.LiquidationPrice != 45000 {
		t.Errorf("LiquidationPrice = %f, want 45000 (from exchange)", pos.LiquidationPrice)
	}
	// verify exchange interactions
	if futures.setLeverageCalls != 1 || futures.setMarginCalls != 1 {
		t.Errorf("SetLeverage/SetMarginType calls = (%d,%d), want (1,1)", futures.setLeverageCalls, futures.setMarginCalls)
	}
	if futures.lastOrderSide != exchange.SideBuy {
		t.Errorf("order side = %q, want BUY", futures.lastOrderSide)
	}
	if futures.lastStopPrice != 48000 || futures.lastTPPrice != 55000 {
		t.Errorf("SL/TP prices = (%f,%f), want (48000,55000)", futures.lastStopPrice, futures.lastTPPrice)
	}
	if exec.Count() != 1 {
		t.Errorf("Count() = %d, want 1", exec.Count())
	}
}

func TestLiveExecutor_OpenShort(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideShort, 10, 100, 52000, 47000, "discord")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}

	if pos.Side != SideShort {
		t.Errorf("Side = %q, want %q", pos.Side, SideShort)
	}
	if pos.Platform != "discord" {
		t.Errorf("Platform = %q, want discord", pos.Platform)
	}
	if pos.StopLoss != 52000 {
		t.Errorf("StopLoss = %f, want 52000", pos.StopLoss)
	}
	if pos.TakeProfit != 47000 {
		t.Errorf("TakeProfit = %f, want 47000", pos.TakeProfit)
	}
	if futures.lastOrderSide != exchange.SideSell {
		t.Errorf("order side = %q, want SELL for short", futures.lastOrderSide)
	}
	if pos.IsPaper {
		t.Error("IsPaper should be false")
	}
}

func TestLiveExecutor_OpenNoSLTP(t *testing.T) {
	exec, _, _, _, _ := newTestLiveExecutor()

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}
	if pos.SLOrderID != 0 {
		t.Errorf("SLOrderID = %d, want 0 (no SL)", pos.SLOrderID)
	}
	if pos.TPOrderID != 0 {
		t.Errorf("TPOrderID = %d, want 0 (no TP)", pos.TPOrderID)
	}
}

func TestLiveExecutor_OpenSLTPErrors(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()
	futures.placeStopErr = fmt.Errorf("stop market failed")
	futures.placeTPErr = fmt.Errorf("take profit failed")

	// SL failure should now abort and reverse the position (safety-critical)
	_, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 48000, 55000, "telegram")
	if err == nil {
		t.Fatal("OpenPosition() should fail when SL placement fails")
	}
}

func TestLiveExecutor_OpenTPErrorContinues(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()
	futures.placeTPErr = fmt.Errorf("take profit failed")

	// TP failure should warn but NOT abort (SL still protects)
	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 48000, 55000, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() should succeed with only TP error: %v", err)
	}
	if pos.TPOrderID != 0 {
		t.Errorf("TPOrderID = %d, want 0 (TP order failed)", pos.TPOrderID)
	}
	if pos.SLOrderID == 0 {
		t.Error("SLOrderID should be set (SL succeeded)")
	}
}

func TestLiveExecutor_OpenUsesCustomAvgPrice(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()
	futures.placeOrderResp = &binance.FuturesOrder{
		OrderID: 101, Symbol: "BTCUSDT", Side: exchange.SideBuy,
		Status: exchange.OrderStatusFilled, Quantity: 0.025,
		ExecutedQty: 0.025, AvgPrice: 49800,
	}

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}
	if pos.EntryPrice != 49800 {
		t.Errorf("EntryPrice = %f, want 49800 (from fill)", pos.EntryPrice)
	}
	if pos.Quantity != 0.025 {
		t.Errorf("Quantity = %f, want 0.025 (from fill)", pos.Quantity)
	}
}

func TestLiveExecutor_OpenFallsBackToMarkPrice(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()
	futures.placeOrderResp = &binance.FuturesOrder{
		OrderID: 101, Symbol: "BTCUSDT", Side: exchange.SideBuy,
		Status: exchange.OrderStatusFilled, Quantity: 0.02,
		ExecutedQty: 0, AvgPrice: 0,
	}

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() error: %v", err)
	}
	if pos.EntryPrice != 50000 {
		t.Errorf("EntryPrice = %f, want 50000 (mark price fallback)", pos.EntryPrice)
	}
}

func TestLiveExecutor_OpenSafetyCheckBlocks(t *testing.T) {
	safety := defaultSafetyChecker()
	exec := NewLiveExecutor(defaultMockFutures(), defaultMockKeys(), safety, NewFundingTracker(), defaultMockPrices())

	_, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 15, 100, 48000, 55000, "telegram")
	if err == nil {
		t.Fatal("expected safety check error")
	}
	if !strings.Contains(err.Error(), "safety check failed") {
		t.Errorf("error = %q, want it to contain 'safety check failed'", err.Error())
	}
	if exec.Count() != 0 {
		t.Errorf("Count() = %d, want 0", exec.Count())
	}
}

func TestLiveExecutor_OpenSafetyCheckMarginExceeded(t *testing.T) {
	safety := defaultSafetyChecker()
	exec := NewLiveExecutor(defaultMockFutures(), defaultMockKeys(), safety, NewFundingTracker(), defaultMockPrices())

	_, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 5, 600, 48000, 55000, "telegram")
	if err == nil {
		t.Fatal("expected safety check error for margin exceeded")
	}
	if !strings.Contains(err.Error(), "safety check failed") {
		t.Errorf("error = %q, want it to contain 'safety check failed'", err.Error())
	}
}

func TestLiveExecutor_OpenErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*mockFutures, *mockLiveKeys, *mockMarkPrice)
		wantErr string
	}{
		{
			name:    "key decryption error",
			setup:   func(_ *mockFutures, k *mockLiveKeys, _ *mockMarkPrice) { k.err = fmt.Errorf("decryption failed") },
			wantErr: "failed to decrypt keys",
		},
		{
			name:    "set leverage error",
			setup:   func(f *mockFutures, _ *mockLiveKeys, _ *mockMarkPrice) { f.setLeverageErr = fmt.Errorf("api error") },
			wantErr: "failed to set leverage",
		},
		{
			name:    "set margin type error",
			setup:   func(f *mockFutures, _ *mockLiveKeys, _ *mockMarkPrice) { f.setMarginTypeErr = fmt.Errorf("api error") },
			wantErr: "failed to set margin type",
		},
		{
			name:    "place order error",
			setup:   func(f *mockFutures, _ *mockLiveKeys, _ *mockMarkPrice) { f.placeOrderErr = fmt.Errorf("order rejected") },
			wantErr: "failed to place order",
		},
		{
			name:    "mark price error",
			setup:   func(_ *mockFutures, _ *mockLiveKeys, p *mockMarkPrice) { p.err = fmt.Errorf("price feed down") },
			wantErr: "failed to get mark price",
		},
		{
			name:    "invalid mark price",
			setup:   func(_ *mockFutures, _ *mockLiveKeys, p *mockMarkPrice) { p.prices["BTCUSDT"] = 0 },
			wantErr: "invalid mark price",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exec, futures, keys, _, prices := newTestLiveExecutor()
			tc.setup(futures, keys, prices)

			_, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 48000, 55000, "telegram")
			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLiveExecutor_OpenGetPositionsError(t *testing.T) {
	exec, futures, _, _, _ := newTestLiveExecutor()
	futures.positionsErr = fmt.Errorf("positions api error")

	pos, err := exec.OpenPosition(1, "BTCUSDT", SideLong, 10, 100, 0, 0, "telegram")
	if err != nil {
		t.Fatalf("OpenPosition() should succeed even with GetPositions error: %v", err)
	}

	wantLiq := CalculateLiquidationPrice(50000, 10, string(SideLong), DefaultMaintenanceMarginRate)
	if math.Abs(pos.LiquidationPrice-wantLiq) > 1e-6 {
		t.Errorf("LiquidationPrice = %f, want %f (calculated)", pos.LiquidationPrice, wantLiq)
	}
}
