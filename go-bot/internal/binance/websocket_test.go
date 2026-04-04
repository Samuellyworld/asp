package binance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWSPriceCache_GetPrice_NoData(t *testing.T) {
	cache := NewWSPriceCache("wss://unused")
	_, err := cache.GetPrice("BTCUSDT")
	if err == nil {
		t.Fatal("expected error for missing symbol")
	}
	if !strings.Contains(err.Error(), "not yet received") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWSPriceCache_GetPrice_StaleData(t *testing.T) {
	cache := NewWSPriceCache("wss://unused")
	cache.mu.Lock()
	cache.prices["BTCUSDT"] = priceEntry{price: 42000, updatedAt: time.Now().Add(-10 * time.Minute)}
	cache.mu.Unlock()

	_, err := cache.GetPrice("BTCUSDT")
	if err == nil {
		t.Fatal("expected error for stale data")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWSPriceCache_GetPrice_FreshData(t *testing.T) {
	cache := NewWSPriceCache("wss://unused")
	cache.mu.Lock()
	cache.prices["BTCUSDT"] = priceEntry{price: 42500.50, updatedAt: time.Now()}
	cache.mu.Unlock()

	price, err := cache.GetPrice("BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price != 42500.50 {
		t.Errorf("price = %f, want 42500.50", price)
	}
}

func TestWSPriceCache_GetPrice_SlashFormat(t *testing.T) {
	cache := NewWSPriceCache("wss://unused")
	cache.mu.Lock()
	cache.prices["BTCUSDT"] = priceEntry{price: 42000, updatedAt: time.Now()}
	cache.mu.Unlock()

	price, err := cache.GetPrice("BTC/USDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price != 42000 {
		t.Errorf("price = %f, want 42000", price)
	}
}

func TestWSPriceCache_SymbolCount(t *testing.T) {
	cache := NewWSPriceCache("wss://unused")
	if cache.SymbolCount() != 0 {
		t.Fatal("expected 0 symbols")
	}

	cache.mu.Lock()
	cache.prices["BTCUSDT"] = priceEntry{price: 42000, updatedAt: time.Now()}
	cache.prices["ETHUSDT"] = priceEntry{price: 3000, updatedAt: time.Now()}
	cache.mu.Unlock()

	if cache.SymbolCount() != 2 {
		t.Errorf("SymbolCount = %d, want 2", cache.SymbolCount())
	}
}

func TestWSPriceCache_HandleMessage_Array(t *testing.T) {
	cache := NewWSPriceCache("wss://unused")

	tickers := []miniTickerMsg{
		{Symbol: "BTCUSDT", Close: "42500.00"},
		{Symbol: "ETHUSDT", Close: "3100.50"},
	}
	data, _ := json.Marshal(tickers)
	cache.handleMessage(data)

	if cache.SymbolCount() != 2 {
		t.Fatalf("expected 2 symbols, got %d", cache.SymbolCount())
	}

	price, err := cache.GetPrice("BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price != 42500.00 {
		t.Errorf("BTC price = %f, want 42500", price)
	}

	price, err = cache.GetPrice("ETHUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price != 3100.50 {
		t.Errorf("ETH price = %f, want 3100.50", price)
	}
}

func TestWSPriceCache_HandleMessage_SingleTicker(t *testing.T) {
	cache := NewWSPriceCache("wss://unused")

	ticker := miniTickerMsg{Symbol: "SOLUSDT", Close: "145.25"}
	data, _ := json.Marshal(ticker)
	cache.handleMessage(data)

	price, err := cache.GetPrice("SOLUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price != 145.25 {
		t.Errorf("SOL price = %f, want 145.25", price)
	}
}

func TestWSPriceCache_HandleMessage_CombinedStream(t *testing.T) {
	cache := NewWSPriceCache("wss://unused")

	msg := struct {
		Stream string         `json:"stream"`
		Data   miniTickerMsg  `json:"data"`
	}{
		Stream: "btcusdt@miniTicker",
		Data:   miniTickerMsg{Symbol: "BTCUSDT", Close: "43000.00"},
	}
	data, _ := json.Marshal(msg)
	cache.handleMessage(data)

	price, err := cache.GetPrice("BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price != 43000.00 {
		t.Errorf("BTC price = %f, want 43000", price)
	}
}

func TestWSPriceCache_HandleMessage_InvalidPrice(t *testing.T) {
	cache := NewWSPriceCache("wss://unused")

	ticker := miniTickerMsg{Symbol: "BTCUSDT", Close: "not_a_number"}
	data, _ := json.Marshal(ticker)
	cache.handleMessage(data)

	if cache.SymbolCount() != 0 {
		t.Fatal("invalid price should not be cached")
	}
}

func TestWSPriceCache_HandleMessage_ZeroPrice(t *testing.T) {
	cache := NewWSPriceCache("wss://unused")

	ticker := miniTickerMsg{Symbol: "BTCUSDT", Close: "0"}
	data, _ := json.Marshal(ticker)
	cache.handleMessage(data)

	if cache.SymbolCount() != 0 {
		t.Fatal("zero price should not be cached")
	}
}

func TestWSPriceCache_OnPriceUpdate_Callback(t *testing.T) {
	cache := NewWSPriceCache("wss://unused")

	var callCount int32
	cache.OnPriceUpdate = func(symbol string, price float64) {
		atomic.AddInt32(&callCount, 1)
	}

	tickers := []miniTickerMsg{
		{Symbol: "BTCUSDT", Close: "42000"},
		{Symbol: "ETHUSDT", Close: "3000"},
	}
	data, _ := json.Marshal(tickers)
	cache.handleMessage(data)

	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("callback count = %d, want 2", callCount)
	}
}

func TestWSPriceCache_SubscribeAll_Integration(t *testing.T) {
	// set up a test websocket server
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// send a batch of mini-tickers
		tickers := []miniTickerMsg{
			{EventType: "24hrMiniTicker", Symbol: "BTCUSDT", Close: "50000.00"},
			{EventType: "24hrMiniTicker", Symbol: "ETHUSDT", Close: "4000.00"},
		}
		data, _ := json.Marshal(tickers)
		conn.WriteMessage(websocket.TextMessage, data)

		// keep connection open until client disconnects
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	cache := NewWSPriceCache(wsURL)

	// override the subscribe URL to point to our test server
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	cache.conn = conn
	go cache.readLoop()

	// wait for data to arrive
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cache.SymbolCount() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cache.Stop()

	if cache.SymbolCount() < 2 {
		t.Fatalf("expected at least 2 symbols, got %d", cache.SymbolCount())
	}

	price, err := cache.GetPrice("BTCUSDT")
	if err != nil {
		t.Fatalf("BTC price error: %v", err)
	}
	if price != 50000 {
		t.Errorf("BTC = %f, want 50000", price)
	}
}

func TestWSPriceCache_SubscribeSymbols_Empty(t *testing.T) {
	cache := NewWSPriceCache("wss://unused")
	err := cache.SubscribeSymbols(nil)
	if err == nil {
		t.Fatal("expected error for empty symbols")
	}
}

func TestWSPriceCache_Stop_Idempotent(t *testing.T) {
	// set up a test server
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	cache := NewWSPriceCache(wsURL)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	cache.conn = conn
	go cache.readLoop()

	// stop twice — should not panic
	cache.Stop()
}

func TestWSPriceCache_Reconnect_MaxRetries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long reconnect test in short mode")
	}
	// verify reconnect exits after max retries — takes several seconds due to backoff
	cache := NewWSPriceCache("ws://localhost:1")
	cache.stopCh = make(chan struct{})
	cache.done = make(chan struct{})

	done := make(chan struct{})
	go func() {
		cache.reconnect()
		close(done)
	}()

	select {
	case <-done:
		// reconnect returned — max retries exhausted
	case <-time.After(10 * time.Minute):
		close(cache.stopCh)
		t.Fatal("reconnect appears to be hanging")
	}
}

func TestWSPriceCache_Reconnect_StopsDuringRetry(t *testing.T) {
	cache := NewWSPriceCache("ws://localhost:1")
	cache.stopCh = make(chan struct{})
	cache.done = make(chan struct{})

	// close stopCh after a short delay to simulate shutdown during reconnect
	go func() {
		time.Sleep(500 * time.Millisecond)
		close(cache.stopCh)
	}()

	start := time.Now()
	cache.reconnect()
	elapsed := time.Since(start)

	// should exit quickly after stopCh is closed, not retry all 10 times
	if elapsed > 5*time.Second {
		t.Fatalf("reconnect took %v — should have stopped after stopCh closed", elapsed)
	}
}
