package binance

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(1200)
	if rl.Remaining() != 1200 {
		t.Fatalf("expected 1200 tokens, got %d", rl.Remaining())
	}
}

func TestWaitConsumesTokens(t *testing.T) {
	rl := NewRateLimiter(100)
	ctx := context.Background()

	if err := rl.Wait(ctx, 30); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rl.Remaining() != 70 {
		t.Fatalf("expected 70 tokens, got %d", rl.Remaining())
	}

	if err := rl.Wait(ctx, 70); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rl.Remaining() != 0 {
		t.Fatalf("expected 0 tokens, got %d", rl.Remaining())
	}
}

func TestWaitBlocksWhenExhausted(t *testing.T) {
	rl := NewRateLimiter(10)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// consume all tokens
	if err := rl.Wait(ctx, 10); err != nil {
		t.Fatalf("first wait failed: %v", err)
	}

	// this should block and timeout
	err := rl.Wait(ctx, 1)
	if err == nil {
		t.Fatal("expected error when tokens exhausted and context expired")
	}
}

func TestWaitCancelledContext(t *testing.T) {
	rl := NewRateLimiter(10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// consume all tokens first so Wait will block
	_ = rl.Wait(context.Background(), 10)

	err := rl.Wait(ctx, 1)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestRecordResponse429TriggersBackoff(t *testing.T) {
	rl := NewRateLimiter(100)
	ctx := context.Background()

	// consume some tokens
	_ = rl.Wait(ctx, 50)

	// simulate 429 response
	rl.RecordResponse(429)

	if rl.Remaining() != 0 {
		t.Fatalf("tokens should be drained after 429, got %d", rl.Remaining())
	}

	// next wait should be delayed by backoff
	ctx2, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	err := rl.Wait(ctx2, 1)
	if err == nil {
		t.Fatal("expected timeout during backoff after 429")
	}
}

func TestRecordResponse418TriggersBackoff(t *testing.T) {
	rl := NewRateLimiter(100)
	rl.RecordResponse(418)

	if rl.Remaining() != 0 {
		t.Fatalf("tokens should be drained after 418, got %d", rl.Remaining())
	}
}

func TestRecordResponseSuccessResetsBackoff(t *testing.T) {
	rl := NewRateLimiter(100)

	// trigger backoff
	rl.RecordResponse(429)

	// reset with success
	rl.RecordResponse(200)

	if rl.backoffCount != 0 {
		t.Fatalf("expected backoff count 0 after success, got %d", rl.backoffCount)
	}
}

func TestWeightForEndpoint(t *testing.T) {
	tests := []struct {
		path     string
		expected int
	}{
		{"/api/v3/account", 20},
		{"/api/v3/order", 1},
		{"/api/v3/openOrders", 6},
		{"/api/v3/ticker/24hr", 2},
		{"/api/v3/depth", 5},
		{"/api/v3/klines", 2},
		{"/fapi/v1/order", 1},
		{"/fapi/v2/positionRisk", 5},
		{"/fapi/v2/balance", 5},
		{"/unknown/endpoint", DefaultWeight},
	}

	for _, tt := range tests {
		got := WeightForEndpoint(tt.path)
		if got != tt.expected {
			t.Errorf("WeightForEndpoint(%q) = %d, want %d", tt.path, got, tt.expected)
		}
	}
}

func TestConcurrentWait(t *testing.T) {
	rl := NewRateLimiter(100)
	ctx := context.Background()

	var wg sync.WaitGroup
	var errCount int
	var mu sync.Mutex

	// 50 goroutines each consuming 2 tokens = 100 total
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := rl.Wait(ctx, 2); err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if errCount != 0 {
		t.Fatalf("got %d errors, expected none", errCount)
	}
	if rl.Remaining() != 0 {
		t.Fatalf("expected 0 remaining, got %d", rl.Remaining())
	}
}

func TestRefillAfterInterval(t *testing.T) {
	rl := NewRateLimiter(100)
	ctx := context.Background()

	// consume all
	_ = rl.Wait(ctx, 100)
	if rl.Remaining() != 0 {
		t.Fatalf("expected 0, got %d", rl.Remaining())
	}

	// manually backdate the last reset
	rl.mu.Lock()
	rl.lastReset = time.Now().Add(-2 * RefillInterval)
	rl.mu.Unlock()

	// should be fully refilled now
	if rl.Remaining() != 100 {
		t.Fatalf("expected 100 after refill, got %d", rl.Remaining())
	}
}

func TestSetRateLimiterSharing(t *testing.T) {
	client := NewClient("http://localhost", true)
	orderClient := NewOrderClient("http://localhost", true)

	// share rate limiter
	orderClient.SetRateLimiter(client.RateLimiter())

	// consume from one
	ctx := context.Background()
	_ = client.RateLimiter().Wait(ctx, 100)

	// should see consumed tokens through the other
	if client.RateLimiter().Remaining() != orderClient.rateLimiter.Remaining() {
		t.Fatal("shared rate limiter should show same remaining tokens")
	}
}

func TestFuturesClientHasRateLimiter(t *testing.T) {
	fc := NewFuturesClient("http://localhost", true)
	if fc.rateLimiter == nil {
		t.Fatal("futures client should have a rate limiter")
	}
	if fc.rateLimiter.maxTokens != FuturesWeightLimit {
		t.Fatalf("futures rate limiter should have %d max tokens, got %d", FuturesWeightLimit, fc.rateLimiter.maxTokens)
	}
}
