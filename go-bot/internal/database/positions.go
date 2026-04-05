// position persistence repository for paper trading positions.
// saves/loads positions to postgres so they survive bot restarts.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PersistedPosition is a flat row representation that maps to the positions table.
// Both spot paper and leverage paper positions serialize into this.
type PersistedPosition struct {
	ID              int
	InternalID      string
	UserID          int
	Symbol          string
	Side            string  // "LONG" or "SHORT"
	Action          string  // "BUY" or "SELL" (spot paper only)
	PositionType    string  // "SPOT" or "FUTURES"
	Status          string  // "OPEN", "CLOSED"
	EntryPrice      float64
	CurrentPrice    float64
	MarkPrice       float64
	ClosePrice      float64
	Quantity        float64
	PositionSize    float64
	Margin          float64
	NotionalValue   float64
	Leverage        int
	StopLoss        float64
	TakeProfit      float64
	LiquidationPrice float64
	FundingPaid     float64
	MarginType      string
	UnrealizedPnL   float64
	RealizedPnL     float64
	IsPaper         bool
	CloseReason     string
	Platform        string
	OpenedAt        time.Time
	ClosedAt        *time.Time
}

// PositionRepository handles position CRUD for paper trading persistence.
type PositionRepository struct {
	pool *pgxpool.Pool
}

func NewPositionRepository(pool *pgxpool.Pool) *PositionRepository {
	return &PositionRepository{pool: pool}
}

// Insert saves a new position row to the database.
func (r *PositionRepository) Insert(ctx context.Context, p *PersistedPosition) error {
	query := `
		INSERT INTO positions (
			internal_id, user_id, symbol, side, action, position_type, status,
			entry_price, current_price, mark_price, quantity, position_size,
			margin, notional_value, leverage, stop_loss, take_profit,
			liquidation_price, funding_paid, margin_type,
			unrealized_pnl, realized_pnl, is_paper, platform, opened_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17,
			$18, $19, $20,
			$21, $22, $23, $24, $25
		)`

	_, err := r.pool.Exec(ctx, query,
		p.InternalID, p.UserID, p.Symbol, p.Side, nullStr(p.Action), p.PositionType, p.Status,
		p.EntryPrice, p.CurrentPrice, p.MarkPrice, p.Quantity, p.PositionSize,
		p.Margin, p.NotionalValue, p.Leverage, nullFloat(p.StopLoss), nullFloat(p.TakeProfit),
		nullFloat(p.LiquidationPrice), p.FundingPaid, p.MarginType,
		p.UnrealizedPnL, p.RealizedPnL, p.IsPaper, nullStr(p.Platform), p.OpenedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert position %s: %w", p.InternalID, err)
	}
	return nil
}

