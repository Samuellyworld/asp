// bybit api client for v5 spot market data, key validation, balances, and
// basic order management.
package bybit

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

const (
	defaultRecvWindow = "5000"
	spotCategory      = "spot"
)

// Client implements Bybit v5 spot exchange operations.
type Client struct {
	httpClient *http.Client
	baseURL    string
	testnet    bool
	recvWindow string
}

// NewClient creates a Bybit v5 client.
func NewClient(baseURL string, testnet bool) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    strings.TrimRight(baseURL, "/"),
		testnet:    testnet,
		recvWindow: defaultRecvWindow,
	}
}

// Name identifies this exchange implementation for the registry.
func (c *Client) Name() exchange.ExchangeName {
	return exchange.ExchangeBybit
}

type apiResponse struct {
	RetCode int             `json:"retCode"`
	RetMsg  string          `json:"retMsg"`
	Result  json.RawMessage `json:"result"`
}

type tickerResult struct {
	List []tickerItem `json:"list"`
}

type tickerItem struct {
	Symbol       string `json:"symbol"`
	LastPrice    string `json:"lastPrice"`
	PrevPrice24h string `json:"prevPrice24h"`
	Price24hPcnt string `json:"price24hPcnt"`
	Volume24h    string `json:"volume24h"`
	Turnover24h  string `json:"turnover24h"`
}

type orderBookResult struct {
	Symbol string     `json:"s"`
	Bids   [][]string `json:"b"`
	Asks   [][]string `json:"a"`
}

type klineResult struct {
	List [][]string `json:"list"`
}

type walletBalanceResult struct {
	List []walletAccount `json:"list"`
}

type walletAccount struct {
	Coin []walletCoin `json:"coin"`
}

type walletCoin struct {
	Coin          string `json:"coin"`
	WalletBalance string `json:"walletBalance"`
	Locked        string `json:"locked"`
	Free          string `json:"free"`
}

type apiKeyInfoResult struct {
	ReadOnly    int `json:"readOnly"`
	Permissions struct {
		ContractTrade []string `json:"ContractTrade"`
		Derivatives   []string `json:"Derivatives"`
		Spot          []string `json:"Spot"`
		Wallet        []string `json:"Wallet"`
		Options       []string `json:"Options"`
	} `json:"permissions"`
}

type createOrderResult struct {
	OrderID     string `json:"orderId"`
	OrderLinkID string `json:"orderLinkId"`
}

type realtimeOrderResult struct {
	List []orderItem `json:"list"`
}

type orderItem struct {
	OrderID      string `json:"orderId"`
	OrderLinkID  string `json:"orderLinkId"`
	Symbol       string `json:"symbol"`
	Side         string `json:"side"`
	OrderType    string `json:"orderType"`
	OrderStatus  string `json:"orderStatus"`
	Price        string `json:"price"`
	TriggerPrice string `json:"triggerPrice"`
	Qty          string `json:"qty"`
	CumExecQty   string `json:"cumExecQty"`
	AvgPrice     string `json:"avgPrice"`
	CreatedTime  string `json:"createdTime"`
}

// ValidateKeys tests a Bybit key using the v5 API-key information endpoint.
func (c *Client) ValidateKeys(ctx context.Context, apiKey, apiSecret string) (*exchange.APIPermissions, error) {
	body, err := c.signedRequest(ctx, http.MethodGet, "/v5/user/query-api", nil, nil, apiKey, apiSecret)
	if err != nil {
		return nil, err
	}

	var result apiKeyInfoResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse bybit api key response: %w", err)
	}

	perms := &exchange.APIPermissions{
		Withdraw: contains(result.Permissions.Wallet, "Withdraw"),
	}
	if result.ReadOnly == 0 {
		perms.Spot = contains(result.Permissions.Spot, "SpotTrade")
		perms.Futures = contains(result.Permissions.ContractTrade, "Order") ||
			contains(result.Permissions.ContractTrade, "Position") ||
			contains(result.Permissions.Derivatives, "DerivativesTrade")
	}
	return perms, nil
}

