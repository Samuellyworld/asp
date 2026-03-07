package mlclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:8000")
	if c.baseURL != "http://localhost:8000" {
		t.Errorf("expected base url http://localhost:8000, got %s", c.baseURL)
	}
	if c.httpClient == nil {
		t.Error("expected http client to be initialized")
	}
}

func TestHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("expected path /health, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(HealthResponse{Status: "healthy"})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	resp, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if resp.Status != "healthy" {
		t.Errorf("expected status healthy, got %s", resp.Status)
	}
}

func TestIsAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(HealthResponse{Status: "healthy"})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	if !c.IsAvailable(context.Background()) {
		t.Error("expected service to be available")
	}
}

func TestIsAvailableDown(t *testing.T) {
	c := NewClient("http://localhost:99999")
	if c.IsAvailable(context.Background()) {
		t.Error("expected service to be unavailable")
	}
}

func TestPredictPrice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict/price" {
			t.Errorf("expected path /predict/price, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected content-type application/json")
		}

		var req PricePredictionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Symbol != "BTC/USDT" {
			t.Errorf("expected symbol BTC/USDT, got %s", req.Symbol)
		}
		if len(req.Candles) != 1 {
			t.Errorf("expected 1 candle, got %d", len(req.Candles))
		}

		json.NewEncoder(w).Encode(PricePredictionResponse{
			Direction:      "up",
			Magnitude:      3.2,
			Confidence:     0.78,
			Timeframe:      "4h",
			PredictedPrice: 43400,
			CurrentPrice:   42000,
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	resp, err := c.PredictPrice(context.Background(), &PricePredictionRequest{
		Symbol:    "BTC/USDT",
		Candles:   []Candle{{Open: 42000, High: 42500, Low: 41800, Close: 42100, Volume: 1000, Timestamp: 1}},
		Timeframe: "4h",
	})
	if err != nil {
		t.Fatalf("predict price failed: %v", err)
	}
	if resp.Direction != "up" {
		t.Errorf("expected direction up, got %s", resp.Direction)
	}
	if resp.Magnitude != 3.2 {
		t.Errorf("expected magnitude 3.2, got %f", resp.Magnitude)
	}
	if resp.Confidence != 0.78 {
		t.Errorf("expected confidence 0.78, got %f", resp.Confidence)
	}
	if resp.Timeframe != "4h" {
		t.Errorf("expected timeframe 4h, got %s", resp.Timeframe)
	}
}

func TestPredictPriceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"detail":"insufficient data"}`))
	}))
	defer server.Close()

	c := NewClient(server.URL)
	_, err := c.PredictPrice(context.Background(), &PricePredictionRequest{
		Symbol:    "BTC/USDT",
		Candles:   []Candle{},
		Timeframe: "4h",
	})
	if err == nil {
		t.Error("expected error for insufficient data")
	}
}

func TestAnalyzeSentiment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/analyze/sentiment" {
			t.Errorf("expected path /analyze/sentiment, got %s", r.URL.Path)
		}

		var req SentimentRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Text != "BTC breaks resistance!" {
			t.Errorf("expected text 'BTC breaks resistance!', got '%s'", req.Text)
		}

		json.NewEncoder(w).Encode(SentimentResponse{
			Score:      0.82,
			Label:      "BULLISH",
			Confidence: 0.91,
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	resp, err := c.AnalyzeSentiment(context.Background(), "BTC breaks resistance!")
	if err != nil {
		t.Fatalf("sentiment analysis failed: %v", err)
	}
	if resp.Label != "BULLISH" {
		t.Errorf("expected label BULLISH, got %s", resp.Label)
	}
	if resp.Score != 0.82 {
		t.Errorf("expected score 0.82, got %f", resp.Score)
	}
	if resp.Confidence != 0.91 {
		t.Errorf("expected confidence 0.91, got %f", resp.Confidence)
	}
}

func TestAnalyzeSentimentError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"detail":"internal error"}`))
	}))
	defer server.Close()

	c := NewClient(server.URL)
	_, err := c.AnalyzeSentiment(context.Background(), "some text")
	if err == nil {
		t.Error("expected error for server error")
	}
}

func TestCandleJSON(t *testing.T) {
	c := Candle{
		Open:      100.5,
		High:      105.0,
		Low:       98.0,
		Close:     103.0,
		Volume:    50000,
		Timestamp: 1700000000,
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("failed to marshal candle: %v", err)
	}
	var decoded Candle
	json.Unmarshal(data, &decoded)
	if decoded.Open != 100.5 {
		t.Errorf("expected open 100.5, got %f", decoded.Open)
	}
	if decoded.Timestamp != 1700000000 {
		t.Errorf("expected timestamp 1700000000, got %d", decoded.Timestamp)
	}
}

func TestPredictPriceConnectionRefused(t *testing.T) {
	c := NewClient("http://localhost:99999")
	_, err := c.PredictPrice(context.Background(), &PricePredictionRequest{
		Symbol:    "BTC/USDT",
		Candles:   []Candle{{Close: 100}},
		Timeframe: "4h",
	})
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestSentimentConnectionRefused(t *testing.T) {
	c := NewClient("http://localhost:99999")
	_, err := c.AnalyzeSentiment(context.Background(), "test")
	if err == nil {
		t.Error("expected connection error")
	}
}
