// ai decision persistence — logs every Claude decision (BUY/SELL/HOLD) with full context.
// includes rejected, expired, and low-confidence decisions for self-learning.
package database

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AIDecisionRecord is a flat row for the ai_decisions table.
type AIDecisionRecord struct {
	ID               int
	UserID           int
	Symbol           string
	Timeframe        string
	Decision         string  // "BUY", "SELL", "HOLD", "CLOSE"
	Confidence       int
	EntryPrice       float64
	StopLoss         float64
	TakeProfit       float64
	PositionSizeUSD  float64
	RiskRewardRatio  float64
	Reasoning        string
	IndicatorsData   map[string]interface{} // stored as JSONB
	MLPrediction     map[string]interface{} // stored as JSONB
	SentimentData    map[string]interface{} // stored as JSONB
	PromptTokens     int
	CompletionTokens int
	LatencyMs        int
	WasApproved      *bool // nil = pending, true = approved, false = rejected/expired
	WasExecuted      bool
	FilterReason     string // "none", "hold", "low_confidence", "duplicate", "daily_limit", "safety_blocked"
	CreatedAt        time.Time
}

// AIDecisionRepository handles AI decision persistence.
type AIDecisionRepository struct {
	pool *pgxpool.Pool
}

func NewAIDecisionRepository(pool *pgxpool.Pool) *AIDecisionRepository {
	return &AIDecisionRepository{pool: pool}
}

// Insert writes a new AI decision record. Returns the auto-generated ID.
func (r *AIDecisionRepository) Insert(ctx context.Context, d *AIDecisionRecord) (int, error) {
	indJSON, _ := json.Marshal(d.IndicatorsData)
	mlJSON, _ := json.Marshal(d.MLPrediction)
	sentJSON, _ := json.Marshal(d.SentimentData)

	query := `
		INSERT INTO ai_decisions (
			user_id, symbol, timeframe, decision, confidence,
			entry_price, stop_loss, take_profit, position_size_usd, risk_reward_ratio,
			reasoning, indicators_data, ml_prediction, sentiment_data,
			prompt_tokens, completion_tokens, latency_ms,
			was_approved, was_executed
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14,
			$15, $16, $17,
			$18, $19
		)
		RETURNING id`

	var id int
	err := r.pool.QueryRow(ctx, query,
		d.UserID, d.Symbol, nullStr(d.Timeframe), d.Decision, d.Confidence,
		nullFloat(d.EntryPrice), nullFloat(d.StopLoss), nullFloat(d.TakeProfit),
		nullFloat(d.PositionSizeUSD), nullFloat(d.RiskRewardRatio),
		nullStr(d.Reasoning), indJSON, mlJSON, sentJSON,
		d.PromptTokens, d.CompletionTokens, d.LatencyMs,
		d.WasApproved, d.WasExecuted,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to insert ai_decision: %w", err)
	}
	return id, nil
}

// MarkApproved updates was_approved to true for a given decision ID.
func (r *AIDecisionRepository) MarkApproved(ctx context.Context, decisionID int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE ai_decisions SET was_approved = TRUE WHERE id = $1`, decisionID)
	return err
}

// MarkRejected updates was_approved to false for a given decision ID.
func (r *AIDecisionRepository) MarkRejected(ctx context.Context, decisionID int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE ai_decisions SET was_approved = FALSE WHERE id = $1`, decisionID)
	return err
}

