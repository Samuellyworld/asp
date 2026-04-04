// binance market data operations (public endpoints, no auth needed)
package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// binance 24hr ticker response
type ticker24hResponse struct {
	Symbol             string `json:"symbol"`
	LastPrice          string `json:"lastPrice"`
	PriceChange        string `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quoteVolume"`
}

// binance order book response
type orderBookResponse struct {
	Bids [][]json.RawMessage `json:"bids"`
	Asks [][]json.RawMessage `json:"asks"`
}

// returns the current 24hr ticker for a symbol
func (c *Client) GetPrice(ctx context.Context, symbol string) (*exchange.Ticker, error) {
	if err := c.rateLimiter.Wait(ctx, WeightForEndpoint("/api/v3/ticker/24hr")); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	binanceSymbol := toBinanceSymbol(symbol)
	url := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", c.baseURL, binanceSymbol)

	body, err := c.doPublicGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get price for %s: %w", symbol, err)
	}

	var resp ticker24hResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse price response: %w", err)
	}

	price, err := strconv.ParseFloat(resp.LastPrice, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse last price %q: %w", resp.LastPrice, err)
	}
	if price <= 0 {
		return nil, fmt.Errorf("invalid price for %s: %f", symbol, price)
	}
	change, _ := strconv.ParseFloat(resp.PriceChange, 64)
	changePct, _ := strconv.ParseFloat(resp.PriceChangePercent, 64)
	volume, _ := strconv.ParseFloat(resp.Volume, 64)
	quoteVol, _ := strconv.ParseFloat(resp.QuoteVolume, 64)

	return &exchange.Ticker{
		Symbol:      symbol,
		Price:       price,
		PriceChange: change,
		ChangePct:   changePct,
		Volume:      volume,
		QuoteVolume: quoteVol,
	}, nil
}

// returns the current order book for a symbol
func (c *Client) GetOrderBook(ctx context.Context, symbol string, depth int) (*exchange.OrderBook, error) {
	if err := c.rateLimiter.Wait(ctx, WeightForEndpoint("/api/v3/depth")); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	binanceSymbol := toBinanceSymbol(symbol)
	if depth <= 0 || depth > 100 {
		depth = 10
	}
	url := fmt.Sprintf("%s/api/v3/depth?symbol=%s&limit=%d", c.baseURL, binanceSymbol, depth)

	body, err := c.doPublicGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get order book for %s: %w", symbol, err)
	}

	var resp orderBookResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse order book response: %w", err)
	}

	book := &exchange.OrderBook{Symbol: symbol}
	for _, bid := range resp.Bids {
		entry, err := parseBookEntry(bid)
		if err != nil {
			continue
		}
		book.Bids = append(book.Bids, entry)
	}
	for _, ask := range resp.Asks {
		entry, err := parseBookEntry(ask)
		if err != nil {
			continue
		}
		book.Asks = append(book.Asks, entry)
	}

	return book, nil
}

// returns historical kline/candlestick data for a symbol
func (c *Client) GetCandles(ctx context.Context, symbol string, interval string, limit int) ([]exchange.Candle, error) {
	if err := c.rateLimiter.Wait(ctx, WeightForEndpoint("/api/v3/klines")); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	binanceSymbol := toBinanceSymbol(symbol)
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	validIntervals := map[string]bool{
		"1m": true, "3m": true, "5m": true, "15m": true, "30m": true,
		"1h": true, "2h": true, "4h": true, "6h": true, "8h": true, "12h": true,
		"1d": true, "3d": true, "1w": true, "1M": true,
	}
	if !validIntervals[interval] {
		return nil, fmt.Errorf("invalid interval: %s", interval)
	}

	url := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=%s&limit=%d", c.baseURL, binanceSymbol, interval, limit)

	body, err := c.doPublicGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get candles for %s: %w", symbol, err)
	}

	var raw [][]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse candles response: %w", err)
	}

	candles := make([]exchange.Candle, 0, len(raw))
	for _, row := range raw {
		if len(row) < 7 {
			continue
		}
		candle, err := parseCandle(row)
		if err != nil {
			continue
		}
		candles = append(candles, candle)
	}

	return candles, nil
}

// performs a public GET request (no auth headers)
func (c *Client) doPublicGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
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

// converts "BTC/USDT" to "BTCUSDT" for binance api
func toBinanceSymbol(symbol string) string {
	return strings.ReplaceAll(symbol, "/", "")
}

// parses a [price, quantity] order book entry
func parseBookEntry(raw []json.RawMessage) (exchange.OrderBookEntry, error) {
	if len(raw) < 2 {
		return exchange.OrderBookEntry{}, fmt.Errorf("invalid entry")
	}

	var priceStr, qtyStr string
	if err := json.Unmarshal(raw[0], &priceStr); err != nil {
		return exchange.OrderBookEntry{}, err
	}
	if err := json.Unmarshal(raw[1], &qtyStr); err != nil {
		return exchange.OrderBookEntry{}, err
	}

	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return exchange.OrderBookEntry{}, err
	}
	qty, err := strconv.ParseFloat(qtyStr, 64)
	if err != nil {
		return exchange.OrderBookEntry{}, err
	}

	return exchange.OrderBookEntry{Price: price, Quantity: qty}, nil
}

// parses a kline array from binance into a Candle
func parseCandle(row []json.RawMessage) (exchange.Candle, error) {
	if len(row) < 7 {
		return exchange.Candle{}, fmt.Errorf("candle row too short: got %d elements, need 7", len(row))
	}

	var openTimeMs, closeTimeMs float64
	var openStr, highStr, lowStr, closeStr, volStr string

	if err := json.Unmarshal(row[0], &openTimeMs); err != nil {
		return exchange.Candle{}, err
	}
	if err := json.Unmarshal(row[1], &openStr); err != nil {
		return exchange.Candle{}, err
	}
	if err := json.Unmarshal(row[2], &highStr); err != nil {
		return exchange.Candle{}, err
	}
	if err := json.Unmarshal(row[3], &lowStr); err != nil {
		return exchange.Candle{}, err
	}
	if err := json.Unmarshal(row[4], &closeStr); err != nil {
		return exchange.Candle{}, err
	}
	if err := json.Unmarshal(row[5], &volStr); err != nil {
		return exchange.Candle{}, err
	}
	if err := json.Unmarshal(row[6], &closeTimeMs); err != nil {
		return exchange.Candle{}, err
	}

	open, _ := strconv.ParseFloat(openStr, 64)
	high, _ := strconv.ParseFloat(highStr, 64)
	low, _ := strconv.ParseFloat(lowStr, 64)
	cl, _ := strconv.ParseFloat(closeStr, 64)
	vol, _ := strconv.ParseFloat(volStr, 64)

	return exchange.Candle{
		OpenTime:  time.UnixMilli(int64(openTimeMs)),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     cl,
		Volume:    vol,
		CloseTime: time.UnixMilli(int64(closeTimeMs)),
	}, nil
}
