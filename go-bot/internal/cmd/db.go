package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/trading-bot/go-bot/internal/config"
	"github.com/trading-bot/go-bot/internal/database"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "database operations",
}

var dbPingCmd = &cobra.Command{
	Use:   "ping",
	Short: "test database connections",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// test postgresql
		pg, err := database.NewPostgresClient(cfg.Database)
		if err != nil {
			return fmt.Errorf("postgresql connection failed: %w", err)
		}
		defer pg.Close()

		pgLatency, err := pg.Ping(ctx)
		if err != nil {
			return fmt.Errorf("postgresql ping failed: %w", err)
		}
		fmt.Printf("postgresql connected (%s)\n", pgLatency.Round(time.Millisecond))

		// test redis
		redis, err := database.NewRedisClient(cfg.Redis)
		if err != nil {
			return fmt.Errorf("redis connection failed: %w", err)
		}
		defer redis.Close()

		redisLatency, err := redis.Ping(ctx)
		if err != nil {
			return fmt.Errorf("redis ping failed: %w", err)
		}
		fmt.Printf("redis connected (%s)\n", redisLatency.Round(time.Millisecond))

		return nil
	},
}

func init() {
	dbCmd.AddCommand(dbPingCmd)
	rootCmd.AddCommand(dbCmd)
}
