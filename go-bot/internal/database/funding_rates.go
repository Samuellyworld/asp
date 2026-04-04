// funding rate storage — persists futures funding rate history to the TimescaleDB funding_rates hypertable.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FundingRateRecord represents one funding rate observation.
type FundingRateRecord struct {
	Time      time.Time
	Symbol    string
	Rate      float64
	MarkPrice float64
}

// FundingRateRepository handles funding rate persistence.
type FundingRateRepository struct {
	pool *pgxpool.Pool
}

func NewFundingRateRepository(pool *pgxpool.Pool) *FundingRateRepository {
	return &FundingRateRepository{pool: pool}
}

// UpsertBatch inserts funding rates, updating on conflict.
func (r *FundingRateRepository) UpsertBatch(ctx context.Context, rates []*FundingRateRecord) (int, error) {
	if len(rates) == 0 {
		return 0, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO funding_rates (time, symbol, rate, mark_price)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (time, symbol) DO UPDATE SET
			rate = EXCLUDED.rate,
			mark_price = EXCLUDED.mark_price`

	inserted := 0
	for _, fr := range rates {
		_, err := tx.Exec(ctx, query, fr.Time, fr.Symbol, fr.Rate, nullFloat(fr.MarkPrice))
		if err != nil {
			return inserted, fmt.Errorf("upsert funding rate %s %s: %w", fr.Symbol, fr.Time, err)
		}
		inserted++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return inserted, nil
}

// GetRange returns funding rates for a symbol within a time range.
func (r *FundingRateRepository) GetRange(ctx context.Context, symbol string, from, to time.Time) ([]*FundingRateRecord, error) {
	query := `
		SELECT time, symbol, rate, COALESCE(mark_price, 0)
		FROM funding_rates
		WHERE symbol = $1 AND time >= $2 AND time <= $3
		ORDER BY time ASC`

	rows, err := r.pool.Query(ctx, query, symbol, from, to)
	if err != nil {
		return nil, fmt.Errorf("query funding rates: %w", err)
	}
	defer rows.Close()

	var result []*FundingRateRecord
	for rows.Next() {
		fr := &FundingRateRecord{}
		if err := rows.Scan(&fr.Time, &fr.Symbol, &fr.Rate, &fr.MarkPrice); err != nil {
			return nil, fmt.Errorf("scan funding rate: %w", err)
		}
		result = append(result, fr)
	}
	return result, rows.Err()
}

// LatestTime returns the most recent funding rate time for a symbol.
func (r *FundingRateRepository) LatestTime(ctx context.Context, symbol string) (time.Time, error) {
	var t time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(time), '1970-01-01'::timestamptz) FROM funding_rates WHERE symbol = $1`,
		symbol,
	).Scan(&t)
	if err != nil {
		return time.Time{}, fmt.Errorf("query latest funding rate: %w", err)
	}
	return t, nil
}
