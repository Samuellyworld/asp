// exchange interface and shared types for market data operations
package exchange

import (
	"context"
	"time"
)

// ticker holds current price data for a symbol
type Ticker struct {
	Symbol      string
	Price       float64
	PriceChange float64
	ChangePct   float64
	Volume      float64
	QuoteVolume float64
}

// balance holds available and locked amounts for an asset
type Balance struct {
	Asset  string
	Free   float64
	Locked float64
}

// order book entry represents a single bid or ask level
type OrderBookEntry struct {
	Price    float64
	Quantity float64
}

// order book holds the current bids and asks for a symbol
type OrderBook struct {
	Symbol string
	Bids   []OrderBookEntry
	Asks   []OrderBookEntry
}

// candle holds ohlcv data for a time period
type Candle struct {
	OpenTime  time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	CloseTime time.Time
}

// Exchange defines read-only market and account data operations.
// implementations can be swapped (binance, coinbase, mock, etc.)
type Exchange interface {
	GetPrice(ctx context.Context, symbol string) (*Ticker, error)
	GetOrderBook(ctx context.Context, symbol string, depth int) (*OrderBook, error)
	GetCandles(ctx context.Context, symbol string, interval string, limit int) ([]Candle, error)
	GetBalance(ctx context.Context, apiKey, apiSecret string) ([]Balance, error)
}
