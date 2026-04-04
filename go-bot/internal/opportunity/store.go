// package opportunity provides db persistence for trading opportunities
package opportunity

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store handles database operations for opportunities
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new opportunity store
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// OpportunityRow is the db representation of an opportunity
type OpportunityRow struct {
	ID           string
	UserID       int
	Symbol       string
	Action       string
	Status       Status
	Confidence   float64
	EntryPrice   float64
	StopLoss     float64
	TakeProfit   float64
	PositionSize float64
	RiskReward   float64
	Reasoning    string
	ModEntry     *float64
	ModSL        *float64
	ModTP        *float64
	ModSize      *float64
	UseLeverage  bool
	Leverage     int
	PositionSide string
	Platform     string
	MessageID    int
	ChannelID    string
	CreatedAt    time.Time
	ResolvedAt   *time.Time
}

// Save persists an opportunity to the database
func (s *Store) Save(ctx context.Context, row *OpportunityRow) error {
	query := `
		INSERT INTO opportunities (
			id, user_id, symbol, action, status, confidence,
			entry_price, stop_loss, take_profit, position_size, risk_reward,
			reasoning, modified_entry, modified_sl, modified_tp, modified_size,
			use_leverage, leverage, position_side, platform, message_id, channel_id,
			created_at, resolved_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16,
			$17, $18, $19, $20, $21, $22,
			$23, $24
		)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			modified_entry = EXCLUDED.modified_entry,
			modified_sl = EXCLUDED.modified_sl,
			modified_tp = EXCLUDED.modified_tp,
			modified_size = EXCLUDED.modified_size,
			use_leverage = EXCLUDED.use_leverage,
			leverage = EXCLUDED.leverage,
			position_side = EXCLUDED.position_side,
			message_id = EXCLUDED.message_id,
			channel_id = EXCLUDED.channel_id,
			resolved_at = EXCLUDED.resolved_at`

	_, err := s.pool.Exec(ctx, query,
		row.ID, row.UserID, row.Symbol, row.Action, string(row.Status), row.Confidence,
		row.EntryPrice, row.StopLoss, row.TakeProfit, row.PositionSize, row.RiskReward,
		row.Reasoning, row.ModEntry, row.ModSL, row.ModTP, row.ModSize,
		row.UseLeverage, row.Leverage, row.PositionSide, row.Platform, row.MessageID, row.ChannelID,
		row.CreatedAt, row.ResolvedAt,
	)
	if err != nil {
		return fmt.Errorf("save opportunity: %w", err)
	}
	return nil
}

// UpdateStatus updates the status and resolved_at timestamp
func (s *Store) UpdateStatus(ctx context.Context, id string, status Status, resolvedAt *time.Time) error {
	query := `UPDATE opportunities SET status = $1, resolved_at = $2 WHERE id = $3`
	tag, err := s.pool.Exec(ctx, query, string(status), resolvedAt, id)
	if err != nil {
		return fmt.Errorf("update opportunity status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("opportunity %s not found", id)
	}
	return nil
}

// GetPendingForUser retrieves all pending opportunities for a user
func (s *Store) GetPendingForUser(ctx context.Context, userID int) ([]*OpportunityRow, error) {
	query := `
		SELECT id, user_id, symbol, action, status, confidence,
			entry_price, stop_loss, take_profit, position_size, risk_reward,
			reasoning, modified_entry, modified_sl, modified_tp, modified_size,
			use_leverage, leverage, position_side, platform, message_id, channel_id,
			created_at, resolved_at
		FROM opportunities
		WHERE user_id = $1 AND status = 'pending'
		ORDER BY created_at DESC`

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query pending opportunities: %w", err)
	}
	defer rows.Close()

	var result []*OpportunityRow
	for rows.Next() {
		row := &OpportunityRow{}
		err := rows.Scan(
			&row.ID, &row.UserID, &row.Symbol, &row.Action, &row.Status, &row.Confidence,
			&row.EntryPrice, &row.StopLoss, &row.TakeProfit, &row.PositionSize, &row.RiskReward,
			&row.Reasoning, &row.ModEntry, &row.ModSL, &row.ModTP, &row.ModSize,
			&row.UseLeverage, &row.Leverage, &row.PositionSide, &row.Platform, &row.MessageID, &row.ChannelID,
			&row.CreatedAt, &row.ResolvedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan opportunity: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// GetByID retrieves a single opportunity by ID
func (s *Store) GetByID(ctx context.Context, id string) (*OpportunityRow, error) {
	query := `
		SELECT id, user_id, symbol, action, status, confidence,
			entry_price, stop_loss, take_profit, position_size, risk_reward,
			reasoning, modified_entry, modified_sl, modified_tp, modified_size,
			use_leverage, leverage, position_side, platform, message_id, channel_id,
			created_at, resolved_at
		FROM opportunities WHERE id = $1`

	row := &OpportunityRow{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&row.ID, &row.UserID, &row.Symbol, &row.Action, &row.Status, &row.Confidence,
		&row.EntryPrice, &row.StopLoss, &row.TakeProfit, &row.PositionSize, &row.RiskReward,
		&row.Reasoning, &row.ModEntry, &row.ModSL, &row.ModTP, &row.ModSize,
		&row.UseLeverage, &row.Leverage, &row.PositionSide, &row.Platform, &row.MessageID, &row.ChannelID,
		&row.CreatedAt, &row.ResolvedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get opportunity %s: %w", id, err)
	}
	return row, nil
}

// DeleteOlderThan removes resolved opportunities older than the given duration
func (s *Store) DeleteOlderThan(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	query := `DELETE FROM opportunities WHERE status != 'pending' AND resolved_at < $1`
	tag, err := s.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete old opportunities: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ExpirePending marks all pending opportunities older than the given duration as expired
func (s *Store) ExpirePending(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	now := time.Now()
	query := `UPDATE opportunities SET status = 'expired', resolved_at = $1 WHERE status = 'pending' AND created_at < $2`
	tag, err := s.pool.Exec(ctx, query, now, cutoff)
	if err != nil {
		return 0, fmt.Errorf("expire pending opportunities: %w", err)
	}
	return tag.RowsAffected(), nil
}
