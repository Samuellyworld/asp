// tests for binance futures api client with httptest mock server
package binance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// helper to create a mock futures server with a handler
func newTestFuturesServer(handler http.HandlerFunc) (*httptest.Server, *FuturesClient) {
	server := httptest.NewServer(handler)
	client := NewFuturesClient(server.URL, true)
	return server, client
}

func futuresMarketOrderJSON() string {
	return `{
		"orderId": 50001,
		"clientOrderId": "futures_test_1",
		"symbol": "BTCUSDT",
		"side": "BUY",
		"type": "MARKET",
		"status": "FILLED",
		"price": "0",
		"stopPrice": "0",
		"origQty": "0.010",
		"executedQty": "0.010",
		"avgPrice": "42500.00",
		"updateTime": 1704067200000
	}`
}

func futuresLimitOrderJSON() string {
	return `{
		"orderId": 50002,
		"clientOrderId": "futures_limit_1",
		"symbol": "BTCUSDT",
		"side": "SELL",
		"type": "LIMIT",
		"status": "NEW",
		"price": "43000.00",
		"stopPrice": "0",
		"origQty": "0.010",
		"executedQty": "0",
		"avgPrice": "0",
		"updateTime": 1704067200000
	}`
}

func futuresStopMarketOrderJSON() string {
	return `{
		"orderId": 50003,
		"clientOrderId": "sm_1",
		"symbol": "BTCUSDT",
		"side": "SELL",
		"type": "STOP_MARKET",
		"status": "NEW",
		"price": "0",
		"stopPrice": "41000.00",
		"origQty": "0.010",
		"executedQty": "0",
		"avgPrice": "0",
		"updateTime": 1704067200000
	}`
}

func futuresTakeProfitMarketJSON() string {
	return `{
		"orderId": 50004,
		"clientOrderId": "tpm_1",
		"symbol": "BTCUSDT",
		"side": "SELL",
		"type": "TAKE_PROFIT_MARKET",
		"status": "NEW",
		"price": "0",
		"stopPrice": "45000.00",
		"origQty": "0.010",
		"executedQty": "0",
		"avgPrice": "0",
		"updateTime": 1704067200000
	}`
}

func TestSetLeverage(t *testing.T) {
	var gotSymbol, gotLeverage string
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/fapi/v1/leverage") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		gotSymbol = r.FormValue("symbol")
		gotLeverage = r.FormValue("leverage")
		if r.FormValue("timestamp") == "" {
			t.Error("timestamp should be set")
		}
		if r.FormValue("signature") == "" {
			t.Error("signature should be set")
		}
		if r.Header.Get("X-MBX-APIKEY") != "test_key" {
			t.Errorf("api key = %s, want test_key", r.Header.Get("X-MBX-APIKEY"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"leverage":20,"maxNotionalValue":"1000000","symbol":"BTCUSDT"}`))
	})
	defer server.Close()

	err := client.SetLeverage(context.Background(), "BTC/USDT", 20, "test_key", "test_secret")
	if err != nil {
		t.Fatalf("SetLeverage() error: %v", err)
	}
	if gotSymbol != "BTCUSDT" {
		t.Errorf("symbol = %s, want BTCUSDT", gotSymbol)
	}
	if gotLeverage != "20" {
		t.Errorf("leverage = %s, want 20", gotLeverage)
	}
}

func TestSetLeverage_APIError(t *testing.T) {
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":-4028,"msg":"Leverage 200 is not valid"}`))
	})
	defer server.Close()

	err := client.SetLeverage(context.Background(), "BTC/USDT", 200, "key", "secret")
	if err == nil {
		t.Fatal("expected error for invalid leverage")
	}
	if !strings.Contains(err.Error(), "Leverage 200 is not valid") {
		t.Errorf("error should mention invalid leverage, got: %v", err)
	}
}

func TestSetMarginType(t *testing.T) {
	var gotSymbol, gotMarginType string
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/fapi/v1/marginType") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		gotSymbol = r.FormValue("symbol")
		gotMarginType = r.FormValue("marginType")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":200,"msg":"success"}`))
	})
	defer server.Close()

	err := client.SetMarginType(context.Background(), "BTC/USDT", "ISOLATED", "key", "secret")
	if err != nil {
		t.Fatalf("SetMarginType() error: %v", err)
	}
	if gotSymbol != "BTCUSDT" {
		t.Errorf("symbol = %s, want BTCUSDT", gotSymbol)
	}
	if gotMarginType != "ISOLATED" {
		t.Errorf("marginType = %s, want ISOLATED", gotMarginType)
	}
}

