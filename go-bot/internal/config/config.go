package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// holds all application configuration
type Config struct {
	Database   DatabaseConfig
	Redis      RedisConfig
	Security   SecurityConfig
	Telegram   TelegramConfig
	Discord    DiscordConfig
	WhatsApp   WhatsAppConfig
	Binance    BinanceConfig
	Claude     ClaudeConfig
	Trading    TradingConfig
	RustEngine RustEngineConfig
	MLService  MLServiceConfig
	Leverage   LeverageConfig
	API        APIConfig
	DataSources DataSourcesConfig
	LogLevel   string
}

// holds external data source API keys
type DataSourcesConfig struct {
	CryptoPanicToken string
	CoinGlassAPIKey  string
	CoinGeckoAPIKey  string // optional — empty for free tier
}

//holds postgres connection settings
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
}

// returns the postgres connection string
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode)
}

//  holds redis connection settings
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

// returns the redis address in host:port format
func (r RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

// holds encryption settings
type SecurityConfig struct {
	MasterKey string
}

// holds analytics REST API settings
type APIConfig struct {
	Enabled bool
	Key     string // API key for authentication (empty = no auth)
	Port    int    // port for the API server (0 = same as health on :8080)
}

//  holds telegram bot settings
type TelegramConfig struct {
	BotToken string
}

//  holds discord bot settings
type DiscordConfig struct {
	BotToken      string
	ApplicationID string
}

// holds whatsapp cloud api settings
type WhatsAppConfig struct {
	PhoneNumberID string
	AccessToken   string
}

//  holds binance api settings
type BinanceConfig struct {
	Testnet              bool
	TestnetAPIURL        string
	TestnetWSURL         string
	MainnetAPIURL        string
	MainnetWSURL         string
	FuturesTestnetAPIURL string
	FuturesMainnetAPIURL string
}

// returns the appropriate binance api url based on testnet setting
func (b BinanceConfig) APIURL() string {
	if b.Testnet {
		return b.TestnetAPIURL
	}
	return b.MainnetAPIURL
}

// returns the appropriate binance futures api url based on testnet setting
func (b BinanceConfig) FuturesAPIURL() string {
	if b.Testnet {
		return b.FuturesTestnetAPIURL
	}
	return b.FuturesMainnetAPIURL
}

//  returns the appropriate binance websocket url based on testnet setting
func (b BinanceConfig) WSURL() string {
	if b.Testnet {
		return b.TestnetWSURL
	}
	return b.MainnetWSURL
}

//  holds claude ai settings
type ClaudeConfig struct {
	APIKey    string
	Model     string
	MaxTokens int
}

// holds grpc connection settings for the rust indicators engine
type RustEngineConfig struct {
	Address string
}

// holds http connection settings for the python ml service
type MLServiceConfig struct {
	BaseURL string
}

//  holds trading behavior settings
type TradingConfig struct {
	DefaultConfidenceThreshold int
	MaxDailyNotifications      int
	ScannerIntervalMinutes     int
	OpportunityExpiryMinutes   int
	Timeframes                 []string // primary + confirmation timeframes, e.g. ["4h", "1d"]
	DriftCheckIntervalMinutes  int      // how often to run ML drift detection (minutes)
}

// returns the scanner interval as a duration
func (t TradingConfig) ScannerInterval() time.Duration {
	return time.Duration(t.ScannerIntervalMinutes) * time.Minute
}

// returns the opportunity expiry as a duration
func (t TradingConfig) OpportunityExpiry() time.Duration {
	return time.Duration(t.OpportunityExpiryMinutes) * time.Minute
}

// holds leverage trading settings
type LeverageConfig struct {
	HardMaxLeverage         int
	MaxMarginPerTrade       float64
	LiquidationWarningPct   float64
	LiquidationCriticalPct  float64
	LiquidationAutoClosePct float64
	MonitorIntervalSeconds  int
}

// reads configuration from viper and returns a Config struct
func Load() (*Config, error) {
	setDefaults()

	cfg := &Config{
		Database: DatabaseConfig{
			Host:     viper.GetString("database.host"),
			Port:     viper.GetInt("database.port"),
			User:     viper.GetString("database.user"),
			Password: viper.GetString("database.password"),
			Name:     viper.GetString("database.name"),
			SSLMode:  viper.GetString("database.sslmode"),
		},
		Redis: RedisConfig{
			Host:     viper.GetString("redis.host"),
			Port:     viper.GetInt("redis.port"),
			Password: viper.GetString("redis.password"),
			DB:       viper.GetInt("redis.db"),
		},
		Security: SecurityConfig{
			MasterKey: viper.GetString("security.master_key"),
		},
		Telegram: TelegramConfig{
			BotToken: viper.GetString("telegram.bot_token"),
		},
		Discord: DiscordConfig{
			BotToken:      viper.GetString("discord.bot_token"),
			ApplicationID: viper.GetString("discord.application_id"),
		},
		WhatsApp: WhatsAppConfig{
			PhoneNumberID: viper.GetString("whatsapp.phone_number_id"),
			AccessToken:   viper.GetString("whatsapp.access_token"),
		},
		Binance: BinanceConfig{
			Testnet:              viper.GetBool("binance.testnet"),
			TestnetAPIURL:        viper.GetString("binance.testnet_api_url"),
			TestnetWSURL:         viper.GetString("binance.testnet_ws_url"),
			MainnetAPIURL:        viper.GetString("binance.mainnet_api_url"),
			MainnetWSURL:         viper.GetString("binance.mainnet_ws_url"),
			FuturesTestnetAPIURL: viper.GetString("binance.futures_testnet_api_url"),
			FuturesMainnetAPIURL: viper.GetString("binance.futures_mainnet_api_url"),
		},
		Claude: ClaudeConfig{
			APIKey:    viper.GetString("claude.api_key"),
			Model:     viper.GetString("claude.model"),
			MaxTokens: viper.GetInt("claude.max_tokens"),
		},
		RustEngine: RustEngineConfig{
			Address: viper.GetString("rust_engine.address"),
		},
		MLService: MLServiceConfig{
			BaseURL: viper.GetString("ml_service.base_url"),
		},
		Trading: TradingConfig{
			DefaultConfidenceThreshold: viper.GetInt("trading.default_confidence_threshold"),
			MaxDailyNotifications:      viper.GetInt("trading.max_daily_notifications"),
			ScannerIntervalMinutes:     viper.GetInt("trading.scanner_interval_minutes"),
			OpportunityExpiryMinutes:   viper.GetInt("trading.opportunity_expiry_minutes"),
			Timeframes:                 viper.GetStringSlice("trading.timeframes"),
			DriftCheckIntervalMinutes:  viper.GetInt("trading.drift_check_interval_minutes"),
		},
		Leverage: LeverageConfig{
			HardMaxLeverage:         viper.GetInt("leverage.hard_max_leverage"),
			MaxMarginPerTrade:       viper.GetFloat64("leverage.max_margin_per_trade"),
			LiquidationWarningPct:   viper.GetFloat64("leverage.liquidation_warning_pct"),
			LiquidationCriticalPct:  viper.GetFloat64("leverage.liquidation_critical_pct"),
			LiquidationAutoClosePct: viper.GetFloat64("leverage.liquidation_auto_close_pct"),
			MonitorIntervalSeconds:  viper.GetInt("leverage.monitor_interval_seconds"),
		},
		API: APIConfig{
			Enabled: viper.GetBool("api.enabled"),
			Key:     viper.GetString("api.key"),
			Port:    viper.GetInt("api.port"),
		},
		DataSources: DataSourcesConfig{
			CryptoPanicToken: viper.GetString("datasources.cryptopanic_token"),
			CoinGlassAPIKey:  viper.GetString("datasources.coinglass_api_key"),
			CoinGeckoAPIKey:  viper.GetString("datasources.coingecko_api_key"),
		},
		LogLevel: viper.GetString("log_level"),
	}

	return cfg, nil
}

func setDefaults() {
	// database
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "trading_bot")
	viper.SetDefault("database.password", "trading_bot_secret")
	viper.SetDefault("database.name", "trading_bot")
	viper.SetDefault("database.sslmode", "disable")

	// redis
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.password", "redis_secret")
	viper.SetDefault("redis.db", 0)

	// binance
	viper.SetDefault("binance.testnet", true)
	viper.SetDefault("binance.testnet_api_url", "https://testnet.binance.vision")
	viper.SetDefault("binance.testnet_ws_url", "wss://testnet.binance.vision/ws")
	viper.SetDefault("binance.mainnet_api_url", "https://api.binance.com")
	viper.SetDefault("binance.mainnet_ws_url", "wss://stream.binance.com:9443/ws")

	// binance futures
	viper.SetDefault("binance.futures_testnet_api_url", "https://testnet.binancefuture.com")
	viper.SetDefault("binance.futures_mainnet_api_url", "https://fapi.binance.com")

	// claude
	viper.SetDefault("claude.model", "claude-sonnet-4-20250514")
	viper.SetDefault("claude.max_tokens", 4096)

	// rust engine (grpc indicators)
	viper.SetDefault("rust_engine.address", "localhost:50051")

	// python ml service
	viper.SetDefault("ml_service.base_url", "http://localhost:8000")

	// trading
	viper.SetDefault("trading.default_confidence_threshold", 80)
	viper.SetDefault("trading.max_daily_notifications", 10)
	viper.SetDefault("trading.scanner_interval_minutes", 5)
	viper.SetDefault("trading.opportunity_expiry_minutes", 15)
	viper.SetDefault("trading.timeframes", []string{"4h", "1d"})
	viper.SetDefault("trading.drift_check_interval_minutes", 60)

	// leverage
	viper.SetDefault("leverage.hard_max_leverage", 20)
	viper.SetDefault("leverage.max_margin_per_trade", 500)
	viper.SetDefault("leverage.liquidation_warning_pct", 10)
	viper.SetDefault("leverage.liquidation_critical_pct", 5)
	viper.SetDefault("leverage.liquidation_auto_close_pct", 2)
	viper.SetDefault("leverage.monitor_interval_seconds", 30)

	// logging
	viper.SetDefault("log_level", "info")
}

