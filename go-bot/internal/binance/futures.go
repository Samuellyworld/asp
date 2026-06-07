// binance usdt-m futures api client (signed and public endpoints)
package binance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
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
	httpClient  *http.Client
	baseURL     string
	testnet     bool
	rateLimiter *RateLimiter
	filterMu    sync.RWMutex
	filters     map[string]futuresSymbolFilter
}

type futuresSymbolFilter struct {
	QuantityStep      float64
	QuantityPrecision int
	PriceTick         float64
	PricePrecision    int
	MinQuantity       float64
}

type futuresExchangeInfoResponse struct {
	Symbols []futuresExchangeSymbol `json:"symbols"`
}

type futuresExchangeSymbol struct {
	Symbol  string                `json:"symbol"`
	Filters []futuresFilterObject `json:"filters"`
}

type futuresFilterObject struct {
	FilterType string `json:"filterType"`
	TickSize   string `json:"tickSize"`
	StepSize   string `json:"stepSize"`
	MinQty     string `json:"minQty"`
}

// creates a new futures client
func NewFuturesClient(baseURL string, testnet bool) *FuturesClient {
	return &FuturesClient{
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		baseURL:     baseURL,
		testnet:     testnet,
		rateLimiter: NewRateLimiter(FuturesWeightLimit),
		filters:     make(map[string]futuresSymbolFilter),
	}
}

// SetRateLimiter allows sharing a rate limiter across futures clients
func (c *FuturesClient) SetRateLimiter(rl *RateLimiter) {
	c.rateLimiter = rl
}

// sets the leverage for a symbol
func (c *FuturesClient) SetLeverage(ctx context.Context, symbol string, leverage int, apiKey, apiSecret string) error {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("leverage", strconv.Itoa(leverage))

	_, err := c.signedRawRequest(ctx, http.MethodPost, "/fapi/v1/leverage", params, apiKey, apiSecret)
	return err
}

// sets the margin type (isolated or cross) for a symbol
func (c *FuturesClient) SetMarginType(ctx context.Context, symbol string, marginType string, apiKey, apiSecret string) error {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("marginType", marginType)

	_, err := c.signedRawRequest(ctx, http.MethodPost, "/fapi/v1/marginType", params, apiKey, apiSecret)
	var apiErr *apiError
	if err != nil && errors.As(err, &apiErr) && apiErr.Code == -4046 {
		return nil
	}
	return err
}

// places a futures order (market or limit)
func (c *FuturesClient) PlaceOrder(ctx context.Context, symbol string, side exchange.OrderSide, orderType exchange.OrderType, quantity, price float64, apiKey, apiSecret string) (*FuturesOrder, error) {
	filter, hasFilter := c.getSymbolFilter(ctx, symbol)
	if hasFilter {
		quantity = floorToStep(quantity, filter.QuantityStep)
		if price > 0 {
			price = roundToStep(price, filter.PriceTick)
		}
		if filter.MinQuantity > 0 && quantity < filter.MinQuantity {
			return nil, fmt.Errorf("quantity %.8f below futures minimum %.8f for %s", quantity, filter.MinQuantity, symbol)
		}
	}

	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("side", string(side))
	params.Set("type", string(orderType))
	if hasFilter {
		params.Set("quantity", formatFloatWithMaxPrecision(quantity, filter.QuantityPrecision))
	} else {
		params.Set("quantity", formatFloat(quantity))
	}

	if orderType == exchange.OrderTypeLimit {
		if hasFilter {
			params.Set("price", formatFloatWithMaxPrecision(price, filter.PricePrecision))
		} else {
			params.Set("price", formatFloat(price))
		}
		params.Set("timeInForce", "GTC")
	}

	return c.postFuturesOrder(ctx, params, apiKey, apiSecret)
}

