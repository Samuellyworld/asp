// package openai provides an OpenAI Responses API backed trading analyst.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
)

const (
	defaultBaseURL         = "https://api.openai.com/v1"
	defaultModel           = "gpt-5.4"
	defaultMaxOutputTokens = 1024
	maxRetries             = 3
)

type Client struct {
	apiKey          string
	model           string
	maxOutputTokens int
	baseURL         string
	httpClient      *http.Client
}

type Option func(*Client)

func WithModel(model string) Option {
	return func(c *Client) { c.model = model }
}

func WithMaxOutputTokens(max int) Option {
	return func(c *Client) { c.maxOutputTokens = max }
}

func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:          apiKey,
		model:           defaultModel,
		maxOutputTokens: defaultMaxOutputTokens,
		baseURL:         defaultBaseURL,
		httpClient:      &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Analyze(ctx context.Context, input *claude.AnalysisInput) (*claude.Decision, error) {
	start := time.Now()
	reqBody := responsesRequest{
		Model:           c.model,
		Instructions:    claude.SystemPrompt(),
		Input:           claude.UserPrompt(input),
		MaxOutputTokens: c.maxOutputTokens,
		Store:           false,
	}

	respBody, err := c.sendWithRetry(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai api call failed: %w", err)
	}

	text := extractResponseText(respBody)
	if text == "" {
		return nil, fmt.Errorf("empty response from openai")
	}

	decision, err := claude.ParseDecision(text)
	if err != nil {
		return nil, fmt.Errorf("failed to parse openai response: %w", err)
	}
	decision.Timestamp = time.Now()
	decision.Latency = time.Since(start)
	return decision, nil
}

func (c *Client) sendWithRetry(ctx context.Context, reqBody responsesRequest) (*responsesResponse, error) {
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

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

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

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var result responsesResponse
			if err := json.Unmarshal(respBytes, &result); err != nil {
				return nil, fmt.Errorf("failed to decode response: %w", err)
			}
			return &result, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			var apiErr responsesError
			_ = json.Unmarshal(respBytes, &apiErr)
			lastErr = fmt.Errorf("openai retryable error %d: %s", resp.StatusCode, apiErr.Error.Message)
			continue
		}

		var apiErr responsesError
		_ = json.Unmarshal(respBytes, &apiErr)
		return nil, fmt.Errorf("openai api error %d: %s", resp.StatusCode, apiErr.Error.Message)
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

type responsesRequest struct {
	Model           string `json:"model"`
	Instructions    string `json:"instructions"`
	Input           string `json:"input"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
	Store           bool   `json:"store"`
}

type responsesResponse struct {
	ID     string            `json:"id"`
	Output []responsesItem   `json:"output"`
	Error  *responseAPIError `json:"error,omitempty"`
}

type responsesItem struct {
	Type    string             `json:"type"`
	Role    string             `json:"role,omitempty"`
	Content []responsesContent `json:"content,omitempty"`
}

type responsesContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesError struct {
	Error responseAPIError `json:"error"`
}

type responseAPIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func extractResponseText(resp *responsesResponse) string {
	if resp == nil {
		return ""
	}
	for _, item := range resp.Output {
		for _, content := range item.Content {
			if content.Type == "output_text" && content.Text != "" {
				return content.Text
			}
		}
	}
	return ""
}
