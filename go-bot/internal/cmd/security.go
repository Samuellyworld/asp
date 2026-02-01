package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/trading-bot/go-bot/internal/config"
	"github.com/trading-bot/go-bot/internal/security"
)

var securityCmd = &cobra.Command{
	Use:   "security",
	Short: "security operations",
}

var securityTestCmd = &cobra.Command{
	Use:   "test",
	Short: "test encryption round-trip",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if cfg.Security.MasterKey == "" {
			return fmt.Errorf("master encryption key not configured")
		}

		enc, err := security.NewEncryptor(cfg.Security.MasterKey)
		if err != nil {
			return fmt.Errorf("failed to create encryptor: %w", err)
		}

		if err := enc.Test(); err != nil {
			return fmt.Errorf("encryption test failed: %w", err)
		}

		fmt.Println("encrypt - decrypt round-trip passed")
		fmt.Println("different users produce different ciphertexts")

		return nil
	},
}

func init() {
	securityCmd.AddCommand(securityTestCmd)
	rootCmd.AddCommand(securityCmd)
}
