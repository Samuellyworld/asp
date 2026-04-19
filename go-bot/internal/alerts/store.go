package alerts

import (
	"context"
	"database/sql"
	"fmt"
)

// PostgreSQL store for alerts
type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) Create(ctx context.Context, a *Alert) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO alerts (user_id, symbol, alert_type, condition_value, is_active, is_triggered, created_at)
		 VALUES ($1, $2, $3, $4, true, false, NOW()) RETURNING id`,
		a.UserID, a.Symbol, a.AlertType, a.ConditionValue,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert alert: %w", err)
	}
	return id, nil
}

func (s *PostgresStore) ListActive(ctx context.Context, userID int) ([]*Alert, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, symbol, alert_type, condition_value, is_active, is_triggered, triggered_at, created_at
		 FROM alerts WHERE user_id = $1 AND is_active = true ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list active alerts: %w", err)
	}
	defer rows.Close()
	return scanAlerts(rows)
}

func (s *PostgresStore) ListAllActive(ctx context.Context) ([]*Alert, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, symbol, alert_type, condition_value, is_active, is_triggered, triggered_at, created_at
		 FROM alerts WHERE is_active = true`)
	if err != nil {
		return nil, fmt.Errorf("list all active alerts: %w", err)
	}
	defer rows.Close()
	return scanAlerts(rows)
}

func (s *PostgresStore) MarkTriggered(ctx context.Context, alertID int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alerts SET is_triggered = true, is_active = false, triggered_at = NOW() WHERE id = $1`, alertID)
	if err != nil {
		return fmt.Errorf("mark alert triggered: %w", err)
	}
	return nil
}

func (s *PostgresStore) Delete(ctx context.Context, alertID int, userID int) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM alerts WHERE id = $1 AND user_id = $2`, alertID, userID)
	if err != nil {
		return fmt.Errorf("delete alert: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("alert not found or not owned by user")
	}
	return nil
}

func scanAlerts(rows *sql.Rows) ([]*Alert, error) {
	var alerts []*Alert
	for rows.Next() {
		a := &Alert{}
		if err := rows.Scan(&a.ID, &a.UserID, &a.Symbol, &a.AlertType, &a.ConditionValue,
			&a.IsActive, &a.IsTriggered, &a.TriggeredAt, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan alert: %w", err)
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}
