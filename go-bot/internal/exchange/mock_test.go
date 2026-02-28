// tests for the mock exchange client
package exchange

import (
	"context"
	"fmt"
	"testing"
)

func TestMock_GetPrice_Success(t *testing.T) {
	m := NewMock()
	ticker, err := m.GetPrice(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatalf("GetPrice() error: %v", err)
	}
	if ticker.Symbol != "BTC/USDT" {
		t.Errorf("Symbol = %q, want %q", ticker.Symbol, "BTC/USDT")
	}
	if ticker.Price != 42000.00 {
		t.Errorf("Price = %v, want %v", ticker.Price, 42000.00)
	}
}

func TestMock_GetPrice_AllDefaultSymbols(t *testing.T) {
	m := NewMock()
	symbols := []string{"BTC/USDT", "ETH/USDT", "BNB/USDT", "SOL/USDT", "XRP/USDT",
		"ADA/USDT", "DOGE/USDT", "DOT/USDT", "AVAX/USDT", "LINK/USDT"}

	for _, sym := range symbols {
		ticker, err := m.GetPrice(context.Background(), sym)
		if err != nil {
			t.Errorf("GetPrice(%s) error: %v", sym, err)
			continue
		}
		if ticker.Price <= 0 {
			t.Errorf("GetPrice(%s).Price = %v, want positive", sym, ticker.Price)
		}
	}
}

func TestMock_GetPrice_UnknownSymbol(t *testing.T) {
	m := NewMock()
	_, err := m.GetPrice(context.Background(), "ZZZZZ/USDT")
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
}

func TestMock_GetPrice_Error(t *testing.T) {
	m := NewMock()
	m.PriceErr = fmt.Errorf("api down")
	_, err := m.GetPrice(context.Background(), "BTC/USDT")
	if err == nil {
		t.Fatal("expected error when PriceErr is set")
	}
}

func TestMock_GetOrderBook_Success(t *testing.T) {
	m := NewMock()
	book, err := m.GetOrderBook(context.Background(), "BTC/USDT", 5)
	if err != nil {
		t.Fatalf("GetOrderBook() error: %v", err)
	}
	if book.Symbol != "BTC/USDT" {
		t.Errorf("Symbol = %q, want %q", book.Symbol, "BTC/USDT")
	}
	if len(book.Bids) == 0 {
		t.Error("expected bids to be non-empty")
	}
	if len(book.Asks) == 0 {
		t.Error("expected asks to be non-empty")
	}
}

func TestMock_GetOrderBook_DepthLimit(t *testing.T) {
	m := NewMock()
	book, err := m.GetOrderBook(context.Background(), "BTC/USDT", 2)
	if err != nil {
		t.Fatalf("GetOrderBook() error: %v", err)
	}
	if len(book.Bids) != 2 {
		t.Errorf("expected 2 bids with depth=2, got %d", len(book.Bids))
	}
	if len(book.Asks) != 2 {
		t.Errorf("expected 2 asks with depth=2, got %d", len(book.Asks))
	}
}

func TestMock_GetOrderBook_UnknownSymbol(t *testing.T) {
	m := NewMock()
	_, err := m.GetOrderBook(context.Background(), "ZZZZZ/USDT", 5)
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
}

func TestMock_GetOrderBook_Error(t *testing.T) {
	m := NewMock()
	m.BookErr = fmt.Errorf("api down")
	_, err := m.GetOrderBook(context.Background(), "BTC/USDT", 5)
	if err == nil {
		t.Fatal("expected error when BookErr is set")
	}
}

func TestMock_GetCandles_Success(t *testing.T) {
	m := NewMock()
	candles, err := m.GetCandles(context.Background(), "BTC/USDT", "1h", 5)
	if err != nil {
		t.Fatalf("GetCandles() error: %v", err)
	}
	if len(candles) != 5 {
		t.Errorf("expected 5 candles, got %d", len(candles))
	}
	for i, c := range candles {
		if c.High < c.Low {
			t.Errorf("candle %d: High(%v) < Low(%v)", i, c.High, c.Low)
		}
		if c.Volume <= 0 {
			t.Errorf("candle %d: Volume should be positive", i)
		}
	}
}

func TestMock_GetCandles_FullDepth(t *testing.T) {
	m := NewMock()
	candles, err := m.GetCandles(context.Background(), "BTC/USDT", "1h", 100)
	if err != nil {
		t.Fatalf("GetCandles() error: %v", err)
	}
	// should return all 10 (limit > available)
	if len(candles) != 10 {
		t.Errorf("expected 10 candles (all available), got %d", len(candles))
	}
}

func TestMock_GetCandles_UnknownSymbol(t *testing.T) {
	m := NewMock()
	_, err := m.GetCandles(context.Background(), "ZZZZZ/USDT", "1h", 10)
	if err == nil {
		t.Fatal("expected error for unknown symbol")
	}
}

func TestMock_GetCandles_Error(t *testing.T) {
	m := NewMock()
	m.CandleErr = fmt.Errorf("api down")
	_, err := m.GetCandles(context.Background(), "BTC/USDT", "1h", 10)
	if err == nil {
		t.Fatal("expected error when CandleErr is set")
	}
}

func TestMock_GetBalance_Success(t *testing.T) {
	m := NewMock()
	balances, err := m.GetBalance(context.Background(), "test-key", "test-secret")
	if err != nil {
		t.Fatalf("GetBalance() error: %v", err)
	}
	if len(balances) != 3 {
		t.Errorf("expected 3 balances, got %d", len(balances))
	}

	// check USDT balance
	found := false
	for _, b := range balances {
		if b.Asset == "USDT" {
			found = true
			if b.Free != 1000.00 {
				t.Errorf("USDT Free = %v, want %v", b.Free, 1000.00)
			}
		}
	}
	if !found {
		t.Error("USDT balance not found")
	}
}

func TestMock_GetBalance_EmptyCredentials(t *testing.T) {
	m := NewMock()
	_, err := m.GetBalance(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error for empty credentials")
	}
}

func TestMock_GetBalance_Error(t *testing.T) {
	m := NewMock()
	m.BalanceErr = fmt.Errorf("auth failed")
	_, err := m.GetBalance(context.Background(), "key", "secret")
	if err == nil {
		t.Fatal("expected error when BalanceErr is set")
	}
}

func TestMock_ImplementsExchangeInterface(t *testing.T) {
	// compile-time check that Mock satisfies Exchange
	var _ Exchange = (*Mock)(nil)
}

func TestMock_CustomPrices(t *testing.T) {
	m := NewMock()
	m.Prices["CUSTOM/USDT"] = &Ticker{
		Symbol: "CUSTOM/USDT",
		Price:  99.99,
	}

	ticker, err := m.GetPrice(context.Background(), "CUSTOM/USDT")
	if err != nil {
		t.Fatalf("GetPrice() error: %v", err)
	}
	if ticker.Price != 99.99 {
		t.Errorf("Price = %v, want %v", ticker.Price, 99.99)
	}
}

func TestMock_CustomBalances(t *testing.T) {
	m := NewMock()
	m.Balances = []Balance{{Asset: "SHIB", Free: 1000000, Locked: 0}}

	balances, err := m.GetBalance(context.Background(), "key", "secret")
	if err != nil {
		t.Fatalf("GetBalance() error: %v", err)
	}
	if len(balances) != 1 {
		t.Errorf("expected 1 custom balance, got %d", len(balances))
	}
	if balances[0].Asset != "SHIB" {
		t.Errorf("Asset = %q, want %q", balances[0].Asset, "SHIB")
	}
}
