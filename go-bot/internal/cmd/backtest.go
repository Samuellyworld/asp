// backtest command — runs historical strategy backtests with performance reporting.
package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/trading-bot/go-bot/internal/backtest"
	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/config"
	"github.com/trading-bot/go-bot/internal/database"
)

var (
	btSymbol    string
	btInterval  string
	btStart     string
	btEnd       string
	btStrategy  string
	btCapital   float64
	btFeeRate   float64
	btSlippage  float64
	btSource    string // "db", "binance", "csv"
	btCSVFile   string
	btCSVFormat string
	btTrailPct  float64
)

var backtestCmd = &cobra.Command{
	Use:   "backtest",
	Short: "run a strategy backtest on historical data",
	Long: `Run a backtesting simulation on historical candle data.

Sources: database (db), binance api (binance), or csv file (csv).
Strategies: sma-crossover, rsi-mean-reversion.

Examples:
  bot backtest --symbol BTC/USDT --interval 4h --start 2024-01-01 --end 2024-12-31 --strategy sma-crossover
  bot backtest --source csv --csv-file data.csv --strategy rsi-mean-reversion --capital 50000`,
	RunE: runBacktest,
}

func init() {
	backtestCmd.Flags().StringVar(&btSymbol, "symbol", "BTC/USDT", "trading pair")
	backtestCmd.Flags().StringVar(&btInterval, "interval", "4h", "candle interval (1m,5m,15m,1h,4h,1d)")
	backtestCmd.Flags().StringVar(&btStart, "start", "", "start date (YYYY-MM-DD)")
	backtestCmd.Flags().StringVar(&btEnd, "end", "", "end date (YYYY-MM-DD)")
	backtestCmd.Flags().StringVar(&btStrategy, "strategy", "sma-crossover", "strategy name (sma-crossover, rsi-mean-reversion)")
	backtestCmd.Flags().Float64Var(&btCapital, "capital", 10000, "initial capital in USD")
	backtestCmd.Flags().Float64Var(&btFeeRate, "fee-rate", 0.001, "per-trade fee rate (0.001 = 0.1%)")
	backtestCmd.Flags().Float64Var(&btSlippage, "slippage", 0.0005, "simulated slippage (0.0005 = 0.05%)")
	backtestCmd.Flags().StringVar(&btSource, "source", "binance", "data source: db, binance, csv")
	backtestCmd.Flags().StringVar(&btCSVFile, "csv-file", "", "CSV file path (required for --source csv)")
	backtestCmd.Flags().StringVar(&btCSVFormat, "csv-format", "unix_ms", "CSV time format: unix_ms, rfc3339")
	backtestCmd.Flags().Float64Var(&btTrailPct, "trailing-stop", 0, "trailing stop percent (0 = disabled, e.g. 0.02 = 2%)")

	rootCmd.AddCommand(backtestCmd)
}

func runBacktest(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	startTime, endTime, err := parseDateRange(btStart, btEnd)
	if err != nil {
		return err
	}

	loader, cleanup, err := buildLoader(ctx, btSource)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	strategy, err := buildStrategy(btStrategy)
	if err != nil {
		return err
	}

	cfg := backtest.Config{
		Symbol:         btSymbol,
		Interval:       btInterval,
		StartTime:      startTime,
		EndTime:        endTime,
		InitialCapital: btCapital,
		FeeRate:        btFeeRate,
		Slippage:       btSlippage,
		MaxOpenTrades:  1,
	}
	if btTrailPct > 0 {
		cfg.TrailingStop = &backtest.TrailingStopConfig{
			TrailPercent:  btTrailPct,
			ActivationPct: 0,
		}
	}

	engine := backtest.NewEngine(cfg, loader, strategy)

	fmt.Printf("Running backtest: %s %s [%s]\n", btSymbol, btInterval, strategy.Name())
	fmt.Printf("Period: %s → %s | Capital: $%.2f | Fees: %.2f%%\n\n",
		startTime.Format("2006-01-02"), endTime.Format("2006-01-02"), btCapital, btFeeRate*100)

	result, err := engine.Run(ctx)
	if err != nil {
		return fmt.Errorf("backtest failed: %w", err)
	}

	metrics := backtest.ComputeMetrics(result)
	printReport(result, metrics)

	return nil
}

func parseDateRange(start, end string) (time.Time, time.Time, error) {
	layout := "2006-01-02"
	var startTime, endTime time.Time

	if start != "" {
		t, err := time.Parse(layout, start)
		if err != nil {
			return startTime, endTime, fmt.Errorf("invalid start date %q: %w", start, err)
		}
		startTime = t
	} else {
		startTime = time.Now().AddDate(0, -3, 0) // default: 3 months ago
	}

	if end != "" {
		t, err := time.Parse(layout, end)
		if err != nil {
			return startTime, endTime, fmt.Errorf("invalid end date %q: %w", end, err)
		}
		endTime = t
	} else {
		endTime = time.Now()
	}

	if !endTime.After(startTime) {
		return startTime, endTime, fmt.Errorf("end date must be after start date")
	}

	return startTime, endTime, nil
}

