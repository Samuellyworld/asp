package config

import (
	"testing"
	"time"
)

func TestDatabaseConfig_DSN(t *testing.T) {
	tests := []struct {
		name string
		cfg  DatabaseConfig
		want string
	}{
		{
			name: "default values",
			cfg: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "trading_bot",
				Password: "secret",
				Name:     "trading_bot",
				SSLMode:  "disable",
			},
			want: "postgres://trading_bot:secret@localhost:5432/trading_bot?sslmode=disable",
		},
		{
			name: "custom host and port",
			cfg: DatabaseConfig{
				Host:     "db.example.com",
				Port:     5433,
				User:     "user",
				Password: "pass",
				Name:     "mydb",
				SSLMode:  "require",
			},
			want: "postgres://user:pass@db.example.com:5433/mydb?sslmode=require",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.DSN()
			if got != tt.want {
				t.Errorf("DSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRedisConfig_Addr(t *testing.T) {
	tests := []struct {
		name string
		cfg  RedisConfig
		want string
	}{
		{
			name: "default",
			cfg:  RedisConfig{Host: "localhost", Port: 6379},
			want: "localhost:6379",
		},
		{
			name: "custom port",
			cfg:  RedisConfig{Host: "redis.local", Port: 6380},
			want: "redis.local:6380",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.Addr()
			if got != tt.want {
				t.Errorf("Addr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBinanceConfig_APIURL(t *testing.T) {
	cfg := BinanceConfig{
		Testnet:       true,
		TestnetAPIURL: "https://testnet.binance.vision",
		MainnetAPIURL: "https://api.binance.com",
	}

	if got := cfg.APIURL(); got != "https://testnet.binance.vision" {
		t.Errorf("APIURL() with testnet=true = %q, want testnet url", got)
	}

	cfg.Testnet = false
	if got := cfg.APIURL(); got != "https://api.binance.com" {
		t.Errorf("APIURL() with testnet=false = %q, want mainnet url", got)
	}
}

func TestBinanceConfig_WSURL(t *testing.T) {
	cfg := BinanceConfig{
		Testnet:      true,
		TestnetWSURL: "wss://testnet.binance.vision/ws",
		MainnetWSURL: "wss://stream.binance.com:9443/ws",
	}

	if got := cfg.WSURL(); got != "wss://testnet.binance.vision/ws" {
		t.Errorf("WSURL() with testnet=true = %q, want testnet ws url", got)
	}

	cfg.Testnet = false
	if got := cfg.WSURL(); got != "wss://stream.binance.com:9443/ws" {
		t.Errorf("WSURL() with testnet=false = %q, want mainnet ws url", got)
	}
}

func TestTradingConfig_ScannerInterval(t *testing.T) {
	cfg := TradingConfig{ScannerIntervalMinutes: 5}
	want := 5 * time.Minute
	if got := cfg.ScannerInterval(); got != want {
		t.Errorf("ScannerInterval() = %v, want %v", got, want)
	}
}

func TestTradingConfig_OpportunityExpiry(t *testing.T) {
	cfg := TradingConfig{OpportunityExpiryMinutes: 15}
	want := 15 * time.Minute
	if got := cfg.OpportunityExpiry(); got != want {
		t.Errorf("OpportunityExpiry() = %v, want %v", got, want)
	}
}

func validConfig() *Config {
	return &Config{
		Security: SecurityConfig{MasterKey: "this-is-a-valid-master-key-that-is-long-enough"},
		Leverage: LeverageConfig{
			HardMaxLeverage:         20,
			MaxMarginPerTrade:       100,
			LiquidationWarningPct:   15,
			LiquidationCriticalPct:  10,
			LiquidationAutoClosePct: 5,
		},
		Trading: TradingConfig{
			DefaultConfidenceThreshold: 80,
			Timeframes:                 []string{"4h", "1d"},
		},
		Database: DatabaseConfig{Host: "localhost", Port: 5432},
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(cfg *Config)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config",
			modify:  func(cfg *Config) {},
			wantErr: false,
		},
		{
			name:    "empty master key",
			modify:  func(cfg *Config) { cfg.Security.MasterKey = "" },
			wantErr: true,
			errMsg:  "security.master_key is required",
		},
		{
			name:    "master key too short",
			modify:  func(cfg *Config) { cfg.Security.MasterKey = "short" },
			wantErr: true,
			errMsg:  "security.master_key must be at least 32 characters",
		},
		{
			name:    "exactly 32 characters",
			modify:  func(cfg *Config) { cfg.Security.MasterKey = "12345678901234567890123456789012" },
			wantErr: false,
		},
		{
			name:    "leverage too high",
			modify:  func(cfg *Config) { cfg.Leverage.HardMaxLeverage = 200 },
			wantErr: true,
			errMsg:  "leverage.hard_max_leverage must be between 1 and 125, got 200",
		},
		{
			name:    "leverage zero",
			modify:  func(cfg *Config) { cfg.Leverage.HardMaxLeverage = 0 },
			wantErr: true,
			errMsg:  "leverage.hard_max_leverage must be between 1 and 125, got 0",
		},
		{
			name:    "max margin negative",
			modify:  func(cfg *Config) { cfg.Leverage.MaxMarginPerTrade = -1 },
			wantErr: true,
			errMsg:  "leverage.max_margin_per_trade must be positive, got -1.00",
		},
		{
			name:   "liquidation pct order violation",
			modify: func(cfg *Config) { cfg.Leverage.LiquidationCriticalPct = 20 },
			wantErr: true,
		},
		{
			name:    "empty timeframes",
			modify:  func(cfg *Config) { cfg.Trading.Timeframes = nil },
			wantErr: true,
			errMsg:  "trading.timeframes must have at least one entry",
		},
		{
			name:    "invalid timeframe",
			modify:  func(cfg *Config) { cfg.Trading.Timeframes = []string{"4h", "2d"} },
			wantErr: true,
			errMsg:  "trading.timeframes contains invalid timeframe \"2d\"",
		},
		{
			name:    "empty database host",
			modify:  func(cfg *Config) { cfg.Database.Host = "" },
			wantErr: true,
			errMsg:  "database.host is required",
		},
		{
			name:    "invalid database port",
			modify:  func(cfg *Config) { cfg.Database.Port = 0 },
			wantErr: true,
			errMsg:  "database.port must be 1-65535, got 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(cfg)
			err := Validate(cfg)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error, got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	// check database defaults
	if cfg.Database.Host != "localhost" {
		t.Errorf("database.host = %q, want %q", cfg.Database.Host, "localhost")
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("database.port = %d, want %d", cfg.Database.Port, 5432)
	}

	// check redis defaults
	if cfg.Redis.Host != "localhost" {
		t.Errorf("redis.host = %q, want %q", cfg.Redis.Host, "localhost")
	}

	// check binance defaults
	if !cfg.Binance.Testnet {
		t.Error("binance.testnet should default to true")
	}

	// check trading defaults
	if cfg.Trading.DefaultConfidenceThreshold != 80 {
		t.Errorf("trading.default_confidence_threshold = %d, want %d", cfg.Trading.DefaultConfidenceThreshold, 80)
	}
	if cfg.Trading.ScannerIntervalMinutes != 5 {
		t.Errorf("trading.scanner_interval_minutes = %d, want %d", cfg.Trading.ScannerIntervalMinutes, 5)
	}

	// check log level default
	if cfg.LogLevel != "info" {
		t.Errorf("log_level = %q, want %q", cfg.LogLevel, "info")
	}
}
