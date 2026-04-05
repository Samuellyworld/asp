package datasources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBinanceOrderFlowGetSnapshot(t *testing.T) {
	depthResp := binanceDepthResponse{
		Bids: []binanceDepthEntry{
			{"65000.00", "1.5"},
			{"64990.00", "2.0"},
			{"64980.00", "0.8"},
		},
		Asks: []binanceDepthEntry{
			{"65010.00", "1.2"},
			{"65020.00", "1.8"},
			{"65030.00", "0.5"},
		},
	}

	aggTrades := []binanceAggTrade{
		{Price: "65000.00", Quantity: "0.5", IsBuyer: false, Time: 1705312800000}, // buy (taker buy)
		{Price: "65001.00", Quantity: "0.3", IsBuyer: true, Time: 1705312801000},  // sell (taker sell)
		{Price: "65002.00", Quantity: "1.0", IsBuyer: false, Time: 1705312802000}, // buy (large)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v3/depth":
			json.NewEncoder(w).Encode(depthResp)
		case r.URL.Path == "/api/v3/aggTrades":
			json.NewEncoder(w).Encode(aggTrades)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	of := NewBinanceOrderFlow(srv.URL)
	snap, err := of.GetSnapshot(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snap.Symbol != "BTCUSDT" {
		t.Errorf("expected BTCUSDT, got %s", snap.Symbol)
	}

	// check depth calculations
	if snap.BidDepthUSD <= 0 {
		t.Error("expected positive bid depth")
	}
	if snap.AskDepthUSD <= 0 {
		t.Error("expected positive ask depth")
	}

	// depth imbalance should exist
	if snap.DepthImbalance == 0 {
		t.Error("expected non-zero depth imbalance")
	}

	// spread should be positive
	if snap.SpreadBps <= 0 {
		t.Errorf("expected positive spread, got %f", snap.SpreadBps)
	}

	// buy volume should be > 0 (2 buy trades)
	if snap.BuyVolume <= 0 {
		t.Error("expected positive buy volume")
	}

	// sell volume should be > 0 (1 sell trade)
	if snap.SellVolume <= 0 {
		t.Error("expected positive sell volume")
	}

	// buy/sell ratio
	if snap.BuySellRatio <= 0 {
		t.Error("expected positive buy/sell ratio")
	}
}

func TestBinanceOrderFlowDepthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/depth" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode([]binanceAggTrade{})
	}))
	defer srv.Close()

	of := NewBinanceOrderFlow(srv.URL)
	_, err := of.GetSnapshot(context.Background(), "BTCUSDT")
	if err == nil {
		t.Error("expected error when depth fails")
	}
}

func TestBinanceOrderFlowAggTradesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/aggTrades" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(binanceDepthResponse{
			Bids: []binanceDepthEntry{{"65000", "1.0"}},
			Asks: []binanceDepthEntry{{"65010", "1.0"}},
		})
	}))
	defer srv.Close()

	of := NewBinanceOrderFlow(srv.URL)
	_, err := of.GetSnapshot(context.Background(), "BTCUSDT")
	if err == nil {
		t.Error("expected error when aggTrades fails")
	}
}

func TestComputeDepthUSD(t *testing.T) {
	depth := &binanceDepthResponse{
		Bids: []binanceDepthEntry{
			{"100.00", "10.0"}, // 1000 USD
			{"99.00", "5.0"},   // 495 USD
		},
		Asks: []binanceDepthEntry{
			{"101.00", "8.0"}, // 808 USD
		},
	}
	bidUSD, askUSD := computeDepthUSD(depth)
	if bidUSD < 1494 || bidUSD > 1496 {
		t.Errorf("expected bid depth ~1495, got %f", bidUSD)
	}
	if askUSD < 807 || askUSD > 809 {
		t.Errorf("expected ask depth ~808, got %f", askUSD)
	}
}

func TestNormalizeBinanceSymbol(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"BTC/USDT", "BTCUSDT"},
		{"BTCUSDT", "BTCUSDT"},
		{"ETH/BTC", "ETHBTC"},
	}
	for _, tc := range cases {
		got := normalizeBinanceSymbol(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeBinanceSymbol(%s) = %s, want %s", tc.input, got, tc.expected)
		}
	}
}

func TestParseFloat(t *testing.T) {
	cases := []struct {
		input    string
		expected float64
	}{
		{"65000.50", 65000.5},
		{"0", 0},
		{"invalid", 0},
	}
	for _, tc := range cases {
		got := parseFloat(tc.input)
		if got != tc.expected {
			t.Errorf("parseFloat(%s) = %f, want %f", tc.input, got, tc.expected)
		}
	}
}
