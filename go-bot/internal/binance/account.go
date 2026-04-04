// binance authenticated account operations (signed requests)
package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// balance entry from binance account response
type balanceEntry struct {
	Asset  string `json:"asset"`
	Free   string `json:"free"`
	Locked string `json:"locked"`
}

// full account response with balances
type fullAccountResponse struct {
	Balances []balanceEntry `json:"balances"`
}

// returns non-zero balances for the authenticated user
func (c *Client) GetBalance(ctx context.Context, apiKey, apiSecret string) ([]exchange.Balance, error) {
	if err := c.rateLimiter.Wait(ctx, WeightForEndpoint("/api/v3/account")); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

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
		return nil, fmt.Errorf("failed to get balance: %w", err)
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

	var account fullAccountResponse
	if err := json.Unmarshal(body, &account); err != nil {
		return nil, fmt.Errorf("failed to parse account response: %w", err)
	}

	// filter to non-zero balances only
	var balances []exchange.Balance
	for _, b := range account.Balances {
		free, _ := strconv.ParseFloat(b.Free, 64)
		locked, _ := strconv.ParseFloat(b.Locked, 64)
		if free > 0 || locked > 0 {
			balances = append(balances, exchange.Balance{
				Asset:  b.Asset,
				Free:   free,
				Locked: locked,
			})
		}
	}

	return balances, nil
}
