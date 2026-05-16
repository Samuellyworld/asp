package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trading-bot/go-bot/internal/claude"
)

func TestAnalyzeParsesResponsesOutputText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %s, want /responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_1",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"action\":\"BUY\",\"confidence\":72,\"entry\":100,\"stop_loss\":97,\"take_profit\":106,\"position_size\":50,\"reasoning\":\"setup confirmed\"}"}]}]
		}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))
	decision, err := client.Analyze(context.Background(), &claude.AnalysisInput{
		Market: claude.MarketData{Symbol: "BTCUSDT", Price: 100},
	})
	if err != nil {
		t.Fatalf("Analyze() error: %v", err)
	}
	if decision.Action != claude.ActionBuy || decision.Confidence != 72 {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestAnalyzeReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer server.Close()

	client := NewClient("test-key", WithBaseURL(server.URL))
	_, err := client.Analyze(context.Background(), &claude.AnalysisInput{
		Market: claude.MarketData{Symbol: "BTCUSDT", Price: 100},
	})
	if err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("expected bad request error, got %v", err)
	}
}
