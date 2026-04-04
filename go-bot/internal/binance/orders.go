// binance order execution (signed requests for placing and managing orders)
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

// executes orders on binance using signed requests.
// implements exchange.OrderExecutor.
type OrderClient struct {
	httpClient *http.Client
	baseURL    string
	testnet    bool
}

func NewOrderClient(baseURL string, testnet bool) *OrderClient {
	return &OrderClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    baseURL,
		testnet:    testnet,
	}
}

// raw order response from binance POST /api/v3/order
type orderResponse struct {
	OrderID             int64          `json:"orderId"`
	ClientOrderID       string         `json:"clientOrderId"`
	Symbol              string         `json:"symbol"`
	Side                string         `json:"side"`
	Type                string         `json:"type"`
	Status              string         `json:"status"`
	Price               string         `json:"price"`
	StopPrice           string         `json:"stopPrice"`
	OrigQty             string         `json:"origQty"`
	ExecutedQty         string         `json:"executedQty"`
	CummulativeQuoteQty string         `json:"cummulativeQuoteQty"`
	TimeInForce         string         `json:"timeInForce"`
	TransactTime        int64          `json:"transactTime"`
	Fills               []fillResponse `json:"fills"`
}

type fillResponse struct {
	Price           string `json:"price"`
	Qty             string `json:"qty"`
	Commission      string `json:"commission"`
	CommissionAsset string `json:"commissionAsset"`
}

func (r *orderResponse) toOrder() *exchange.Order {
	price, _ := strconv.ParseFloat(r.Price, 64)
	stopPrice, _ := strconv.ParseFloat(r.StopPrice, 64)
	origQty, _ := strconv.ParseFloat(r.OrigQty, 64)
	execQty, _ := strconv.ParseFloat(r.ExecutedQty, 64)
	cumQuoteQty, _ := strconv.ParseFloat(r.CummulativeQuoteQty, 64)

	var avgPrice float64
	if execQty > 0 {
		avgPrice = cumQuoteQty / execQty
	}

	var fills []exchange.Fill
	for _, f := range r.Fills {
		fp, _ := strconv.ParseFloat(f.Price, 64)
		fq, _ := strconv.ParseFloat(f.Qty, 64)
		fc, _ := strconv.ParseFloat(f.Commission, 64)
		fills = append(fills, exchange.Fill{
			Price:           fp,
			Quantity:        fq,
			Commission:      fc,
			CommissionAsset: f.CommissionAsset,
		})
	}

	var createdAt time.Time
	if r.TransactTime > 0 {
		createdAt = time.UnixMilli(r.TransactTime)
	}

	return &exchange.Order{
		OrderID:       r.OrderID,
		ClientOrderID: r.ClientOrderID,
		Symbol:        r.Symbol,
		Side:          exchange.OrderSide(r.Side),
		Type:          exchange.OrderType(r.Type),
		Status:        exchange.OrderStatus(r.Status),
		Price:         price,
		StopPrice:     stopPrice,
		Quantity:      origQty,
		ExecutedQty:   execQty,
		AvgPrice:      avgPrice,
		Fills:         fills,
		CreatedAt:     createdAt,
	}
}

// places a market or limit order on binance
func (c *OrderClient) PlaceOrder(symbol string, side exchange.OrderSide, orderType exchange.OrderType, quantity, price float64, apiKey, apiSecret string) (*exchange.Order, error) {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("side", string(side))
	params.Set("type", string(orderType))

	switch orderType {
	case exchange.OrderTypeMarket:
		if quantity > 0 {
			params.Set("quantity", formatFloat(quantity))
		} else if price > 0 {
			params.Set("quoteOrderQty", formatFloat(price))
		}
	case exchange.OrderTypeLimit:
		params.Set("quantity", formatFloat(quantity))
		params.Set("price", formatFloat(price))
		params.Set("timeInForce", "GTC")
	}

	params.Set("newOrderRespType", "FULL")
	return c.postOrder(params, apiKey, apiSecret)
}

