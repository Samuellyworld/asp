// preferences database operations for scanning, notifications, and trading
package preferences

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// represents a row in scanning_preferences
type Scanning struct {
	UserID             int
	MinConfidence      int
	ScanIntervalMins   int
	EnabledTimeframes  []string
	EnabledIndicators  []string
	UseMLPredictions   bool
	UseSentiment       bool
	IsScanningEnabled  bool
}

//  represents a row in notification_preferences
type Notification struct {
	UserID                    int
	MaxDailyNotifications     int
	OpportunityNotifications  bool
	TradeExecutedNotifications bool
	MilestoneNotifications    bool
	PeriodicUpdateMinutes     int
	DailySummaryEnabled       bool
	DailySummaryHour          int
	Timezone                  string
}

//  represents a row in trading_preferences
type Trading struct {
	UserID               int
	DefaultPositionSize  float64
	MaxPositionSize      float64
	MaxOpenPositions     int
	DailyLossLimit       float64
	DefaultStopLossPct   float64
	DefaultTakeProfitPct float64
	MaxLeverage          int
	MarginMode           string
	AutoCompound         bool
	RiskPerTradePct      float64
}

// handles preferences database operations
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

//  returns scanning preferences for a user
func (r *Repository) GetScanning(ctx context.Context, userID int) (*Scanning, error) {
	query := `
		SELECT user_id, min_confidence, scan_interval_minutes,
			   enabled_timeframes, enabled_indicators,
			   use_ml_predictions, use_sentiment_analysis, is_scanning_enabled
		FROM scanning_preferences WHERE user_id = $1
	`

	s := &Scanning{}
	var timeframesJSON, indicatorsJSON []byte
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&s.UserID, &s.MinConfidence, &s.ScanIntervalMins,
		&timeframesJSON, &indicatorsJSON,
		&s.UseMLPredictions, &s.UseSentiment, &s.IsScanningEnabled,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get scanning preferences: %w", err)
	}

	json.Unmarshal(timeframesJSON, &s.EnabledTimeframes)
	json.Unmarshal(indicatorsJSON, &s.EnabledIndicators)

	return s, nil
}

//  updates scanning preferences for a user
func (r *Repository) UpdateScanning(ctx context.Context, s *Scanning) error {
	timeframesJSON, _ := json.Marshal(s.EnabledTimeframes)
	indicatorsJSON, _ := json.Marshal(s.EnabledIndicators)

	query := `
		UPDATE scanning_preferences SET
			min_confidence = $2,
			scan_interval_minutes = $3,
			enabled_timeframes = $4,
			enabled_indicators = $5,
			use_ml_predictions = $6,
			use_sentiment_analysis = $7,
			is_scanning_enabled = $8
		WHERE user_id = $1
	`

	_, err := r.pool.Exec(ctx, query,
		s.UserID, s.MinConfidence, s.ScanIntervalMins,
		timeframesJSON, indicatorsJSON,
		s.UseMLPredictions, s.UseSentiment, s.IsScanningEnabled,
	)
	if err != nil {
		return fmt.Errorf("failed to update scanning preferences: %w", err)
	}
	return nil
}

//  returns notification preferences for a user
func (r *Repository) GetNotification(ctx context.Context, userID int) (*Notification, error) {
	query := `
		SELECT user_id, max_daily_notifications, opportunity_notifications,
			   trade_executed_notifications, milestone_notifications,
			   periodic_update_minutes, daily_summary_enabled,
			   daily_summary_hour, timezone
		FROM notification_preferences WHERE user_id = $1
	`

	n := &Notification{}
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&n.UserID, &n.MaxDailyNotifications, &n.OpportunityNotifications,
		&n.TradeExecutedNotifications, &n.MilestoneNotifications,
		&n.PeriodicUpdateMinutes, &n.DailySummaryEnabled,
		&n.DailySummaryHour, &n.Timezone,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get notification preferences: %w", err)
	}
	return n, nil
}

//  updates notification preferences for a user
func (r *Repository) UpdateNotification(ctx context.Context, n *Notification) error {
	query := `
		UPDATE notification_preferences SET
			max_daily_notifications = $2,
			opportunity_notifications = $3,
			trade_executed_notifications = $4,
			milestone_notifications = $5,
			periodic_update_minutes = $6,
			daily_summary_enabled = $7,
			daily_summary_hour = $8,
			timezone = $9
		WHERE user_id = $1
	`

	_, err := r.pool.Exec(ctx, query,
		n.UserID, n.MaxDailyNotifications, n.OpportunityNotifications,
		n.TradeExecutedNotifications, n.MilestoneNotifications,
		n.PeriodicUpdateMinutes, n.DailySummaryEnabled,
		n.DailySummaryHour, n.Timezone,
	)
	if err != nil {
		return fmt.Errorf("failed to update notification preferences: %w", err)
	}
	return nil
}

//	 returns trading preferences for a user
func (r *Repository) GetTrading(ctx context.Context, userID int) (*Trading, error) {
	query := `
		SELECT user_id, default_position_size, max_position_size,
			   max_open_positions, daily_loss_limit,
			   default_stop_loss_pct, default_take_profit_pct,
			   max_leverage, margin_mode, auto_compound, risk_per_trade_pct
		FROM trading_preferences WHERE user_id = $1
	`

	t := &Trading{}
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&t.UserID, &t.DefaultPositionSize, &t.MaxPositionSize,
		&t.MaxOpenPositions, &t.DailyLossLimit,
		&t.DefaultStopLossPct, &t.DefaultTakeProfitPct,
		&t.MaxLeverage, &t.MarginMode, &t.AutoCompound, &t.RiskPerTradePct,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trading preferences: %w", err)
	}
	return t, nil
}

//  updates trading preferences for a user
func (r *Repository) UpdateTrading(ctx context.Context, t *Trading) error {
	query := `
		UPDATE trading_preferences SET
			default_position_size = $2,
			max_position_size = $3,
			max_open_positions = $4,
			daily_loss_limit = $5,
			default_stop_loss_pct = $6,
			default_take_profit_pct = $7,
			max_leverage = $8,
			margin_mode = $9,
			auto_compound = $10,
			risk_per_trade_pct = $11
		WHERE user_id = $1
	`

	_, err := r.pool.Exec(ctx, query,
		t.UserID, t.DefaultPositionSize, t.MaxPositionSize,
		t.MaxOpenPositions, t.DailyLossLimit,
		t.DefaultStopLossPct, t.DefaultTakeProfitPct,
		t.MaxLeverage, t.MarginMode, t.AutoCompound, t.RiskPerTradePct,
	)
	if err != nil {
		return fmt.Errorf("failed to update trading preferences: %w", err)
	}
	return nil
}
