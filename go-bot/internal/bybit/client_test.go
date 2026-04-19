package bybit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trading-bot/go-bot/internal/exchange"
)

func TestClientName(t *testing.T) {
	var _ exchange.FullExchange = (*Client)(nil)

	client := NewClient("https://api-testnet.bybit.com", true)
	if client.Name() != exchange.ExchangeBybit {
		t.Fatalf("Name() = %s, want %s", client.Name(), exchange.ExchangeBybit)
	}
}

func TestValidateKeys(t *testing.T) {
	const apiKey = "test-key"
	const apiSecret = "test-secret"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v5/user/query-api" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		timestamp := r.Header.Get("X-BAPI-TIMESTAMP")
		if timestamp == "" {
			t.Fatal("missing timestamp header")
		}
		if got := r.Header.Get("X-BAPI-API-KEY"); got != apiKey {
			t.Fatalf("api key header = %q, want %q", got, apiKey)
		}
		wantSig := sign(timestamp+apiKey+defaultRecvWindow, apiSecret)
		if got := r.Header.Get("X-BAPI-SIGN"); got != wantSig {
			t.Fatalf("signature = %q, want %q", got, wantSig)
		}
		writeBybitResult(w, map[string]any{
			"readOnly": 0,
			"permissions": map[string]any{
				"Spot":          []string{"SpotTrade"},
				"ContractTrade": []string{"Order", "Position"},
				"Wallet":        []string{},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	perms, err := client.ValidateKeys(context.Background(), apiKey, apiSecret)
	if err != nil {
		t.Fatalf("ValidateKeys() error: %v", err)
	}
	if !perms.Spot || !perms.Futures || perms.Withdraw {
		t.Fatalf("unexpected permissions: %+v", perms)
	}
}

func TestValidateKeysReadOnlyHasNoTradingPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeBybitResult(w, map[string]any{
			"readOnly": 1,
			"permissions": map[string]any{
				"Spot":   []string{"SpotTrade"},
				"Wallet": []string{"Withdraw"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	perms, err := client.ValidateKeys(context.Background(), "key", "secret")
	if err != nil {
		t.Fatalf("ValidateKeys() error: %v", err)
	}
	if perms.Spot || perms.Futures || !perms.Withdraw {
		t.Fatalf("unexpected permissions: %+v", perms)
	}
}

func TestGetPrice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v5/market/tickers" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("category"); got != "spot" {
			t.Fatalf("category = %q, want spot", got)
		}
		if got := r.URL.Query().Get("symbol"); got != "BTCUSDT" {
			t.Fatalf("symbol = %q, want BTCUSDT", got)
		}
		writeBybitResult(w, map[string]any{
			"list": []map[string]string{{
				"symbol":       "BTCUSDT",
				"lastPrice":    "43000.5",
				"prevPrice24h": "42000.5",
				"price24hPcnt": "0.023809",
				"volume24h":    "12.5",
				"turnover24h":  "537506.25",
			}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	ticker, err := client.GetPrice(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatalf("GetPrice() error: %v", err)
	}
	if ticker.Price != 43000.5 {
		t.Fatalf("price = %f, want 43000.5", ticker.Price)
	}
	if ticker.ChangePct <= 2.3 || ticker.ChangePct >= 2.4 {
		t.Fatalf("change pct = %f, want about 2.38", ticker.ChangePct)
	}
}

func TestGetOrderBook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeBybitResult(w, map[string]any{
			"s": "BTCUSDT",
			"b": [][]string{{"42999.5", "1.2"}},
			"a": [][]string{{"43000.5", "0.8"}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	book, err := client.GetOrderBook(context.Background(), "BTCUSDT", 1)
	if err != nil {
		t.Fatalf("GetOrderBook() error: %v", err)
	}
	if len(book.Bids) != 1 || len(book.Asks) != 1 {
		t.Fatalf("unexpected book: %+v", book)
	}
}

func TestGetCandlesSortsChronologically(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("interval"); got != "240" {
			t.Fatalf("interval = %q, want 240", got)
		}
		writeBybitResult(w, map[string]any{
			"list": [][]string{
				{"2000", "101", "103", "100", "102", "5"},
				{"1000", "100", "102", "99", "101", "4"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	candles, err := client.GetCandles(context.Background(), "BTC/USDT", "4h", 2)
	if err != nil {
		t.Fatalf("GetCandles() error: %v", err)
	}
	if len(candles) != 2 {
		t.Fatalf("len(candles) = %d, want 2", len(candles))
	}
	if !candles[0].OpenTime.Before(candles[1].OpenTime) {
		t.Fatal("candles not sorted chronologically")
	}
}

func TestGetBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "accountType=UNIFIED") {
			t.Fatalf("expected accountType=UNIFIED, got %s", r.URL.RawQuery)
		}
		writeBybitResult(w, map[string]any{
			"list": []map[string]any{{
				"coin": []map[string]string{{
					"coin":          "USDT",
					"walletBalance": "100.5",
					"locked":        "2.5",
				}},
			}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, true)
	balances, err := client.GetBalance(context.Background(), "key", "secret")
	if err != nil {
		t.Fatalf("GetBalance() error: %v", err)
	}
	if len(balances) != 1 || balances[0].Asset != "USDT" || balances[0].Free != 98 {
		t.Fatalf("unexpected balances: %+v", balances)
	}
}

func TestPlaceOrderRejectsZeroQuantity(t *testing.T) {
	client := NewClient("https://api-testnet.bybit.com", true)
	_, err := client.PlaceOrder("BTC/USDT", exchange.SideBuy, exchange.OrderTypeMarket, 0, 0, "key", "secret")
	if err == nil {
		t.Fatal("expected zero quantity error")
	}
}

func writeBybitResult(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"retCode":    0,
		"retMsg":     "OK",
		"result":     result,
		"retExtInfo": map[string]any{},
		"time":       1700000000000,
	})
}
