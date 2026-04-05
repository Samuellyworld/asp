package datasources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCoinGeckoGetMarketData(t *testing.T) {
	resp := cgCoinResponse{
		ID:     "bitcoin",
		Symbol: "btc",
		Name:   "Bitcoin",
	}
	resp.MarketData.CurrentPrice = map[string]float64{"usd": 65000}
	resp.MarketData.MarketCap = map[string]float64{"usd": 1200000000000}
	resp.MarketData.TotalVolume = map[string]float64{"usd": 30000000000}
	resp.MarketData.CirculatingSupply = 19500000
	resp.MarketData.TotalSupply = 21000000
	resp.MarketData.ATH = map[string]float64{"usd": 69000}
	resp.MarketData.ATHChangePercentage = map[string]float64{"usd": -5.8}
	resp.MarketData.MarketCapRank = 1
	resp.CommunityData.TwitterFollowers = 6000000
	resp.CommunityData.RedditSubscribers = 5000000
	resp.CommunityData.RedditActiveAccounts = 10000
	resp.DeveloperData.Forks = 35000
	resp.DeveloperData.Stars = 75000
	resp.DeveloperData.CommitCount4Weeks = 100
	resp.CommunityScore = 85.5
	resp.DeveloperScore = 92.3

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cg := NewCoinGeckoProvider("")
	cg.baseURL = srv.URL

	market, err := cg.GetMarketData(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if market.MarketCapRank != 1 {
		t.Errorf("expected rank 1, got %d", market.MarketCapRank)
	}
	if market.MarketCap != 1200000000000 {
		t.Errorf("expected market cap 1.2T, got %f", market.MarketCap)
	}
	if market.TwitterFollowers != 6000000 {
		t.Errorf("expected 6M twitter followers, got %d", market.TwitterFollowers)
	}
	if market.GithubStars != 75000 {
		t.Errorf("expected 75000 github stars, got %d", market.GithubStars)
	}
}

func TestCoinGeckoGetMetrics(t *testing.T) {
	resp := cgCoinResponse{ID: "bitcoin", Symbol: "btc"}
	resp.MarketData.MarketCap = map[string]float64{"usd": 1200000000000}
	resp.MarketData.TotalVolume = map[string]float64{"usd": 30000000000}
	resp.CommunityData.RedditActiveAccounts = 8000

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cg := NewCoinGeckoProvider("")
	cg.baseURL = srv.URL

	metrics, err := cg.GetMetrics(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metrics.ActiveAddresses24h != 8000 {
		t.Errorf("expected 8000 active addresses, got %d", metrics.ActiveAddresses24h)
	}
	// NVT = market cap / volume = 1.2T / 30B = 40
	if metrics.NVTRatio < 39 || metrics.NVTRatio > 41 {
		t.Errorf("expected NVT ~40, got %f", metrics.NVTRatio)
	}
}

func TestCoinGeckoSupportedSymbols(t *testing.T) {
	cg := NewCoinGeckoProvider("")
	syms := cg.SupportedSymbols()
	if len(syms) < 5 {
		t.Errorf("expected at least 5 supported symbols, got %d", len(syms))
	}
}

func TestCoinGeckoServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	cg := NewCoinGeckoProvider("")
	cg.baseURL = srv.URL

	_, err := cg.GetMarketData(context.Background(), "BTCUSDT")
	if err == nil {
		t.Error("expected error for 429 response")
	}
}

func TestSymbolToCoinGeckoID(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"BTCUSDT", "bitcoin"},
		{"ETHUSDT", "ethereum"},
		{"SOLUSDT", "solana"},
		{"UNKNOWNUSDT", "unknown"},
	}
	for _, tc := range cases {
		got := symbolToCoinGeckoID(tc.input)
		if got != tc.expected {
			t.Errorf("symbolToCoinGeckoID(%s) = %s, want %s", tc.input, got, tc.expected)
		}
	}
}

func TestCoinGeckoProAPIKey(t *testing.T) {
	cg := NewCoinGeckoProvider("my-pro-key")
	if cg.baseURL != "https://pro-api.coingecko.com/api/v3" {
		t.Errorf("expected pro base URL, got %s", cg.baseURL)
	}
	cg2 := NewCoinGeckoProvider("")
	if cg2.baseURL != "https://api.coingecko.com/api/v3" {
		t.Errorf("expected free base URL, got %s", cg2.baseURL)
	}
}
