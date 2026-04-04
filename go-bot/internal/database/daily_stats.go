// daily stats aggregation — increments per-user daily counters for trades, pnl, decisions.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DailyStatsRecord represents one row in the daily_stats table.
type DailyStatsRecord struct {
	ID                 int
	UserID             int
	Date               time.Time
	TotalTrades        int
	WinningTrades      int
	LosingTrades       int
	RealizedPnL        float64
	UnrealizedPnL      float64
	FeesPaid           float64
	FundingPaid        float64
	AIDecisionsMade    int
	AIDecisionsApproved int
	NotificationsSent  int
}

// DailyStatsRepository handles daily stats upserts and queries.
type DailyStatsRepository struct {
	pool *pgxpool.Pool
}

func NewDailyStatsRepository(pool *pgxpool.Pool) *DailyStatsRepository {
	return &DailyStatsRepository{pool: pool}
}

// IncrementTrade increments trade counters and accumulates PnL.
// Uses UPSERT (INSERT ON CONFLICT UPDATE) to create the row if it doesn't exist yet.
func (r *DailyStatsRepository) IncrementTrade(ctx context.Context, userID int, pnl float64, fee float64, isWin bool) error {
	winInc, lossInc := 0, 0
	if isWin {
		winInc = 1
	} else {
		lossInc = 1
	}

	query := `
		INSERT INTO daily_stats (user_id, date, total_trades, winning_trades, losing_trades, realized_pnl, fees_paid)
		VALUES ($1, CURRENT_DATE, 1, $2, $3, $4, $5)
		ON CONFLICT (user_id, date) DO UPDATE SET
			total_trades = daily_stats.total_trades + 1,
			winning_trades = daily_stats.winning_trades + $2,
			losing_trades = daily_stats.losing_trades + $3,
			realized_pnl = daily_stats.realized_pnl + $4,
			fees_paid = daily_stats.fees_paid + $5,
			updated_at = NOW()`

	_, err := r.pool.Exec(ctx, query, userID, winInc, lossInc, pnl, fee)
	if err != nil {
		return fmt.Errorf("failed to increment trade stats for user %d: %w", userID, err)
	}
	return nil
}

// IncrementDecision increments the ai_decisions_made counter, and optionally ai_decisions_approved.
func (r *DailyStatsRepository) IncrementDecision(ctx context.Context, userID int, approved bool) error {
	approvedInc := 0
	if approved {
		approvedInc = 1
	}

	query := `
		INSERT INTO daily_stats (user_id, date, ai_decisions_made, ai_decisions_approved)
		VALUES ($1, CURRENT_DATE, 1, $2)
		ON CONFLICT (user_id, date) DO UPDATE SET
			ai_decisions_made = daily_stats.ai_decisions_made + 1,
			ai_decisions_approved = daily_stats.ai_decisions_approved + $2,
			updated_at = NOW()`

	_, err := r.pool.Exec(ctx, query, userID, approvedInc)
	if err != nil {
		return fmt.Errorf("failed to increment decision stats for user %d: %w", userID, err)
	}
	return nil
}

// IncrementNotification increments the notifications_sent counter.
func (r *DailyStatsRepository) IncrementNotification(ctx context.Context, userID int) error {
	query := `
		INSERT INTO daily_stats (user_id, date, notifications_sent)
		VALUES ($1, CURRENT_DATE, 1)
		ON CONFLICT (user_id, date) DO UPDATE SET
			notifications_sent = daily_stats.notifications_sent + 1,
			updated_at = NOW()`

	_, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to increment notification stats for user %d: %w", userID, err)
	}
	return nil
}

// IncrementFunding adds to the funding_paid counter.
func (r *DailyStatsRepository) IncrementFunding(ctx context.Context, userID int, amount float64) error {
	query := `
		INSERT INTO daily_stats (user_id, date, funding_paid)
		VALUES ($1, CURRENT_DATE, $2)
		ON CONFLICT (user_id, date) DO UPDATE SET
			funding_paid = daily_stats.funding_paid + $2,
			updated_at = NOW()`

	_, err := r.pool.Exec(ctx, query, userID, amount)
	if err != nil {
		return fmt.Errorf("failed to increment funding stats for user %d: %w", userID, err)
	}
	return nil
}

// GetForDate returns the daily stats for a specific user and date.
func (r *DailyStatsRepository) GetForDate(ctx context.Context, userID int, date time.Time) (*DailyStatsRecord, error) {
	query := `
		SELECT id, user_id, date, total_trades, winning_trades, losing_trades,
		       COALESCE(realized_pnl, 0), COALESCE(unrealized_pnl, 0),
		       COALESCE(fees_paid, 0), COALESCE(funding_paid, 0),
		       ai_decisions_made, ai_decisions_approved, notifications_sent
		FROM daily_stats
		WHERE user_id = $1 AND date = $2`

	d := &DailyStatsRecord{}
	err := r.pool.QueryRow(ctx, query, userID, date).Scan(
		&d.ID, &d.UserID, &d.Date,
		&d.TotalTrades, &d.WinningTrades, &d.LosingTrades,
		&d.RealizedPnL, &d.UnrealizedPnL,
		&d.FeesPaid, &d.FundingPaid,
		&d.AIDecisionsMade, &d.AIDecisionsApproved, &d.NotificationsSent,
	)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// GetRange returns daily stats for a user between two dates (inclusive).
func (r *DailyStatsRepository) GetRange(ctx context.Context, userID int, from, to time.Time) ([]*DailyStatsRecord, error) {
	query := `
		SELECT id, user_id, date, total_trades, winning_trades, losing_trades,
		       COALESCE(realized_pnl, 0), COALESCE(unrealized_pnl, 0),
		       COALESCE(fees_paid, 0), COALESCE(funding_paid, 0),
		       ai_decisions_made, ai_decisions_approved, notifications_sent
		FROM daily_stats
		WHERE user_id = $1 AND date >= $2 AND date <= $3
		ORDER BY date ASC`

	rows, err := r.pool.Query(ctx, query, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily stats: %w", err)
	}
	defer rows.Close()

	var results []*DailyStatsRecord
	for rows.Next() {
		d := &DailyStatsRecord{}
		if err := rows.Scan(
			&d.ID, &d.UserID, &d.Date,
			&d.TotalTrades, &d.WinningTrades, &d.LosingTrades,
			&d.RealizedPnL, &d.UnrealizedPnL,
			&d.FeesPaid, &d.FundingPaid,
			&d.AIDecisionsMade, &d.AIDecisionsApproved, &d.NotificationsSent,
		); err != nil {
			return nil, fmt.Errorf("failed to scan daily stats row: %w", err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}
