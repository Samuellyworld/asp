// backtesting execution engine — replays historical candles through a strategy,
// manages virtual positions, and collects trade results.
package backtest

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/trailingstop"
)

// Config controls the backtesting parameters.
type Config struct {
	Symbol         string
	Interval       string
	StartTime      time.Time
	EndTime        time.Time
	InitialCapital float64
	FeeRate        float64 // per-trade fee rate (e.g. 0.001 = 0.1%)
	MaxOpenTrades  int     // max concurrent positions (0 = 1)
	Slippage       float64 // simulated slippage as fraction (e.g. 0.0005 = 0.05%)
	TrailingStop   *TrailingStopConfig
	WindowSize     int // number of candles fed to strategy (0 = all available)
}

// TrailingStopConfig enables trailing stops on backtest positions.
type TrailingStopConfig struct {
	TrailPercent   float64 // trail distance as fraction
	ActivationPct  float64 // activation threshold as fraction (0 = immediate)
}

// Trade records a completed round-trip trade.
type Trade struct {
	EntryTime   time.Time
	ExitTime    time.Time
	Side        Action // BUY = long, SELL = short
	EntryPrice  float64
	ExitPrice   float64
	Quantity    float64
	EntryFee    float64
	ExitFee     float64
	PnL         float64
	PnLPercent  float64
	ExitReason  string
	Bars        int // number of candles held
}

// position tracks an open backtest position.
type position struct {
	side        Action
	entryPrice  float64
	quantity    float64
	stopLoss    float64
	takeProfit  float64
	entryTime   time.Time
	entryFee    float64
	entryBar    int
	trailing    *trailingstop.TrailingStop
}

// EquityPoint is a snapshot of equity at a given time.
type EquityPoint struct {
	Time   time.Time
	Equity float64
}

// Result holds the complete backtest output.
type Result struct {
	Config       Config
	Trades       []Trade
	EquityCurve  []EquityPoint
	FinalEquity  float64
	TotalCandles int
	Duration     time.Duration
}

// Engine runs backtests.
type Engine struct {
	config   Config
	loader   CandleLoader
	strategy Strategy
}

// NewEngine creates a backtesting engine.
func NewEngine(cfg Config, loader CandleLoader, strategy Strategy) *Engine {
	if cfg.MaxOpenTrades <= 0 {
		cfg.MaxOpenTrades = 1
	}
	return &Engine{config: cfg, loader: loader, strategy: strategy}
}

// Run executes the backtest and returns results.
func (e *Engine) Run(ctx context.Context) (*Result, error) {
	start := time.Now()

	candles, err := e.loader.LoadCandles(ctx, e.config.Symbol, e.config.Interval, e.config.StartTime, e.config.EndTime)
	if err != nil {
		return nil, fmt.Errorf("load candles: %w", err)
	}
	if len(candles) < 2 {
		return nil, fmt.Errorf("insufficient candles: got %d, need at least 2", len(candles))
	}

	capital := e.config.InitialCapital
	var positions []*position
	var trades []Trade
	equity := []EquityPoint{{Time: candles[0].OpenTime, Equity: capital}}

	for i := 0; i < len(candles); i++ {
		candle := candles[i]

		// check exits on existing positions
		positions, capital, trades = e.checkExits(positions, capital, trades, candle, i)

		// get strategy signal
		signal := e.getSignal(candles, i)

		// open new positions if signal and capacity
		if signal != nil && signal.Action != ActionHold && len(positions) < e.config.MaxOpenTrades {
			pos := e.openPosition(signal, candle, capital, i)
			if pos != nil {
				capital -= pos.quantity*pos.entryPrice + pos.entryFee
				positions = append(positions, pos)
			}
		}

		// record equity (capital + unrealized)
		unrealized := e.unrealizedPnL(positions, candle.Close)
		equity = append(equity, EquityPoint{
			Time:   candle.OpenTime,
			Equity: capital + unrealized,
		})
	}

	// force-close remaining positions at last candle
	lastCandle := candles[len(candles)-1]
	for _, pos := range positions {
		trade := e.closePosition(pos, lastCandle, len(candles)-1, "end_of_data")
		capital += trade.Quantity*trade.ExitPrice - trade.ExitFee
		trades = append(trades, trade)
	}

	return &Result{
		Config:       e.config,
		Trades:       trades,
		EquityCurve:  equity,
		FinalEquity:  capital,
		TotalCandles: len(candles),
		Duration:     time.Since(start),
	}, nil
}

func (e *Engine) getSignal(candles []exchange.Candle, idx int) *Signal {
	windowStart := 0
	if e.config.WindowSize > 0 && idx >= e.config.WindowSize {
		windowStart = idx - e.config.WindowSize + 1
	}
	// pass the full slice but use the adjusted index
	return e.strategy.OnCandle(candles[windowStart:idx+1], idx-windowStart)
}

