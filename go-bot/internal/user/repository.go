// user database operations
package user

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// user represents a row in the users table
type User struct {
	ID               int
	UUID             string
	TelegramID       *int64
	DiscordID        *int64
	Username         *string
	IsActivated      bool
	IsBanned         bool
	TradingMode      string
	LeverageEnabled  bool
	LastActiveChannel *string
	LastActiveAt     *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// credentials represents a row in user_api_credentials
type Credentials struct {
	ID                int
	UserID            int
	Exchange          string
	APIKeyEncrypted   []byte
	APISecretEncrypted []byte
	Salt              []byte
	Permissions       map[string]bool
	IsTestnet         bool
	IsValid           bool
	LastValidatedAt   *time.Time
	CreatedAt         time.Time
}

// repository handles user database operations
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// findByTelegramID looks up a user by their telegram id
func (r *Repository) FindByTelegramID(ctx context.Context, telegramID int64) (*User, error) {
	query := `
		SELECT id, uuid, telegram_id, discord_id, username, is_activated, is_banned,
			   trading_mode, leverage_enabled, last_active_channel, last_active_at,
			   created_at, updated_at
		FROM users WHERE telegram_id = $1
	`

	u := &User{}
	err := r.pool.QueryRow(ctx, query, telegramID).Scan(
		&u.ID, &u.UUID, &u.TelegramID, &u.DiscordID, &u.Username,
		&u.IsActivated, &u.IsBanned, &u.TradingMode, &u.LeverageEnabled,
		&u.LastActiveChannel, &u.LastActiveAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by telegram id: %w", err)
	}
	return u, nil
}

// create inserts a new user and returns the user with populated id/uuid
func (r *Repository) Create(ctx context.Context, telegramID int64, username string) (*User, error) {
	query := `
		INSERT INTO users (telegram_id, username, last_active_channel, last_active_at)
		VALUES ($1, $2, 'telegram', NOW())
		RETURNING id, uuid, telegram_id, username, is_activated, is_banned,
				  trading_mode, leverage_enabled, last_active_channel, last_active_at,
				  created_at, updated_at
	`

	u := &User{}
	err := r.pool.QueryRow(ctx, query, telegramID, username).Scan(
		&u.ID, &u.UUID, &u.TelegramID, &u.Username, &u.IsActivated,
		&u.IsBanned, &u.TradingMode, &u.LeverageEnabled,
		&u.LastActiveChannel, &u.LastActiveAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return u, nil
}

// createDefaultPreferences calls the database function to set up defaults
func (r *Repository) CreateDefaultPreferences(ctx context.Context, userID int) error {
	_, err := r.pool.Exec(ctx, "SELECT create_default_user_preferences($1)", userID)
	if err != nil {
		return fmt.Errorf("failed to create default preferences: %w", err)
	}
	return nil
}

// activate sets is_activated = true
func (r *Repository) Activate(ctx context.Context, userID int) error {
	_, err := r.pool.Exec(ctx, "UPDATE users SET is_activated = TRUE WHERE id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to activate user: %w", err)
	}
	return nil
}

// updateLastActive updates the user's last activity timestamp and channel
func (r *Repository) UpdateLastActive(ctx context.Context, userID int, channel string) error {
	query := `UPDATE users SET last_active_channel = $2, last_active_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, userID, channel)
	if err != nil {
		return fmt.Errorf("failed to update last active: %w", err)
	}
	return nil
}

// saveCredentials stores encrypted api credentials for a user
func (r *Repository) SaveCredentials(ctx context.Context, cred *Credentials) (*Credentials, error) {
	permsJSON, err := json.Marshal(cred.Permissions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal permissions: %w", err)
	}

	query := `
		INSERT INTO user_api_credentials 
			(user_id, exchange, api_key_encrypted, api_secret_encrypted, salt, 
			 permissions, is_testnet, is_valid, last_validated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (user_id, exchange) 
		DO UPDATE SET 
			api_key_encrypted = EXCLUDED.api_key_encrypted,
			api_secret_encrypted = EXCLUDED.api_secret_encrypted,
			salt = EXCLUDED.salt,
			permissions = EXCLUDED.permissions,
			is_testnet = EXCLUDED.is_testnet,
			is_valid = EXCLUDED.is_valid,
			last_validated_at = NOW()
		RETURNING id, created_at
	`

	err = r.pool.QueryRow(ctx, query,
		cred.UserID, cred.Exchange, cred.APIKeyEncrypted, cred.APISecretEncrypted,
		cred.Salt, permsJSON, cred.IsTestnet, cred.IsValid,
	).Scan(&cred.ID, &cred.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to save credentials: %w", err)
	}

	return cred, nil
}

// getCredentials retrieves encrypted credentials for a user + exchange
func (r *Repository) GetCredentials(ctx context.Context, userID int, exchange string) (*Credentials, error) {
	query := `
		SELECT id, user_id, exchange, api_key_encrypted, api_secret_encrypted,
			   salt, permissions, is_testnet, is_valid, last_validated_at, created_at
		FROM user_api_credentials
		WHERE user_id = $1 AND exchange = $2
	`

	c := &Credentials{}
	var permsJSON []byte
	err := r.pool.QueryRow(ctx, query, userID, exchange).Scan(
		&c.ID, &c.UserID, &c.Exchange, &c.APIKeyEncrypted, &c.APISecretEncrypted,
		&c.Salt, &permsJSON, &c.IsTestnet, &c.IsValid, &c.LastValidatedAt, &c.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %w", err)
	}

	if err := json.Unmarshal(permsJSON, &c.Permissions); err != nil {
		return nil, fmt.Errorf("failed to parse permissions: %w", err)
	}

	return c, nil
}

// findByDiscordID looks up a user by their discord id
func (r *Repository) FindByDiscordID(ctx context.Context, discordID int64) (*User, error) {
	query := `
		SELECT id, uuid, telegram_id, discord_id, username, is_activated, is_banned,
			   trading_mode, leverage_enabled, last_active_channel, last_active_at,
			   created_at, updated_at
		FROM users WHERE discord_id = $1
	`

	u := &User{}
	err := r.pool.QueryRow(ctx, query, discordID).Scan(
		&u.ID, &u.UUID, &u.TelegramID, &u.DiscordID, &u.Username,
		&u.IsActivated, &u.IsBanned, &u.TradingMode, &u.LeverageEnabled,
		&u.LastActiveChannel, &u.LastActiveAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by discord id: %w", err)
	}
	return u, nil
}

// createFromDiscord inserts a new user from discord and returns the user with populated id/uuid
func (r *Repository) CreateFromDiscord(ctx context.Context, discordID int64, username string) (*User, error) {
	query := `
		INSERT INTO users (discord_id, username, last_active_channel, last_active_at)
		VALUES ($1, $2, 'discord', NOW())
		RETURNING id, uuid, discord_id, username, is_activated, is_banned,
				  trading_mode, leverage_enabled, last_active_channel, last_active_at,
				  created_at, updated_at
	`

	u := &User{}
	err := r.pool.QueryRow(ctx, query, discordID, username).Scan(
		&u.ID, &u.UUID, &u.DiscordID, &u.Username, &u.IsActivated,
		&u.IsBanned, &u.TradingMode, &u.LeverageEnabled,
		&u.LastActiveChannel, &u.LastActiveAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create discord user: %w", err)
	}
	return u, nil
}

// hasValidCredentials checks if a user has valid api credentials
func (r *Repository) HasValidCredentials(ctx context.Context, userID int) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM user_api_credentials WHERE user_id = $1 AND is_valid = TRUE)",
		userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check credentials: %w", err)
	}
	return exists, nil
}
