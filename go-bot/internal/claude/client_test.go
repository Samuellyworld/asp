package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("test-key")
	if c.apiKey != "test-key" {
		t.Errorf("expected api key test-key, got %s", c.apiKey)
	}
	if c.model != defaultModel {
		t.Errorf("expected model %s, got %s", defaultModel, c.model)
	}
	if c.maxTokens != defaultMaxTokens {
		t.Errorf("expected max tokens %d, got %d", defaultMaxTokens, c.maxTokens)
	}
	if c.baseURL != defaultBaseURL {
		t.Errorf("expected base url %s, got %s", defaultBaseURL, c.baseURL)
	}
}

func TestNewClientWithOptions(t *testing.T) {
	c := NewClient("key",
		WithModel("claude-3-opus"),
		WithMaxTokens(2048),
		WithBaseURL("http://localhost:9999"),
	)
	if c.model != "claude-3-opus" {
		t.Errorf("expected model claude-3-opus, got %s", c.model)
	}
	if c.maxTokens != 2048 {
		t.Errorf("expected max tokens 2048, got %d", c.maxTokens)
	}
	if c.baseURL != "http://localhost:9999" {
		t.Errorf("expected base url http://localhost:9999, got %s", c.baseURL)
	}
}

func TestAnalyzeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Errorf("expected path /messages, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected api key header test-key")
		}
		if r.Header.Get("anthropic-version") != apiVersion {
			t.Errorf("expected anthropic-version %s", apiVersion)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected content-type application/json")
		}

		var req apiRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.System == "" {
			t.Error("expected system prompt")
		}
		if len(req.Messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(req.Messages))
		}
		if !strings.Contains(req.Messages[0].Content, "BTC/USDT") {
			t.Error("expected user prompt to contain symbol")
		}

		resp := apiResponse{
			ID: "msg_test",
			Content: []apiContentBlock{
				{
					Type: "text",
					Text: `{"action":"BUY","confidence":85,"entry":42450,"stop_loss":41800,"take_profit":44200,"position_size":200,"reasoning":"Strong confluence at support with bullish indicators."}`,
				},
			},
			StopReason: "end_turn",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient("test-key", WithBaseURL(server.URL))
	input := &AnalysisInput{
		Market: MarketData{
			Symbol:    "BTC/USDT",
			Price:     42450,
			Volume24h: 28500000000,
			Change24h: -2.1,
		},
		Indicators: &Indicators{
			RSI:        32.5,
			MACDValue:  -50,
			MACDSignal: -80,
			MACDHist:   30,
			BBUpper:    44000,
			BBMiddle:   42500,
			BBLower:    41000,
			EMA12:      42300,
			EMA26:      42100,
			VolumeSpike: false,
		},
		Prediction: &MLPrediction{
			Direction:  "up",
			Magnitude:  3.2,
			Confidence: 0.78,
			Timeframe:  "4h",
		},
		Sentiment: &Sentiment{
			Score:      0.82,
			Label:      "BULLISH",
			Confidence: 0.91,
		},
	}

	decision, err := c.Analyze(context.Background(), input)
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}

	if decision.Action != ActionBuy {
		t.Errorf("expected action BUY, got %s", decision.Action)
	}
	if decision.Confidence != 85 {
		t.Errorf("expected confidence 85, got %.0f", decision.Confidence)
	}
	if decision.Plan.Entry != 42450 {
		t.Errorf("expected entry 42450, got %.0f", decision.Plan.Entry)
	}
	if decision.Plan.StopLoss != 41800 {
		t.Errorf("expected stop loss 41800, got %.0f", decision.Plan.StopLoss)
	}
	if decision.Plan.TakeProfit != 44200 {
		t.Errorf("expected take profit 44200, got %.0f", decision.Plan.TakeProfit)
	}
	if decision.Plan.PositionSize != 200 {
		t.Errorf("expected position size 200, got %.0f", decision.Plan.PositionSize)
	}
	if decision.Plan.RiskReward <= 0 {
		t.Error("expected positive risk/reward")
	}
	if decision.Reasoning == "" {
		t.Error("expected non-empty reasoning")
	}
	if decision.Latency <= 0 {
		t.Error("expected positive latency")
	}
}

