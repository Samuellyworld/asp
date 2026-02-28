// telegram bot run command
package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/config"
	"github.com/trading-bot/go-bot/internal/database"
	"github.com/trading-bot/go-bot/internal/preferences"
	"github.com/trading-bot/go-bot/internal/security"
	"github.com/trading-bot/go-bot/internal/telegram"
	"github.com/trading-bot/go-bot/internal/user"
	"github.com/trading-bot/go-bot/internal/watchlist"
)

func init() {
	rootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "start the telegram bot",
	RunE:  runBot,
}

func runBot(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	if cfg.Telegram.BotToken == "" {
		return fmt.Errorf("telegram.bot_token is required to run the bot")
	}

	// connect to postgres
	pg, err := database.NewPostgresClient(cfg.Database)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}
	defer pg.Close()

	ctx := context.Background()
	if _, err := pg.Ping(ctx); err != nil {
		return fmt.Errorf("postgres ping failed: %w", err)
	}
	log.Println("connected to postgres")

	// set up encryption
	encryptor, err := security.NewEncryptor(cfg.Security.MasterKey)
	if err != nil {
		return fmt.Errorf("failed to initialize encryptor: %w", err)
	}

	// set up services
	auditLogger := security.NewAuditLogger(pg.Pool())
	userRepo := user.NewRepository(pg.Pool())
	binanceClient := binance.NewClient(cfg.Binance.APIURL(), cfg.Binance.Testnet)
	userSvc := user.NewService(userRepo, encryptor, auditLogger, binanceClient, cfg.Binance.Testnet)
	wizard := user.NewSetupWizard()

	// set up watchlist and preferences services
	watchRepo := watchlist.NewRepository(pg.Pool())
	watchSvc := watchlist.NewService(watchRepo)
	prefsRepo := preferences.NewRepository(pg.Pool())
	prefsSvc := preferences.NewService(prefsRepo)

	// set up telegram bot
	bot := telegram.NewBot(cfg.Telegram.BotToken)
	handler := telegram.NewHandler(bot, userSvc, wizard, watchSvc, prefsSvc, binanceClient)

	log.Println("telegram bot started, polling for updates...")

	// graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// start polling loop
	offset := 0
	go func() {
		for {
			updates, err := bot.GetUpdates(offset, 30)
			if err != nil {
				log.Printf("error getting updates: %v", err)
				continue
			}

			for _, update := range updates {
				handler.HandleUpdate(ctx, update)
				offset = update.UpdateID + 1
			}
		}
	}()

	// wait for shutdown signal
	sig := <-sigCh
	log.Printf("received %s, shutting down...", sig)

	return nil
}
