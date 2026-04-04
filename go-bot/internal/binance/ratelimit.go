// binance api rate limiter using token bucket for request weight management
package binance

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

// binance api limits: 1200 request weight per minute for spot, 2400 for futures
const (
	SpotWeightLimit    = 1200
	FuturesWeightLimit = 2400
	RefillInterval     = time.Minute
	DefaultWeight      = 1
)

// known endpoint weights (binance assigns different weights to different endpoints)
var endpointWeights = map[string]int{
	"/api/v3/account":       20,
	"/api/v3/order":         1,
	"/api/v3/openOrders":    6,
	"/api/v3/ticker/24hr":   2,
	"/api/v3/depth":         5,
	"/api/v3/klines":        2,
	"/fapi/v1/order":        1,
	"/fapi/v1/leverage":     1,
	"/fapi/v1/marginType":   1,
	"/fapi/v2/positionRisk": 5,
	"/fapi/v2/balance":      5,
	"/fapi/v1/premiumIndex": 1,
	"/fapi/v1/fundingRate":  1,
}

// RateLimiter tracks consumed weight and blocks when the budget is exhausted
type RateLimiter struct {
	mu        sync.Mutex
	tokens    int
	maxTokens int
	lastReset time.Time

	// backoff state for 429 responses
	backoffUntil time.Time
	backoffCount int
}

// NewRateLimiter creates a rate limiter with the given weight limit per minute
func NewRateLimiter(maxWeight int) *RateLimiter {
	return &RateLimiter{
		tokens:    maxWeight,
		maxTokens: maxWeight,
		lastReset: time.Now(),
	}
}

// WeightForEndpoint returns the known weight for a binance endpoint
func WeightForEndpoint(path string) int {
	if w, ok := endpointWeights[path]; ok {
		return w
	}
	return DefaultWeight
}

// Wait blocks until enough tokens are available or the context is cancelled.
// Returns an error if the context expires before tokens become available.
func (r *RateLimiter) Wait(ctx context.Context, weight int) error {
	for {
		r.mu.Lock()
		r.refillLocked()

		// check if we're in a backoff period from a 429
		if time.Now().Before(r.backoffUntil) {
			waitDur := time.Until(r.backoffUntil)
			r.mu.Unlock()
			log.Printf("[ratelimit] backing off for %v after 429 response", waitDur)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitDur):
				continue
			}
		}

		if r.tokens >= weight {
			r.tokens -= weight
			r.mu.Unlock()
			return nil
		}

		// not enough tokens — calculate how long until refill
		elapsed := time.Since(r.lastReset)
		waitDur := RefillInterval - elapsed
		if waitDur < 0 {
			waitDur = 0
		}
		r.mu.Unlock()

		if waitDur == 0 {
			continue // tokens should have been refilled, retry
		}

		log.Printf("[ratelimit] rate limit near: %d/%d weight used, waiting %v for refill", r.maxTokens-r.tokens, r.maxTokens, waitDur)
		select {
		case <-ctx.Done():
			return fmt.Errorf("rate limit wait cancelled: %w", ctx.Err())
		case <-time.After(waitDur):
			// tokens will be refilled on next iteration
		}
	}
}

// RecordResponse should be called after each API response.
// If the response was a 429, it triggers exponential backoff.
func (r *RateLimiter) RecordResponse(statusCode int) {
	if statusCode == 429 || statusCode == 418 {
		r.mu.Lock()
		defer r.mu.Unlock()

		r.backoffCount++
		// exponential backoff: 2^count seconds, max 5 minutes, with jitter
		backoffSecs := math.Min(float64(int(1)<<r.backoffCount), 300)
		jitter := rand.Float64() * backoffSecs * 0.5
		backoffDur := time.Duration(backoffSecs+jitter) * time.Second

		r.backoffUntil = time.Now().Add(backoffDur)
		r.tokens = 0 // drain remaining tokens since we hit the limit

		log.Printf("[ratelimit] received %d response, backing off for %v (attempt %d)", statusCode, backoffDur, r.backoffCount)
	} else {
		// successful request, reset backoff counter
		r.mu.Lock()
		if r.backoffCount > 0 {
			r.backoffCount = 0
		}
		r.mu.Unlock()
	}
}

// refillLocked refills tokens based on elapsed time. Must be called with mu held.
func (r *RateLimiter) refillLocked() {
	now := time.Now()
	if now.Sub(r.lastReset) >= RefillInterval {
		r.tokens = r.maxTokens
		r.lastReset = now
	}
}

// Remaining returns the current available weight tokens (for monitoring)
func (r *RateLimiter) Remaining() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refillLocked()
	return r.tokens
}