func TestAnalyzeWithoutOptionalData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req apiRequest
		json.NewDecoder(r.Body).Decode(&req)

		// verify prompt doesn't contain indicator/ml/sentiment sections
		prompt := req.Messages[0].Content
		if strings.Contains(prompt, "Technical Indicators") {
			t.Error("should not contain indicators section when nil")
		}
		if strings.Contains(prompt, "ML Price Prediction") {
			t.Error("should not contain prediction section when nil")
		}
		if strings.Contains(prompt, "Sentiment") {
			t.Error("should not contain sentiment section when nil")
		}

		resp := apiResponse{
			Content: []apiContentBlock{{
				Type: "text",
				Text: `{"action":"HOLD","confidence":40,"entry":0,"stop_loss":0,"take_profit":0,"position_size":0,"reasoning":"Insufficient data."}`,
			}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient("key", WithBaseURL(server.URL))
	decision, err := c.Analyze(context.Background(), &AnalysisInput{
		Market: MarketData{Symbol: "ETH/USDT", Price: 2200},
	})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if decision.Action != ActionHold {
		t.Errorf("expected HOLD, got %s", decision.Action)
	}
}

func TestRetryOnRateLimit(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(apiError{
				Error: struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				}{Type: "rate_limit_error", Message: "too many requests"},
			})
			return
		}

		resp := apiResponse{
			Content: []apiContentBlock{{
				Type: "text",
				Text: `{"action":"HOLD","confidence":50,"entry":0,"stop_loss":0,"take_profit":0,"position_size":0,"reasoning":"Hold for now."}`,
			}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient("key", WithBaseURL(server.URL))
	decision, err := c.Analyze(context.Background(), &AnalysisInput{
		Market: MarketData{Symbol: "BTC/USDT", Price: 42000},
	})
	if err != nil {
		t.Fatalf("expected retry to succeed, got: %v", err)
	}
	if decision.Action != ActionHold {
		t.Errorf("expected HOLD, got %s", decision.Action)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestNonRetryableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(apiError{
			Error: struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}{Type: "invalid_request_error", Message: "bad request"},
		})
	}))
	defer server.Close()

	c := NewClient("key", WithBaseURL(server.URL))
	_, err := c.Analyze(context.Background(), &AnalysisInput{
		Market: MarketData{Symbol: "BTC/USDT", Price: 42000},
	})
	if err == nil {
		t.Error("expected error for bad request")
	}
	if !strings.Contains(err.Error(), "bad request") {
		t.Errorf("expected error to contain 'bad request', got: %v", err)
	}
}

func TestConnectionRefused(t *testing.T) {
	c := NewClient("key", WithBaseURL("http://localhost:99999"))
	_, err := c.Analyze(context.Background(), &AnalysisInput{
		Market: MarketData{Symbol: "BTC/USDT", Price: 42000},
	})
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestExtractText(t *testing.T) {
	resp := &apiResponse{
		Content: []apiContentBlock{
			{Type: "text", Text: "hello world"},
		},
	}
	text := extractText(resp)
	if text != "hello world" {
		t.Errorf("expected 'hello world', got %q", text)
	}
}

func TestExtractTextEmpty(t *testing.T) {
	resp := &apiResponse{Content: []apiContentBlock{}}
	text := extractText(resp)
	if text != "" {
		t.Errorf("expected empty string, got %q", text)
	}
}

func TestEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(apiResponse{Content: []apiContentBlock{}})
	}))
	defer server.Close()

	c := NewClient("key", WithBaseURL(server.URL))
	_, err := c.Analyze(context.Background(), &AnalysisInput{
		Market: MarketData{Symbol: "BTC/USDT", Price: 42000},
	})
	if err == nil {
		t.Error("expected error for empty response")
	}
}

func TestSellDecision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := apiResponse{
			Content: []apiContentBlock{{
				Type: "text",
				Text: `{"action":"SELL","confidence":75,"entry":42450,"stop_loss":43200,"take_profit":40500,"position_size":150,"reasoning":"Bearish divergence."}`,
			}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient("key", WithBaseURL(server.URL))
	decision, err := c.Analyze(context.Background(), &AnalysisInput{
		Market: MarketData{Symbol: "BTC/USDT", Price: 42450},
	})
	if err != nil {
		t.Fatalf("analyze failed: %v", err)
	}
	if decision.Action != ActionSell {
		t.Errorf("expected SELL, got %s", decision.Action)
	}
	if decision.Plan.RiskReward <= 0 {
		t.Error("expected positive risk/reward for sell")
	}
}
