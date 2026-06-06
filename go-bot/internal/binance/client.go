// binance api client for key validation and permission detection
package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

type Client struct {
	httpClient  *http.Client
	baseURL     string
	testnet     bool
	rateLimiter *RateLimiter
}

// APIPermissions is kept as a package alias for existing callers.
type APIPermissions = exchange.APIPermissions

// account info response from binance /api/v3/account
type accountResponse struct {
	CanTrade    bool     `json:"canTrade"`
	CanWithdraw bool     `json:"canWithdraw"`
	CanDeposit  bool     `json:"canDeposit"`
	AccountType string   `json:"accountType"`
	Permissions []string `json:"permissions"`
}

// describes the API-key-specific permissions returned
// by Binance's wallet API on mainnet.
type apiRestrictionsResponse struct {
	EnableWithdrawals          bool `json:"enableWithdrawals"`
	EnableFutures              bool `json:"enableFutures"`
	EnableSpotAndMarginTrading bool `json:"enableSpotAndMarginTrading"`
}

// error response from binance api
type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
}

func NewClient(baseURL string, testnet bool) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		baseURL:     baseURL,
		testnet:     testnet,
		rateLimiter: NewRateLimiter(SpotWeightLimit),
	}
}

// SetRateLimiter allows sharing a rate limiter across clients
func (c *Client) SetRateLimiter(rl *RateLimiter) {
	c.rateLimiter = rl
}

// RateLimiter returns the client's rate limiter (for sharing with other clients)
func (c *Client) RateLimiter() *RateLimiter {
	return c.rateLimiter
}

// sign a query string with the api secret using hmac-sha256
func sign(queryString, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(queryString))
	return hex.EncodeToString(mac.Sum(nil))
}

// validateKeys tests the api key/secret pair against binance and returns permissions
func (c *Client) ValidateKeys(ctx context.Context, apiKey, apiSecret string) (*APIPermissions, error) {
	if err := c.rateLimiter.Wait(ctx, WeightForEndpoint("/api/v3/account")); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	body, err := c.signedGET(ctx, apiKey, apiSecret, "/api/v3/account")
	if err != nil {
		return nil, err
	}

	var account accountResponse
	if err := json.Unmarshal(body, &account); err != nil {
		return nil, fmt.Errorf("failed to parse account response: %w", err)
	}

	perms := &APIPermissions{
		Spot: account.CanTrade,
	}

	// check for futures permission in the permissions array
	for _, p := range account.Permissions {
		if p == "FUTURES" {
			perms.Futures = true
			break
		}
	}

	if !c.testnet {
		restrictions, err := c.getAPIRestrictions(ctx, apiKey, apiSecret)
		if err != nil {
			return nil, err
		}
		perms.Withdraw = restrictions.EnableWithdrawals
		perms.Futures = perms.Futures || restrictions.EnableFutures
		perms.Spot = perms.Spot || restrictions.EnableSpotAndMarginTrading
	}

	return perms, nil
}

func (c *Client) getAPIRestrictions(ctx context.Context, apiKey, apiSecret string) (*apiRestrictionsResponse, error) {
	body, err := c.signedGET(ctx, apiKey, apiSecret, "/sapi/v1/account/apiRestrictions")
	if err != nil {
		return nil, fmt.Errorf("failed to verify api key permissions: %w", err)
	}

	var restrictions apiRestrictionsResponse
	if err := json.Unmarshal(body, &restrictions); err != nil {
		return nil, fmt.Errorf("failed to parse api key permissions response: %w", err)
	}
	return &restrictions, nil
}

func (c *Client) signedGET(ctx context.Context, apiKey, apiSecret, path string) ([]byte, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	queryString := "timestamp=" + timestamp
	signature := sign(queryString, apiSecret)

	url := fmt.Sprintf("%s%s?%s&signature=%s", c.baseURL, path, queryString, signature)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-MBX-APIKEY", apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to binance: %w", err)
	}
	defer resp.Body.Close()
	c.rateLimiter.RecordResponse(resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr apiError
		if json.Unmarshal(body, &apiErr) == nil {
			return nil, fmt.Errorf("binance api error (code %d): %s", apiErr.Code, apiErr.Message)
		}
		return nil, fmt.Errorf("binance api returned status %d", resp.StatusCode)
	}

	return body, nil
}
