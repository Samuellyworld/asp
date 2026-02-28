// bot run command
package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/config"
	"github.com/trading-bot/go-bot/internal/database"
	"github.com/trading-bot/go-bot/internal/discord"
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
	Short: "start the bot (telegram and/or discord)",
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

	if cfg.Telegram.BotToken == "" && cfg.Discord.BotToken == "" {
		return fmt.Errorf("at least one bot token (telegram or discord) is required")
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

	// set up watchlist and preferences services
	watchRepo := watchlist.NewRepository(pg.Pool())
	watchSvc := watchlist.NewService(watchRepo)
	prefsRepo := preferences.NewRepository(pg.Pool())
	prefsSvc := preferences.NewService(prefsRepo)

	// graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// start telegram bot if configured
	if cfg.Telegram.BotToken != "" {
		wizard := user.NewSetupWizard()
		bot := telegram.NewBot(cfg.Telegram.BotToken)
		handler := telegram.NewHandler(bot, userSvc, wizard, watchSvc, prefsSvc, binanceClient)

		log.Println("telegram bot started, polling for updates...")

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
	}

	// start discord bot if configured
	if cfg.Discord.BotToken != "" {
		discordBot := discord.NewBot(cfg.Discord.BotToken, cfg.Discord.ApplicationID)
		discordHandler := discord.NewHandler(discordBot, userSvc, watchSvc, prefsSvc, binanceClient)

		// register slash commands
		if err := discordBot.RegisterCommands(discord.SlashCommands()); err != nil {
			log.Printf("warning: failed to register discord slash commands: %v", err)
		}

		gateway := discord.NewGateway(cfg.Discord.BotToken, discordBot, discordHandler)

		go func() {
			for {
				if err := gateway.Run(ctx); err != nil {
					log.Printf("discord gateway error: %v, reconnecting in 5s...", err)
					time.Sleep(5 * time.Second)
				}
			}
		}()

		log.Println("discord bot started")
	}

	// wait for shutdown signal
	sig := <-sigCh
	log.Printf("received %s, shutting down...", sig)

	return nil
}
