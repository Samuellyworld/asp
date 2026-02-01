package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/trading-bot/go-bot/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "configuration management",
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "validate configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if err := config.Validate(cfg); err != nil {
			return fmt.Errorf("config validation failed: %w", err)
		}

		fmt.Println("✓ configuration is valid")
		return nil
	},
}

func init() {
	configCmd.AddCommand(configValidateCmd)
	rootCmd.AddCommand(configCmd)
}
