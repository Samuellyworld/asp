// dead-letter queue for failed order placements.
// records orders that couldn't be placed on the exchange for manual review.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FailedOrder is a record of an order placement that failed on the exchange.
type FailedOrder struct {
	ID           int
	UserID       int
	PositionID   string
	Symbol       string
	Side         string
	OrderType    string
	Quantity     float64
	Price        float64
	StopPrice    float64
	TradeType    string
	ErrorMessage string
	Resolved     bool
	ResolvedAt   *time.Time
	ResolveNote  string
	CreatedAt    time.Time
}

// FailedOrderRepository handles persistence of failed order records.
type FailedOrderRepository struct {
	pool *pgxpool.Pool
}

// NewFailedOrderRepository creates a new repository.
func NewFailedOrderRepository(pool *pgxpool.Pool) *FailedOrderRepository {
	return &FailedOrderRepository{pool: pool}
}

// Insert records a failed order placement.
func (r *FailedOrderRepository) Insert(ctx context.Context, fo *FailedOrder) (int, error) {
	query := `
		INSERT INTO failed_orders (
			user_id, position_id, symbol, side, order_type,
			quantity, price, stop_price, trade_type, error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`

	var id int
	err := r.pool.QueryRow(ctx, query,
		fo.UserID, nullStr(fo.PositionID), fo.Symbol, fo.Side, fo.OrderType,
		fo.Quantity, fo.Price, fo.StopPrice, fo.TradeType, fo.ErrorMessage,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to insert failed order: %w", err)
	}
	return id, nil
}

// ListUnresolved returns all unresolved failed orders for a user (or all users if userID=0).
func (r *FailedOrderRepository) ListUnresolved(ctx context.Context, userID int) ([]*FailedOrder, error) {
	query := `
		SELECT id, user_id, COALESCE(position_id, ''), symbol, side, order_type,
		       quantity, price, stop_price, trade_type, error_message,
		       resolved, resolved_at, COALESCE(resolve_note, ''), created_at
		FROM failed_orders
		WHERE resolved = FALSE`

	args := []any{}
	if userID > 0 {
		query += " AND user_id = $1"
		args = append(args, userID)
	}
	query += " ORDER BY created_at DESC LIMIT 100"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list unresolved orders: %w", err)
	}
	defer rows.Close()

	var results []*FailedOrder
	for rows.Next() {
		fo := &FailedOrder{}
		if err := rows.Scan(
			&fo.ID, &fo.UserID, &fo.PositionID, &fo.Symbol, &fo.Side, &fo.OrderType,
			&fo.Quantity, &fo.Price, &fo.StopPrice, &fo.TradeType, &fo.ErrorMessage,
			&fo.Resolved, &fo.ResolvedAt, &fo.ResolveNote, &fo.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan failed order: %w", err)
		}
		results = append(results, fo)
	}
	return results, rows.Err()
}

// Resolve marks a failed order as resolved with a note.
func (r *FailedOrderRepository) Resolve(ctx context.Context, id int, note string) error {
	query := `
		UPDATE failed_orders
		SET resolved = TRUE, resolved_at = NOW(), resolve_note = $2
		WHERE id = $1 AND resolved = FALSE`

	tag, err := r.pool.Exec(ctx, query, id, note)
	if err != nil {
		return fmt.Errorf("failed to resolve order %d: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("failed order %d not found or already resolved", id)
	}
	return nil
}

// CountUnresolved returns the number of unresolved failed orders.
func (r *FailedOrderRepository) CountUnresolved(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM failed_orders WHERE resolved = FALSE").Scan(&count)
	return count, err
}
