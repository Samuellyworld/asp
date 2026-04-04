// binance websocket client. subscribes to mini-ticker streams for real-time
// price updates. maintains an in-memory price cache that satisfies the
// PriceProvider interface used by all executors and monitors.
package binance

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// real-time price cache backed by binance websocket streams.
// implements the same GetPrice(symbol) (float64, error) contract
// as the REST-based priceAdapter, but with sub-second latency.
type WSPriceCache struct {
	mu     sync.RWMutex
	prices map[string]priceEntry // map["BTCUSDT"] -> entry
	wsURL  string
	conn   *websocket.Conn
	stopCh chan struct{}
	done   chan struct{}

	// callback fired on every price update (optional)
	OnPriceUpdate func(symbol string, price float64)
}

type priceEntry struct {
	price     float64
	updatedAt time.Time
}

// binance mini-ticker websocket message
type miniTickerMsg struct {
	EventType string `json:"e"` // "24hrMiniTicker"
	Symbol    string `json:"s"` // "BTCUSDT"
	Close     string `json:"c"` // current close price
	Open      string `json:"o"`
	High      string `json:"h"`
	Low       string `json:"l"`
	Volume    string `json:"v"`
}

// NewWSPriceCache creates a websocket price cache.
// wsURL is the binance websocket base URL (e.g. "wss://stream.binance.com:9443").
func NewWSPriceCache(wsURL string) *WSPriceCache {
	return &WSPriceCache{
		prices: make(map[string]priceEntry),
		wsURL:  strings.TrimRight(wsURL, "/"),
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// SubscribeAll connects to the !miniTicker@arr stream which sends prices
// for all trading pairs. This is the simplest approach for bots that
// monitor many symbols.
func (w *WSPriceCache) SubscribeAll() error {
	url := w.wsURL + "/ws/!miniTicker@arr"
	return w.connect(url)
}

// SubscribeSymbols connects to individual mini-ticker streams for specific symbols.
// symbols should be in binance format (e.g. "btcusdt", "ethusdt").
func (w *WSPriceCache) SubscribeSymbols(symbols []string) error {
	if len(symbols) == 0 {
		return fmt.Errorf("no symbols to subscribe to")
	}

	var streams []string
	for _, s := range symbols {
		streams = append(streams, strings.ToLower(s)+"@miniTicker")
	}
	url := w.wsURL + "/stream?streams=" + strings.Join(streams, "/")
	return w.connect(url)
}

func (w *WSPriceCache) connect(url string) error {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}
	w.conn = conn

	go w.readLoop()
	return nil
}

// GetPrice returns the latest cached price for a symbol.
// symbol can be in either format: "BTC/USDT" or "BTCUSDT".
func (w *WSPriceCache) GetPrice(symbol string) (float64, error) {
	key := toBinanceSymbol(symbol)

	w.mu.RLock()
	entry, ok := w.prices[key]
	w.mu.RUnlock()

	if !ok {
		return 0, fmt.Errorf("no websocket price for %s (not yet received)", symbol)
	}

	// stale check — if no update in 5 minutes, something is wrong
	if time.Since(entry.updatedAt) > 5*time.Minute {
		return 0, fmt.Errorf("stale websocket price for %s (last update: %s ago)", symbol, time.Since(entry.updatedAt).Truncate(time.Second))
	}

	return entry.price, nil
}

// SymbolCount returns how many symbols have cached prices.
func (w *WSPriceCache) SymbolCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.prices)
}

// Stop closes the websocket connection and stops the read loop.
func (w *WSPriceCache) Stop() {
	select {
	case <-w.stopCh:
		return // already stopped
	default:
		close(w.stopCh)
	}

	if w.conn != nil {
		w.conn.Close()
	}

	// wait for read loop to finish
	<-w.done
}

func (w *WSPriceCache) readLoop() {
	defer close(w.done)

	for {
		select {
		case <-w.stopCh:
			return
		default:
		}

		_, message, err := w.conn.ReadMessage()
		if err != nil {
			select {
			case <-w.stopCh:
				return
			default:
			}

			slog.Warn("websocket read error, reconnecting", "error", err)
			w.reconnect()
			continue
		}

		w.handleMessage(message)
	}
}

func (w *WSPriceCache) handleMessage(data []byte) {
	// try array format first (from !miniTicker@arr)
	var tickers []miniTickerMsg
	if err := json.Unmarshal(data, &tickers); err == nil {
		for _, t := range tickers {
			w.updatePrice(t)
		}
		return
	}

	// try combined stream format (from /stream?streams=...)
	var combined struct {
		Stream string        `json:"stream"`
		Data   miniTickerMsg `json:"data"`
	}
	if err := json.Unmarshal(data, &combined); err == nil && combined.Data.Symbol != "" {
		w.updatePrice(combined.Data)
		return
	}

	// try single ticker format
	var single miniTickerMsg
	if err := json.Unmarshal(data, &single); err == nil && single.Symbol != "" {
		w.updatePrice(single)
	}
}

func (w *WSPriceCache) updatePrice(t miniTickerMsg) {
	price, err := strconv.ParseFloat(t.Close, 64)
	if err != nil || price <= 0 {
		return
	}

	w.mu.Lock()
	w.prices[t.Symbol] = priceEntry{price: price, updatedAt: time.Now()}
	w.mu.Unlock()

	if w.OnPriceUpdate != nil {
		w.OnPriceUpdate(t.Symbol, price)
	}
}

func (w *WSPriceCache) reconnect() {
	maxBackoff := 60 * time.Second
	backoff := time.Second

	for {
		select {
		case <-w.stopCh:
			return
		default:
		}

		time.Sleep(backoff)

		url := w.wsURL + "/ws/!miniTicker@arr"
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			slog.Warn("websocket reconnect failed", "error", err, "retry_in", backoff)
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		w.conn = conn
		slog.Info("websocket reconnected")
		return
	}
}
