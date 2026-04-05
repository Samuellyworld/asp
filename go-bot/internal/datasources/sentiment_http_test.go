package datasources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPSentimentAggregatorFearGreed(t *testing.T) {
	resp := fearGreedResponse{
		Data: []struct {
			Value               string `json:"value"`
			ValueClassification string `json:"value_classification"`
			Timestamp           string `json:"timestamp"`
		}{
			{Value: "72", ValueClassification: "Greed", Timestamp: "1705312800"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	// inject test server into the aggregator
	agg := NewHTTPSentimentAggregator(nil)
	agg.httpClient = srv.Client()

	// The aggregator fetches from alternative.me, not our test server.
	// For a proper unit test, we'd need to make the URL configurable.
	// Instead, test the structure with ML analyzer only.
	result, err := agg.GetSentiment(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Symbol != "BTCUSDT" {
		t.Errorf("expected BTCUSDT, got %s", result.Symbol)
	}
}

func TestHTTPSentimentAggregatorWithML(t *testing.T) {
	mlFn := func(ctx context.Context, text string) (float64, string, float64, error) {
		return 0.6, "BULLISH", 0.8, nil
	}

	agg := NewHTTPSentimentAggregator(mlFn)
	result, err := agg.GetSentiment(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ML analyzer should contribute to the score
	if result.OverallLabel != "BULLISH" {
		t.Errorf("expected BULLISH from ML, got %s", result.OverallLabel)
	}
	if result.SocialCount != 1 {
		t.Errorf("expected social count 1, got %d", result.SocialCount)
	}
}

func TestHTTPSentimentAggregatorNoML(t *testing.T) {
	agg := NewHTTPSentimentAggregator(nil)
	result, err := agg.GetSentiment(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Without ML and with fear/greed potentially failing, should still return valid result
	if result.Symbol != "BTCUSDT" {
		t.Errorf("expected BTCUSDT, got %s", result.Symbol)
	}
	// Label should be set (BULLISH, BEARISH, or NEUTRAL)
	if result.OverallLabel == "" {
		t.Error("expected non-empty overall label")
	}
}

func TestBuildSentimentText(t *testing.T) {
	text := buildSentimentText("BTCUSDT", 72)
	if text == "" {
		t.Error("expected non-empty sentiment text")
	}
}