func TestSetMarginType_APIError(t *testing.T) {
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":-4046,"msg":"No need to change margin type."}`))
	})
	defer server.Close()

	err := client.SetMarginType(context.Background(), "BTC/USDT", "CROSSED", "key", "secret")
	if err == nil {
		t.Fatal("expected error for margin type already set")
	}
	if !strings.Contains(err.Error(), "No need to change margin type") {
		t.Errorf("error should mention margin type, got: %v", err)
	}
}

func TestFuturesPlaceOrder_Market(t *testing.T) {
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/fapi/v1/order") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
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
		if r.FormValue("quantity") != "0.01" {
			t.Errorf("quantity = %s, want 0.01", r.FormValue("quantity"))
		}
		if r.Header.Get("X-MBX-APIKEY") != "test_key" {
			t.Errorf("api key = %s, want test_key", r.Header.Get("X-MBX-APIKEY"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(futuresMarketOrderJSON()))
	})
	defer server.Close()

	order, err := client.PlaceOrder(context.Background(), "BTC/USDT", exchange.SideBuy, exchange.OrderTypeMarket, 0.01, 0, "test_key", "test_secret")
	if err != nil {
		t.Fatalf("PlaceOrder() error: %v", err)
	}
	if order.OrderID != 50001 {
		t.Errorf("OrderID = %d, want 50001", order.OrderID)
	}
	if order.Symbol != "BTCUSDT" {
		t.Errorf("Symbol = %s, want BTCUSDT", order.Symbol)
	}
	if order.Side != exchange.SideBuy {
		t.Errorf("Side = %s, want BUY", order.Side)
	}
	if order.Type != "MARKET" {
		t.Errorf("Type = %s, want MARKET", order.Type)
	}
	if order.Status != exchange.OrderStatusFilled {
		t.Errorf("Status = %s, want FILLED", order.Status)
	}
	if order.ExecutedQty != 0.01 {
		t.Errorf("ExecutedQty = %v, want 0.01", order.ExecutedQty)
	}
	if order.AvgPrice != 42500 {
		t.Errorf("AvgPrice = %v, want 42500", order.AvgPrice)
	}
	if order.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestFuturesPlaceOrder_Limit(t *testing.T) {
	var gotTIF, gotPrice string
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		gotTIF = r.FormValue("timeInForce")
		gotPrice = r.FormValue("price")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(futuresLimitOrderJSON()))
	})
	defer server.Close()

	order, err := client.PlaceOrder(context.Background(), "BTC/USDT", exchange.SideSell, exchange.OrderTypeLimit, 0.01, 43000, "key", "secret")
	if err != nil {
		t.Fatalf("PlaceOrder() error: %v", err)
	}
	if gotTIF != "GTC" {
		t.Errorf("timeInForce = %s, want GTC", gotTIF)
	}
	if gotPrice != "43000" {
		t.Errorf("price = %s, want 43000", gotPrice)
	}
	if order.OrderID != 50002 {
		t.Errorf("OrderID = %d, want 50002", order.OrderID)
	}
	if order.Status != exchange.OrderStatusNew {
		t.Errorf("Status = %s, want NEW", order.Status)
	}
}

func TestPlaceStopMarket(t *testing.T) {
	var gotType, gotStopPrice string
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		gotType = r.FormValue("type")
		gotStopPrice = r.FormValue("stopPrice")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(futuresStopMarketOrderJSON()))
	})
	defer server.Close()

	order, err := client.PlaceStopMarket(context.Background(), "BTC/USDT", exchange.SideSell, 0.01, 41000, "key", "secret")
	if err != nil {
		t.Fatalf("PlaceStopMarket() error: %v", err)
	}
	if gotType != "STOP_MARKET" {
		t.Errorf("type = %s, want STOP_MARKET", gotType)
	}
	if gotStopPrice != "41000" {
		t.Errorf("stopPrice = %s, want 41000", gotStopPrice)
	}
	if order.OrderID != 50003 {
		t.Errorf("OrderID = %d, want 50003", order.OrderID)
	}
	if order.Type != "STOP_MARKET" {
		t.Errorf("Type = %s, want STOP_MARKET", order.Type)
	}
	if order.StopPrice != 41000 {
		t.Errorf("StopPrice = %v, want 41000", order.StopPrice)
	}
}

