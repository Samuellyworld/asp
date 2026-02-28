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
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	testnet    bool
}

// api permission flags returned by binance
type APIPermissions struct {
	Spot     bool
	Futures  bool
	Withdraw bool
}

// account info response from binance /api/v3/account
type accountResponse struct {
	CanTrade    bool   `json:"canTrade"`
	CanWithdraw bool   `json:"canWithdraw"`
	CanDeposit  bool   `json:"canDeposit"`
	AccountType string `json:"accountType"`
	Permissions []string `json:"permissions"`
}

// error response from binance api
type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
}

func NewClient(baseURL string, testnet bool) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    baseURL,
		testnet:    testnet,
	}
}

// sign a query string with the api secret using hmac-sha256
func sign(queryString, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(queryString))
	return hex.EncodeToString(mac.Sum(nil))
}

// validateKeys tests the api key/secret pair against binance and returns permissions
func (c *Client) ValidateKeys(ctx context.Context, apiKey, apiSecret string) (*APIPermissions, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	queryString := "timestamp=" + timestamp
	signature := sign(queryString, apiSecret)

	url := fmt.Sprintf("%s/api/v3/account?%s&signature=%s", c.baseURL, queryString, signature)

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

	var account accountResponse
	if err := json.Unmarshal(body, &account); err != nil {
		return nil, fmt.Errorf("failed to parse account response: %w", err)
	}

	perms := &APIPermissions{
		Spot:     account.CanTrade,
		Withdraw: account.CanWithdraw,
	}

	// check for futures permission in the permissions array
	for _, p := range account.Permissions {
		if p == "FUTURES" {
			perms.Futures = true
			break
		}
	}

	return perms, nil
}

// PermissionsToJSON converts permissions to the jsonb format for the database
func (p *APIPermissions) ToJSON() map[string]bool {
	return map[string]bool{
		"spot":     p.Spot,
		"futures":  p.Futures,
		"withdraw": p.Withdraw,
	}
}