// GetPrice returns the current Bybit spot ticker for a symbol.
func (c *Client) GetPrice(ctx context.Context, symbol string) (*exchange.Ticker, error) {
	q := url.Values{}
	q.Set("category", spotCategory)
	q.Set("symbol", toBybitSymbol(symbol))

	body, err := c.publicGet(ctx, "/v5/market/tickers", q)
	if err != nil {
		return nil, fmt.Errorf("failed to get bybit price for %s: %w", symbol, err)
	}

	var result tickerResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse bybit ticker response: %w", err)
	}
	if len(result.List) == 0 {
		return nil, fmt.Errorf("bybit returned no ticker for %s", symbol)
	}

	item := result.List[0]
	price, err := parsePositiveFloat(item.LastPrice, "lastPrice")
	if err != nil {
		return nil, err
	}
	prev, _ := strconv.ParseFloat(item.PrevPrice24h, 64)
	pct, _ := strconv.ParseFloat(item.Price24hPcnt, 64)
	volume, _ := strconv.ParseFloat(item.Volume24h, 64)
	turnover, _ := strconv.ParseFloat(item.Turnover24h, 64)

	return &exchange.Ticker{
		Symbol:      symbol,
		Price:       price,
		PriceChange: price - prev,
		ChangePct:   pct * 100,
		Volume:      volume,
		QuoteVolume: turnover,
	}, nil
}

// GetOrderBook returns the current Bybit spot order book.
func (c *Client) GetOrderBook(ctx context.Context, symbol string, depth int) (*exchange.OrderBook, error) {
	if depth <= 0 || depth > 200 {
		depth = 10
	}

	q := url.Values{}
	q.Set("category", spotCategory)
	q.Set("symbol", toBybitSymbol(symbol))
	q.Set("limit", strconv.Itoa(depth))

	body, err := c.publicGet(ctx, "/v5/market/orderbook", q)
	if err != nil {
		return nil, fmt.Errorf("failed to get bybit order book for %s: %w", symbol, err)
	}

	var result orderBookResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse bybit order book response: %w", err)
	}

	book := &exchange.OrderBook{Symbol: symbol}
	book.Bids = parseBookEntries(result.Bids)
	book.Asks = parseBookEntries(result.Asks)
	return book, nil
}

// GetCandles returns Bybit spot klines in chronological order.
func (c *Client) GetCandles(ctx context.Context, symbol string, interval string, limit int) ([]exchange.Candle, error) {
	bybitInterval, ok := toBybitInterval(interval)
	if !ok {
		return nil, fmt.Errorf("invalid bybit interval: %s", interval)
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	q := url.Values{}
	q.Set("category", spotCategory)
	q.Set("symbol", toBybitSymbol(symbol))
	q.Set("interval", bybitInterval)
	q.Set("limit", strconv.Itoa(limit))

	body, err := c.publicGet(ctx, "/v5/market/kline", q)
	if err != nil {
		return nil, fmt.Errorf("failed to get bybit candles for %s: %w", symbol, err)
	}

	var result klineResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse bybit kline response: %w", err)
	}

	candles := make([]exchange.Candle, 0, len(result.List))
	for _, row := range result.List {
		candle, err := parseKline(row)
		if err != nil {
			continue
		}
		candles = append(candles, candle)
	}
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].OpenTime.Before(candles[j].OpenTime)
	})
	return candles, nil
}

// GetBalance returns non-zero Bybit unified-account wallet balances.
func (c *Client) GetBalance(ctx context.Context, apiKey, apiSecret string) ([]exchange.Balance, error) {
	q := url.Values{}
	q.Set("accountType", "UNIFIED")

	body, err := c.signedRequest(ctx, http.MethodGet, "/v5/account/wallet-balance", q, nil, apiKey, apiSecret)
	if err != nil {
		return nil, err
	}

	var result walletBalanceResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse bybit wallet response: %w", err)
	}

	var balances []exchange.Balance
	for _, account := range result.List {
		for _, coin := range account.Coin {
			total, _ := strconv.ParseFloat(coin.WalletBalance, 64)
			locked, _ := strconv.ParseFloat(coin.Locked, 64)
			free := total - locked
			if coin.Free != "" {
				if parsed, err := strconv.ParseFloat(coin.Free, 64); err == nil {
					free = parsed
				}
			}
			balances = append(balances, exchange.Balance{
				Asset:  coin.Coin,
				Free:   free,
				Locked: locked,
			})
		}
	}
	return balances, nil
}

