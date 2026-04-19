// prometheus metrics definitions and registration
package cmd

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// trade execution metrics
	tradesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trading_bot_trades_total",
		Help: "Total trades executed",
	}, []string{"executor", "direction", "status"}) // executor=paper|live|leverage_paper|leverage_live, status=success|error

	tradePnL = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "trading_bot_trade_pnl_pct",
		Help:    "Trade PnL percentage distribution",
		Buckets: []float64{-10, -5, -2, -1, -0.5, 0, 0.5, 1, 2, 5, 10, 20},
	}, []string{"executor"})

	openPositions = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "trading_bot_open_positions",
		Help: "Number of currently open positions",
	}, []string{"executor"})

	// scanner / opportunity metrics
	scansTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "trading_bot_scans_total",
		Help: "Total scanner scan cycles executed",
	})

	opportunitiesFound = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trading_bot_opportunities_found_total",
		Help: "Total trading opportunities detected",
	}, []string{"action"}) // action=buy|sell|hold

	// ai / pipeline metrics
	claudeRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trading_bot_claude_requests_total",
		Help: "Total Claude API requests",
	}, []string{"status"}) // status=success|error

	claudeLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "trading_bot_claude_latency_seconds",
		Help:    "Claude API request latency",
		Buckets: prometheus.DefBuckets,
	})

	mlRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trading_bot_ml_requests_total",
		Help: "Total ML service requests",
	}, []string{"endpoint", "status"})

	// infrastructure metrics
	dbQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "trading_bot_db_query_seconds",
		Help:    "Database query latency",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation"})

	dbErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "trading_bot_db_errors_total",
		Help: "Total database errors",
	})

	circuitBreakerState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "trading_bot_circuit_breaker_state",
		Help: "Circuit breaker state (0=closed, 1=open)",
	}, []string{"breaker"}) // breaker=portfolio|db

	// websocket metrics
	wsConnected = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "trading_bot_ws_connected",
		Help: "WebSocket connection status (0=disconnected, 1=connected)",
	})

	wsPriceUpdates = promauto.NewCounter(prometheus.CounterOpts{
		Name: "trading_bot_ws_price_updates_total",
		Help: "Total WebSocket price updates received",
	})

	// data ingestion metrics
	candlesIngested = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trading_bot_candles_ingested_total",
		Help: "Total candles ingested from exchange",
	}, []string{"interval"})

	// exchange rate limiting
	exchangeRateLimitHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trading_bot_exchange_rate_limit_hits_total",
		Help: "Times exchange rate limit was hit",
	}, []string{"exchange"})

	// slippage tracking
	slippageBps = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "trading_bot_slippage_bps",
		Help:    "Order slippage in basis points",
		Buckets: []float64{0, 1, 2, 5, 10, 20, 50, 100},
	})

	// notification metrics
	notificationsSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trading_bot_notifications_sent_total",
		Help: "Total notifications sent",
	}, []string{"channel", "status"}) // channel=telegram|discord|whatsapp

	// watchdog alerts
	infraAlerts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trading_bot_infra_alerts_total",
		Help: "Total infrastructure alert events",
	}, []string{"service", "type"}) // type=down|recovery
)
