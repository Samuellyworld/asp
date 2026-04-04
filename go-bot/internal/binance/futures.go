// binance usdt-m futures api client (signed and public endpoints)
package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// base urls for binance futures api
const (
	FuturesMainnetURL = "https://fapi.binance.com"
	FuturesTestnetURL = "https://testnet.binancefuture.com"
)

// client for binance usdt-m futures api
type FuturesClient struct {
	httpClient *http.Client
	baseURL    string
	testnet    bool
}

// creates a new futures client
func NewFuturesClient(baseURL string, testnet bool) *FuturesClient {
	return &FuturesClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    baseURL,
		testnet:    testnet,
	}
}

// sets the leverage for a symbol
func (c *FuturesClient) SetLeverage(symbol string, leverage int, apiKey, apiSecret string) error {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("leverage", strconv.Itoa(leverage))

	_, err := c.signedRawRequest(context.TODO(), http.MethodPost, "/fapi/v1/leverage", params, apiKey, apiSecret)
	return err
}

// sets the margin type (isolated or cross) for a symbol
func (c *FuturesClient) SetMarginType(symbol string, marginType string, apiKey, apiSecret string) error {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("marginType", marginType)

	_, err := c.signedRawRequest(context.TODO(), http.MethodPost, "/fapi/v1/marginType", params, apiKey, apiSecret)
	return err
}

// places a futures order (market or limit)
func (c *FuturesClient) PlaceOrder(symbol string, side exchange.OrderSide, orderType exchange.OrderType, quantity, price float64, apiKey, apiSecret string) (*FuturesOrder, error) {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("side", string(side))
	params.Set("type", string(orderType))
	params.Set("quantity", formatFloat(quantity))

	if orderType == exchange.OrderTypeLimit {
		params.Set("price", formatFloat(price))
		params.Set("timeInForce", "GTC")
	}

	return c.postFuturesOrder(params, apiKey, apiSecret)
}

// places a stop market order for futures
func (c *FuturesClient) PlaceStopMarket(symbol string, side exchange.OrderSide, quantity, stopPrice float64, apiKey, apiSecret string) (*FuturesOrder, error) {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("side", string(side))
	params.Set("type", "STOP_MARKET")
	params.Set("quantity", formatFloat(quantity))
	params.Set("stopPrice", formatFloat(stopPrice))

	return c.postFuturesOrder(params, apiKey, apiSecret)
}

// places a take profit market order for futures
func (c *FuturesClient) PlaceTakeProfitMarket(symbol string, side exchange.OrderSide, quantity, stopPrice float64, apiKey, apiSecret string) (*FuturesOrder, error) {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("side", string(side))
	params.Set("type", "TAKE_PROFIT_MARKET")
	params.Set("quantity", formatFloat(quantity))
	params.Set("stopPrice", formatFloat(stopPrice))

	return c.postFuturesOrder(params, apiKey, apiSecret)
}

// cancels an existing futures order by id
func (c *FuturesClient) CancelOrder(symbol string, orderID int64, apiKey, apiSecret string) error {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("orderId", strconv.FormatInt(orderID, 10))

	_, err := c.signedRawRequest(context.TODO(), http.MethodDelete, "/fapi/v1/order", params, apiKey, apiSecret)
	return err
}

// returns the status of a specific futures order
func (c *FuturesClient) GetOrder(symbol string, orderID int64, apiKey, apiSecret string) (*FuturesOrder, error) {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("orderId", strconv.FormatInt(orderID, 10))

	body, err := c.signedRawRequest(context.TODO(), http.MethodGet, "/fapi/v1/order", params, apiKey, apiSecret)
	if err != nil {
		return nil, err
	}

	var raw futuresOrderResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse futures order: %w", err)
	}
	return raw.toFuturesOrder(), nil
}