// PlaceOrder creates a spot market or limit order on Bybit.
func (c *Client) PlaceOrder(symbol string, side exchange.OrderSide, orderType exchange.OrderType, quantity, price float64, apiKey, apiSecret string) (*exchange.Order, error) {
	if quantity <= 0 {
		return nil, fmt.Errorf("bybit spot orders require a positive base quantity")
	}

	req := map[string]any{
		"category":    spotCategory,
		"symbol":      toBybitSymbol(symbol),
		"side":        toBybitSide(side),
		"orderType":   toBybitOrderType(orderType),
		"qty":         formatFloat(quantity),
		"orderFilter": "Order",
	}
	if orderType == exchange.OrderTypeLimit {
		if price <= 0 {
			return nil, fmt.Errorf("bybit limit orders require a positive price")
		}
		req["price"] = formatFloat(price)
		req["timeInForce"] = "GTC"
	}
	if orderType == exchange.OrderTypeMarket {
		req["timeInForce"] = "IOC"
	}

	body, err := c.signedRequest(context.Background(), http.MethodPost, "/v5/order/create", nil, req, apiKey, apiSecret)
	if err != nil {
		return nil, err
	}

	order, err := parseCreateOrder(body)
	if err != nil {
		return nil, err
	}
	order.Symbol = symbol
	order.Side = side
	order.Type = orderType
	order.Status = exchange.OrderStatusNew
	order.Quantity = quantity
	order.Price = price
	return order, nil
}

// PlaceStopLoss creates a Bybit spot TP/SL order using triggerPrice.
func (c *Client) PlaceStopLoss(symbol string, side exchange.OrderSide, quantity, stopPrice, price float64, apiKey, apiSecret string) (*exchange.Order, error) {
	return c.placeTriggeredLimit(symbol, side, exchange.OrderTypeStopLoss, quantity, stopPrice, price, apiKey, apiSecret)
}

// PlaceTakeProfit creates a Bybit spot TP/SL order using triggerPrice.
func (c *Client) PlaceTakeProfit(symbol string, side exchange.OrderSide, quantity, stopPrice, price float64, apiKey, apiSecret string) (*exchange.Order, error) {
	return c.placeTriggeredLimit(symbol, side, exchange.OrderTypeTakeProfit, quantity, stopPrice, price, apiKey, apiSecret)
}

func (c *Client) placeTriggeredLimit(symbol string, side exchange.OrderSide, orderType exchange.OrderType, quantity, triggerPrice, price float64, apiKey, apiSecret string) (*exchange.Order, error) {
	if quantity <= 0 || triggerPrice <= 0 || price <= 0 {
		return nil, fmt.Errorf("bybit triggered spot orders require positive quantity, trigger price, and price")
	}
	req := map[string]any{
		"category":     spotCategory,
		"symbol":       toBybitSymbol(symbol),
		"side":         toBybitSide(side),
		"orderType":    "Limit",
		"qty":          formatFloat(quantity),
		"price":        formatFloat(price),
		"triggerPrice": formatFloat(triggerPrice),
		"timeInForce":  "GTC",
		"orderFilter":  "tpslOrder",
	}

	body, err := c.signedRequest(context.Background(), http.MethodPost, "/v5/order/create", nil, req, apiKey, apiSecret)
	if err != nil {
		return nil, err
	}

	order, err := parseCreateOrder(body)
	if err != nil {
		return nil, err
	}
	order.Symbol = symbol
	order.Side = side
	order.Type = orderType
	order.Status = exchange.OrderStatusNew
	order.Quantity = quantity
	order.Price = price
	order.StopPrice = triggerPrice
	return order, nil
}

// CancelOrder cancels an active Bybit spot order.
func (c *Client) CancelOrder(symbol string, orderID int64, apiKey, apiSecret string) error {
	req := map[string]any{
		"category": spotCategory,
		"symbol":   toBybitSymbol(symbol),
		"orderId":  strconv.FormatInt(orderID, 10),
	}
	_, err := c.signedRequest(context.Background(), http.MethodPost, "/v5/order/cancel", nil, req, apiKey, apiSecret)
	return err
}

