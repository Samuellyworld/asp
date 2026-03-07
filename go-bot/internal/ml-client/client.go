// package mlclient provides an http client for the python ml service
package mlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

//wraps the http connection to the python ml service
type Client struct {
	baseURL    string
	httpClient *http.Client
}

//  represents ohlcv price data for ml predictions
type Candle struct {
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
	Timestamp int64   `json:"timestamp"`
}

//  is the input for the price prediction endpoint
type PricePredictionRequest struct {
	Symbol    string   `json:"symbol"`
	Candles   []Candle `json:"candles"`
	Timeframe string   `json:"timeframe"`
}

// is the output from the price prediction endpoint
type PricePredictionResponse struct {
	Direction      string  `json:"direction"`
	Magnitude      float64 `json:"magnitude"`
	Confidence     float64 `json:"confidence"`
	Timeframe      string  `json:"timeframe"`
	PredictedPrice float64 `json:"predicted_price"`
	CurrentPrice   float64 `json:"current_price"`
}

//  is the input for the sentiment analysis endpoint
type SentimentRequest struct {
	Text string `json:"text"`
}

//  is the output from the sentiment analysis endpoint
type SentimentResponse struct {
	Score      float64 `json:"score"`
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
}

//  is the output from the health check endpoint
type HealthResponse struct {
	Status string `json:"status"`
}

// creates a new ml service client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// checks if the ml service is running
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	resp, err := c.get(ctx, "/health")
	if err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	var result HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode health response: %w", err)
	}
	return &result, nil
}

//  calls the price prediction endpoint
func (c *Client) PredictPrice(ctx context.Context, req *PricePredictionRequest) (*PricePredictionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.post(ctx, "/predict/price", body)
	if err != nil {
		return nil, fmt.Errorf("price prediction failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("price prediction returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result PricePredictionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode prediction response: %w", err)
	}
	return &result, nil
}

//  calls the sentiment analysis endpoint
func (c *Client) AnalyzeSentiment(ctx context.Context, text string) (*SentimentResponse, error) {
	req := SentimentRequest{Text: text}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.post(ctx, "/analyze/sentiment", body)
	if err != nil {
		return nil, fmt.Errorf("sentiment analysis failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sentiment analysis returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result SentimentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode sentiment response: %w", err)
	}
	return &result, nil
}

//  returns true if the ml service is reachable
func (c *Client) IsAvailable(ctx context.Context) bool {
	health, err := c.Health(ctx)
	return err == nil && health.Status == "healthy"
}

func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

func (c *Client) post(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}