// places a stop-loss limit order
func (c *OrderClient) PlaceStopLoss(symbol string, side exchange.OrderSide, quantity, stopPrice, price float64, apiKey, apiSecret string) (*exchange.Order, error) {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("side", string(side))
	params.Set("type", string(exchange.OrderTypeStopLoss))
	params.Set("quantity", formatFloat(quantity))
	params.Set("stopPrice", formatFloat(stopPrice))
	params.Set("price", formatFloat(price))
	params.Set("timeInForce", "GTC")
	params.Set("newOrderRespType", "FULL")
	return c.postOrder(params, apiKey, apiSecret)
}

// places a take-profit limit order
func (c *OrderClient) PlaceTakeProfit(symbol string, side exchange.OrderSide, quantity, stopPrice, price float64, apiKey, apiSecret string) (*exchange.Order, error) {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("side", string(side))
	params.Set("type", string(exchange.OrderTypeTakeProfit))
	params.Set("quantity", formatFloat(quantity))
	params.Set("stopPrice", formatFloat(stopPrice))
	params.Set("price", formatFloat(price))
	params.Set("timeInForce", "GTC")
	params.Set("newOrderRespType", "FULL")
	return c.postOrder(params, apiKey, apiSecret)
}

// cancels an existing order by id
func (c *OrderClient) CancelOrder(symbol string, orderID int64, apiKey, apiSecret string) error {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("orderId", strconv.FormatInt(orderID, 10))

	_, err := c.signedRequest(http.MethodDelete, "/api/v3/order", params, apiKey, apiSecret)
	return err
}

// returns the status of a specific order
func (c *OrderClient) GetOrder(symbol string, orderID int64, apiKey, apiSecret string) (*exchange.Order, error) {
	params := url.Values{}
	params.Set("symbol", toBinanceSymbol(symbol))
	params.Set("orderId", strconv.FormatInt(orderID, 10))
	return c.signedRequest(http.MethodGet, "/api/v3/order", params, apiKey, apiSecret)
}

// returns all open orders for a symbol
func (c *OrderClient) GetOpenOrders(symbol string, apiKey, apiSecret string) ([]exchange.Order, error) {
	params := url.Values{}
	if symbol != "" {
		params.Set("symbol", toBinanceSymbol(symbol))
	}

	body, err := c.signedRawRequest(http.MethodGet, "/api/v3/openOrders", params, apiKey, apiSecret)
	if err != nil {
		return nil, err
	}

	var rawOrders []orderResponse
	if err := json.Unmarshal(body, &rawOrders); err != nil {
		return nil, fmt.Errorf("failed to parse open orders: %w", err)
	}

	orders := make([]exchange.Order, len(rawOrders))
	for i, r := range rawOrders {
		orders[i] = *r.toOrder()
	}
	return orders, nil
}

// posts an order and returns the parsed response
func (c *OrderClient) postOrder(params url.Values, apiKey, apiSecret string) (*exchange.Order, error) {
	return c.signedRequest(http.MethodPost, "/api/v3/order", params, apiKey, apiSecret)
}

// sends a signed request and parses a single order response
func (c *OrderClient) signedRequest(method, path string, params url.Values, apiKey, apiSecret string) (*exchange.Order, error) {
	body, err := c.signedRawRequest(method, path, params, apiKey, apiSecret)
	if err != nil {
		return nil, err
	}

	var raw orderResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse order response: %w", err)
	}
	return raw.toOrder(), nil
}

// sends a signed request and returns the raw response body
func (c *OrderClient) signedRawRequest(method, path string, params url.Values, apiKey, apiSecret string) ([]byte, error) {
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
		req, err = http.NewRequestWithContext(context.Background(), method, reqURL, strings.NewReader(bodyStr))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		reqURL = fmt.Sprintf("%s%s?%s&signature=%s", c.baseURL, path, queryString, signature)
		req, err = http.NewRequestWithContext(context.Background(), method, reqURL, nil)
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

// formats float without trailing zeros
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
