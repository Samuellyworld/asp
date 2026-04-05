// database circuit breaker — tracks DB failures and degrades gracefully
// when postgres is unreachable. prevents cascading failures.
package database

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DBState represents the current circuit breaker state.
type DBState int

const (
	DBStateClosed   DBState = iota // healthy, all requests pass through
	DBStateOpen                    // unhealthy, requests fail fast
	DBStateHalfOpen                // testing, limited requests pass through
)

// DBCircuitBreaker wraps a pgxpool.Pool and tracks consecutive failures.
// When failures exceed the threshold, it opens the circuit and fails fast.
type DBCircuitBreaker struct {
	pool *pgxpool.Pool

	mu              sync.RWMutex
	state           DBState
	failures        int
	lastFailure     time.Time
	lastSuccess     time.Time
	failureThreshold int
	resetTimeout    time.Duration
}

// NewDBCircuitBreaker wraps a connection pool with circuit breaker logic.
func NewDBCircuitBreaker(pool *pgxpool.Pool, failureThreshold int, resetTimeout time.Duration) *DBCircuitBreaker {
	return &DBCircuitBreaker{
		pool:             pool,
		state:            DBStateClosed,
		failureThreshold: failureThreshold,
		resetTimeout:     resetTimeout,
	}
}

// Pool returns the underlying pool. Use this for direct access when circuit state
// is already checked.
func (cb *DBCircuitBreaker) Pool() *pgxpool.Pool {
	return cb.pool
}

// IsHealthy returns true if the DB is considered available.
func (cb *DBCircuitBreaker) IsHealthy() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == DBStateClosed
}

// State returns the current circuit breaker state.
func (cb *DBCircuitBreaker) State() DBState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Allow checks if a request should be allowed through.
// Returns false if the circuit is open and the reset timeout hasn't elapsed.
func (cb *DBCircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case DBStateClosed:
		return true
	case DBStateOpen:
		if time.Since(cb.lastFailure) >= cb.resetTimeout {
			cb.state = DBStateHalfOpen
			slog.Info("db circuit breaker entering half-open state")
			return true
		}
		return false
	case DBStateHalfOpen:
		return true
	default:
		return true
	}
}

// RecordSuccess records a successful DB operation.
func (cb *DBCircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastSuccess = time.Now()
	if cb.state == DBStateHalfOpen {
		cb.state = DBStateClosed
		cb.failures = 0
		slog.Info("db circuit breaker closed (recovered)")
	}
	cb.failures = 0
}

// RecordFailure records a failed DB operation.
func (cb *DBCircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.failureThreshold && cb.state == DBStateClosed {
		cb.state = DBStateOpen
		slog.Error("db circuit breaker opened — DB operations will fail fast",
			"consecutive_failures", cb.failures,
			"threshold", cb.failureThreshold,
			"reset_timeout", cb.resetTimeout,
		)
	}

	if cb.state == DBStateHalfOpen {
		cb.state = DBStateOpen
		slog.Warn("db circuit breaker re-opened (half-open probe failed)")
	}
}

// Exec wraps pool.Exec with circuit breaker logic. Returns error immediately if circuit is open.
func (cb *DBCircuitBreaker) Exec(ctx context.Context, sql string, args ...any) error {
	if !cb.Allow() {
		return ErrCircuitOpen
	}

	_, err := cb.pool.Exec(ctx, sql, args...)
	if err != nil {
		cb.RecordFailure()
		return err
	}
	cb.RecordSuccess()
	return nil
}

// Ping checks DB connectivity through the circuit breaker.
func (cb *DBCircuitBreaker) Ping(ctx context.Context) error {
	if !cb.Allow() {
		return ErrCircuitOpen
	}

	err := cb.pool.Ping(ctx)
	if err != nil {
		cb.RecordFailure()
		return err
	}
	cb.RecordSuccess()
	return nil
}

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = &CircuitOpenError{}

// CircuitOpenError indicates the DB circuit breaker is open.
type CircuitOpenError struct{}

func (e *CircuitOpenError) Error() string {
	return "database circuit breaker is open — DB unavailable"
}
