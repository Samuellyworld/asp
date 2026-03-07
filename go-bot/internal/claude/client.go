package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

const (
	defaultBaseURL   = "https://api.anthropic.com/v1"
	defaultModel     = "claude-sonnet-4-20250514"
	defaultMaxTokens = 1024
	apiVersion       = "2023-06-01"
	maxRetries       = 3
)

// wraps the claude api with retry and backoff
type Client struct {
	apiKey     string
	model      string
	maxTokens  int
	baseURL    string
	httpClient *http.Client
}

// optional configuration for the client
type Option func(*Client)

// overrides the default model
func WithModel(model string) Option {
	return func(c *Client) { c.model = model }
}

// overrides the default max tokens
func WithMaxTokens(maxTokens int) Option {
	return func(c *Client) { c.maxTokens = maxTokens }
}

// overrides the base url (useful for testing)
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// overrides the default http client
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// creates a new claude client
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:    apiKey,
		model:     defaultModel,
		maxTokens: defaultMaxTokens,
		baseURL:   defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// sends all context to claude and returns a structured trading decision
func (c *Client) Analyze(ctx context.Context, input *AnalysisInput) (*Decision, error) {
	start := time.Now()

	systemPrompt := buildSystemPrompt()
	userPrompt := buildUserPrompt(input)

	reqBody := apiRequest{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    systemPrompt,
		Messages: []apiMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	respBody, err := c.sendWithRetry(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("claude api call failed: %w", err)
	}

	text := extractText(respBody)
	if text == "" {
		return nil, fmt.Errorf("empty response from claude")
	}

	decision, err := ParseDecision(text)
	if err != nil {
		return nil, fmt.Errorf("failed to parse claude response: %w", err)
	}

	decision.Timestamp = time.Now()
	decision.Latency = time.Since(start)

	return decision, nil
}

// sends the request with exponential backoff on rate limits
func (c *Client) sendWithRetry(ctx context.Context, reqBody apiRequest) (*apiResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/messages", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", apiVersion)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		// success
		if resp.StatusCode == http.StatusOK {
			var result apiResponse
			if err := json.Unmarshal(respBytes, &result); err != nil {
				return nil, fmt.Errorf("failed to decode response: %w", err)
			}
			return &result, nil
		}

		// rate limited or overloaded — retry
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 529 {
			var apiErr apiError
			json.Unmarshal(respBytes, &apiErr)
			lastErr = fmt.Errorf("rate limited (attempt %d/%d): %s", attempt+1, maxRetries+1, apiErr.Error.Message)
			continue
		}

		// non-retryable error
		var apiErr apiError
		json.Unmarshal(respBytes, &apiErr)
		return nil, fmt.Errorf("claude api error %d: %s", resp.StatusCode, apiErr.Error.Message)
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// pulls the text from the first content block
func extractText(resp *apiResponse) string {
	for _, block := range resp.Content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return ""
}