// UpdateClose marks a position as closed with final pnl data.
func (r *PositionRepository) UpdateClose(ctx context.Context, internalID string, status string, closeReason string,
	closePrice float64, realizedPnL float64, fundingPaid float64, closedAt time.Time) error {

	query := `
		UPDATE positions
		SET status = $2, close_reason = $3, close_price = $4,
			realized_pnl = $5, funding_paid = $6, closed_at = $7,
			last_updated_at = NOW()
		WHERE internal_id = $1`

	tag, err := r.pool.Exec(ctx, query, internalID, status, closeReason, closePrice, realizedPnL, fundingPaid, closedAt)
	if err != nil {
		return fmt.Errorf("failed to update close for %s: %w", internalID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("position not found in database: %s", internalID)
	}
	return nil
}

// UpdatePrice updates the current/mark price and unrealized pnl.
func (r *PositionRepository) UpdatePrice(ctx context.Context, internalID string, currentPrice, markPrice, unrealizedPnL float64) error {
	query := `
		UPDATE positions
		SET current_price = $2, mark_price = $3, unrealized_pnl = $4, last_updated_at = NOW()
		WHERE internal_id = $1 AND status = 'OPEN'`

	_, err := r.pool.Exec(ctx, query, internalID, currentPrice, markPrice, unrealizedPnL)
	return err
}

// UpdateSLTP updates stop loss and take profit on an open position.
func (r *PositionRepository) UpdateSLTP(ctx context.Context, internalID string, stopLoss, takeProfit float64) error {
	query := `
		UPDATE positions
		SET stop_loss = $2, take_profit = $3, last_updated_at = NOW()
		WHERE internal_id = $1 AND status = 'OPEN'`

	_, err := r.pool.Exec(ctx, query, internalID, nullFloat(stopLoss), nullFloat(takeProfit))
	return err
}

// LoadOpenPaperSpot loads all open paper SPOT positions for startup recovery.
func (r *PositionRepository) LoadOpenPaperSpot(ctx context.Context) ([]*PersistedPosition, error) {
	return r.loadOpen(ctx, "SPOT", true)
}

// LoadOpenPaperFutures loads all open paper FUTURES positions for startup recovery.
func (r *PositionRepository) LoadOpenPaperFutures(ctx context.Context) ([]*PersistedPosition, error) {
	return r.loadOpen(ctx, "FUTURES", true)
}

// LoadOpenLiveSpot loads all open non-paper SPOT positions for startup recovery.
func (r *PositionRepository) LoadOpenLiveSpot(ctx context.Context) ([]*PersistedPosition, error) {
	return r.loadOpen(ctx, "SPOT", false)
}

// LoadOpenLiveFutures loads all open non-paper FUTURES positions for startup recovery.
func (r *PositionRepository) LoadOpenLiveFutures(ctx context.Context) ([]*PersistedPosition, error) {
	return r.loadOpen(ctx, "FUTURES", false)
}

func (r *PositionRepository) loadOpen(ctx context.Context, posType string, isPaper bool) ([]*PersistedPosition, error) {
	query := `
		SELECT id, internal_id, user_id, symbol, side, action, position_type, status,
			   entry_price, COALESCE(current_price, 0), COALESCE(mark_price, 0),
			   quantity, COALESCE(position_size, 0),
			   COALESCE(margin, 0), COALESCE(notional_value, 0), COALESCE(leverage, 1),
			   COALESCE(stop_loss, 0), COALESCE(take_profit, 0),
			   COALESCE(liquidation_price, 0), COALESCE(funding_paid, 0),
			   COALESCE(margin_type, 'isolated'),
			   COALESCE(unrealized_pnl, 0), COALESCE(realized_pnl, 0),
			   is_paper, COALESCE(platform, ''), opened_at
		FROM positions
		WHERE is_paper = $1 AND status = 'OPEN' AND position_type = $2
		ORDER BY opened_at ASC`

	rows, err := r.pool.Query(ctx, query, isPaper, posType)
	if err != nil {
		return nil, fmt.Errorf("failed to load open %s positions: %w", posType, err)
	}
	defer rows.Close()

	var positions []*PersistedPosition
	for rows.Next() {
		p := &PersistedPosition{}
		var action *string
		err := rows.Scan(
			&p.ID, &p.InternalID, &p.UserID, &p.Symbol, &p.Side, &action, &p.PositionType, &p.Status,
			&p.EntryPrice, &p.CurrentPrice, &p.MarkPrice,
			&p.Quantity, &p.PositionSize,
			&p.Margin, &p.NotionalValue, &p.Leverage,
			&p.StopLoss, &p.TakeProfit,
			&p.LiquidationPrice, &p.FundingPaid,
			&p.MarginType,
			&p.UnrealizedPnL, &p.RealizedPnL,
			&p.IsPaper, &p.Platform, &p.OpenedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan position row: %w", err)
		}
		if action != nil {
			p.Action = *action
		}
		positions = append(positions, p)
	}

	return positions, rows.Err()
}

// MaxInternalID returns the highest numeric suffix for a given prefix (e.g. "pt_", "lp_").
// Used on startup to resume ID generation without collisions.
func (r *PositionRepository) MaxInternalID(ctx context.Context, prefix string) (int, error) {
	query := `
		SELECT COALESCE(MAX(CAST(SUBSTRING(internal_id FROM $1) AS INTEGER)), 0)
		FROM positions
		WHERE internal_id LIKE $2`

	// e.g. prefix="pt_" -> pattern="^pt_(\d+)" for regex, like="pt_%"
	pattern := fmt.Sprintf("^%s(\\d+)$", prefix)
	like := prefix + "%"

	var maxID int
	err := r.pool.QueryRow(ctx, query, pattern, like).Scan(&maxID)
	if err != nil && err != pgx.ErrNoRows {
		return 0, fmt.Errorf("failed to get max internal_id for %s: %w", prefix, err)
	}
	return maxID, nil
}

// helpers for nullable columns

func nullFloat(v float64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func nullStr(v string) interface{} {
	if v == "" {
		return nil
	}
	return v
}

// ListByUser returns positions for a user, optionally filtered by status (OPEN/CLOSED).
// Pass empty string for status to return all. Results limited to `limit` rows.
func (r *PositionRepository) ListByUser(ctx context.Context, userID int, status string, limit int) ([]*PersistedPosition, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := `
		SELECT id, internal_id, user_id, symbol, side, action, position_type, status,
			   entry_price, COALESCE(current_price, 0), COALESCE(mark_price, 0),
			   COALESCE(close_price, 0),
			   quantity, COALESCE(position_size, 0),
			   COALESCE(margin, 0), COALESCE(notional_value, 0), COALESCE(leverage, 1),
			   COALESCE(stop_loss, 0), COALESCE(take_profit, 0),
			   COALESCE(liquidation_price, 0), COALESCE(funding_paid, 0),
			   COALESCE(margin_type, 'isolated'),
			   COALESCE(unrealized_pnl, 0), COALESCE(realized_pnl, 0),
			   is_paper, COALESCE(close_reason, ''), COALESCE(platform, ''),
			   opened_at, closed_at
		FROM positions
		WHERE user_id = $1`

	args := []any{userID}
	argIdx := 2

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY opened_at DESC LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list positions for user %d: %w", userID, err)
	}
	defer rows.Close()

	return scanPositionRows(rows)
}

// GetByID returns a single position by database ID.
func (r *PositionRepository) GetByID(ctx context.Context, id int) (*PersistedPosition, error) {
	query := `
		SELECT id, internal_id, user_id, symbol, side, action, position_type, status,
			   entry_price, COALESCE(current_price, 0), COALESCE(mark_price, 0),
			   COALESCE(close_price, 0),
			   quantity, COALESCE(position_size, 0),
			   COALESCE(margin, 0), COALESCE(notional_value, 0), COALESCE(leverage, 1),
			   COALESCE(stop_loss, 0), COALESCE(take_profit, 0),
			   COALESCE(liquidation_price, 0), COALESCE(funding_paid, 0),
			   COALESCE(margin_type, 'isolated'),
			   COALESCE(unrealized_pnl, 0), COALESCE(realized_pnl, 0),
			   is_paper, COALESCE(close_reason, ''), COALESCE(platform, ''),
			   opened_at, closed_at
		FROM positions
		WHERE id = $1`

	p := &PersistedPosition{}
	var action, closeReason *string
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.InternalID, &p.UserID, &p.Symbol, &p.Side, &action, &p.PositionType, &p.Status,
		&p.EntryPrice, &p.CurrentPrice, &p.MarkPrice, &p.ClosePrice,
		&p.Quantity, &p.PositionSize,
		&p.Margin, &p.NotionalValue, &p.Leverage,
		&p.StopLoss, &p.TakeProfit,
		&p.LiquidationPrice, &p.FundingPaid, &p.MarginType,
		&p.UnrealizedPnL, &p.RealizedPnL,
		&p.IsPaper, &closeReason, &p.Platform,
		&p.OpenedAt, &p.ClosedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get position %d: %w", id, err)
	}
	if action != nil {
		p.Action = *action
	}
	if closeReason != nil {
		p.CloseReason = *closeReason
	}
	return p, nil
}

func scanPositionRows(rows pgx.Rows) ([]*PersistedPosition, error) {
	var positions []*PersistedPosition
	for rows.Next() {
		p := &PersistedPosition{}
		var action, closeReason *string
		err := rows.Scan(
			&p.ID, &p.InternalID, &p.UserID, &p.Symbol, &p.Side, &action, &p.PositionType, &p.Status,
			&p.EntryPrice, &p.CurrentPrice, &p.MarkPrice, &p.ClosePrice,
			&p.Quantity, &p.PositionSize,
			&p.Margin, &p.NotionalValue, &p.Leverage,
			&p.StopLoss, &p.TakeProfit,
			&p.LiquidationPrice, &p.FundingPaid, &p.MarginType,
			&p.UnrealizedPnL, &p.RealizedPnL,
			&p.IsPaper, &closeReason, &p.Platform,
			&p.OpenedAt, &p.ClosedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan position row: %w", err)
		}
		if action != nil {
			p.Action = *action
		}
		if closeReason != nil {
			p.CloseReason = *closeReason
		}
		positions = append(positions, p)
	}
	return positions, rows.Err()
}
