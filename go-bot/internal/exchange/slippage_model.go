// slippage persistence and per-symbol adaptive slippage model.
// learns from historical fill data to provide better slippage estimates.
package exchange

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SlippageStore persists slippage records to postgres
type SlippageStore struct {
	pool *pgxpool.Pool
}

// NewSlippageStore creates a new slippage store
func NewSlippageStore(pool *pgxpool.Pool) *SlippageStore {
	return &SlippageStore{pool: pool}
}

// Save persists a slippage record to the database
func (s *SlippageStore) Save(ctx context.Context, rec *SlippageRecord) error {
	query := `
		INSERT INTO slippage_records (symbol, side, expected_price, actual_price, slippage_bps, quantity, is_paper, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := s.pool.Exec(ctx, query,
		rec.Symbol, rec.Side, rec.ExpectedPrice, rec.ActualPrice,
		rec.SlippageBps, rec.Quantity, rec.IsPaper, rec.RecordedAt,
	)
	if err != nil {
		return fmt.Errorf("save slippage record: %w", err)
	}
	return nil
}

// LoadRecent loads recent slippage records for a symbol from the database
func (s *SlippageStore) LoadRecent(ctx context.Context, symbol string, limit int) ([]SlippageRecord, error) {
	query := `
		SELECT symbol, side, expected_price, actual_price, slippage_bps, quantity, is_paper, recorded_at
		FROM slippage_records
		WHERE symbol = $1 AND is_paper = FALSE
		ORDER BY recorded_at DESC
		LIMIT $2`
	rows, err := s.pool.Query(ctx, query, symbol, limit)
	if err != nil {
		return nil, fmt.Errorf("load slippage records: %w", err)
	}
	defer rows.Close()

	var records []SlippageRecord
	for rows.Next() {
		var r SlippageRecord
		err := rows.Scan(&r.Symbol, &r.Side, &r.ExpectedPrice, &r.ActualPrice,
			&r.SlippageBps, &r.Quantity, &r.IsPaper, &r.RecordedAt)
		if err != nil {
			return nil, fmt.Errorf("scan slippage record: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// SlippageModel provides per-symbol slippage estimates that improve with data.
// Uses exponentially-weighted moving average + percentile-based worst-case.
type SlippageModel struct {
	mu       sync.RWMutex
	models   map[string]*symbolModel
	store    *SlippageStore
	fallback float64 // default estimate when no data exists (bps)
}

type symbolModel struct {
	ewmaBps     float64   // exponentially weighted moving average
	p75Bps      float64   // 75th percentile slippage
	p95Bps      float64   // 95th percentile (worst case)
	sampleCount int
	lastUpdated time.Time
	records     []float64 // recent slippage values for percentile calc
}

// NewSlippageModel creates an adaptive slippage model
func NewSlippageModel(store *SlippageStore, fallbackBps float64) *SlippageModel {
	if fallbackBps <= 0 {
		fallbackBps = 5.0 // default 5 bps
	}
	return &SlippageModel{
		models:   make(map[string]*symbolModel),
		store:    store,
		fallback: fallbackBps,
	}
}

// EstimateBps returns the expected slippage in bps for a symbol.
// Uses the EWMA as the primary estimate.
func (m *SlippageModel) EstimateBps(symbol string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sm, ok := m.models[symbol]
	if !ok || sm.sampleCount < 3 {
		return m.fallback
	}
	return sm.ewmaBps
}

// WorstCaseBps returns the 95th percentile slippage for position sizing.
func (m *SlippageModel) WorstCaseBps(symbol string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sm, ok := m.models[symbol]
	if !ok || sm.sampleCount < 5 {
		return m.fallback * 3
	}
	return sm.p95Bps
}

// Update incorporates a new slippage observation
func (m *SlippageModel) Update(rec *SlippageRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sm, ok := m.models[rec.Symbol]
	if !ok {
		sm = &symbolModel{
			records: make([]float64, 0, 100),
		}
		m.models[rec.Symbol] = sm
	}

	absBps := math.Abs(rec.SlippageBps)
	sm.records = append(sm.records, absBps)
	// keep last 200 observations
	if len(sm.records) > 200 {
		sm.records = sm.records[len(sm.records)-200:]
	}
	sm.sampleCount++
	sm.lastUpdated = time.Now()

	// update EWMA (alpha = 0.1 for slow adaptation)
	alpha := 0.1
	if sm.sampleCount == 1 {
		sm.ewmaBps = absBps
	} else {
		sm.ewmaBps = alpha*absBps + (1-alpha)*sm.ewmaBps
	}

	// update percentiles
	sm.p75Bps = percentile(sm.records, 75)
	sm.p95Bps = percentile(sm.records, 95)

	// persist to DB async
	if m.store != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = m.store.Save(ctx, rec)
		}()
	}
}

// LoadHistory loads historical data from DB to warm up the model
func (m *SlippageModel) LoadHistory(ctx context.Context, symbols []string) error {
	if m.store == nil {
		return nil
	}
	for _, symbol := range symbols {
		records, err := m.store.LoadRecent(ctx, symbol, 200)
		if err != nil {
			return fmt.Errorf("load history for %s: %w", symbol, err)
		}
		for i := len(records) - 1; i >= 0; i-- {
			m.Update(&records[i])
		}
	}
	return nil
}

// Stats returns model statistics for all symbols
func (m *SlippageModel) Stats() map[string]SlippageModelStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]SlippageModelStats)
	for symbol, sm := range m.models {
		result[symbol] = SlippageModelStats{
			Symbol:      symbol,
			EWMA:        sm.ewmaBps,
			P75:         sm.p75Bps,
			P95:         sm.p95Bps,
			SampleCount: sm.sampleCount,
			LastUpdated: sm.lastUpdated,
		}
	}
	return result
}

// SlippageModelStats holds model parameters for a symbol
type SlippageModelStats struct {
	Symbol      string
	EWMA        float64
	P75         float64
	P95         float64
	SampleCount int
	LastUpdated time.Time
}

// percentile computes the pth percentile of sorted values
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	idx := p / 100 * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
