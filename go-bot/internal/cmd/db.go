package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "run database migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		pg, err := database.NewPostgresClient(cfg.Database)
		if err != nil {
			return fmt.Errorf("postgresql connection failed: %w", err)
		}
		defer pg.Close()

		// create migrations tracking table
		if err := pg.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
			filename VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		)`); err != nil {
			return fmt.Errorf("failed to create migrations table: %w", err)
		}

		// collect migration files from local, repo-root, and container-mounted paths.
		migrationDirs := []string{
			"../migrations",
			"../go-bot/migrations",
			"root-migrations",
			"migrations",
			"go-bot/migrations",
			"/app/root-migrations",
			"/app/migrations",
		}
		var files []string
		seen := make(map[string]bool)
		for _, dir := range migrationDirs {
			matches, _ := filepath.Glob(filepath.Join(dir, "*.sql"))
			for _, m := range matches {
				base := filepath.Base(m)
				if !seen[base] {
					seen[base] = true
					files = append(files, m)
				}
			}
		}
		sort.Slice(files, func(i, j int) bool {
			left := filepath.Base(files[i])
			right := filepath.Base(files[j])
			if left == right {
				return files[i] < files[j]
			}
			return left < right
		})

		if len(files) == 0 {
			fmt.Println("no migration files found")
			return nil
		}

		applied := 0
		for _, f := range files {
			base := filepath.Base(f)

			// check if already applied
			var count int
			row := pg.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE filename = $1", base)
			if err := row.Scan(&count); err != nil {
				return fmt.Errorf("failed to check migration %s: %w", base, err)
			}
			if count > 0 {
				fmt.Printf("  skip: %s (already applied)\n", base)
				continue
			}

			// read and execute
			sql, err := os.ReadFile(f)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", f, err)
			}

			if err := pg.Exec(ctx, string(sql)); err != nil {
				return fmt.Errorf("migration %s failed: %w", base, err)
			}

			if err := pg.Exec(ctx, "INSERT INTO schema_migrations (filename) VALUES ($1)", base); err != nil {
				return fmt.Errorf("failed to record migration %s: %w", base, err)
			}

			fmt.Printf("  applied: %s\n", base)
			applied++
		}

		fmt.Printf("migrations complete (%d applied, %d skipped)\n", applied, len(files)-applied)
		return nil
	},
}

func init() {
	dbCmd.AddCommand(dbPingCmd)
	dbCmd.AddCommand(dbMigrateCmd)
	rootCmd.AddCommand(dbCmd)
}