// GetOrder returns a Bybit spot order by id.
func (c *Client) GetOrder(symbol string, orderID int64, apiKey, apiSecret string) (*exchange.Order, error) {
	q := url.Values{}
	q.Set("category", spotCategory)
	q.Set("symbol", toBybitSymbol(symbol))
	q.Set("orderId", strconv.FormatInt(orderID, 10))

	body, err := c.signedRequest(context.Background(), http.MethodGet, "/v5/order/realtime", q, nil, apiKey, apiSecret)
	if err != nil {
		return nil, err
	}

	var result realtimeOrderResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse bybit order response: %w", err)
	}
	if len(result.List) == 0 {
		return nil, fmt.Errorf("bybit order %d not found", orderID)
	}
	return result.List[0].toOrder(), nil
}

// GetOpenOrders returns active Bybit spot orders for a symbol.
func (c *Client) GetOpenOrders(symbol string, apiKey, apiSecret string) ([]exchange.Order, error) {
	q := url.Values{}
	q.Set("category", spotCategory)
	if symbol != "" {
		q.Set("symbol", toBybitSymbol(symbol))
	}

	body, err := c.signedRequest(context.Background(), http.MethodGet, "/v5/order/realtime", q, nil, apiKey, apiSecret)
	if err != nil {
		return nil, err
	}

	var result realtimeOrderResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse bybit open orders response: %w", err)
	}

	orders := make([]exchange.Order, 0, len(result.List))
	for _, item := range result.List {
		orders = append(orders, *item.toOrder())
	}
	return orders, nil
}

func (c *Client) publicGet(ctx context.Context, path string, q url.Values) ([]byte, error) {
	reqURL := c.baseURL + path
	if len(q) > 0 {
		reqURL += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create bybit request: %w", err)
	}
	return c.do(req)
}

func (c *Client) signedRequest(ctx context.Context, method, path string, q url.Values, body any, apiKey, apiSecret string) ([]byte, error) {
	if q == nil {
		q = url.Values{}
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	queryString := q.Encode()
	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to encode bybit request body: %w", err)
		}
	}

	payload := queryString
	if method != http.MethodGet {
		payload = string(bodyBytes)
	}
	signature := sign(timestamp+apiKey+c.recvWindow+payload, apiSecret)

	reqURL := c.baseURL + path
	if method == http.MethodGet && queryString != "" {
		reqURL += "?" + queryString
	}

	var reader io.Reader
	if len(bodyBytes) > 0 {
		reader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create bybit request: %w", err)
	}
	req.Header.Set("X-BAPI-API-KEY", apiKey)
	req.Header.Set("X-BAPI-TIMESTAMP", timestamp)
	req.Header.Set("X-BAPI-RECV-WINDOW", c.recvWindow)
	req.Header.Set("X-BAPI-SIGN", signature)
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bybit request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read bybit response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bybit returned status %d: %s", resp.StatusCode, string(raw))
	}

	var api apiResponse
	if err := json.Unmarshal(raw, &api); err != nil {
		return nil, fmt.Errorf("failed to parse bybit envelope: %w", err)
	}
	if api.RetCode != 0 {
		return nil, fmt.Errorf("bybit api error (code %d): %s", api.RetCode, api.RetMsg)
	}
	return api.Result, nil
}