// places a stop market order for futures
func (c *FuturesClient) PlaceStopMarket(ctx context.Context, symbol string, side exchange.OrderSide, quantity, stopPrice float64, apiKey, apiSecret string) (*FuturesOrder, error) {
	filter, hasFilter := c.getSymbolFilter(ctx, symbol)
	if hasFilter {
		quantity = floorToStep(quantity, filter.QuantityStep)
		stopPrice = roundToStep(stopPrice, filter.PriceTick)
		if filter.MinQuantity > 0 && quantity < filter.MinQuantity {
			return nil, fmt.Errorf("quantity %.8f below futures minimum %.8f for %s", quantity, filter.MinQuantity, symbol)
		}
	}

	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("side", string(side))
	params.Set("type", "STOP_MARKET")
	if hasFilter {
		params.Set("quantity", formatFloatWithMaxPrecision(quantity, filter.QuantityPrecision))
		params.Set("stopPrice", formatFloatWithMaxPrecision(stopPrice, filter.PricePrecision))
	} else {
		params.Set("quantity", formatFloat(quantity))
		params.Set("stopPrice", formatFloat(stopPrice))
	}

	return c.postFuturesOrder(ctx, params, apiKey, apiSecret)
}

// places a take profit market order for futures
func (c *FuturesClient) PlaceTakeProfitMarket(ctx context.Context, symbol string, side exchange.OrderSide, quantity, stopPrice float64, apiKey, apiSecret string) (*FuturesOrder, error) {
	filter, hasFilter := c.getSymbolFilter(ctx, symbol)
	if hasFilter {
		quantity = floorToStep(quantity, filter.QuantityStep)
		stopPrice = roundToStep(stopPrice, filter.PriceTick)
		if filter.MinQuantity > 0 && quantity < filter.MinQuantity {
			return nil, fmt.Errorf("quantity %.8f below futures minimum %.8f for %s", quantity, filter.MinQuantity, symbol)
		}
	}

	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("side", string(side))
	params.Set("type", "TAKE_PROFIT_MARKET")
	if hasFilter {
		params.Set("quantity", formatFloatWithMaxPrecision(quantity, filter.QuantityPrecision))
		params.Set("stopPrice", formatFloatWithMaxPrecision(stopPrice, filter.PricePrecision))
	} else {
		params.Set("quantity", formatFloat(quantity))
		params.Set("stopPrice", formatFloat(stopPrice))
	}

	return c.postFuturesOrder(ctx, params, apiKey, apiSecret)
}

// cancels an existing futures order by id
func (c *FuturesClient) CancelOrder(ctx context.Context, symbol string, orderID int64, apiKey, apiSecret string) error {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("orderId", strconv.FormatInt(orderID, 10))

	_, err := c.signedRawRequest(ctx, http.MethodDelete, "/fapi/v1/order", params, apiKey, apiSecret)
	return err
}