func TestPlaceTakeProfitMarket(t *testing.T) {
	var gotType, gotStopPrice string
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		gotType = r.FormValue("type")
		gotStopPrice = r.FormValue("stopPrice")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(futuresTakeProfitMarketJSON()))
	})
	defer server.Close()

	order, err := client.PlaceTakeProfitMarket(context.Background(), "BTC/USDT", exchange.SideSell, 0.01, 45000, "key", "secret")
	if err != nil {
		t.Fatalf("PlaceTakeProfitMarket() error: %v", err)
	}
	if gotType != "TAKE_PROFIT_MARKET" {
		t.Errorf("type = %s, want TAKE_PROFIT_MARKET", gotType)
	}
	if gotStopPrice != "45000" {
		t.Errorf("stopPrice = %s, want 45000", gotStopPrice)
	}
	if order.OrderID != 50004 {
		t.Errorf("OrderID = %d, want 50004", order.OrderID)
	}
	if order.Type != "TAKE_PROFIT_MARKET" {
		t.Errorf("Type = %s, want TAKE_PROFIT_MARKET", order.Type)
	}
	if order.StopPrice != 45000 {
		t.Errorf("StopPrice = %v, want 45000", order.StopPrice)
	}
}

func TestFuturesCancelOrder(t *testing.T) {
	var gotMethod, gotOrderID string
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotOrderID = r.URL.Query().Get("orderId")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(futuresMarketOrderJSON()))
	})
	defer server.Close()

	err := client.CancelOrder(context.Background(), "BTC/USDT", 50001, "key", "secret")
	if err != nil {
		t.Fatalf("CancelOrder() error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if gotOrderID != "50001" {
		t.Errorf("orderId = %s, want 50001", gotOrderID)
	}
}

func TestFuturesCancelOrder_Error(t *testing.T) {
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":-2011,"msg":"Unknown order sent."}`))
	})
	defer server.Close()

	err := client.CancelOrder(context.Background(), "BTC/USDT", 99999, "key", "secret")
	if err == nil {
		t.Fatal("expected error for unknown order")
	}
	if !strings.Contains(err.Error(), "Unknown order") {
		t.Errorf("error should mention unknown order, got: %v", err)
	}
}

func TestFuturesGetOrder(t *testing.T) {
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/fapi/v1/order") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(futuresMarketOrderJSON()))
	})
	defer server.Close()

	order, err := client.GetOrder(context.Background(), "BTC/USDT", 50001, "key", "secret")
	if err != nil {
		t.Fatalf("GetOrder() error: %v", err)
	}
	if order.OrderID != 50001 {
		t.Errorf("OrderID = %d, want 50001", order.OrderID)
	}
	if order.Symbol != "BTCUSDT" {
		t.Errorf("Symbol = %s, want BTCUSDT", order.Symbol)
	}
}

