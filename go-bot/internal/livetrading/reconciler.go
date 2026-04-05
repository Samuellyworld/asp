// exchange reconciliation loop. periodically verifies that the bot's
// in-memory position state matches the exchange's actual state.
// detects: partial fills, stuck orders, quantity mismatches, orphaned positions.
package livetrading

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// ReconcilerConfig controls how often and what to reconcile.
type ReconcilerConfig struct {
	Interval         time.Duration // how often to run full reconciliation (default: 5m)
	OrderCheckDelay  time.Duration // wait this long after order placement before verifying (default: 10s)
	StaleOrderAge    time.Duration // orders older than this without fill are flagged (default: 5m)
}

// DefaultReconcilerConfig returns sensible defaults.
func DefaultReconcilerConfig() ReconcilerConfig {
	return ReconcilerConfig{
		Interval:        5 * time.Minute,
		OrderCheckDelay: 10 * time.Second,
		StaleOrderAge:   5 * time.Minute,
	}
}

// Mismatch describes a discrepancy between bot state and exchange state.
type Mismatch struct {
	PositionID string
	Symbol     string
	UserID     int
	Type       string // "partial_fill", "quantity_mismatch", "stale_order", "orphaned_sl_tp"
	Expected   float64
	Actual     float64
	Details    string
	DetectedAt time.Time
}

// OnMismatchFunc is called when reconciliation finds a discrepancy.
type OnMismatchFunc func(Mismatch)

// Reconciler periodically checks in-memory positions against exchange state.
type Reconciler struct {
	executor *Executor
	orders   exchange.OrderExecutor
	keys     KeyDecryptor
	config   ReconcilerConfig
	onMismatch OnMismatchFunc

	mu           sync.Mutex
	mismatches   []Mismatch
	pendingOrders map[string]time.Time // positionID -> order placement time

	cancel  context.CancelFunc
	done    chan struct{}
	running bool
}

// NewReconciler creates a new exchange reconciler.
func NewReconciler(executor *Executor, orders exchange.OrderExecutor, keys KeyDecryptor, config ReconcilerConfig) *Reconciler {
	return &Reconciler{
		executor:      executor,
		orders:        orders,
		keys:          keys,
		config:        config,
		mismatches:    make([]Mismatch, 0),
		pendingOrders: make(map[string]time.Time),
	}
}

// SetOnMismatch registers a callback for detected mismatches.
func (r *Reconciler) SetOnMismatch(fn OnMismatchFunc) {
	r.onMismatch = fn
}

// TrackOrder records that an order was just placed for a position.
// The reconciler will wait OrderCheckDelay before verifying the fill.
func (r *Reconciler) TrackOrder(positionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingOrders[positionID] = time.Now()
}

// Mismatches returns all detected mismatches (most recent first).
func (r *Reconciler) Mismatches() []Mismatch {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Mismatch, len(r.mismatches))
	copy(result, r.mismatches)
	return result
}

// Start launches the background reconciliation loop.
func (r *Reconciler) Start(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	ctx, r.cancel = context.WithCancel(ctx)
	r.done = make(chan struct{})
	r.mu.Unlock()

	go r.run(ctx)
}

// Stop terminates the reconciliation loop.
func (r *Reconciler) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.cancel()
	r.mu.Unlock()
	<-r.done
}

func (r *Reconciler) run(ctx context.Context) {
	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
		close(r.done)
	}()

	ticker := time.NewTicker(r.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Reconcile()
		}
	}
}

// Reconcile runs a single reconciliation pass over all open positions.
// Verifies main order fills and SL/TP order status against the exchange.
func (r *Reconciler) Reconcile() {
	positions := r.executor.AllOpen()
	if len(positions) == 0 {
		return
	}

	for _, pos := range positions {
		// skip positions that were just opened (wait for fill to settle)
		r.mu.Lock()
		placedAt, isPending := r.pendingOrders[pos.ID]
		r.mu.Unlock()
		if isPending && time.Since(placedAt) < r.config.OrderCheckDelay {
			continue
		}

		// remove from pending once we've waited long enough
		if isPending {
			r.mu.Lock()
			delete(r.pendingOrders, pos.ID)
			r.mu.Unlock()
		}

		r.reconcilePosition(pos)
	}
}