// returns the status of a specific futures order
func (c *FuturesClient) GetOrder(ctx context.Context, symbol string, orderID int64, apiKey, apiSecret string) (*FuturesOrder, error) {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("orderId", strconv.FormatInt(orderID, 10))

	body, err := c.signedRawRequest(ctx, http.MethodGet, "/fapi/v1/order", params, apiKey, apiSecret)
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
func (c *FuturesClient) GetPositions(ctx context.Context, apiKey, apiSecret string) ([]FuturesPosition, error) {
	params := url.Values{}

	body, err := c.signedRawRequest(ctx, http.MethodGet, "/fapi/v2/positionRisk", params, apiKey, apiSecret)
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
func (c *FuturesClient) GetFuturesBalance(ctx context.Context, apiKey, apiSecret string) ([]FuturesBalance, error) {
	params := url.Values{}

	body, err := c.signedRawRequest(ctx, http.MethodGet, "/fapi/v2/balance", params, apiKey, apiSecret)
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
func (c *FuturesClient) GetMarkPrice(ctx context.Context, symbol string) (*MarkPrice, error) {
	reqURL := fmt.Sprintf("%s/fapi/v1/premiumIndex?symbol=%s", c.baseURL, toBinanceSymbol(symbol))

	body, err := c.publicGet(ctx, reqURL)
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
func (c *FuturesClient) GetFundingRate(ctx context.Context, symbol string) (*FundingRate, error) {
	reqURL := fmt.Sprintf("%s/fapi/v1/fundingRate?symbol=%s&limit=1", c.baseURL, toBinanceSymbol(symbol))

	body, err := c.publicGet(ctx, reqURL)
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

func (c *FuturesClient) getSymbolFilter(ctx context.Context, symbol string) (futuresSymbolFilter, bool) {
	binanceSymbol := toBinanceSymbol(symbol)

	c.filterMu.RLock()
	filter, ok := c.filters[binanceSymbol]
	c.filterMu.RUnlock()
	if ok {
		return filter, true
	}

	reqURL := fmt.Sprintf("%s/fapi/v1/exchangeInfo?symbol=%s", c.baseURL, binanceSymbol)
	body, err := c.publicGet(ctx, reqURL)
	if err != nil {
		return futuresSymbolFilter{}, false
	}

	var raw futuresExchangeInfoResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return futuresSymbolFilter{}, false
	}

	for _, s := range raw.Symbols {
		if s.Symbol != binanceSymbol {
			continue
		}
		var parsed futuresSymbolFilter
		for _, f := range s.Filters {
			switch f.FilterType {
			case "LOT_SIZE":
				parsed.QuantityStep, _ = strconv.ParseFloat(f.StepSize, 64)
				parsed.QuantityPrecision = stepPrecision(f.StepSize)
				parsed.MinQuantity, _ = strconv.ParseFloat(f.MinQty, 64)
			case "PRICE_FILTER":
				parsed.PriceTick, _ = strconv.ParseFloat(f.TickSize, 64)
				parsed.PricePrecision = stepPrecision(f.TickSize)
			}
		}
		if parsed.QuantityStep <= 0 && parsed.PriceTick <= 0 {
			return futuresSymbolFilter{}, false
		}
		c.filterMu.Lock()
		c.filters[binanceSymbol] = parsed
		c.filterMu.Unlock()
		return parsed, true
	}

	return futuresSymbolFilter{}, false
}

func floorToStep(value, step float64) float64 {
	if value <= 0 || step <= 0 {
		return value
	}
	return math.Floor((value/step)+1e-9) * step
}

func roundToStep(value, step float64) float64 {
	if value <= 0 || step <= 0 {
		return value
	}
	return math.Round(value/step) * step
}

func stepPrecision(step string) int {
	step = strings.TrimSpace(step)
	if dot := strings.IndexByte(step, '.'); dot >= 0 {
		frac := strings.TrimRight(step[dot+1:], "0")
		return len(frac)
	}
	return 0
}

func formatFloatWithMaxPrecision(v float64, precision int) string {
	if precision < 0 {
		return formatFloat(v)
	}
	s := strconv.FormatFloat(v, 'f', precision, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "-0" {
		return "0"
	}
	return s
}

// posts a futures order and returns the parsed response
func (c *FuturesClient) postFuturesOrder(ctx context.Context, params url.Values, apiKey, apiSecret string) (*FuturesOrder, error) {
	body, err := c.signedRawRequest(ctx, http.MethodPost, "/fapi/v1/order", params, apiKey, apiSecret)
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
	if err := c.rateLimiter.Wait(ctx, WeightForEndpoint(path)); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

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
	defer func() { _ = resp.Body.Close() }()
	c.rateLimiter.RecordResponse(resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr apiError
		if json.Unmarshal(body, &apiErr) == nil {
			return nil, &apiErr
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
	defer func() { _ = resp.Body.Close() }()
	c.rateLimiter.RecordResponse(resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr apiError
		if json.Unmarshal(body, &apiErr) == nil {
			return nil, &apiErr
		}
		return nil, fmt.Errorf("binance api returned status %d", resp.StatusCode)
	}

	return body, nil
}
