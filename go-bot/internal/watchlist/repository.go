// watchlist database operations
package watchlist

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// represents a row in the watchlists table
type Item struct {
	ID       int
	UserID   int
	Symbol   string
	IsActive bool
	Priority int
	AddedAt  time.Time
}

//  handles watchlist database operations
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// returns all active watchlist items for a user, ordered by priority
func (r *Repository) GetByUserID(ctx context.Context, userID int) ([]Item, error) {
	query := `
		SELECT id, user_id, symbol, is_active, priority, added_at
		FROM watchlists
		WHERE user_id = $1 AND is_active = TRUE
		ORDER BY priority ASC, added_at ASC
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get watchlist: %w", err)
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.UserID, &item.Symbol, &item.IsActive, &item.Priority, &item.AddedAt); err != nil {
			return nil, fmt.Errorf("failed to scan watchlist item: %w", err)
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

//  adds a symbol to the user's watchlist
func (r *Repository) Add(ctx context.Context, userID int, symbol string) error {
	// get the next priority number
	var maxPriority int
	err := r.pool.QueryRow(ctx,
		"SELECT COALESCE(MAX(priority), 0) FROM watchlists WHERE user_id = $1",
		userID,
	).Scan(&maxPriority)
	if err != nil {
		return fmt.Errorf("failed to get max priority: %w", err)
	}

	query := `
		INSERT INTO watchlists (user_id, symbol, priority)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, symbol) DO UPDATE SET is_active = TRUE
	`

	_, err = r.pool.Exec(ctx, query, userID, symbol, maxPriority+1)
	if err != nil {
		return fmt.Errorf("failed to add to watchlist: %w", err)
	}
	return nil
}

// deactivates a symbol from the user's watchlist
func (r *Repository) Remove(ctx context.Context, userID int, symbol string) error {
	result, err := r.pool.Exec(ctx,
		"UPDATE watchlists SET is_active = FALSE WHERE user_id = $1 AND symbol = $2",
		userID, symbol,
	)
	if err != nil {
		return fmt.Errorf("failed to remove from watchlist: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("symbol %s not found in your watchlist", symbol)
	}
	return nil
}

// checks if a symbol is in the user's active watchlist
func (r *Repository) Exists(ctx context.Context, userID int, symbol string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM watchlists WHERE user_id = $1 AND symbol = $2 AND is_active = TRUE)",
		userID, symbol,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check watchlist: %w", err)
	}
	return exists, nil
}

// returns the number of active symbols in a user's watchlist
func (r *Repository) Count(ctx context.Context, userID int) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM watchlists WHERE user_id = $1 AND is_active = TRUE",
		userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count watchlist: %w", err)
	}
	return count, nil
}

//  removes all items and repopulates with the default top-10 watchlist
func (r *Repository) Reset(ctx context.Context, userID int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// deactivate all current items
	_, err = tx.Exec(ctx, "UPDATE watchlists SET is_active = FALSE WHERE user_id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to clear watchlist: %w", err)
	}

	// call the database function to repopulate defaults
	_, err = tx.Exec(ctx, "SELECT populate_default_watchlist($1)", userID)
	if err != nil {
		return fmt.Errorf("failed to populate default watchlist: %w", err)
	}

	return tx.Commit(ctx)
}

// returns a specific watchlist item
func (r *Repository) FindBySymbol(ctx context.Context, userID int, symbol string) (*Item, error) {
	query := `
		SELECT id, user_id, symbol, is_active, priority, added_at
		FROM watchlists
		WHERE user_id = $1 AND symbol = $2 AND is_active = TRUE
	`

	item := &Item{}
	err := r.pool.QueryRow(ctx, query, userID, symbol).Scan(
		&item.ID, &item.UserID, &item.Symbol, &item.IsActive, &item.Priority, &item.AddedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find watchlist item: %w", err)
	}
	return item, nil
}
