// tests for binance order execution with httptest mock server
package binance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// helper to create a mock binance server with a handler
func newTestOrderServer(handler http.HandlerFunc) (*httptest.Server, *OrderClient) {
	server := httptest.NewServer(handler)
	client := NewOrderClient(server.URL, true)
	return server, client
}

// helper to create a standard order response json
func marketOrderJSON() string {
	return `{
		"orderId": 12345,
		"clientOrderId": "test_client_1",
		"symbol": "BTCUSDT",
		"side": "BUY",
		"type": "MARKET",
		"status": "FILLED",
		"price": "0.00000000",
		"stopPrice": "0.00000000",
		"origQty": "0.00100000",
		"executedQty": "0.00100000",
		"cummulativeQuoteQty": "42.45000000",
		"timeInForce": "GTC",
		"transactTime": 1704067200000,
		"fills": [
			{
				"price": "42450.00000000",
				"qty": "0.00100000",
				"commission": "0.00000100",
				"commissionAsset": "BTC"
			}
		]
	}`
}

func stopLossOrderJSON() string {
	return `{
		"orderId": 12346,
		"clientOrderId": "sl_1",
		"symbol": "BTCUSDT",
		"side": "SELL",
		"type": "STOP_LOSS_LIMIT",
		"status": "NEW",
		"price": "41800.00000000",
		"stopPrice": "41800.00000000",
		"origQty": "0.00100000",
		"executedQty": "0.00000000",
		"cummulativeQuoteQty": "0.00000000",
		"timeInForce": "GTC",
		"transactTime": 1704067200000,
		"fills": []
	}`
}

func takeProfitOrderJSON() string {
	return `{
		"orderId": 12347,
		"clientOrderId": "tp_1",
		"symbol": "BTCUSDT",
		"side": "SELL",
		"type": "TAKE_PROFIT_LIMIT",
		"status": "NEW",
		"price": "44200.00000000",
		"stopPrice": "44200.00000000",
		"origQty": "0.00100000",
		"executedQty": "0.00000000",
		"cummulativeQuoteQty": "0.00000000",
		"timeInForce": "GTC",
		"transactTime": 1704067200000,
		"fills": []
	}`
}

func TestPlaceOrder_MarketBuy(t *testing.T) {
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/v3/order") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// verify required params
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		if r.FormValue("symbol") != "BTCUSDT" {
			t.Errorf("symbol = %s, want BTCUSDT", r.FormValue("symbol"))
		}
		if r.FormValue("side") != "BUY" {
			t.Errorf("side = %s, want BUY", r.FormValue("side"))
		}
		if r.FormValue("type") != "MARKET" {
			t.Errorf("type = %s, want MARKET", r.FormValue("type"))
		}
		if r.FormValue("timestamp") == "" {
			t.Error("timestamp should be set")
		}
		if r.FormValue("signature") == "" {
			t.Error("signature should be set")
		}
		if r.Header.Get("X-MBX-APIKEY") != "test_key" {
			t.Errorf("api key header = %s, want test_key", r.Header.Get("X-MBX-APIKEY"))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(marketOrderJSON()))
	})
	defer server.Close()

	order, err := client.PlaceOrder("BTC/USDT", exchange.SideBuy, exchange.OrderTypeMarket, 0.001, 0, "test_key", "test_secret")
	if err != nil {
		t.Fatalf("PlaceOrder() error: %v", err)
	}

	if order.OrderID != 12345 {
		t.Errorf("OrderID = %d, want 12345", order.OrderID)
	}
	if order.Symbol != "BTCUSDT" {
		t.Errorf("Symbol = %s, want BTCUSDT", order.Symbol)
	}
	if order.Side != exchange.SideBuy {
		t.Errorf("Side = %s, want BUY", order.Side)
	}
	if order.Status != exchange.OrderStatusFilled {
		t.Errorf("Status = %s, want FILLED", order.Status)
	}
	if order.ExecutedQty != 0.001 {
		t.Errorf("ExecutedQty = %v, want 0.001", order.ExecutedQty)
	}
	if order.AvgPrice != 42450 {
		t.Errorf("AvgPrice = %v, want 42450", order.AvgPrice)
	}
	if len(order.Fills) != 1 {
		t.Fatalf("Fills count = %d, want 1", len(order.Fills))
	}
	if order.Fills[0].Price != 42450 {
		t.Errorf("Fill price = %v, want 42450", order.Fills[0].Price)
	}
	if order.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestPlaceOrder_MarketWithQuoteQty(t *testing.T) {
	var gotQuoteQty string
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotQuoteQty = r.FormValue("quoteOrderQty")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(marketOrderJSON()))
	})
	defer server.Close()

	// quantity=0, price=100 should use quoteOrderQty
	_, err := client.PlaceOrder("BTC/USDT", exchange.SideBuy, exchange.OrderTypeMarket, 0, 100, "key", "secret")
	if err != nil {
		t.Fatalf("PlaceOrder() error: %v", err)
	}
	if gotQuoteQty != "100" {
		t.Errorf("quoteOrderQty = %s, want 100", gotQuoteQty)
	}
}

