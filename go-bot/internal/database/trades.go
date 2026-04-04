// trade record persistence — logs every fill (paper and live) to the trades table.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TradeRecord is a flat row representation for the trades table.
type TradeRecord struct {
	ID              int
	UserID          int
	PositionID      *int    // nullable FK to positions
	ExchangeOrderID string  // binance order ID or "" for paper
	Symbol          string
	Side            string  // "BUY" or "SELL"
	TradeType       string  // "SPOT", "FUTURES_LONG", "FUTURES_SHORT"
	Quantity        float64
	Price           float64
	Fee             float64
	FeeCurrency     string
	IsPaper         bool
	ExecutedAt      time.Time
}

// TradeRepository handles trade record persistence.
type TradeRepository struct {
	pool *pgxpool.Pool
}

func NewTradeRepository(pool *pgxpool.Pool) *TradeRepository {
	return &TradeRepository{pool: pool}
}

// Insert writes a new trade record. Returns the auto-generated ID.
func (r *TradeRepository) Insert(ctx context.Context, t *TradeRecord) (int, error) {
	query := `
		INSERT INTO trades (
			user_id, position_id, exchange_order_id, symbol, side, trade_type,
			quantity, price, fee, fee_currency, is_paper, executed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id`

	var id int
	err := r.pool.QueryRow(ctx, query,
		t.UserID, t.PositionID, nullStr(t.ExchangeOrderID),
		t.Symbol, t.Side, t.TradeType,
		t.Quantity, t.Price, t.Fee, nullStr(t.FeeCurrency),
		t.IsPaper, t.ExecutedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to insert trade: %w", err)
	}
	return id, nil
}

// ListByPosition returns all trades for a given position internal ID.
func (r *TradeRepository) ListByPosition(ctx context.Context, positionID int) ([]*TradeRecord, error) {
	query := `
		SELECT id, user_id, position_id, COALESCE(exchange_order_id, ''), symbol, side, trade_type,
		       quantity, price, COALESCE(fee, 0), COALESCE(fee_currency, ''), is_paper, executed_at
		FROM trades
		WHERE position_id = $1
		ORDER BY executed_at ASC`

	rows, err := r.pool.Query(ctx, query, positionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list trades for position %d: %w", positionID, err)
	}
	defer rows.Close()

	var trades []*TradeRecord
	for rows.Next() {
		t := &TradeRecord{}
		var posID *int
		if err := rows.Scan(
			&t.ID, &t.UserID, &posID, &t.ExchangeOrderID, &t.Symbol, &t.Side, &t.TradeType,
			&t.Quantity, &t.Price, &t.Fee, &t.FeeCurrency, &t.IsPaper, &t.ExecutedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan trade row: %w", err)
		}
		if posID != nil {
			t.PositionID = posID
		}
		trades = append(trades, t)
	}
	return trades, rows.Err()
}

// ListByUser returns trades for a user within a date range.
func (r *TradeRepository) ListByUser(ctx context.Context, userID int, from, to time.Time) ([]*TradeRecord, error) {
	query := `
		SELECT id, user_id, position_id, COALESCE(exchange_order_id, ''), symbol, side, trade_type,
		       quantity, price, COALESCE(fee, 0), COALESCE(fee_currency, ''), is_paper, executed_at
		FROM trades
		WHERE user_id = $1 AND executed_at >= $2 AND executed_at < $3
		ORDER BY executed_at ASC`

	rows, err := r.pool.Query(ctx, query, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to list trades for user %d: %w", userID, err)
	}
	defer rows.Close()

	var trades []*TradeRecord
	for rows.Next() {
		t := &TradeRecord{}
		var posID *int
		if err := rows.Scan(
			&t.ID, &t.UserID, &posID, &t.ExchangeOrderID, &t.Symbol, &t.Side, &t.TradeType,
			&t.Quantity, &t.Price, &t.Fee, &t.FeeCurrency, &t.IsPaper, &t.ExecutedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan trade row: %w", err)
		}
		if posID != nil {
			t.PositionID = posID
		}
		trades = append(trades, t)
	}
	return trades, rows.Err()
}