func TestGetPositions(t *testing.T) {
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/fapi/v2/positionRisk") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := []futuresPositionResponse{
			{
				Symbol:           "BTCUSDT",
				PositionSide:     "BOTH",
				PositionAmt:      "0.010",
				EntryPrice:       "42500.00",
				MarkPrice:        "42800.00",
				UnrealizedProfit: "3.00",
				LiquidationPrice: "38000.00",
				Leverage:         "20",
				MarginType:       "isolated",
				IsolatedMargin:   "21.25",
				Notional:         "428.00",
			},
			{
				Symbol:           "ETHUSDT",
				PositionSide:     "BOTH",
				PositionAmt:      "-0.5",
				EntryPrice:       "2200.00",
				MarkPrice:        "2180.00",
				UnrealizedProfit: "10.00",
				LiquidationPrice: "2500.00",
				Leverage:         "10",
				MarginType:       "cross",
				IsolatedMargin:   "0",
				Notional:         "-1090.00",
			},
		}
		data, _ := json.Marshal(resp)
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})
	defer server.Close()

	positions, err := client.GetPositions(context.Background(), "key", "secret")
	if err != nil {
		t.Fatalf("GetPositions() error: %v", err)
	}
	if len(positions) != 2 {
		t.Fatalf("got %d positions, want 2", len(positions))
	}

	btc := positions[0]
	if btc.Symbol != "BTCUSDT" {
		t.Errorf("positions[0].Symbol = %s, want BTCUSDT", btc.Symbol)
	}
	if btc.PositionSide != "BOTH" {
		t.Errorf("positions[0].PositionSide = %s, want BOTH", btc.PositionSide)
	}
	if btc.PositionAmt != 0.01 {
		t.Errorf("positions[0].PositionAmt = %v, want 0.01", btc.PositionAmt)
	}
	if btc.EntryPrice != 42500 {
		t.Errorf("positions[0].EntryPrice = %v, want 42500", btc.EntryPrice)
	}
	if btc.MarkPrice != 42800 {
		t.Errorf("positions[0].MarkPrice = %v, want 42800", btc.MarkPrice)
	}
	if btc.UnrealizedProfit != 3.0 {
		t.Errorf("positions[0].UnrealizedProfit = %v, want 3", btc.UnrealizedProfit)
	}
	if btc.LiquidationPrice != 38000 {
		t.Errorf("positions[0].LiquidationPrice = %v, want 38000", btc.LiquidationPrice)
	}
	if btc.Leverage != 20 {
		t.Errorf("positions[0].Leverage = %d, want 20", btc.Leverage)
	}
	if btc.MarginType != "isolated" {
		t.Errorf("positions[0].MarginType = %s, want isolated", btc.MarginType)
	}
	if btc.IsolatedMargin != 21.25 {
		t.Errorf("positions[0].IsolatedMargin = %v, want 21.25", btc.IsolatedMargin)
	}
	if btc.Notional != 428 {
		t.Errorf("positions[0].Notional = %v, want 428", btc.Notional)
	}

	eth := positions[1]
	if eth.Symbol != "ETHUSDT" {
		t.Errorf("positions[1].Symbol = %s, want ETHUSDT", eth.Symbol)
	}
	if eth.PositionAmt != -0.5 {
		t.Errorf("positions[1].PositionAmt = %v, want -0.5", eth.PositionAmt)
	}
	if eth.Leverage != 10 {
		t.Errorf("positions[1].Leverage = %d, want 10", eth.Leverage)
	}
	if eth.MarginType != "cross" {
		t.Errorf("positions[1].MarginType = %s, want cross", eth.MarginType)
	}
}

func TestGetFuturesBalance(t *testing.T) {
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/fapi/v2/balance") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := []futuresBalanceResponse{
			{
				Asset:              "USDT",
				Balance:            "10000.50",
				AvailableBalance:   "8500.25",
				CrossWalletBalance: "9500.00",
			},
			{
				Asset:              "BNB",
				Balance:            "5.00",
				AvailableBalance:   "5.00",
				CrossWalletBalance: "5.00",
			},
		}
		data, _ := json.Marshal(resp)
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})
	defer server.Close()

	balances, err := client.GetFuturesBalance(context.Background(), "key", "secret")
	if err != nil {
		t.Fatalf("GetFuturesBalance() error: %v", err)
	}
	if len(balances) != 2 {
		t.Fatalf("got %d balances, want 2", len(balances))
	}

	usdt := balances[0]
	if usdt.Asset != "USDT" {
		t.Errorf("balances[0].Asset = %s, want USDT", usdt.Asset)
	}
	if usdt.Balance != 10000.50 {
		t.Errorf("balances[0].Balance = %v, want 10000.5", usdt.Balance)
	}
	if usdt.AvailableBalance != 8500.25 {
		t.Errorf("balances[0].AvailableBalance = %v, want 8500.25", usdt.AvailableBalance)
	}
	if usdt.CrossWalletBalance != 9500 {
		t.Errorf("balances[0].CrossWalletBalance = %v, want 9500", usdt.CrossWalletBalance)
	}

	bnb := balances[1]
	if bnb.Asset != "BNB" {
		t.Errorf("balances[1].Asset = %s, want BNB", bnb.Asset)
	}
	if bnb.Balance != 5 {
		t.Errorf("balances[1].Balance = %v, want 5", bnb.Balance)
	}
}

