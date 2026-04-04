// exchange registry — manages multiple exchange implementations.
// allows runtime selection of exchange by name (e.g. per-user exchange preference).
package exchange

import (
	"context"
	"fmt"
	"sync"
)

// ExchangeName identifies a supported exchange.
type ExchangeName string

const (
	ExchangeBinance  ExchangeName = "binance"
	ExchangeCoinbase ExchangeName = "coinbase"
	ExchangeBybit    ExchangeName = "bybit"
	ExchangeOKX      ExchangeName = "okx"
	ExchangeMock     ExchangeName = "mock"
)

// FullExchange combines read-only market data + order execution.
type FullExchange interface {
	Exchange
	OrderExecutor
	Name() ExchangeName
}

// Registry holds all registered exchange implementations.
type Registry struct {
	mu        sync.RWMutex
	exchanges map[ExchangeName]FullExchange
	primary   ExchangeName // default exchange if none specified
}

// NewRegistry creates an empty exchange registry.
func NewRegistry() *Registry {
	return &Registry{
		exchanges: make(map[ExchangeName]FullExchange),
	}
}

// Register adds an exchange implementation. The first registered becomes primary.
func (r *Registry) Register(ex FullExchange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exchanges[ex.Name()] = ex
	if r.primary == "" {
		r.primary = ex.Name()
	}
}

// SetPrimary sets the default exchange used when no exchange is specified.
func (r *Registry) SetPrimary(name ExchangeName) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.exchanges[name]; !ok {
		return fmt.Errorf("exchange %q not registered", name)
	}
	r.primary = name
	return nil
}

// Get returns the exchange with the given name.
func (r *Registry) Get(name ExchangeName) (FullExchange, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ex, ok := r.exchanges[name]
	if !ok {
		return nil, fmt.Errorf("exchange %q not registered", name)
	}
	return ex, nil
}

// Primary returns the default exchange.
func (r *Registry) Primary() (FullExchange, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.primary == "" {
		return nil, fmt.Errorf("no exchanges registered")
	}
	ex, ok := r.exchanges[r.primary]
	if !ok {
		return nil, fmt.Errorf("primary exchange %q not found", r.primary)
	}
	return ex, nil
}

// Names returns all registered exchange names.
func (r *Registry) Names() []ExchangeName {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]ExchangeName, 0, len(r.exchanges))
	for name := range r.exchanges {
		names = append(names, name)
	}
	return names
}

// PriceAcross fetches a price from all registered exchanges (useful for cross-exchange comparison).
func (r *Registry) PriceAcross(ctx context.Context, symbol string) (map[ExchangeName]*Ticker, error) {
	r.mu.RLock()
	exchanges := make(map[ExchangeName]FullExchange, len(r.exchanges))
	for k, v := range r.exchanges {
		exchanges[k] = v
	}
	r.mu.RUnlock()

	type result struct {
		name   ExchangeName
		ticker *Ticker
		err    error
	}

	results := make(chan result, len(exchanges))
	for name, ex := range exchanges {
		go func(n ExchangeName, e Exchange) {
			ticker, err := e.GetPrice(ctx, symbol)
			results <- result{name: n, ticker: ticker, err: err}
		}(name, ex)
	}

	prices := make(map[ExchangeName]*Ticker)
	var lastErr error
	for range exchanges {
		r := <-results
		if r.err != nil {
			lastErr = r.err
			continue
		}
		prices[r.name] = r.ticker
	}

	if len(prices) == 0 && lastErr != nil {
		return nil, fmt.Errorf("no exchange returned a price: %w", lastErr)
	}
	return prices, nil
}
