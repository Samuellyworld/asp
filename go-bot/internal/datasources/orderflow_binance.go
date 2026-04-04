// binance order flow provider — derives order flow signals from
// binance order book depth and recent aggregated trades.
package datasources

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"encoding/json"
	"time"
)

// BinanceOrderFlow implements OrderFlowProvider using Binance REST APIs.
type BinanceOrderFlow struct {
	baseURL    string
	httpClient *http.Client
}

// NewBinanceOrderFlow creates an order flow provider for Binance.
func NewBinanceOrderFlow(baseURL string) *BinanceOrderFlow {
	return &BinanceOrderFlow{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// binanceDepthEntry is [price, quantity] from the depth endpoint
type binanceDepthEntry [2]string

// binanceDepthResponse from GET /api/v3/depth
type binanceDepthResponse struct {
	Bids []binanceDepthEntry `json:"bids"`
	Asks []binanceDepthEntry `json:"asks"`
}

// binanceAggTrade from GET /api/v3/aggTrades
type binanceAggTrade struct {
	Price    string `json:"p"`
	Quantity string `json:"q"`
	IsBuyer  bool   `json:"m"` // true = buyer is maker (i.e. seller is aggressor), inverted
	Time     int64  `json:"T"`
}

// GetSnapshot fetches order flow data from Binance depth + aggTrades endpoints.
func (b *BinanceOrderFlow) GetSnapshot(ctx context.Context, symbol string) (*OrderFlowSnapshot, error) {
	// normalize symbol "BTC/USDT" -> "BTCUSDT"
	sym := normalizeBinanceSymbol(symbol)

	// fetch depth and recent trades in parallel
	type depthResult struct {
		depth *binanceDepthResponse
		err   error
	}
	type tradesResult struct {
		trades []binanceAggTrade
		err    error
	}

	depthCh := make(chan depthResult, 1)
	tradeCh := make(chan tradesResult, 1)

	go func() {
		d, err := b.fetchDepth(ctx, sym, 20)
		depthCh <- depthResult{d, err}
	}()

	go func() {
		t, err := b.fetchAggTrades(ctx, sym, 500)
		tradeCh <- tradesResult{t, err}
	}()

	dr := <-depthCh
	tr := <-tradeCh

	if dr.err != nil {
		return nil, fmt.Errorf("depth fetch failed: %w", dr.err)
	}
	if tr.err != nil {
		return nil, fmt.Errorf("aggTrades fetch failed: %w", tr.err)
	}

	snap := &OrderFlowSnapshot{
		Symbol:    symbol,
		FetchedAt: time.Now(),
	}

	// compute order book depth
	snap.BidDepthUSD, snap.AskDepthUSD = computeDepthUSD(dr.depth)
	if snap.BidDepthUSD+snap.AskDepthUSD > 0 {
		snap.DepthImbalance = (snap.BidDepthUSD - snap.AskDepthUSD) / (snap.BidDepthUSD + snap.AskDepthUSD)
	}

	// compute spread
	if len(dr.depth.Bids) > 0 && len(dr.depth.Asks) > 0 {
		bestBid := parseFloat(dr.depth.Bids[0][0])
		bestAsk := parseFloat(dr.depth.Asks[0][0])
		if bestBid > 0 {
			snap.SpreadBps = ((bestAsk - bestBid) / bestBid) * 10000
		}
	}

	// compute buy/sell volume from aggtrades
	largeThresholdUSD := 50000.0
	for _, t := range tr.trades {
		price := parseFloat(t.Price)
		qty := parseFloat(t.Quantity)
		usdVol := price * qty

		// binance "m" field: true means buyer is maker = sell-side aggressor
		if t.IsBuyer {
			snap.SellVolume += usdVol
			if usdVol >= largeThresholdUSD {
				snap.LargeSellOrders++
			}
		} else {
			snap.BuyVolume += usdVol
			if usdVol >= largeThresholdUSD {
				snap.LargeBuyOrders++
			}
		}
	}

	if snap.SellVolume > 0 {
		snap.BuySellRatio = snap.BuyVolume / snap.SellVolume
	}

	return snap, nil
}

func (b *BinanceOrderFlow) fetchDepth(ctx context.Context, symbol string, limit int) (*binanceDepthResponse, error) {
	url := fmt.Sprintf("%s/api/v3/depth?symbol=%s&limit=%d", b.baseURL, symbol, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("depth endpoint returned %d", resp.StatusCode)
	}

	var depth binanceDepthResponse
	if err := json.NewDecoder(resp.Body).Decode(&depth); err != nil {
		return nil, fmt.Errorf("depth decode failed: %w", err)
	}
	return &depth, nil
}

func (b *BinanceOrderFlow) fetchAggTrades(ctx context.Context, symbol string, limit int) ([]binanceAggTrade, error) {
	url := fmt.Sprintf("%s/api/v3/aggTrades?symbol=%s&limit=%d", b.baseURL, symbol, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aggTrades endpoint returned %d", resp.StatusCode)
	}

	var trades []binanceAggTrade
	if err := json.NewDecoder(resp.Body).Decode(&trades); err != nil {
		return nil, fmt.Errorf("aggTrades decode failed: %w", err)
	}
	return trades, nil
}

func computeDepthUSD(depth *binanceDepthResponse) (bidUSD, askUSD float64) {
	for _, entry := range depth.Bids {
		price := parseFloat(entry[0])
		qty := parseFloat(entry[1])
		bidUSD += price * qty
	}
	for _, entry := range depth.Asks {
		price := parseFloat(entry[0])
		qty := parseFloat(entry[1])
		askUSD += price * qty
	}
	return
}

func normalizeBinanceSymbol(symbol string) string {
	result := make([]byte, 0, len(symbol))
	for i := 0; i < len(symbol); i++ {
		if symbol[i] != '/' {
			result = append(result, symbol[i])
		}
	}
	return string(result)
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// round to N decimal places (unused currently but handy)
func roundTo(val float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(val*p) / p
}