func TestGetMarkPrice(t *testing.T) {
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/fapi/v1/premiumIndex") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("symbol") != "BTCUSDT" {
			t.Errorf("symbol = %s, want BTCUSDT", r.URL.Query().Get("symbol"))
		}
		// should not have auth headers for public endpoint
		if r.Header.Get("X-MBX-APIKEY") != "" {
			t.Error("public endpoint should not have api key header")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"symbol": "BTCUSDT",
			"markPrice": "42750.50",
			"indexPrice": "42748.00",
			"lastFundingRate": "0.00010000",
			"nextFundingTime": 1704096000000
		}`))
	})
	defer server.Close()

	mp, err := client.GetMarkPrice(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatalf("GetMarkPrice() error: %v", err)
	}
	if mp.Symbol != "BTCUSDT" {
		t.Errorf("Symbol = %s, want BTCUSDT", mp.Symbol)
	}
	if mp.MarkPrice != 42750.50 {
		t.Errorf("MarkPrice = %v, want 42750.5", mp.MarkPrice)
	}
	if mp.IndexPrice != 42748 {
		t.Errorf("IndexPrice = %v, want 42748", mp.IndexPrice)
	}
	if mp.LastFundingRate != 0.0001 {
		t.Errorf("LastFundingRate = %v, want 0.0001", mp.LastFundingRate)
	}
	if mp.NextFundingTime != 1704096000000 {
		t.Errorf("NextFundingTime = %d, want 1704096000000", mp.NextFundingTime)
	}
}

func TestGetFundingRate(t *testing.T) {
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/fapi/v1/fundingRate") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("symbol") != "ETHUSDT" {
			t.Errorf("symbol = %s, want ETHUSDT", r.URL.Query().Get("symbol"))
		}
		if r.URL.Query().Get("limit") != "1" {
			t.Errorf("limit = %s, want 1", r.URL.Query().Get("limit"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{
			"symbol": "ETHUSDT",
			"fundingRate": "0.00015000",
			"fundingTime": 1704067200000
		}]`))
	})
	defer server.Close()

	fr, err := client.GetFundingRate(context.Background(), "ETH/USDT")
	if err != nil {
		t.Fatalf("GetFundingRate() error: %v", err)
	}
	if fr.Symbol != "ETHUSDT" {
		t.Errorf("Symbol = %s, want ETHUSDT", fr.Symbol)
	}
	if fr.FundingRate != 0.00015 {
		t.Errorf("FundingRate = %v, want 0.00015", fr.FundingRate)
	}
	if fr.FundingTime != 1704067200000 {
		t.Errorf("FundingTime = %d, want 1704067200000", fr.FundingTime)
	}
}

func TestGetFundingRate_Empty(t *testing.T) {
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	})
	defer server.Close()

	_, err := client.GetFundingRate(context.Background(), "INVALID/USDT")
	if err == nil {
		t.Fatal("expected error for empty funding rate response")
	}
	if !strings.Contains(err.Error(), "no funding rate data") {
		t.Errorf("error should mention no data, got: %v", err)
	}
}

func TestFutures_NetworkError(t *testing.T) {
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {})
	server.Close()

	// signed endpoint
	err := client.SetLeverage(context.Background(), "BTC/USDT", 10, "key", "secret")
	if err == nil {
		t.Error("expected error for closed server on SetLeverage")
	}

	// public endpoint
	_, err = client.GetMarkPrice(context.Background(), "BTC/USDT")
	if err == nil {
		t.Error("expected error for closed server on GetMarkPrice")
	}
}

func TestFutures_APIErrorParsing(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantMsg    string
	}{
		{
			name:       "invalid api key",
			statusCode: http.StatusForbidden,
			body:       `{"code":-2015,"msg":"Invalid API-key, IP, or permissions for action."}`,
			wantMsg:    "Invalid API-key",
		},
		{
			name:       "insufficient margin",
			statusCode: http.StatusBadRequest,
			body:       `{"code":-2019,"msg":"Margin is insufficient."}`,
			wantMsg:    "Margin is insufficient",
		},
		{
			name:       "non-json error",
			statusCode: http.StatusInternalServerError,
			body:       `internal server error`,
			wantMsg:    "status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.body))
			})
			defer server.Close()

			_, err := client.PlaceOrder(context.Background(), "BTC/USDT", exchange.SideBuy, exchange.OrderTypeMarket, 0.01, 0, "key", "secret")
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %v, want to contain %q", err, tt.wantMsg)
			}
		})
	}
}

func TestFutures_PublicEndpoint_APIError(t *testing.T) {
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":-1121,"msg":"Invalid symbol."}`))
	})
	defer server.Close()

	_, err := client.GetMarkPrice(context.Background(), "INVALID")
	if err == nil {
		t.Fatal("expected error for invalid symbol")
	}
	if !strings.Contains(err.Error(), "Invalid symbol") {
		t.Errorf("error should mention invalid symbol, got: %v", err)
	}
}

