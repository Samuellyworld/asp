// postgresql database client
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trading-bot/go-bot/internal/config"
)

type PostgresClient struct {
	pool *pgxpool.Pool
}

func NewPostgresClient(cfg config.DatabaseConfig) (*PostgresClient, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	poolConfig.MaxConns = 10
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	return &PostgresClient{pool: pool}, nil
}

func (c *PostgresClient) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	err := c.pool.Ping(ctx)
	return time.Since(start), err
}

func (c *PostgresClient) Close() {
	c.pool.Close()
}

func (c *PostgresClient) Pool() *pgxpool.Pool {
	return c.pool
}

func (c *PostgresClient) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := c.pool.Exec(ctx, sql, args...)
	return err
}

func (c *PostgresClient) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return c.pool.QueryRow(ctx, sql, args...)
}