func buildLoader(ctx context.Context, source string) (backtest.CandleLoader, func(), error) {
	switch source {
	case "db":
		cfg, err := config.Load()
		if err != nil {
			return nil, nil, fmt.Errorf("load config for db: %w", err)
		}
		pg, err := database.NewPostgresClient(cfg.Database)
		if err != nil {
			return nil, nil, fmt.Errorf("connect to db: %w", err)
		}
		return backtest.NewDBLoader(pg.Pool()), pg.Close, nil

	case "binance":
		cfg, err := config.Load()
		if err != nil {
			return nil, nil, fmt.Errorf("load config: %w", err)
		}
		client := binance.NewClient(cfg.Binance.APIURL(), cfg.Binance.Testnet)
		return backtest.NewBinanceLoader(client), nil, nil

	case "csv":
		if btCSVFile == "" {
			return nil, nil, fmt.Errorf("--csv-file is required when --source csv")
		}
		return backtest.NewCSVLoader(btCSVFile, btCSVFormat, true), nil, nil

	default:
		return nil, nil, fmt.Errorf("unknown data source: %s (use db, binance, or csv)", source)
	}
}

func buildStrategy(name string) (backtest.Strategy, error) {
	switch name {
	case "sma-crossover":
		return backtest.NewSMACrossover(10, 30, 0.02, 0.04, 0.2), nil
	case "rsi-mean-reversion":
		return backtest.NewRSIMeanReversion(14, 30, 70, 0.02, 0.04, 0.2), nil
	default:
		return nil, fmt.Errorf("unknown strategy: %s (available: sma-crossover, rsi-mean-reversion)", name)
	}
}

func printReport(result *backtest.Result, m *backtest.Metrics) {
	sep := strings.Repeat("─", 55)

	fmt.Println(sep)
	fmt.Println("  BACKTEST RESULTS")
	fmt.Println(sep)

	fmt.Printf("  Symbol:          %s\n", result.Config.Symbol)
	fmt.Printf("  Strategy:        %s\n", result.Config.Interval)
	fmt.Printf("  Period:          %s → %s\n",
		m.StartDate.Format("2006-01-02"), m.EndDate.Format("2006-01-02"))
	fmt.Printf("  Candles:         %d\n", result.TotalCandles)
	fmt.Printf("  Execution Time:  %s\n", result.Duration.Round(time.Millisecond))

	fmt.Println(sep)
	fmt.Println("  RETURNS")
	fmt.Println(sep)

	fmt.Printf("  Initial Capital: $%.2f\n", result.Config.InitialCapital)
	fmt.Printf("  Final Equity:    $%.2f\n", result.FinalEquity)
	fmt.Printf("  Total Return:    $%.2f (%.2f%%)\n", m.TotalReturn, m.TotalReturnPct)
	fmt.Printf("  Annualized:      %.2f%%\n", m.AnnualizedReturn)
	fmt.Printf("  Total Fees:      $%.2f\n", m.TotalFees)

	fmt.Println(sep)
	fmt.Println("  TRADE STATISTICS")
	fmt.Println(sep)

	fmt.Printf("  Total Trades:    %d\n", m.TotalTrades)
	fmt.Printf("  Win Rate:        %.1f%% (%d W / %d L)\n", m.WinRate, m.WinningTrades, m.LosingTrades)
	fmt.Printf("  Profit Factor:   %.2f\n", m.ProfitFactor)
	fmt.Printf("  Avg Win:         $%.2f (%.2f%%)\n", m.AvgWin, m.AvgWinPct)
	fmt.Printf("  Avg Loss:        $%.2f (%.2f%%)\n", m.AvgLoss, m.AvgLossPct)
	fmt.Printf("  Largest Win:     $%.2f\n", m.LargestWin)
	fmt.Printf("  Largest Loss:    $%.2f\n", m.LargestLoss)
	fmt.Printf("  Avg Bars Held:   %.1f\n", m.AvgBarsHeld)
	fmt.Printf("  Max Consec Wins: %d\n", m.MaxConsecWins)
	fmt.Printf("  Max Consec Loss: %d\n", m.MaxConsecLosses)

	fmt.Println(sep)
	fmt.Println("  RISK METRICS")
	fmt.Println(sep)

	fmt.Printf("  Max Drawdown:    %.2f%% ($%.2f)\n", m.MaxDrawdown, m.MaxDrawdownUSD)
	fmt.Printf("  Sharpe Ratio:    %.2f\n", m.SharpeRatio)
	fmt.Printf("  Sortino Ratio:   %.2f\n", m.SortinoRatio)
	fmt.Printf("  Calmar Ratio:    %.2f\n", m.CalmarRatio)

	fmt.Println(sep)

	// top 5 trades
	if len(result.Trades) > 0 {
		fmt.Println("  TOP TRADES")
		fmt.Println(sep)
		sorted := backtest.TradesByPnL(result.Trades)
		count := 5
		if len(sorted) < count {
			count = len(sorted)
		}
		for i := 0; i < count; i++ {
			t := sorted[i]
			fmt.Printf("  %s %s→%.2f exit→%.2f  P&L: $%.2f (%.1f%%) [%s]\n",
				t.Side, t.EntryTime.Format("01/02"), t.EntryPrice, t.ExitPrice,
				t.PnL, t.PnLPercent, t.ExitReason)
		}
		fmt.Println(sep)
	}
}