func sign(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func toBybitSymbol(symbol string) string {
	return strings.ToUpper(strings.ReplaceAll(symbol, "/", ""))
}

func toBybitInterval(interval string) (string, bool) {
	intervals := map[string]string{
		"1m": "1", "3m": "3", "5m": "5", "15m": "15", "30m": "30",
		"1h": "60", "2h": "120", "4h": "240", "6h": "360", "12h": "720",
		"1d": "D", "1w": "W", "1M": "M",
	}
	v, ok := intervals[interval]
	return v, ok
}

func toBybitSide(side exchange.OrderSide) string {
	if side == exchange.SideSell {
		return "Sell"
	}
	return "Buy"
}

func toBybitOrderType(orderType exchange.OrderType) string {
	if orderType == exchange.OrderTypeLimit {
		return "Limit"
	}
	return "Market"
}

func fromBybitSide(side string) exchange.OrderSide {
	if strings.EqualFold(side, "Sell") {
		return exchange.SideSell
	}
	return exchange.SideBuy
}

func fromBybitOrderType(orderType string) exchange.OrderType {
	if strings.EqualFold(orderType, "Limit") {
		return exchange.OrderTypeLimit
	}
	return exchange.OrderTypeMarket
}

func fromBybitStatus(status string) exchange.OrderStatus {
	switch status {
	case "New", "Untriggered", "Triggered":
		return exchange.OrderStatusNew
	case "PartiallyFilled":
		return exchange.OrderStatusPartiallyFilled
	case "Filled":
		return exchange.OrderStatusFilled
	case "Cancelled":
		return exchange.OrderStatusCanceled
	case "Rejected", "Deactivated":
		return exchange.OrderStatusRejected
	default:
		return exchange.OrderStatus(status)
	}
}

func parseBookEntries(raw [][]string) []exchange.OrderBookEntry {
	entries := make([]exchange.OrderBookEntry, 0, len(raw))
	for _, row := range raw {
		if len(row) < 2 {
			continue
		}
		price, err := strconv.ParseFloat(row[0], 64)
		if err != nil {
			continue
		}
		qty, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			continue
		}
		entries = append(entries, exchange.OrderBookEntry{Price: price, Quantity: qty})
	}
	return entries
}

func parseKline(row []string) (exchange.Candle, error) {
	if len(row) < 6 {
		return exchange.Candle{}, fmt.Errorf("kline row too short")
	}
	openTime, err := strconv.ParseInt(row[0], 10, 64)
	if err != nil {
		return exchange.Candle{}, err
	}
	open, err := strconv.ParseFloat(row[1], 64)
	if err != nil {
		return exchange.Candle{}, err
	}
	high, err := strconv.ParseFloat(row[2], 64)
	if err != nil {
		return exchange.Candle{}, err
	}
	low, err := strconv.ParseFloat(row[3], 64)
	if err != nil {
		return exchange.Candle{}, err
	}
	closePrice, err := strconv.ParseFloat(row[4], 64)
	if err != nil {
		return exchange.Candle{}, err
	}
	volume, err := strconv.ParseFloat(row[5], 64)
	if err != nil {
		return exchange.Candle{}, err
	}
	openAt := time.UnixMilli(openTime)
	return exchange.Candle{
		OpenTime:  openAt,
		Open:      open,
		High:      high,
		Low:       low,
		Close:     closePrice,
		Volume:    volume,
		CloseTime: openAt,
	}, nil
}

func parseCreateOrder(body []byte) (*exchange.Order, error) {
	var result createOrderResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse bybit create order response: %w", err)
	}
	orderID, err := strconv.ParseInt(result.OrderID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bybit order id %q: %w", result.OrderID, err)
	}
	return &exchange.Order{OrderID: orderID, ClientOrderID: result.OrderLinkID}, nil
}

func (o orderItem) toOrder() *exchange.Order {
	orderID, _ := strconv.ParseInt(o.OrderID, 10, 64)
	price, _ := strconv.ParseFloat(o.Price, 64)
	stopPrice, _ := strconv.ParseFloat(o.TriggerPrice, 64)
	qty, _ := strconv.ParseFloat(o.Qty, 64)
	execQty, _ := strconv.ParseFloat(o.CumExecQty, 64)
	avgPrice, _ := strconv.ParseFloat(o.AvgPrice, 64)
	createdMs, _ := strconv.ParseInt(o.CreatedTime, 10, 64)

	var createdAt time.Time
	if createdMs > 0 {
		createdAt = time.UnixMilli(createdMs)
	}

	return &exchange.Order{
		OrderID:       orderID,
		ClientOrderID: o.OrderLinkID,
		Symbol:        o.Symbol,
		Side:          fromBybitSide(o.Side),
		Type:          fromBybitOrderType(o.OrderType),
		Status:        fromBybitStatus(o.OrderStatus),
		Price:         price,
		StopPrice:     stopPrice,
		Quantity:      qty,
		ExecutedQty:   execQty,
		AvgPrice:      avgPrice,
		CreatedAt:     createdAt,
	}
}

func parsePositiveFloat(value, field string) (float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse bybit %s %q: %w", field, value, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("invalid bybit %s: %f", field, parsed)
	}
	return parsed, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