func TestFuturesOrderResponse_ToFuturesOrder(t *testing.T) {
	raw := &futuresOrderResponse{
		OrderID:       100,
		ClientOrderID: "test_client",
		Symbol:        "BTCUSDT",
		Side:          "BUY",
		Type:          "LIMIT",
		Status:        "NEW",
		Price:         "43000.50",
		StopPrice:     "0",
		OrigQty:       "0.005",
		ExecutedQty:   "0",
		AvgPrice:      "0",
		UpdateTime:    1704067200000,
	}

	order := raw.toFuturesOrder()
	if order.OrderID != 100 {
		t.Errorf("OrderID = %d, want 100", order.OrderID)
	}
	if order.ClientOrderID != "test_client" {
		t.Errorf("ClientOrderID = %s, want test_client", order.ClientOrderID)
	}
	if order.Price != 43000.50 {
		t.Errorf("Price = %v, want 43000.5", order.Price)
	}
	if order.Quantity != 0.005 {
		t.Errorf("Quantity = %v, want 0.005", order.Quantity)
	}
	if order.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestFuturesPositionResponse_Conversion(t *testing.T) {
	raw := &futuresPositionResponse{
		Symbol:           "BTCUSDT",
		PositionSide:     "LONG",
		PositionAmt:      "0.5",
		EntryPrice:       "42000",
		MarkPrice:        "43000",
		UnrealizedProfit: "500",
		LiquidationPrice: "35000",
		Leverage:         "10",
		MarginType:       "isolated",
		IsolatedMargin:   "2100",
		Notional:         "21500",
	}

	pos := raw.toFuturesPosition()
	if pos.Symbol != "BTCUSDT" {
		t.Errorf("Symbol = %s, want BTCUSDT", pos.Symbol)
	}
	if pos.PositionSide != "LONG" {
		t.Errorf("PositionSide = %s, want LONG", pos.PositionSide)
	}
	if pos.PositionAmt != 0.5 {
		t.Errorf("PositionAmt = %v, want 0.5", pos.PositionAmt)
	}
	if pos.Leverage != 10 {
		t.Errorf("Leverage = %d, want 10", pos.Leverage)
	}
}

func TestFuturesBalanceResponse_Conversion(t *testing.T) {
	raw := &futuresBalanceResponse{
		Asset:              "USDT",
		Balance:            "5000.50",
		AvailableBalance:   "4000.25",
		CrossWalletBalance: "4500.00",
	}

	bal := raw.toFuturesBalance()
	if bal.Asset != "USDT" {
		t.Errorf("Asset = %s, want USDT", bal.Asset)
	}
	if bal.Balance != 5000.50 {
		t.Errorf("Balance = %v, want 5000.5", bal.Balance)
	}
	if bal.AvailableBalance != 4000.25 {
		t.Errorf("AvailableBalance = %v, want 4000.25", bal.AvailableBalance)
	}
	if bal.CrossWalletBalance != 4500 {
		t.Errorf("CrossWalletBalance = %v, want 4500", bal.CrossWalletBalance)
	}
}

func TestFuturesSignedRequest_IncludesSignature(t *testing.T) {
	var hasSig bool
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		hasSig = r.URL.Query().Get("signature") != ""
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(futuresMarketOrderJSON()))
	})
	defer server.Close()

	_, _ = client.GetOrder(context.Background(), "BTC/USDT", 1, "key", "secret")
	if !hasSig {
		t.Error("request should include signature parameter")
	}
}

func TestFuturesSignedRequest_SetsAPIKeyHeader(t *testing.T) {
	var gotAPIKey string
	server, client := newTestFuturesServer(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("X-MBX-APIKEY")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(futuresMarketOrderJSON()))
	})
	defer server.Close()

	_, _ = client.GetOrder(context.Background(), "BTC/USDT", 1, "my_futures_key", "my_secret")
	if gotAPIKey != "my_futures_key" {
		t.Errorf("X-MBX-APIKEY = %s, want my_futures_key", gotAPIKey)
	}
}

func TestNewFuturesClient(t *testing.T) {
	client := NewFuturesClient(FuturesMainnetURL, false)
	if client.baseURL != FuturesMainnetURL {
		t.Errorf("baseURL = %s, want %s", client.baseURL, FuturesMainnetURL)
	}
	if client.testnet {
		t.Error("testnet should be false")
	}

	testClient := NewFuturesClient(FuturesTestnetURL, true)
	if testClient.baseURL != FuturesTestnetURL {
		t.Errorf("baseURL = %s, want %s", testClient.baseURL, FuturesTestnetURL)
	}
	if !testClient.testnet {
		t.Error("testnet should be true")
	}
}
