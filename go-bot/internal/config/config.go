package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// holds all application configuration
type Config struct {
	Database DatabaseConfig
	Redis    RedisConfig
	Security SecurityConfig
	Telegram TelegramConfig
	Discord  DiscordConfig
	Binance  BinanceConfig
	Claude   ClaudeConfig
	Trading  TradingConfig
	LogLevel string
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

//  holds telegram bot settings
type TelegramConfig struct {
	BotToken string
}

//  holds discord bot settings
type DiscordConfig struct {
	BotToken      string
	ApplicationID string
}

//  holds binance api settings
type BinanceConfig struct {
	Testnet       bool
	TestnetAPIURL string
	TestnetWSURL  string
	MainnetAPIURL string
	MainnetWSURL  string
}

// returns the appropriate binance api url based on testnet setting
func (b BinanceConfig) APIURL() string {
	if b.Testnet {
		return b.TestnetAPIURL
	}
	return b.MainnetAPIURL
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

//  holds trading behavior settings
type TradingConfig struct {
	DefaultConfidenceThreshold int
	MaxDailyNotifications      int
	ScannerIntervalMinutes     int
	OpportunityExpiryMinutes   int
}

// returns the scanner interval as a duration
func (t TradingConfig) ScannerInterval() time.Duration {
	return time.Duration(t.ScannerIntervalMinutes) * time.Minute
}

// returns the opportunity expiry as a duration
func (t TradingConfig) OpportunityExpiry() time.Duration {
	return time.Duration(t.OpportunityExpiryMinutes) * time.Minute
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
		Binance: BinanceConfig{
			Testnet:       viper.GetBool("binance.testnet"),
			TestnetAPIURL: viper.GetString("binance.testnet_api_url"),
			TestnetWSURL:  viper.GetString("binance.testnet_ws_url"),
			MainnetAPIURL: viper.GetString("binance.mainnet_api_url"),
			MainnetWSURL:  viper.GetString("binance.mainnet_ws_url"),
		},
		Claude: ClaudeConfig{
			APIKey:    viper.GetString("claude.api_key"),
			Model:     viper.GetString("claude.model"),
			MaxTokens: viper.GetInt("claude.max_tokens"),
		},
		Trading: TradingConfig{
			DefaultConfidenceThreshold: viper.GetInt("trading.default_confidence_threshold"),
			MaxDailyNotifications:      viper.GetInt("trading.max_daily_notifications"),
			ScannerIntervalMinutes:     viper.GetInt("trading.scanner_interval_minutes"),
			OpportunityExpiryMinutes:   viper.GetInt("trading.opportunity_expiry_minutes"),
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

	// claude
	viper.SetDefault("claude.model", "claude-sonnet-4-20250514")
	viper.SetDefault("claude.max_tokens", 4096)

	// trading
	viper.SetDefault("trading.default_confidence_threshold", 80)
	viper.SetDefault("trading.max_daily_notifications", 10)
	viper.SetDefault("trading.scanner_interval_minutes", 5)
	viper.SetDefault("trading.opportunity_expiry_minutes", 15)

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
	return nil
}
