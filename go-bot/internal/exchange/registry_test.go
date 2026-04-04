package exchange

import (
	"context"
	"testing"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	mock := &mockFullExchange{name: ExchangeBinance, mock: NewMock()}

	reg.Register(mock)

	ex, err := reg.Get(ExchangeBinance)
	if err != nil {
		t.Fatalf("got error: %v", err)
	}
	if ex.Name() != ExchangeBinance {
		t.Errorf("expected binance, got %s", ex.Name())
	}
}

func TestRegistryPrimaryIsFirstRegistered(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockFullExchange{name: ExchangeBinance, mock: NewMock()})
	reg.Register(&mockFullExchange{name: ExchangeMock, mock: NewMock()})

	primary, err := reg.Primary()
	if err != nil {
		t.Fatalf("got error: %v", err)
	}
	if primary.Name() != ExchangeBinance {
		t.Errorf("expected binance as primary, got %s", primary.Name())
	}
}

func TestRegistrySetPrimary(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockFullExchange{name: ExchangeBinance, mock: NewMock()})
	reg.Register(&mockFullExchange{name: ExchangeMock, mock: NewMock()})

	err := reg.SetPrimary(ExchangeMock)
	if err != nil {
		t.Fatalf("SetPrimary failed: %v", err)
	}

	primary, err := reg.Primary()
	if err != nil {
		t.Fatalf("got error: %v", err)
	}
	if primary.Name() != ExchangeMock {
		t.Errorf("expected mock as primary, got %s", primary.Name())
	}
}

func TestRegistrySetPrimaryNotRegistered(t *testing.T) {
	reg := NewRegistry()
	err := reg.SetPrimary(ExchangeBybit)
	if err == nil {
		t.Fatal("expected error for unregistered exchange")
	}
}

func TestRegistryGetNotRegistered(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Get(ExchangeBybit)
	if err == nil {
		t.Fatal("expected error for unregistered exchange")
	}
}

func TestRegistryPrimaryEmpty(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Primary()
	if err == nil {
		t.Fatal("expected error for empty registry")
	}
}

func TestRegistryNames(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockFullExchange{name: ExchangeBinance, mock: NewMock()})
	reg.Register(&mockFullExchange{name: ExchangeMock, mock: NewMock()})

	names := reg.Names()
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
}

func TestRegistryPriceAcross(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockFullExchange{name: ExchangeBinance, mock: NewMock()})
	reg.Register(&mockFullExchange{name: ExchangeMock, mock: NewMock()})

	prices, err := reg.PriceAcross(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatalf("got error: %v", err)
	}
	if len(prices) != 2 {
		t.Errorf("expected 2 prices, got %d", len(prices))
	}
}

// mockFullExchange wraps Mock to implement FullExchange
type mockFullExchange struct {
	name ExchangeName
	mock *Mock
	// embed order executor for the interface
}

func (m *mockFullExchange) Name() ExchangeName { return m.name }
func (m *mockFullExchange) GetPrice(ctx context.Context, symbol string) (*Ticker, error) {
	return m.mock.GetPrice(ctx, symbol)
}
func (m *mockFullExchange) GetOrderBook(ctx context.Context, symbol string, depth int) (*OrderBook, error) {
	return m.mock.GetOrderBook(ctx, symbol, depth)
}
func (m *mockFullExchange) GetCandles(ctx context.Context, symbol string, interval string, limit int) ([]Candle, error) {
	return m.mock.GetCandles(ctx, symbol, interval, limit)
}
func (m *mockFullExchange) GetBalance(ctx context.Context, apiKey, apiSecret string) ([]Balance, error) {
	return m.mock.GetBalance(ctx, apiKey, apiSecret)
}
func (m *mockFullExchange) PlaceOrder(symbol string, side OrderSide, orderType OrderType, quantity, price float64, apiKey, apiSecret string) (*Order, error) {
	return &Order{OrderID: 1, Symbol: symbol, Side: side, AvgPrice: price, ExecutedQty: quantity, Status: OrderStatusFilled}, nil
}
func (m *mockFullExchange) PlaceStopLoss(symbol string, side OrderSide, quantity, stopPrice, price float64, apiKey, apiSecret string) (*Order, error) {
	return &Order{OrderID: 2, Symbol: symbol, Status: OrderStatusNew}, nil
}
func (m *mockFullExchange) PlaceTakeProfit(symbol string, side OrderSide, quantity, stopPrice, price float64, apiKey, apiSecret string) (*Order, error) {
	return &Order{OrderID: 3, Symbol: symbol, Status: OrderStatusNew}, nil
}
func (m *mockFullExchange) CancelOrder(symbol string, orderID int64, apiKey, apiSecret string) error {
	return nil
}
func (m *mockFullExchange) GetOrder(symbol string, orderID int64, apiKey, apiSecret string) (*Order, error) {
	return &Order{OrderID: orderID, Status: OrderStatusFilled}, nil
}
func (m *mockFullExchange) GetOpenOrders(symbol string, apiKey, apiSecret string) ([]Order, error) {
	return nil, nil
}