// returns all futures positions
func (c *FuturesClient) GetPositions(apiKey, apiSecret string) ([]FuturesPosition, error) {
	params := url.Values{}

	body, err := c.signedRawRequest(context.TODO(), http.MethodGet, "/fapi/v2/positionRisk", params, apiKey, apiSecret)
	if err != nil {
		return nil, err
	}

	var rawPositions []futuresPositionResponse
	if err := json.Unmarshal(body, &rawPositions); err != nil {
		return nil, fmt.Errorf("failed to parse futures positions: %w", err)
	}

	positions := make([]FuturesPosition, len(rawPositions))
	for i, r := range rawPositions {
		positions[i] = r.toFuturesPosition()
	}
	return positions, nil
}

// returns futures account balances
func (c *FuturesClient) GetFuturesBalance(apiKey, apiSecret string) ([]FuturesBalance, error) {
	params := url.Values{}

	body, err := c.signedRawRequest(context.TODO(), http.MethodGet, "/fapi/v2/balance", params, apiKey, apiSecret)
	if err != nil {
		return nil, err
	}

	var rawBalances []futuresBalanceResponse
	if err := json.Unmarshal(body, &rawBalances); err != nil {
		return nil, fmt.Errorf("failed to parse futures balances: %w", err)
	}

	balances := make([]FuturesBalance, len(rawBalances))
	for i, r := range rawBalances {
		balances[i] = r.toFuturesBalance()
	}
	return balances, nil
}

// returns the current mark price for a symbol (public endpoint)
func (c *FuturesClient) GetMarkPrice(symbol string) (*MarkPrice, error) {
	reqURL := fmt.Sprintf("%s/fapi/v1/premiumIndex?symbol=%s", c.baseURL, toBinanceSymbol(symbol))

	body, err := c.publicGet(context.TODO(), reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get mark price for %s: %w", symbol, err)
	}

	var raw markPriceResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse mark price: %w", err)
	}
	return raw.toMarkPrice(), nil
}

// returns the latest funding rate for a symbol (public endpoint)
func (c *FuturesClient) GetFundingRate(symbol string) (*FundingRate, error) {
	reqURL := fmt.Sprintf("%s/fapi/v1/fundingRate?symbol=%s&limit=1", c.baseURL, toBinanceSymbol(symbol))

	body, err := c.publicGet(context.TODO(), reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get funding rate for %s: %w", symbol, err)
	}

	var raw []fundingRateResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse funding rate: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("no funding rate data for %s", symbol)
	}
	return raw[0].toFundingRate(), nil
}

// posts a futures order and returns the parsed response
func (c *FuturesClient) postFuturesOrder(params url.Values, apiKey, apiSecret string) (*FuturesOrder, error) {
	body, err := c.signedRawRequest(context.TODO(), http.MethodPost, "/fapi/v1/order", params, apiKey, apiSecret)
	if err != nil {
		return nil, err
	}

	var raw futuresOrderResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse futures order: %w", err)
	}
	return raw.toFuturesOrder(), nil
}

// sends a signed request and returns the raw response body
func (c *FuturesClient) signedRawRequest(ctx context.Context, method, path string, params url.Values, apiKey, apiSecret string) ([]byte, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	params.Set("timestamp", timestamp)
	queryString := params.Encode()
	signature := sign(queryString, apiSecret)

	var reqURL string
	var req *http.Request
	var err error

	if method == http.MethodPost {
		reqURL = fmt.Sprintf("%s%s", c.baseURL, path)
		bodyStr := queryString + "&signature=" + signature
		req, err = http.NewRequestWithContext(ctx, method, reqURL, strings.NewReader(bodyStr))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		reqURL = fmt.Sprintf("%s%s?%s&signature=%s", c.baseURL, path, queryString, signature)
		req, err = http.NewRequestWithContext(ctx, method, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
	}

	req.Header.Set("X-MBX-APIKEY", apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
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

	return body, nil
}

// performs a public GET request (no auth needed)
func (c *FuturesClient) publicGet(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
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

	return body, nil
}