func (e *Engine) openPosition(signal *Signal, candle exchange.Candle, capital float64, bar int) *position {
	allocSize := signal.Size
	if allocSize <= 0 || allocSize > 1 {
		allocSize = 0.1 // default 10%
	}

	available := capital * allocSize
	if available <= 0 {
		return nil
	}

	price := candle.Close
	// apply slippage
	if signal.Action == ActionBuy {
		price *= (1 + e.config.Slippage)
	} else {
		price *= (1 - e.config.Slippage)
	}

	fee := available * e.config.FeeRate
	qty := (available - fee) / price
	if qty <= 0 {
		return nil
	}

	pos := &position{
		side:       signal.Action,
		entryPrice: price,
		quantity:   qty,
		stopLoss:   signal.StopLoss,
		takeProfit: signal.TakeProfit,
		entryTime:  candle.OpenTime,
		entryFee:   fee,
		entryBar:   bar,
	}

	if e.config.TrailingStop != nil {
		ts := &trailingstop.TrailingStop{
			TrailPercent:  e.config.TrailingStop.TrailPercent * 100, // convert fraction to percent
			ActivationPct: e.config.TrailingStop.ActivationPct * 100,
		}
		pos.trailing = ts
	}

	return pos
}

func (e *Engine) checkExits(positions []*position, capital float64, trades []Trade, candle exchange.Candle, bar int) ([]*position, float64, []Trade) {
	var remaining []*position

	for _, pos := range positions {
		reason := ""
		exitPrice := 0.0

		if pos.side == ActionBuy {
			// long position
			if pos.stopLoss > 0 && candle.Low <= pos.stopLoss {
				reason = "stop_loss"
				exitPrice = pos.stopLoss
			} else if pos.takeProfit > 0 && candle.High >= pos.takeProfit {
				reason = "take_profit"
				exitPrice = pos.takeProfit
			}

			// trailing stop
			if pos.trailing != nil {
				newStop, _ := pos.trailing.UpdateLong(pos.entryPrice, candle.High)
				if newStop > 0 && pos.trailing.IsHitLong(candle.Low) && reason == "" {
					reason = "trailing_stop"
					exitPrice = pos.trailing.StopPrice
				}
			}
		} else {
			// short position
			if pos.stopLoss > 0 && candle.High >= pos.stopLoss {
				reason = "stop_loss"
				exitPrice = pos.stopLoss
			} else if pos.takeProfit > 0 && candle.Low <= pos.takeProfit {
				reason = "take_profit"
				exitPrice = pos.takeProfit
			}

			if pos.trailing != nil {
				newStop, _ := pos.trailing.UpdateShort(pos.entryPrice, candle.Low)
				if newStop > 0 && pos.trailing.IsHitShort(candle.High) && reason == "" {
					reason = "trailing_stop"
					exitPrice = pos.trailing.StopPrice
				}
			}
		}

		if reason != "" {
			trade := e.closePosition(pos, exchange.Candle{OpenTime: candle.OpenTime, Close: exitPrice}, bar, reason)
			capital += trade.Quantity*trade.ExitPrice - trade.ExitFee
			trades = append(trades, trade)
		} else {
			remaining = append(remaining, pos)
		}
	}

	return remaining, capital, trades
}

func (e *Engine) closePosition(pos *position, candle exchange.Candle, bar int, reason string) Trade {
	exitPrice := candle.Close
	// apply slippage on exit
	if pos.side == ActionBuy {
		exitPrice *= (1 - e.config.Slippage)
	} else {
		exitPrice *= (1 + e.config.Slippage)
	}

	exitFee := pos.quantity * exitPrice * e.config.FeeRate

	var pnl float64
	if pos.side == ActionBuy {
		pnl = (exitPrice-pos.entryPrice)*pos.quantity - pos.entryFee - exitFee
	} else {
		pnl = (pos.entryPrice-exitPrice)*pos.quantity - pos.entryFee - exitFee
	}
	pnlPct := pnl / (pos.entryPrice * pos.quantity) * 100

	return Trade{
		EntryTime:  pos.entryTime,
		ExitTime:   candle.OpenTime,
		Side:       pos.side,
		EntryPrice: pos.entryPrice,
		ExitPrice:  exitPrice,
		Quantity:   pos.quantity,
		EntryFee:   pos.entryFee,
		ExitFee:    exitFee,
		PnL:        pnl,
		PnLPercent: pnlPct,
		ExitReason: reason,
		Bars:       bar - pos.entryBar,
	}
}

func (e *Engine) unrealizedPnL(positions []*position, currentPrice float64) float64 {
	total := 0.0
	for _, pos := range positions {
		notional := pos.quantity * currentPrice
		exitFee := notional * e.config.FeeRate
		if pos.side == ActionBuy {
			total += (currentPrice-pos.entryPrice)*pos.quantity - pos.entryFee - exitFee
		} else {
			total += (pos.entryPrice-currentPrice)*pos.quantity - pos.entryFee - exitFee
		}
	}
	return total
}

// positionValue returns the total notional value of open positions.
func positionValue(positions []*position) float64 {
	total := 0.0
	for _, pos := range positions {
		total += pos.quantity * pos.entryPrice
	}
	return total
}

// abs returns the absolute value of a float64.
func abs(x float64) float64 {
	return math.Abs(x)
}
