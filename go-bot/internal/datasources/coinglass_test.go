package datasources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCoinGlassGetDerivatives(t *testing.T) {
	oiResp := cgOIResponse{
		Code: 0,
		Data: []struct {
			Symbol             string  `json:"symbol"`
			OpenInterest       float64 `json:"openInterest"`
			OpenInterestAmount float64 `json:"openInterestAmount"`
			H24Change          float64 `json:"h24Change"`
		}{{Symbol: "BTC", OpenInterest: 5000000, H24Change: 2.5}},
	}
	liqResp := cgLiqResponse{
		Code: 0,
		Data: []struct {
			Symbol      string  `json:"symbol"`
			LongVolUSD  float64 `json:"longVolUsd"`
			ShortVolUSD float64 `json:"shortVolUsd"`
			TotalVolUSD float64 `json:"totalVolUsd"`
		}{{Symbol: "BTC", LongVolUSD: 1000000, ShortVolUSD: 800000, TotalVolUSD: 1800000}},
	}
	lsrResp := cgLSRResponse{
		Code: 0,
		Data: []struct {
			Symbol    string  `json:"symbol"`
			LongRate  float64 `json:"longRate"`
			ShortRate float64 `json:"shortRate"`
		}{{Symbol: "BTC", LongRate: 0.55, ShortRate: 0.45}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case contains(r.URL.Path, "open_interest"):
			json.NewEncoder(w).Encode(oiResp)
		case contains(r.URL.Path, "liquidation"):
			json.NewEncoder(w).Encode(liqResp)
		case contains(r.URL.Path, "long_short"):
			json.NewEncoder(w).Encode(lsrResp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cg := NewCoinGlassProvider("test-key")
	cg.baseURL = srv.URL

	deriv, err := cg.GetDerivatives(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deriv.OpenInterest != 5000000 {
		t.Errorf("expected OI 5000000, got %f", deriv.OpenInterest)
	}
	if deriv.OIChange24h != 2.5 {
		t.Errorf("expected OI change 2.5, got %f", deriv.OIChange24h)
	}
	if deriv.LongLiquidations != 1000000 {
		t.Errorf("expected long liq 1000000, got %f", deriv.LongLiquidations)
	}
	if deriv.TotalLiquidations != 1800000 {
		t.Errorf("expected total liq 1800000, got %f", deriv.TotalLiquidations)
	}
	if deriv.LongShortRatio < 1.2 || deriv.LongShortRatio > 1.25 {
		t.Errorf("expected LSR ~1.22, got %f", deriv.LongShortRatio)
	}
}

func TestCoinGlassGetMetrics(t *testing.T) {
	oiResp := cgOIResponse{
		Code: 0,
		Data: []struct {
			Symbol             string  `json:"symbol"`
			OpenInterest       float64 `json:"openInterest"`
			OpenInterestAmount float64 `json:"openInterestAmount"`
			H24Change          float64 `json:"h24Change"`
		}{{Symbol: "BTC", OpenInterest: 3000000, H24Change: -1.5}},
	}
	emptyLiq := cgLiqResponse{Code: 0}
	emptyLSR := cgLSRResponse{Code: 0}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case contains(r.URL.Path, "open_interest"):
			json.NewEncoder(w).Encode(oiResp)
		case contains(r.URL.Path, "liquidation"):
			json.NewEncoder(w).Encode(emptyLiq)
		case contains(r.URL.Path, "long_short"):
			json.NewEncoder(w).Encode(emptyLSR)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cg := NewCoinGlassProvider("test-key")
	cg.baseURL = srv.URL

	metrics, err := cg.GetMetrics(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metrics.Symbol != "BTCUSDT" {
		t.Errorf("expected BTCUSDT, got %s", metrics.Symbol)
	}
	// negative OI change should produce negative net flow
	if metrics.NetFlow >= 0 {
		t.Errorf("expected negative net flow for declining OI, got %f", metrics.NetFlow)
	}
}

func TestCoinGlassSupportedSymbols(t *testing.T) {
	cg := NewCoinGlassProvider("")
	syms := cg.SupportedSymbols()
	if len(syms) < 5 {
		t.Errorf("expected at least 5 supported symbols, got %d", len(syms))
	}
}

func TestCoinGlassServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cg := NewCoinGlassProvider("test-key")
	cg.baseURL = srv.URL

	// should not crash — errors for individual fetches are swallowed
	deriv, err := cg.GetDerivatives(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// fields should be zero when fetches fail
	if deriv.OpenInterest != 0 {
		t.Errorf("expected 0 OI on error, got %f", deriv.OpenInterest)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