func (r *Reconciler) reconcilePosition(pos *LivePosition) {
	apiKey, apiSecret, err := r.keys.DecryptKeys(pos.UserID)
	if err != nil {
		slog.Warn("reconciler: failed to decrypt keys", "position", pos.ID, "error", err)
		return
	}

	// 1. Verify main order fill status
	if pos.MainOrderID > 0 {
		mainOrder, err := r.orders.GetOrder(pos.Symbol, pos.MainOrderID, apiKey, apiSecret)
		if err != nil {
			slog.Warn("reconciler: failed to query main order", "position", pos.ID, "order", pos.MainOrderID, "error", err)
			return
		}

		// check for partial fills
		if mainOrder.Status == exchange.OrderStatusPartiallyFilled {
			m := Mismatch{
				PositionID: pos.ID,
				Symbol:     pos.Symbol,
				UserID:     pos.UserID,
				Type:       "partial_fill",
				Expected:   pos.Quantity,
				Actual:     mainOrder.ExecutedQty,
				Details:    fmt.Sprintf("main order %d partially filled: %.8f of %.8f", pos.MainOrderID, mainOrder.ExecutedQty, pos.Quantity),
				DetectedAt: time.Now(),
			}
			r.recordMismatch(m)
		}

		// check quantity mismatch (order filled but different qty than we recorded)
		if mainOrder.Status == exchange.OrderStatusFilled && mainOrder.ExecutedQty > 0 {
			diff := mainOrder.ExecutedQty - pos.Quantity
			if diff < 0 {
				diff = -diff
			}
			// tolerate tiny rounding differences (< 0.1%)
			threshold := pos.Quantity * 0.001
			if threshold < 1e-8 {
				threshold = 1e-8
			}
			if diff > threshold {
				m := Mismatch{
					PositionID: pos.ID,
					Symbol:     pos.Symbol,
					UserID:     pos.UserID,
					Type:       "quantity_mismatch",
					Expected:   pos.Quantity,
					Actual:     mainOrder.ExecutedQty,
					Details:    fmt.Sprintf("main order %d filled qty %.8f != position qty %.8f", pos.MainOrderID, mainOrder.ExecutedQty, pos.Quantity),
					DetectedAt: time.Now(),
				}
				r.recordMismatch(m)
			}
		}
	}

	// 2. Verify SL order is still active (hasn't been canceled behind our back)
	if pos.SLOrderID > 0 {
		slOrder, err := r.orders.GetOrder(pos.Symbol, pos.SLOrderID, apiKey, apiSecret)
		if err != nil {
			slog.Warn("reconciler: failed to query SL order", "position", pos.ID, "order", pos.SLOrderID, "error", err)
		} else if slOrder.Status == exchange.OrderStatusCanceled || slOrder.Status == exchange.OrderStatusExpired || slOrder.Status == exchange.OrderStatusRejected {
			m := Mismatch{
				PositionID: pos.ID,
				Symbol:     pos.Symbol,
				UserID:     pos.UserID,
				Type:       "orphaned_sl_tp",
				Details:    fmt.Sprintf("SL order %d is %s — position has no stop loss protection", pos.SLOrderID, slOrder.Status),
				DetectedAt: time.Now(),
			}
			r.recordMismatch(m)
		}
	}

	// 3. Check for stale orders (placed long ago, still not filled)
	if time.Since(pos.OpenedAt) > r.config.StaleOrderAge {
		if pos.MainOrderID > 0 {
			mainOrder, err := r.orders.GetOrder(pos.Symbol, pos.MainOrderID, apiKey, apiSecret)
			if err == nil && mainOrder.Status == exchange.OrderStatusNew {
				m := Mismatch{
					PositionID: pos.ID,
					Symbol:     pos.Symbol,
					UserID:     pos.UserID,
					Type:       "stale_order",
					Details:    fmt.Sprintf("main order %d still NEW after %s", pos.MainOrderID, time.Since(pos.OpenedAt).Round(time.Second)),
					DetectedAt: time.Now(),
				}
				r.recordMismatch(m)
			}
		}
	}
}

func (r *Reconciler) recordMismatch(m Mismatch) {
	slog.Error("reconciliation mismatch detected",
		"position", m.PositionID,
		"symbol", m.Symbol,
		"type", m.Type,
		"expected", m.Expected,
		"actual", m.Actual,
		"details", m.Details,
	)

	r.mu.Lock()
	r.mismatches = append(r.mismatches, m)
	// keep last 1000 mismatches
	if len(r.mismatches) > 1000 {
		r.mismatches = r.mismatches[len(r.mismatches)-1000:]
	}
	r.mu.Unlock()

	if r.onMismatch != nil {
		r.onMismatch(m)
	}
}