// MarkExecuted sets was_executed to true.
func (r *AIDecisionRepository) MarkExecuted(ctx context.Context, decisionID int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE ai_decisions SET was_executed = TRUE WHERE id = $1`, decisionID)
	return err
}

// RecentBySymbol loads the last N decisions for a symbol+user (for feeding history to Claude).
func (r *AIDecisionRepository) RecentBySymbol(ctx context.Context, userID int, symbol string, limit int) ([]*AIDecisionRecord, error) {
	query := `
		SELECT id, user_id, symbol, COALESCE(timeframe, ''), decision, confidence,
		       COALESCE(entry_price, 0), COALESCE(stop_loss, 0), COALESCE(take_profit, 0),
		       COALESCE(position_size_usd, 0), COALESCE(risk_reward_ratio, 0),
		       COALESCE(reasoning, ''),
		       COALESCE(indicators_data, '{}'::jsonb), COALESCE(ml_prediction, '{}'::jsonb),
		       COALESCE(sentiment_data, '{}'::jsonb),
		       COALESCE(prompt_tokens, 0), COALESCE(completion_tokens, 0), COALESCE(latency_ms, 0),
		       was_approved, was_executed, created_at
		FROM ai_decisions
		WHERE user_id = $1 AND symbol = $2
		ORDER BY created_at DESC
		LIMIT $3`

	rows, err := r.pool.Query(ctx, query, userID, symbol, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent decisions: %w", err)
	}
	defer rows.Close()

	var results []*AIDecisionRecord
	for rows.Next() {
		d := &AIDecisionRecord{}
		var indJSON, mlJSON, sentJSON []byte
		if err := rows.Scan(
			&d.ID, &d.UserID, &d.Symbol, &d.Timeframe, &d.Decision, &d.Confidence,
			&d.EntryPrice, &d.StopLoss, &d.TakeProfit,
			&d.PositionSizeUSD, &d.RiskRewardRatio,
			&d.Reasoning,
			&indJSON, &mlJSON, &sentJSON,
			&d.PromptTokens, &d.CompletionTokens, &d.LatencyMs,
			&d.WasApproved, &d.WasExecuted, &d.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan ai_decision row: %w", err)
		}
		if err := json.Unmarshal(indJSON, &d.IndicatorsData); err != nil {
			log.Printf("warning: failed to unmarshal indicators data for decision %d: %v", d.ID, err)
		}
		if err := json.Unmarshal(mlJSON, &d.MLPrediction); err != nil {
			log.Printf("warning: failed to unmarshal ml prediction for decision %d: %v", d.ID, err)
		}
		if err := json.Unmarshal(sentJSON, &d.SentimentData); err != nil {
			log.Printf("warning: failed to unmarshal sentiment data for decision %d: %v", d.ID, err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// CountByUserToday returns count of decisions made today for a user.
func (r *AIDecisionRepository) CountByUserToday(ctx context.Context, userID int) (int, int, error) {
	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE was_approved = TRUE) as approved
		FROM ai_decisions
		WHERE user_id = $1 AND created_at >= CURRENT_DATE`

	var total, approved int
	err := r.pool.QueryRow(ctx, query, userID).Scan(&total, &approved)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count decisions: %w", err)
	}
	return total, approved, nil
}

// WinRateBySymbol calculates the win/loss ratio for executed trades on a symbol.
func (r *AIDecisionRepository) WinRateBySymbol(ctx context.Context, userID int, symbol string) (wins, losses int, err error) {
	query := `
		SELECT
			COUNT(*) FILTER (WHERE p.realized_pnl > 0) as wins,
			COUNT(*) FILTER (WHERE p.realized_pnl <= 0) as losses
		FROM ai_decisions d
		JOIN positions p ON p.user_id = d.user_id AND p.symbol = d.symbol
			AND p.status = 'CLOSED' AND p.opened_at >= d.created_at - interval '1 minute'
			AND p.opened_at <= d.created_at + interval '5 minutes'
		WHERE d.user_id = $1 AND d.symbol = $2 AND d.was_executed = TRUE`

	err = r.pool.QueryRow(ctx, query, userID, symbol).Scan(&wins, &losses)
	return
}

// TradeOutcomeRow holds a completed trade outcome for self-learning feedback.
type TradeOutcomeRow struct {
	Symbol     string
	Decision   string
	Confidence int
	EntryPrice float64
	ExitPrice  float64
	PnLPct     float64
	Timeframe  string
	CreatedAt  time.Time
}

// RecentOutcomes fetches the most recent completed trade outcomes (decisions that
// were executed and have a matching closed position with P&L).
func (r *AIDecisionRepository) RecentOutcomes(ctx context.Context, limit int) ([]*TradeOutcomeRow, error) {
	query := `
		SELECT d.symbol, d.decision, d.confidence,
		       COALESCE(d.entry_price, 0), COALESCE(p.exit_price, 0),
		       COALESCE(p.realized_pnl / NULLIF(p.position_size, 0) * 100, 0) as pnl_pct,
		       COALESCE(d.timeframe, '4h'),
		       d.created_at
		FROM ai_decisions d
		JOIN positions p ON p.user_id = d.user_id AND p.symbol = d.symbol
			AND p.status = 'CLOSED'
			AND p.opened_at >= d.created_at - interval '1 minute'
			AND p.opened_at <= d.created_at + interval '5 minutes'
		WHERE d.was_executed = TRUE AND d.decision IN ('BUY', 'SELL')
		ORDER BY d.created_at DESC
		LIMIT $1`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent outcomes: %w", err)
	}
	defer rows.Close()

	var results []*TradeOutcomeRow
	for rows.Next() {
		o := &TradeOutcomeRow{}
		if err := rows.Scan(
			&o.Symbol, &o.Decision, &o.Confidence,
			&o.EntryPrice, &o.ExitPrice, &o.PnLPct,
			&o.Timeframe, &o.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan outcome row: %w", err)
		}
		results = append(results, o)
	}
	return results, rows.Err()
}
