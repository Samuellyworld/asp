// mock exchange client for testing without network calls
package exchange

import (
	"context"
	"fmt"
	"time"
)

// Mock implements Exchange with deterministic data
type Mock struct {
	Prices     map[string]*Ticker
	OrderBooks map[string]*OrderBook
	CandleData map[string][]Candle
	Balances   []Balance

	PriceErr   error
	BookErr    error
	CandleErr  error
	BalanceErr error
}

// creates a new mock exchange pre-loaded with realistic test data
func NewMock() *Mock {
	return &Mock{
		Prices:     defaultPrices(),
		OrderBooks: defaultOrderBooks(),
		CandleData: defaultCandles(),
		Balances:   defaultBalances(),
	}
}

func (m *Mock) GetPrice(_ context.Context, symbol string) (*Ticker, error) {
	if m.PriceErr != nil {
		return nil, m.PriceErr
	}
	t, ok := m.Prices[symbol]
	if !ok {
		return nil, fmt.Errorf("symbol not found: %s", symbol)
	}
	return t, nil
}

func (m *Mock) GetOrderBook(_ context.Context, symbol string, depth int) (*OrderBook, error) {
	if m.BookErr != nil {
		return nil, m.BookErr
	}
	ob, ok := m.OrderBooks[symbol]
	if !ok {
		return nil, fmt.Errorf("symbol not found: %s", symbol)
	}
	result := &OrderBook{Symbol: ob.Symbol}
	bids := ob.Bids
	asks := ob.Asks
	if depth > 0 && depth < len(bids) {
		bids = bids[:depth]
	}
	if depth > 0 && depth < len(asks) {
		asks = asks[:depth]
	}
	result.Bids = bids
	result.Asks = asks
	return result, nil
}

func (m *Mock) GetCandles(_ context.Context, symbol string, _ string, limit int) ([]Candle, error) {
	if m.CandleErr != nil {
		return nil, m.CandleErr
	}
	candles, ok := m.CandleData[symbol]
	if !ok {
		return nil, fmt.Errorf("symbol not found: %s", symbol)
	}
	if limit > 0 && limit < len(candles) {
		candles = candles[:limit]
	}
	return candles, nil
}

func (m *Mock) GetBalance(_ context.Context, apiKey, apiSecret string) ([]Balance, error) {
	if m.BalanceErr != nil {
		return nil, m.BalanceErr
	}
	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("api key and secret are required")
	}
	return m.Balances, nil
}

func defaultPrices() map[string]*Ticker {
	return map[string]*Ticker{
		"BTC/USDT":  {Symbol: "BTC/USDT", Price: 42000.00, PriceChange: -850.00, ChangePct: -1.98, Volume: 28500.5, QuoteVolume: 1197021000},
		"ETH/USDT":  {Symbol: "ETH/USDT", Price: 2280.00, PriceChange: 45.00, ChangePct: 2.01, Volume: 185000.0, QuoteVolume: 421800000},
		"BNB/USDT":  {Symbol: "BNB/USDT", Price: 310.50, PriceChange: -2.30, ChangePct: -0.74, Volume: 420000.0, QuoteVolume: 130410000},
		"SOL/USDT":  {Symbol: "SOL/USDT", Price: 98.50, PriceChange: 5.20, ChangePct: 5.58, Volume: 2800000.0, QuoteVolume: 275800000},
		"XRP/USDT":  {Symbol: "XRP/USDT", Price: 0.5850, PriceChange: -0.0120, ChangePct: -2.01, Volume: 45000000.0, QuoteVolume: 26325000},
		"ADA/USDT":  {Symbol: "ADA/USDT", Price: 0.3850, PriceChange: 0.0080, ChangePct: 2.12, Volume: 58000000.0, QuoteVolume: 22330000},
		"DOGE/USDT": {Symbol: "DOGE/USDT", Price: 0.0825, PriceChange: 0.0015, ChangePct: 1.85, Volume: 980000000.0, QuoteVolume: 80850000},
		"DOT/USDT":  {Symbol: "DOT/USDT", Price: 7.25, PriceChange: -0.15, ChangePct: -2.03, Volume: 8500000.0, QuoteVolume: 61625000},
		"AVAX/USDT": {Symbol: "AVAX/USDT", Price: 35.80, PriceChange: 1.20, ChangePct: 3.46, Volume: 3200000.0, QuoteVolume: 114560000},
		"LINK/USDT": {Symbol: "LINK/USDT", Price: 14.50, PriceChange: 0.35, ChangePct: 2.47, Volume: 12000000.0, QuoteVolume: 174000000},
	}
}

func defaultOrderBooks() map[string]*OrderBook {
	return map[string]*OrderBook{
		"BTC/USDT": {
			Symbol: "BTC/USDT",
			Bids: []OrderBookEntry{
				{Price: 41999.00, Quantity: 0.500},
				{Price: 41998.00, Quantity: 1.200},
				{Price: 41995.00, Quantity: 0.350},
				{Price: 41990.00, Quantity: 2.100},
				{Price: 41985.00, Quantity: 0.800},
			},
			Asks: []OrderBookEntry{
				{Price: 42001.00, Quantity: 0.800},
				{Price: 42002.00, Quantity: 0.450},
				{Price: 42005.00, Quantity: 1.500},
				{Price: 42010.00, Quantity: 0.250},
				{Price: 42015.00, Quantity: 1.800},
			},
		},
		"ETH/USDT": {
			Symbol: "ETH/USDT",
			Bids: []OrderBookEntry{
				{Price: 2279.00, Quantity: 5.0},
				{Price: 2278.00, Quantity: 10.0},
				{Price: 2275.00, Quantity: 3.5},
				{Price: 2270.00, Quantity: 15.0},
				{Price: 2265.00, Quantity: 8.0},
			},
			Asks: []OrderBookEntry{
				{Price: 2281.00, Quantity: 4.0},
				{Price: 2282.00, Quantity: 7.5},
				{Price: 2285.00, Quantity: 12.0},
				{Price: 2290.00, Quantity: 2.5},
				{Price: 2295.00, Quantity: 9.0},
			},
		},
	}
}

func defaultCandles() map[string][]Candle {
	now := time.Now().UTC()
	candles := make([]Candle, 10)
	for i := range candles {
		t := now.Add(-time.Duration(10-i) * time.Hour)
		candles[i] = Candle{
			OpenTime:  t,
			Open:      41800 + float64(i)*50,
			High:      41900 + float64(i)*50,
			Low:       41700 + float64(i)*50,
			Close:     41850 + float64(i)*50,
			Volume:    1500 + float64(i)*100,
			CloseTime: t.Add(time.Hour),
		}
	}
	return map[string][]Candle{
		"BTC/USDT": candles,
		"ETH/USDT": candles,
	}
}

func defaultBalances() []Balance {
	return []Balance{
		{Asset: "USDT", Free: 1000.00, Locked: 0},
		{Asset: "BTC", Free: 0.05, Locked: 0},
		{Asset: "ETH", Free: 1.5, Locked: 0.5},
	}
}
