// candle storage — persists OHLCV candle data to the TimescaleDB candles hypertable.
// supports batch upserts and range queries for backtesting and analysis.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CandleRecord represents one OHLCV candle row.
type CandleRecord struct {
	Time        time.Time
	Symbol      string
	Interval    string // "1m", "5m", "15m", "1h", "4h", "1d"
	Open        float64
	High        float64
	Low         float64
	Close       float64
	Volume      float64
	QuoteVolume float64
	TradeCount  int
}

// CandleRepository handles candle persistence and retrieval.
type CandleRepository struct {
	pool *pgxpool.Pool
}

func NewCandleRepository(pool *pgxpool.Pool) *CandleRepository {
	return &CandleRepository{pool: pool}
}

// UpsertBatch inserts multiple candles, updating on conflict (same time+symbol+interval).
func (r *CandleRepository) UpsertBatch(ctx context.Context, candles []*CandleRecord) (int, error) {
	if len(candles) == 0 {
		return 0, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO candles (time, symbol, interval, open, high, low, close, volume, quote_volume, trade_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (time, symbol, interval) DO UPDATE SET
			open = EXCLUDED.open,
			high = EXCLUDED.high,
			low = EXCLUDED.low,
			close = EXCLUDED.close,
			volume = EXCLUDED.volume,
			quote_volume = EXCLUDED.quote_volume,
			trade_count = EXCLUDED.trade_count`

	inserted := 0
	for _, c := range candles {
		_, err := tx.Exec(ctx, query,
			c.Time, c.Symbol, c.Interval,
			c.Open, c.High, c.Low, c.Close,
			c.Volume, c.QuoteVolume, c.TradeCount,
		)
		if err != nil {
			return inserted, fmt.Errorf("upsert candle %s %s: %w", c.Symbol, c.Time, err)
		}
		inserted++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return inserted, nil
}

// GetRange returns candles for a symbol+interval within a time range, ordered by time ascending.
func (r *CandleRepository) GetRange(ctx context.Context, symbol, interval string, from, to time.Time) ([]*CandleRecord, error) {
	query := `
		SELECT time, symbol, interval, open, high, low, close, volume,
		       COALESCE(quote_volume, 0), COALESCE(trade_count, 0)
		FROM candles
		WHERE symbol = $1 AND interval = $2 AND time >= $3 AND time <= $4
		ORDER BY time ASC`

	rows, err := r.pool.Query(ctx, query, symbol, interval, from, to)
	if err != nil {
		return nil, fmt.Errorf("query candles: %w", err)
	}
	defer rows.Close()

	var result []*CandleRecord
	for rows.Next() {
		c := &CandleRecord{}
		if err := rows.Scan(
			&c.Time, &c.Symbol, &c.Interval,
			&c.Open, &c.High, &c.Low, &c.Close,
			&c.Volume, &c.QuoteVolume, &c.TradeCount,
		); err != nil {
			return nil, fmt.Errorf("scan candle: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// LatestTime returns the most recent candle time for a symbol+interval (or zero if none).
func (r *CandleRepository) LatestTime(ctx context.Context, symbol, interval string) (time.Time, error) {
	var t time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(time), '1970-01-01'::timestamptz) FROM candles WHERE symbol = $1 AND interval = $2`,
		symbol, interval,
	).Scan(&t)
	if err != nil {
		return time.Time{}, fmt.Errorf("query latest candle time: %w", err)
	}
	return t, nil
}

// Count returns the total candle count for a symbol+interval.
func (r *CandleRepository) Count(ctx context.Context, symbol, interval string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM candles WHERE symbol = $1 AND interval = $2`,
		symbol, interval,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count candles: %w", err)
	}
	return n, nil
}