//  checks that all required config values are present and valid
func Validate(cfg *Config) error {
	if cfg.Security.MasterKey == "" {
		return fmt.Errorf("security.master_key is required")
	}
	if len(cfg.Security.MasterKey) < 32 {
		return fmt.Errorf("security.master_key must be at least 32 characters")
	}

	// leverage safety bounds
	if cfg.Leverage.HardMaxLeverage <= 0 || cfg.Leverage.HardMaxLeverage > 125 {
		return fmt.Errorf("leverage.hard_max_leverage must be between 1 and 125, got %d", cfg.Leverage.HardMaxLeverage)
	}
	if cfg.Leverage.MaxMarginPerTrade <= 0 {
		return fmt.Errorf("leverage.max_margin_per_trade must be positive, got %.2f", cfg.Leverage.MaxMarginPerTrade)
	}
	if cfg.Leverage.LiquidationWarningPct <= 0 || cfg.Leverage.LiquidationWarningPct > 50 {
		return fmt.Errorf("leverage.liquidation_warning_pct must be between 0 and 50, got %.2f", cfg.Leverage.LiquidationWarningPct)
	}
	if cfg.Leverage.LiquidationCriticalPct <= 0 || cfg.Leverage.LiquidationCriticalPct >= cfg.Leverage.LiquidationWarningPct {
		return fmt.Errorf("leverage.liquidation_critical_pct must be between 0 and liquidation_warning_pct (%.2f), got %.2f",
			cfg.Leverage.LiquidationWarningPct, cfg.Leverage.LiquidationCriticalPct)
	}
	if cfg.Leverage.LiquidationAutoClosePct <= 0 || cfg.Leverage.LiquidationAutoClosePct >= cfg.Leverage.LiquidationCriticalPct {
		return fmt.Errorf("leverage.liquidation_auto_close_pct must be between 0 and liquidation_critical_pct (%.2f), got %.2f",
			cfg.Leverage.LiquidationCriticalPct, cfg.Leverage.LiquidationAutoClosePct)
	}

	// trading config bounds
	if cfg.Trading.DefaultConfidenceThreshold < 0 || cfg.Trading.DefaultConfidenceThreshold > 100 {
		return fmt.Errorf("trading.default_confidence_threshold must be 0-100, got %d", cfg.Trading.DefaultConfidenceThreshold)
	}
	if len(cfg.Trading.Timeframes) == 0 {
		return fmt.Errorf("trading.timeframes must have at least one entry")
	}
	validTF := map[string]bool{"1m": true, "3m": true, "5m": true, "15m": true, "30m": true, "1h": true, "2h": true, "4h": true, "6h": true, "8h": true, "12h": true, "1d": true, "3d": true, "1w": true, "1M": true}
	for _, tf := range cfg.Trading.Timeframes {
		if !validTF[tf] {
			return fmt.Errorf("trading.timeframes contains invalid timeframe %q", tf)
		}
	}

	// database connection
	if cfg.Database.Host == "" {
		return fmt.Errorf("database.host is required")
	}
	if cfg.Database.Port <= 0 || cfg.Database.Port > 65535 {
		return fmt.Errorf("database.port must be 1-65535, got %d", cfg.Database.Port)
	}

	return nil
}