func TestPlaceOrder_LimitOrder(t *testing.T) {
	var gotTIF string
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotTIF = r.FormValue("timeInForce")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(marketOrderJSON()))
	})
	defer server.Close()

	_, err := client.PlaceOrder("BTC/USDT", exchange.SideBuy, exchange.OrderTypeLimit, 0.001, 42000, "key", "secret")
	if err != nil {
		t.Fatalf("PlaceOrder() error: %v", err)
	}
	if gotTIF != "GTC" {
		t.Errorf("timeInForce = %s, want GTC", gotTIF)
	}
}

func TestPlaceOrder_APIError(t *testing.T) {
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":-1013,"msg":"Filter failure: MIN_NOTIONAL"}`))
	})
	defer server.Close()

	_, err := client.PlaceOrder("BTC/USDT", exchange.SideBuy, exchange.OrderTypeMarket, 0.00001, 0, "key", "secret")
	if err == nil {
		t.Fatal("expected error for api error response")
	}
	if !strings.Contains(err.Error(), "MIN_NOTIONAL") {
		t.Errorf("error should contain MIN_NOTIONAL, got: %v", err)
	}
}

func TestPlaceStopLoss(t *testing.T) {
	var gotType, gotStopPrice string
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotType = r.FormValue("type")
		gotStopPrice = r.FormValue("stopPrice")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(stopLossOrderJSON()))
	})
	defer server.Close()

	order, err := client.PlaceStopLoss("BTC/USDT", exchange.SideSell, 0.001, 41800, 41800, "key", "secret")
	if err != nil {
		t.Fatalf("PlaceStopLoss() error: %v", err)
	}

	if gotType != "STOP_LOSS_LIMIT" {
		t.Errorf("type = %s, want STOP_LOSS_LIMIT", gotType)
	}
	if gotStopPrice != "41800" {
		t.Errorf("stopPrice = %s, want 41800", gotStopPrice)
	}
	if order.OrderID != 12346 {
		t.Errorf("OrderID = %d, want 12346", order.OrderID)
	}
	if order.Status != exchange.OrderStatusNew {
		t.Errorf("Status = %s, want NEW", order.Status)
	}
}

func TestPlaceTakeProfit(t *testing.T) {
	var gotType string
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotType = r.FormValue("type")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(takeProfitOrderJSON()))
	})
	defer server.Close()

	order, err := client.PlaceTakeProfit("BTC/USDT", exchange.SideSell, 0.001, 44200, 44200, "key", "secret")
	if err != nil {
		t.Fatalf("PlaceTakeProfit() error: %v", err)
	}

	if gotType != "TAKE_PROFIT_LIMIT" {
		t.Errorf("type = %s, want TAKE_PROFIT_LIMIT", gotType)
	}
	if order.OrderID != 12347 {
		t.Errorf("OrderID = %d, want 12347", order.OrderID)
	}
}

func TestCancelOrder(t *testing.T) {
	var gotMethod, gotOrderID string
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotOrderID = r.URL.Query().Get("orderId")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(marketOrderJSON()))
	})
	defer server.Close()

	err := client.CancelOrder("BTC/USDT", 12345, "key", "secret")
	if err != nil {
		t.Fatalf("CancelOrder() error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if gotOrderID != "12345" {
		t.Errorf("orderId = %s, want 12345", gotOrderID)
	}
}

func TestCancelOrder_Error(t *testing.T) {
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":-2011,"msg":"Unknown order sent."}`))
	})
	defer server.Close()

	err := client.CancelOrder("BTC/USDT", 99999, "key", "secret")
	if err == nil {
		t.Fatal("expected error for unknown order")
	}
	if !strings.Contains(err.Error(), "Unknown order") {
		t.Errorf("error should mention unknown order, got: %v", err)
	}
}

func TestGetOrder(t *testing.T) {
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(marketOrderJSON()))
	})
	defer server.Close()

	order, err := client.GetOrder("BTC/USDT", 12345, "key", "secret")
	if err != nil {
		t.Fatalf("GetOrder() error: %v", err)
	}
	if order.OrderID != 12345 {
		t.Errorf("OrderID = %d, want 12345", order.OrderID)
	}
}

