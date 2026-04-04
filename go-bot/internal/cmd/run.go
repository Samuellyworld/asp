// bot run command — initializes all services and starts the bot
package cmd

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/trading-bot/go-bot/internal/api"
	"github.com/trading-bot/go-bot/internal/analysis"
	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/circuitbreaker"
	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/config"
	"github.com/trading-bot/go-bot/internal/database"
	"github.com/trading-bot/go-bot/internal/discord"
	"github.com/trading-bot/go-bot/internal/leverage"
	"github.com/trading-bot/go-bot/internal/livetrading"
	mlclient "github.com/trading-bot/go-bot/internal/ml-client"
	"github.com/trading-bot/go-bot/internal/opportunity"
	"github.com/trading-bot/go-bot/internal/papertrading"
	"github.com/trading-bot/go-bot/internal/pipeline"
	"github.com/trading-bot/go-bot/internal/preferences"
	"github.com/trading-bot/go-bot/internal/scanner"
	"github.com/trading-bot/go-bot/internal/security"
	"github.com/trading-bot/go-bot/internal/telegram"
	"github.com/trading-bot/go-bot/internal/user"
	"github.com/trading-bot/go-bot/internal/watchlist"
	"github.com/trading-bot/go-bot/internal/whatsapp"
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

	// set up structured logging based on config level
	logLevel := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	if cfg.Telegram.BotToken == "" && cfg.Discord.BotToken == "" {
		return fmt.Errorf("at least one bot token (telegram or discord) is required")
	}

	// --- phase 1: core infrastructure ---

	pg, err := database.NewPostgresClient(cfg.Database)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}
	defer pg.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := pg.Ping(ctx); err != nil {
		return fmt.Errorf("postgres ping failed: %w", err)
	}
	log.Println("connected to postgres")

	// start health check + analytics API server
	healthSrv := newHealthServer(pg.Pool(), nil)
	healthSrv.SetBinanceURL(cfg.Binance.APIURL())
	if cfg.MLService.BaseURL != "" {
		healthSrv.SetMLURL(cfg.MLService.BaseURL)
	}
	httpMux, httpSrv := healthSrv.start(":8080")
	defer httpSrv.Shutdown(ctx)
	log.Println("health check server started on :8080")

	encryptor, err := security.NewEncryptor(cfg.Security.MasterKey)
	if err != nil {
		return fmt.Errorf("failed to initialize encryptor: %w", err)
	}

	auditLogger := security.NewAuditLogger(pg.Pool())
	userRepo := user.NewRepository(pg.Pool())
	binanceClient := binance.NewClient(cfg.Binance.APIURL(), cfg.Binance.Testnet)
	userSvc := user.NewService(userRepo, encryptor, auditLogger, binanceClient, cfg.Binance.Testnet)

	watchRepo := watchlist.NewRepository(pg.Pool())
	watchSvc := watchlist.NewService(watchRepo)
	prefsRepo := preferences.NewRepository(pg.Pool())
	prefsSvc := preferences.NewService(prefsRepo)

	// --- phase 2: analysis pipeline ---

	// grpc client for rust indicators engine (optional — degrades gracefully)
	var indicatorProvider pipeline.IndicatorProvider
	if cfg.RustEngine.Address != "" {
		grpcClient, err := analysis.NewClient(cfg.RustEngine.Address)
		if err != nil {
			log.Printf("warning: rust engine not available at %s: %v", cfg.RustEngine.Address, err)
		} else {
			defer grpcClient.Close()
			indicatorProvider = grpcClient
			log.Printf("connected to rust engine at %s", cfg.RustEngine.Address)
		}
	}

	// http client for python ml service (optional)
	var mlProvider pipeline.MLProvider
	if cfg.MLService.BaseURL != "" {
		mlProvider = mlclient.NewClient(cfg.MLService.BaseURL)
		log.Printf("ml service configured at %s", cfg.MLService.BaseURL)
	}

	// claude ai client
	var aiProvider pipeline.AIProvider
	if cfg.Claude.APIKey != "" {
		claudeClient := claude.NewClient(
			cfg.Claude.APIKey,
			claude.WithModel(cfg.Claude.Model),
			claude.WithMaxTokens(cfg.Claude.MaxTokens),
		)
		aiProvider = claudeClient
		log.Println("claude ai client initialized")
	}

	// assemble the analysis pipeline
	pipe := pipeline.New(binanceClient, indicatorProvider, mlProvider, aiProvider)

	// --- phase 3: trading infrastructure ---

	// websocket price feed — provides sub-second price updates with REST fallback
	wsCache := binance.NewWSPriceCache(cfg.Binance.WSURL())
	if err := wsCache.SubscribeAll(); err != nil {
		log.Printf("warning: websocket price feed not available: %v (falling back to REST)", err)
	} else {
		defer wsCache.Stop()
		log.Println("websocket price feed connected")
	}

	// price adapter uses WS cache with REST fallback
	prices := &wsPriceAdapter{ws: wsCache, rest: binanceClient}

	// opportunity manager
	oppConfig := opportunity.DefaultConfig()
	if cfg.Trading.OpportunityExpiryMinutes > 0 {
		oppConfig.ExpiryDuration = cfg.Trading.OpportunityExpiry()
	}
	oppManager := opportunity.NewManager(oppConfig)
	oppManager.StartExpiry()
	defer oppManager.StopExpiry()

	// paper trading
	paperExecutor := papertrading.NewExecutor(prices)

	// position persistence — wire store and recover open positions
	posRepo := database.NewPositionRepository(pg.Pool())
	paperExecutor.SetStore(&spotPositionStoreAdapter{repo: posRepo})

	// trade logging repositories (shared by all executors and scanner)
	tradeRepo := database.NewTradeRepository(pg.Pool())
	dailyStatsRepo := database.NewDailyStatsRepository(pg.Pool())
	paperExecutor.SetTradeLogger(&spotTradeLoggerAdapter{trades: tradeRepo, daily: dailyStatsRepo})

	paperMonitor := papertrading.NewMonitor(paperExecutor, prices, papertrading.DefaultMonitorConfig())

	// live trading adapters
	credRepo := &credRepoAdapter{repo: userRepo}
	keyDecryptor := livetrading.NewKeyDecryptorAdapter(credRepo, encryptor, auditLogger)
	orderClient := binance.NewOrderClient(cfg.Binance.APIURL(), cfg.Binance.Testnet)
	orderClient.SetRateLimiter(binanceClient.RateLimiter()) // share spot rate limiter
	balanceProvider := livetrading.NewBalanceProviderAdapter(keyDecryptor, binanceClient)

	// live trading safety
	safetyConfig := livetrading.DefaultSafetyConfig()
	lossTracker := livetrading.NewLossTracker()
	confirmMgr := livetrading.NewConfirmationManager()
	safetyChecker := livetrading.NewSafetyChecker(
		safetyConfig, balanceProvider, nil, lossTracker, confirmMgr,
	)

	// live trading executor and monitor
	liveExecutor := livetrading.NewExecutor(orderClient, keyDecryptor, safetyChecker, lossTracker)
	liveExecutor.SetStore(&livePositionStoreAdapter{repo: posRepo})
	liveExecutor.SetTradeLogger(&liveTradeLoggerAdapter{trades: tradeRepo, daily: dailyStatsRepo})
	emergencyStop := livetrading.NewEmergencyStop(liveExecutor)

	// wire safety checker's position counter to the live executor
	safetyChecker = livetrading.NewSafetyChecker(
		safetyConfig, balanceProvider, liveExecutor, lossTracker, confirmMgr,
	)

	liveMonitor := livetrading.NewMonitor(
		liveExecutor, orderClient, keyDecryptor, prices, livetrading.DefaultMonitorConfig(),
	)

	// --- phase 4: leverage trading ---

	// futures client
	futuresClient := binance.NewFuturesClient(cfg.Binance.FuturesAPIURL(), cfg.Binance.Testnet)
	markPrices := &markPriceAdapter{client: futuresClient}

	// leverage safety checker
	levBalanceProvider := &futuresBalanceAdapter{futures: futuresClient, keys: keyDecryptor}
	levStatusProvider := &leverageStatusAdapter{userSvc: userSvc}
	levSafetyConfig := leverage.SafetyConfig{
		HardMaxLeverage:       cfg.Leverage.HardMaxLeverage,
		UserMaxLeverage:       10, // default, per-user override later
		MaxMarginPerTrade:     cfg.Leverage.MaxMarginPerTrade,
		MinLiquidationDistance: cfg.Leverage.LiquidationWarningPct,
		RequireLeverageEnabled: true,
	}
	levSafetyChecker := leverage.NewSafetyChecker(levSafetyConfig, levBalanceProvider, levStatusProvider)

	// funding tracker
	fundingTracker := leverage.NewFundingTracker()

	// paper leverage executor
	levPaperExecutor := leverage.NewPaperExecutor(prices, levSafetyChecker, fundingTracker)
	levPaperExecutor.SetStore(&leveragePositionStoreAdapter{repo: posRepo})
	levPaperExecutor.SetTradeLogger(&leverageTradeLoggerAdapter{trades: tradeRepo, daily: dailyStatsRepo})

	// live leverage executor
	levLiveExecutor := leverage.NewLiveExecutor(futuresClient, keyDecryptor, levSafetyChecker, fundingTracker, markPrices)

	// leverage monitor (uses paper executor by default — handles both via interfaces)
	levMonitorConfig := leverage.DefaultMonitorConfig()
	levPaperMonitor := leverage.NewMonitor(levPaperExecutor, levPaperExecutor, markPrices, fundingTracker, levMonitorConfig, leverage.WithMarkPriceUpdater(levPaperExecutor))

	levLiveMonitor := leverage.NewMonitor(levLiveExecutor, levLiveExecutor, markPrices, fundingTracker, levMonitorConfig, leverage.WithMarkPriceUpdater(levLiveExecutor))

	// --- portfolio circuit breaker (shared across all executors) ---
	cbConfig := circuitbreaker.DefaultConfig()
	portfolioBreaker := circuitbreaker.New(cbConfig)
	paperExecutor.SetCircuitBreaker(portfolioBreaker)
	liveExecutor.SetCircuitBreaker(portfolioBreaker)
	levPaperExecutor.SetCircuitBreaker(portfolioBreaker)
	levLiveExecutor.SetCircuitBreaker(portfolioBreaker)
	log.Printf("portfolio circuit breaker enabled (daily loss limit: $%.0f, max consecutive losses: %d, cooldown: %s)",
		cbConfig.MaxDailyLoss, cbConfig.MaxConsecutiveLosses, cbConfig.CooldownDuration)

	// --- position recovery (restore open paper positions from database) ---
	if err := recoverSpotPositions(ctx, posRepo, paperExecutor); err != nil {
		slog.Error("spot position recovery failed", "error", err)
	}
	if err := recoverLeveragePositions(ctx, posRepo, levPaperExecutor); err != nil {
		slog.Error("leverage position recovery failed", "error", err)
	}
	if err := recoverLivePositions(ctx, posRepo, liveExecutor); err != nil {
		slog.Error("live position recovery failed", "error", err)
	}

	// --- phase 3: scanner ---

	// scanner notifier bridges telegram/discord bots
	notifier := &scannerNotifier{}

	// graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// --- start telegram bot ---

	var telegramBot *telegram.Bot
	if cfg.Telegram.BotToken != "" {
		wizard := user.NewSetupWizard()
		telegramBot = telegram.NewBot(cfg.Telegram.BotToken)
		handler := telegram.NewHandler(telegramBot, userSvc, wizard, watchSvc, prefsSvc, binanceClient)

		handler.SetTradingDeps(&telegram.TradingDeps{
			OppManager:       oppManager,
			PaperExecutor:    paperExecutor,
			PaperMonitor:     paperMonitor,
			LiveExecutor:     liveExecutor,
			LiveMonitor:      liveMonitor,
			Emergency:        emergencyStop,
			Confirm:          confirmMgr,
			SafetyConfig:     safetyConfig,
			LevPaperExecutor: levPaperExecutor,
			LevLiveExecutor:  levLiveExecutor,
			LevMonitor:       levPaperMonitor,
		})

		notifier.telegramBot = telegramBot

		log.Println("telegram bot started, polling for updates...")

		offset := 0
		go func() {
			for {
				select {
				case <-ctx.Done():
					log.Println("telegram polling stopped")
					return
				default:
				}

				updates, err := telegramBot.GetUpdates(offset, 30)
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

	// --- start discord bot ---

	if cfg.Discord.BotToken != "" {
		discordBot := discord.NewBot(cfg.Discord.BotToken, cfg.Discord.ApplicationID)
		discordHandler := discord.NewHandler(discordBot, userSvc, watchSvc, prefsSvc, binanceClient)

		discordHandler.SetTradingDeps(&discord.TradingDeps{
			OppManager:       oppManager,
			PaperExecutor:    paperExecutor,
			PaperMonitor:     paperMonitor,
			LiveExecutor:     liveExecutor,
			LiveMonitor:      liveMonitor,
			Emergency:        emergencyStop,
			Confirm:          confirmMgr,
			SafetyConfig:     safetyConfig,
			LevPaperExecutor: levPaperExecutor,
			LevLiveExecutor:  levLiveExecutor,
			LevMonitor:       levLiveMonitor,
		})

		// register all slash commands (base + trading)
		allCommands := append(discord.SlashCommands(), discord.TradingSlashCommands()...)
		if err := discordBot.RegisterCommands(allCommands); err != nil {
			log.Printf("warning: failed to register discord slash commands: %v", err)
		}

		notifier.discordBot = discordBot

		gateway := discord.NewGateway(cfg.Discord.BotToken, discordBot, discordHandler)

		go func() {
			for {
				select {
				case <-ctx.Done():
					log.Println("discord gateway stopped")
					return
				default:
				}

				if err := gateway.Run(ctx); err != nil {
					log.Printf("discord gateway error: %v, reconnecting in 5s...", err)
					select {
					case <-ctx.Done():
						log.Println("discord gateway stopped")
						return
					case <-time.After(5 * time.Second):
					}
				}
			}
		}()

		log.Println("discord bot started")
	}

	// --- start whatsapp bot ---

	if cfg.WhatsApp.PhoneNumberID != "" && cfg.WhatsApp.AccessToken != "" {
		waBot := whatsapp.NewBot(cfg.WhatsApp.PhoneNumberID, cfg.WhatsApp.AccessToken)
		notifier.whatsappBot = waBot
		log.Println("whatsapp notifications enabled")
	}

	// --- start scanner ---

	scannerCfg := scanner.DefaultConfig()
	if cfg.Trading.ScannerIntervalMinutes > 0 {
		scannerCfg.Interval = cfg.Trading.ScannerInterval()
	}
	if cfg.Trading.MaxDailyNotifications > 0 {
		scannerCfg.DefaultMaxDaily = cfg.Trading.MaxDailyNotifications
	}
	if cfg.Trading.DefaultConfidenceThreshold > 0 {
		scannerCfg.DefaultMinConfidence = cfg.Trading.DefaultConfidenceThreshold
	}

	bgScanner := scanner.New(userSvc, watchSvc, prefsSvc, pipe, notifier, scannerCfg)

	// wire decision logging to persist AI decisions and daily stats
	decisionRepo := database.NewAIDecisionRepository(pg.Pool())
	bgScanner.SetLogger(&decisionLoggerAdapter{decisions: decisionRepo, daily: dailyStatsRepo})

	// candle repository (shared by analytics API and data ingestion)
	candleRepo := database.NewCandleRepository(pg.Pool())

	// --- analytics REST API ---
	if cfg.API.Enabled {
		apiSrv := api.NewServer(posRepo, tradeRepo, decisionRepo, dailyStatsRepo, candleRepo, cfg.API.Key)
		apiSrv.RegisterRoutes(httpMux)
		log.Println("analytics API enabled on :8080/api/*")
	}

	bgScanner.Start(ctx)
	defer bgScanner.Stop()
	log.Printf("scanner started (%s interval)", scannerCfg.Interval)

	// --- data ingestion (background candle fetching) ---
	symbolProvider := &watchlistSymbolProvider{userSvc: userSvc, watchSvc: watchSvc}
	dataIngest := pipeline.NewDataIngestion(binanceClient, &candleStoreAdapter{repo: candleRepo}, symbolProvider, pipeline.DefaultIngestionConfig())
	dataIngest.Start(ctx)
	defer dataIngest.Stop()
	log.Println("data ingestion started (5m poll interval)")

	// --- monitor event routing ---
	// wire these BEFORE starting monitors to avoid dropping events from the first scan cycle

	// paper trading events -> notifications
	paperMonitor.OnEvent = func(event papertrading.Event) {
		routePaperEvent(event, telegramBot, notifier)
	}

	// live trading events -> notifications
	liveMonitor.OnEvent = func(event livetrading.Event) {
		routeLiveEvent(event, telegramBot, notifier)
	}

	// leverage paper monitor events -> notifications
	levPaperMonitor.OnEvent = func(event leverage.LevEvent) {
		routeLeverageEvent(event, telegramBot, notifier)
	}

	// leverage live monitor events -> notifications
	levLiveMonitor.OnEvent = func(event leverage.LevEvent) {
		routeLeverageEvent(event, telegramBot, notifier)
	}

	// --- start monitors now that event routing is wired ---
	paperMonitor.Start(ctx)
	defer paperMonitor.Stop()
	log.Println("paper trading monitor started (60s interval)")

	liveMonitor.Start(ctx)
	defer liveMonitor.Stop()
	log.Println("live trading monitor started (30s interval)")

	levPaperMonitor.Start(ctx)
	defer levPaperMonitor.Stop()
	log.Println("leverage paper monitor started (30s interval)")

	levLiveMonitor.Start(ctx)
	defer levLiveMonitor.Stop()
	log.Println("leverage live monitor started (30s interval)")

	// --- wait for shutdown ---

	sig := <-sigCh
	log.Printf("received %s, shutting down gracefully (10s timeout)...", sig)
	cancel()

	// give deferred cleanup functions time to finish
	shutdownDone := make(chan struct{})
	go func() {
		// deferred functions will run when this goroutine's caller returns
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
		log.Println("shutdown complete")
	case sig2 := <-sigCh:
		log.Printf("received %s during shutdown, forcing exit", sig2)
	}

	return nil
}

// routes paper trading events to the appropriate notification channel
func routePaperEvent(event papertrading.Event, tgBot *telegram.Bot, notifier *scannerNotifier) {
	if event.Position == nil {
		return
	}

	var msg string
	switch event.Type {
	case papertrading.EventTPHit:
		msg = papertrading.FormatTPHit(event.Position)
	case papertrading.EventSLHit:
		msg = papertrading.FormatSLHit(event.Position)
	case papertrading.EventManualClose:
		msg = papertrading.FormatManualClose(event.Position)
	case papertrading.EventMilestone:
		msg = papertrading.FormatMilestone(event.Position, event.Milestone)
	case papertrading.EventPeriodicUpdate:
		msg = papertrading.FormatPeriodicUpdate(event.Position)
	default:
		return
	}

	if msg == "" {
		return
	}

	// route based on platform
	if event.Position.Platform == "discord" {
		if err := notifier.discordBot.SendMessage("", msg); err != nil { log.Printf("warning: failed to send notification: %v", err) }
	} else if tgBot != nil {
		// would need chat id — in production, stored on position or user
		log.Printf("paper event [%s]: %s", event.Type, msg)
	}
}

// routes live trading events to the appropriate notification channel
func routeLiveEvent(event livetrading.Event, tgBot *telegram.Bot, notifier *scannerNotifier) {
	if event.Position == nil {
		return
	}

	var msg string
	switch event.Type {
	case livetrading.EventTPHit, livetrading.EventSLHit:
		msg = livetrading.FormatPositionClosed(event.Position)
	case livetrading.EventPeriodicUpdate:
		msg = fmt.Sprintf("📊 Live Update: %s | Entry: $%.2f | Size: $%.2f",
			event.Position.Symbol, event.Position.EntryPrice, event.Position.PositionSize)
	default:
		return
	}

	if msg == "" {
		return
	}

	if event.Position.Platform == "discord" {
		if err := notifier.discordBot.SendMessage("", msg); err != nil { log.Printf("warning: failed to send notification: %v", err) }
	} else if tgBot != nil {
		log.Printf("live event [%s]: %s", event.Type, msg)
	}
}

// routes leverage trading events to the appropriate notification channel
func routeLeverageEvent(event leverage.LevEvent, tgBot *telegram.Bot, notifier *scannerNotifier) {
	if event.Position == nil {
		return
	}

	var msg string
	switch event.Type {
	case leverage.LevEventTPHit, leverage.LevEventSLHit, leverage.LevEventClosed:
		msg = leverage.FormatLeverageClosed(event.Position)
	case leverage.LevEventLiqWarning:
		msg = leverage.FormatLiquidationWarning(event.Position, event.DistancePct)
	case leverage.LevEventLiqCritical:
		msg = leverage.FormatLiquidationCritical(event.Position, event.DistancePct)
	case leverage.LevEventAutoClose:
		msg = leverage.FormatLiquidationAutoClose(event.Position, event.DistancePct)
	case leverage.LevEventFundingFee:
		msg = leverage.FormatFundingFee(event.Position, event.FundingRate, event.FundingAmount)
	case leverage.LevEventPeriodicUpdate:
		msg = leverage.FormatLeverageUpdate(event.Position, event.DistancePct)
	default:
		return
	}

	if msg == "" {
		return
	}

	if event.Position.Platform == "discord" && notifier.discordBot != nil {
		if err := notifier.discordBot.SendMessage("", msg); err != nil { log.Printf("warning: failed to send notification: %v", err) }
	} else if tgBot != nil {
		log.Printf("leverage event [%s]: %s", event.Type, msg)
	}
}
