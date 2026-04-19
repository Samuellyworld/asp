// ai subcommand — interactive analysis of a single symbol
package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/trading-bot/go-bot/internal/analysis"
	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/config"
	mlclient "github.com/trading-bot/go-bot/internal/ml-client"
	"github.com/trading-bot/go-bot/internal/pipeline"
)

var aiCmd = &cobra.Command{
	Use:   "ai",
	Short: "AI analysis commands",
}

var aiAnalyzeCmd = &cobra.Command{
	Use:   "analyze [SYMBOL]",
	Short: "run full analysis pipeline on a symbol (e.g. BTC/USDT)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		symbol := args[0]

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// binance client
		binanceClient := binance.NewClient(cfg.Binance.APIURL(), cfg.Binance.Testnet)

		// rust indicators (optional)
		var indicatorProvider pipeline.IndicatorProvider
		if cfg.RustEngine.Address != "" {
			grpcClient, err := analysis.NewClient(cfg.RustEngine.Address)
			if err != nil {
				fmt.Printf("⚠️  Rust engine unavailable: %v\n", err)
			} else {
				defer grpcClient.Close()
				indicatorProvider = grpcClient
			}
		}

		// ml service (optional)
		var mlProvider pipeline.MLProvider
		if cfg.MLService.BaseURL != "" {
			mlProvider = mlclient.NewClient(cfg.MLService.BaseURL)
		}

		// claude (required)
		var aiProvider pipeline.AIProvider
		if cfg.Claude.APIKey != "" {
			aiProvider = claude.NewClient(
				cfg.Claude.APIKey,
				claude.WithModel(cfg.Claude.Model),
				claude.WithMaxTokens(cfg.Claude.MaxTokens),
			)
		} else {
			return fmt.Errorf("CLAUDE_API_KEY is required for ai analyze")
		}

		pipe := pipeline.New(binanceClient, indicatorProvider, mlProvider, aiProvider)
		pipe.SetTimeframes(cfg.Trading.Timeframes)

		fmt.Printf("🔍 Analyzing %s...\n\n", symbol)

		result, err := pipe.Analyze(ctx, symbol)
		if err != nil {
			return fmt.Errorf("analysis failed: %w", err)
		}

		// print results
		if result.Ticker != nil {
			fmt.Printf("💰 Price: $%.2f | 24h: %.2f%%\n", result.Ticker.Price, result.Ticker.ChangePct)
		}

		if result.Indicators != nil {
			rsiVal := 0.0
			if result.Indicators.RSI != nil {
				rsiVal = result.Indicators.RSI.Value
			}
			macdSig := 0.0
			if result.Indicators.MACD != nil {
				macdSig = result.Indicators.MACD.SignalLine
			}
			fmt.Printf("📊 RSI: %.1f | MACD Signal: %.4f\n", rsiVal, macdSig)
		}

		if result.Prediction != nil {
			fmt.Printf("🧠 ML: %s %.1f%% (confidence: %.0f%%)\n",
				result.Prediction.Direction, result.Prediction.Magnitude, result.Prediction.Confidence*100)
		}

		if result.Sentiment != nil {
			fmt.Printf("💬 Sentiment: %s (%.2f, confidence: %.0f%%)\n",
				result.Sentiment.Label, result.Sentiment.Score, result.Sentiment.Confidence*100)
		}

		if result.Decision != nil {
			d := result.Decision
			fmt.Printf("\n🤖 Decision: %s | Confidence: %.0f%%\n", d.Action, d.Confidence)
			if d.Plan.Entry > 0 {
				fmt.Printf("   Entry: $%.2f | SL: $%.2f | TP: $%.2f\n", d.Plan.Entry, d.Plan.StopLoss, d.Plan.TakeProfit)
				fmt.Printf("   Position: $%.0f | R/R: 1:%.1f\n", d.Plan.PositionSize, d.Plan.RiskReward)
			}
			if d.Reasoning != "" {
				fmt.Printf("   Reasoning: %s\n", d.Reasoning)
			}
		}

		fmt.Printf("\n⏱️  Pipeline latency: %s\n", result.Latency.Round(time.Millisecond))

		if len(result.Errors) > 0 {
			fmt.Printf("⚠️  Warnings: %v\n", result.Errors)
		}

		return nil
	},
}

func init() {
	aiCmd.AddCommand(aiAnalyzeCmd)
	rootCmd.AddCommand(aiCmd)
}