func TestGetOpenOrders(t *testing.T) {
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		resp := []orderResponse{
			{OrderID: 1, Symbol: "BTCUSDT", Side: "SELL", Type: "STOP_LOSS_LIMIT", Status: "NEW", OrigQty: "0.001"},
			{OrderID: 2, Symbol: "BTCUSDT", Side: "SELL", Type: "TAKE_PROFIT_LIMIT", Status: "NEW", OrigQty: "0.001"},
		}
		data, _ := json.Marshal(resp)
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})
	defer server.Close()

	orders, err := client.GetOpenOrders("BTC/USDT", "key", "secret")
	if err != nil {
		t.Fatalf("GetOpenOrders() error: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("got %d orders, want 2", len(orders))
	}
	if orders[0].OrderID != 1 {
		t.Errorf("first order ID = %d, want 1", orders[0].OrderID)
	}
	if orders[1].OrderID != 2 {
		t.Errorf("second order ID = %d, want 2", orders[1].OrderID)
	}
}

func TestGetOpenOrders_Empty(t *testing.T) {
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	})
	defer server.Close()

	orders, err := client.GetOpenOrders("BTC/USDT", "key", "secret")
	if err != nil {
		t.Fatalf("GetOpenOrders() error: %v", err)
	}
	if len(orders) != 0 {
		t.Errorf("got %d orders, want 0", len(orders))
	}
}

func TestOrderResponse_ToOrder_AvgPriceCalculation(t *testing.T) {
	tests := []struct {
		name     string
		execQty  string
		cumQuote string
		wantAvg  float64
	}{
		{"normal fill", "0.001", "42.45", 42450},
		{"zero exec qty", "0", "0", 0},
		{"partial fill", "0.0005", "21.225", 42450},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := &orderResponse{
				ExecutedQty:         tt.execQty,
				CummulativeQuoteQty: tt.cumQuote,
			}
			order := raw.toOrder()
			if order.AvgPrice != tt.wantAvg {
				t.Errorf("AvgPrice = %v, want %v", order.AvgPrice, tt.wantAvg)
			}
		})
	}
}

func TestOrderResponse_ToOrder_Fills(t *testing.T) {
	raw := &orderResponse{
		Fills: []fillResponse{
			{Price: "42450", Qty: "0.0005", Commission: "0.00000050", CommissionAsset: "BTC"},
			{Price: "42451", Qty: "0.0005", Commission: "0.00000050", CommissionAsset: "BTC"},
		},
	}
	order := raw.toOrder()
	if len(order.Fills) != 2 {
		t.Fatalf("fills count = %d, want 2", len(order.Fills))
	}
	if order.Fills[0].Price != 42450 {
		t.Errorf("fill[0].Price = %v, want 42450", order.Fills[0].Price)
	}
	if order.Fills[1].Price != 42451 {
		t.Errorf("fill[1].Price = %v, want 42451", order.Fills[1].Price)
	}
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{42450.0, "42450"},
		{0.001, "0.001"},
		{100.50, "100.5"},
		{0.00000100, "0.000001"},
	}
	for _, tt := range tests {
		got := formatFloat(tt.input)
		if got != tt.want {
			t.Errorf("formatFloat(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPlaceOrder_NetworkError(t *testing.T) {
	// use a closed server to trigger network error
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {})
	server.Close()

	_, err := client.PlaceOrder("BTC/USDT", exchange.SideBuy, exchange.OrderTypeMarket, 0.001, 0, "key", "secret")
	if err == nil {
		t.Fatal("expected error for closed server")
	}
}

func TestPlaceOrder_InvalidJSON(t *testing.T) {
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	})
	defer server.Close()

	_, err := client.PlaceOrder("BTC/USDT", exchange.SideBuy, exchange.OrderTypeMarket, 0.001, 0, "key", "secret")
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestGetOpenOrders_APIError(t *testing.T) {
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"code":-2015,"msg":"Invalid API-key, IP, or permissions for action."}`))
	})
	defer server.Close()

	_, err := client.GetOpenOrders("BTC/USDT", "bad_key", "secret")
	if err == nil {
		t.Fatal("expected error for forbidden response")
	}
	if !strings.Contains(err.Error(), "Invalid API-key") {
		t.Errorf("error should mention api key, got: %v", err)
	}
}

func TestSignedRequest_SetsAPIKeyHeader(t *testing.T) {
	var gotAPIKey string
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("X-MBX-APIKEY")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(marketOrderJSON()))
	})
	defer server.Close()

	_, _ = client.GetOrder("BTC/USDT", 1, "my_api_key", "my_secret")
	if gotAPIKey != "my_api_key" {
		t.Errorf("X-MBX-APIKEY = %s, want my_api_key", gotAPIKey)
	}
}

func TestSignedRequest_IncludesSignature(t *testing.T) {
	var hasSig bool
	server, client := newTestOrderServer(func(w http.ResponseWriter, r *http.Request) {
		hasSig = r.URL.Query().Get("signature") != ""
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(marketOrderJSON()))
	})
	defer server.Close()

	_, _ = client.GetOrder("BTC/USDT", 1, "key", "secret")
	if !hasSig {
		t.Error("request should include signature parameter")
	}
}
