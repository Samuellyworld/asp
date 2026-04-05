package datasources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBinanceFundingRateGetRates(t *testing.T) {
	resp := binanceFundingRateResponse{
		Symbol:      "BTCUSDT",
		FundingRate: "0.00015",
		FundingTime: 1705334400000,
		MarkPrice:   "65000.50",
		IndexPrice:  "65001.00",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fapi/v1/premiumIndex" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		sym := r.URL.Query().Get("symbol")
		if sym != "BTCUSDT" {
			t.Errorf("expected BTCUSDT, got %s", sym)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	bf := NewBinanceFundingRate(srv.URL)
	data, err := bf.GetFundingRates(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Symbol != "BTCUSDT" {
		t.Errorf("expected BTCUSDT, got %s", data.Symbol)
	}
	rate, ok := data.Rates["binance"]
	if !ok {
		t.Fatal("expected binance rate in map")
	}
	if rate < 0.00014 || rate > 0.00016 {
		t.Errorf("expected rate ~0.00015, got %f", rate)
	}
	if data.MaxRate != rate {
		t.Errorf("expected max rate = rate for single exchange")
	}
	if data.Spread != 0 {
		t.Errorf("expected 0 spread for single exchange, got %f", data.Spread)
	}
	if data.Annualized <= 0 {
		t.Errorf("expected positive annualized rate, got %f", data.Annualized)
	}
	if data.NextFunding.IsZero() {
		t.Error("expected non-zero next funding time")
	}
}

func TestBinanceFundingRateServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	bf := NewBinanceFundingRate(srv.URL)
	_, err := bf.GetFundingRates(context.Background(), "BTCUSDT")
	if err == nil {
		t.Error("expected error for 503 response")
	}
}

func TestBinanceFundingRateSymbolNormalization(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sym := r.URL.Query().Get("symbol")
		if sym != "BTCUSDT" {
			t.Errorf("expected BTCUSDT after normalization, got %s", sym)
		}
		resp := binanceFundingRateResponse{
			Symbol:      "BTCUSDT",
			FundingRate: "0.0001",
			FundingTime: 1705334400000,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	bf := NewBinanceFundingRate(srv.URL)
	_, err := bf.GetFundingRates(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
