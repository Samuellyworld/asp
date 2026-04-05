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

// --- ensemble prediction ---

type EnsemblePredictionRequest struct {
	Symbol    string   `json:"symbol"`
	Candles   []Candle `json:"candles"`
	Timeframe string   `json:"timeframe"`
}

type EnsemblePredictionResponse struct {
	Direction      string                 `json:"direction"`
	Magnitude      float64                `json:"magnitude"`
	Confidence     float64                `json:"confidence"`
	Timeframe      string                 `json:"timeframe"`
	PredictedPrice float64                `json:"predicted_price"`
	CurrentPrice   float64                `json:"current_price"`
	ModelCount     int                    `json:"model_count"`
	ModelDetails   map[string]interface{} `json:"model_details"`
	IsEnsemble     bool                   `json:"is_ensemble"`
}

// --- pattern detection ---

type PatternDetectRequest struct {
	Symbol  string   `json:"symbol"`
	Candles []Candle `json:"candles"`
}

type PatternDetectResponse struct {
	Patterns      []map[string]interface{} `json:"patterns"`
	PatternCount  int                      `json:"pattern_count"`
	Signal        string                   `json:"signal"`
	SignalStrength float64                 `json:"signal_strength"`
	Summary       string                   `json:"summary"`
}

// --- drift detection ---

type DriftCheckRequest struct {
	Candles []Candle `json:"candles"`
}

type DriftCheckResponse struct {
	DriftDetected  bool                   `json:"drift_detected"`
	Reason         string                 `json:"reason"`
	Recommendation string                 `json:"recommendation"`
	Checks         map[string]interface{} `json:"checks"`
	Timestamp      string                 `json:"timestamp"`
}

// --- retraining ---

type RetrainRequest struct {
	Candles []Candle `json:"candles"`
	Epochs  int      `json:"epochs"`
}

type RetrainResponse struct {
	Success             bool                   `json:"success"`
	Message             string                 `json:"message,omitempty"`
	Reason              string                 `json:"reason,omitempty"`
	Promoted            bool                   `json:"promoted,omitempty"`
	NewModelMetrics     map[string]interface{} `json:"new_model_metrics,omitempty"`
	CurrentModelMetrics map[string]interface{} `json:"current_model_metrics,omitempty"`
	TrainingSamples     int                    `json:"training_samples,omitempty"`
	ValidationSamples   int                    `json:"validation_samples,omitempty"`
}

// --- walk-forward validation ---

type WalkForwardRequest struct {
	Candles []Candle `json:"candles"`
	NSplits int      `json:"n_splits"`
}

type WalkForwardResponse struct {
	Success              bool                     `json:"success"`
	Reason               string                   `json:"reason,omitempty"`
	NSplits              int                      `json:"n_splits,omitempty"`
	Folds                []map[string]interface{} `json:"folds,omitempty"`
	AvgDirectionAccuracy float64                  `json:"avg_direction_accuracy,omitempty"`
	AvgMagnitudeRMSE     float64                  `json:"avg_magnitude_rmse,omitempty"`
}

// --- RL agent ---

type RLTrainRequest struct {
	Candles        []Candle `json:"candles"`
	Episodes       int      `json:"episodes"`
	InitialBalance float64  `json:"initial_balance"`
}

type RLTrainResponse struct {
	Success          bool    `json:"success"`
	Reason           string  `json:"reason,omitempty"`
	Episodes         int     `json:"episodes,omitempty"`
	AvgRewardLast20  float64 `json:"avg_reward_last_20,omitempty"`
	AvgPnlLast20     float64 `json:"avg_pnl_last_20,omitempty"`
	BestPnl          float64 `json:"best_pnl,omitempty"`
	FinalEpsilon     float64 `json:"final_epsilon,omitempty"`
}

type RLActionRequest struct {
	Candles    []Candle `json:"candles"`
	Balance    float64  `json:"balance"`
	Position   float64  `json:"position"`
	EntryPrice float64  `json:"entry_price"`
}

type RLActionResponse struct {
	Action    string    `json:"action"`
	ActionIdx int       `json:"action_idx"`
	QValues   []float64 `json:"q_values"`
	Exploring bool      `json:"exploring"`
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

// EnsemblePredict calls the ensemble prediction endpoint.
func (c *Client) EnsemblePredict(ctx context.Context, req *EnsemblePredictionRequest) (*EnsemblePredictionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.post(ctx, "/ensemble/predict", body)
	if err != nil {
		return nil, fmt.Errorf("ensemble prediction failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ensemble prediction returned %d: %s", resp.StatusCode, string(respBody))
	}
	var result EnsemblePredictionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode ensemble response: %w", err)
	}
	return &result, nil
}

// DetectPatterns calls the pattern detection endpoint.
func (c *Client) DetectPatterns(ctx context.Context, req *PatternDetectRequest) (*PatternDetectResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.post(ctx, "/patterns/detect", body)
	if err != nil {
		return nil, fmt.Errorf("pattern detection failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pattern detection returned %d: %s", resp.StatusCode, string(respBody))
	}
	var result PatternDetectResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode patterns response: %w", err)
	}
	return &result, nil
}

// CheckDrift calls the drift detection endpoint.
func (c *Client) CheckDrift(ctx context.Context, req *DriftCheckRequest) (*DriftCheckResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.post(ctx, "/drift/check", body)
	if err != nil {
		return nil, fmt.Errorf("drift check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("drift check returned %d: %s", resp.StatusCode, string(respBody))
	}
	var result DriftCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode drift response: %w", err)
	}
	return &result, nil
}

// Retrain triggers model retraining with new data.
func (c *Client) Retrain(ctx context.Context, req *RetrainRequest) (*RetrainResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.post(ctx, "/retrain", body)
	if err != nil {
		return nil, fmt.Errorf("retrain failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("retrain returned %d: %s", resp.StatusCode, string(respBody))
	}
	var result RetrainResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode retrain response: %w", err)
	}
	return &result, nil
}

// WalkForwardValidate runs walk-forward cross-validation.
func (c *Client) WalkForwardValidate(ctx context.Context, req *WalkForwardRequest) (*WalkForwardResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.post(ctx, "/walk-forward", body)
	if err != nil {
		return nil, fmt.Errorf("walk-forward failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("walk-forward returned %d: %s", resp.StatusCode, string(respBody))
	}
	var result WalkForwardResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode walk-forward response: %w", err)
	}
	return &result, nil
}

// TrainRL trains the RL agent from historical candle data.
func (c *Client) TrainRL(ctx context.Context, req *RLTrainRequest) (*RLTrainResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.post(ctx, "/rl/train", body)
	if err != nil {
		return nil, fmt.Errorf("rl train failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("rl train returned %d: %s", resp.StatusCode, string(respBody))
	}
	var result RLTrainResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode rl train response: %w", err)
	}
	return &result, nil
}

// GetRLAction gets an action suggestion from the RL agent.
func (c *Client) GetRLAction(ctx context.Context, req *RLActionRequest) (*RLActionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	resp, err := c.post(ctx, "/rl/action", body)
	if err != nil {
		return nil, fmt.Errorf("rl action failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("rl action returned %d: %s", resp.StatusCode, string(respBody))
	}
	var result RLActionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode rl action response: %w", err)
	}
	return &result, nil
}
