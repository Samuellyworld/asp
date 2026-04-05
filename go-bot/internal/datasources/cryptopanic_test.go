package datasources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCryptoPanicGetNews(t *testing.T) {
	resp := cpResponse{
		Results: []cpPost{
			{
				Title:       "Bitcoin hits new ATH",
				URL:         "https://example.com/article1",
				Source:      cpSource{Title: "CoinDesk"},
				PublishedAt: "2025-01-15T12:00:00Z",
				Currencies:  []cpCoin{{Code: "BTC", Title: "Bitcoin"}},
				Votes:       cpVotes{Positive: 10, Liked: 5, Negative: 1},
				Kind:        "news",
			},
			{
				Title:       "BTC whale moves 1000 coins",
				URL:         "https://example.com/article2",
				Source:      cpSource{Title: "CryptoSlate"},
				PublishedAt: "2025-01-15T11:00:00Z",
				Currencies:  []cpCoin{{Code: "BTC", Title: "Bitcoin"}},
				Votes:       cpVotes{Negative: 8, Toxic: 2},
				Kind:        "news",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify auth token is passed
		token := r.URL.Query().Get("auth_token")
		if token != "test-token" {
			t.Errorf("expected auth_token=test-token, got %s", token)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cp := NewCryptoPanicProvider("test-token")
	cp.baseURL = srv.URL

	items, err := cp.GetNews(context.Background(), "BTCUSDT", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// first article has mostly positive votes -> positive sentiment
	if items[0].Sentiment <= 0 {
		t.Errorf("expected positive sentiment for bullish article, got %f", items[0].Sentiment)
	}
	if items[0].Source != "CryptoPanic/CoinDesk" {
		t.Errorf("expected source CryptoPanic/CoinDesk, got %s", items[0].Source)
	}

	// second article has mostly negative votes -> negative sentiment
	if items[1].Sentiment >= 0 {
		t.Errorf("expected negative sentiment for FUD article, got %f", items[1].Sentiment)
	}
}

func TestCryptoPanicLimit(t *testing.T) {
	resp := cpResponse{
		Results: make([]cpPost, 20),
	}
	for i := range resp.Results {
		resp.Results[i] = cpPost{
			Title:       "News article",
			URL:         "https://example.com",
			Source:      cpSource{Title: "Source"},
			PublishedAt: "2025-01-15T12:00:00Z",
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cp := NewCryptoPanicProvider("token")
	cp.baseURL = srv.URL

	items, err := cp.GetNews(context.Background(), "BTCUSDT", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items (limit), got %d", len(items))
	}
}

func TestCryptoPanicServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	cp := NewCryptoPanicProvider("bad-token")
	cp.baseURL = srv.URL

	_, err := cp.GetNews(context.Background(), "BTCUSDT", 5)
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestDeriveCPSentiment(t *testing.T) {
	cases := []struct {
		name     string
		votes    cpVotes
		positive bool
	}{
		{"mostly positive", cpVotes{Positive: 10, Liked: 5, Saved: 3, Negative: 1}, true},
		{"mostly negative", cpVotes{Negative: 10, Toxic: 5, Positive: 1}, false},
		{"neutral (empty)", cpVotes{}, false}, // 0 returned
		{"balanced", cpVotes{Positive: 5, Negative: 5}, false}, // ~0
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score := deriveCPSentiment(tc.votes)
			if tc.positive && score <= 0 {
				t.Errorf("expected positive score, got %f", score)
			}
			if !tc.positive && tc.name == "mostly negative" && score >= 0 {
				t.Errorf("expected negative score, got %f", score)
			}
			if score < -1 || score > 1 {
				t.Errorf("score out of [-1,1] range: %f", score)
			}
		})
	}
}
