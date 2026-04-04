// backtesting strategy interface and built-in strategies.
package backtest

import (
	"github.com/trading-bot/go-bot/internal/exchange"
)

// Action represents a trade signal action.
type Action string

const (
	ActionBuy  Action = "BUY"
	ActionSell Action = "SELL"
	ActionHold Action = "HOLD"
)

// Signal is emitted by a strategy on each candle.
type Signal struct {
	Action     Action
	Entry      float64 // desired entry price (0 = market)
	StopLoss   float64
	TakeProfit float64
	Size       float64 // fraction of capital to allocate, 0-1
	Reason     string
}

// Strategy evaluates a candle window and returns a trade signal.
type Strategy interface {
	Name() string
	// OnCandle receives the full candle history up to the current bar and returns a signal.
	OnCandle(candles []exchange.Candle, currentIndex int) *Signal
}

// SMACrossover implements a simple moving average crossover strategy.
type SMACrossover struct {
	FastPeriod int
	SlowPeriod int
	StopPct    float64 // stop loss as fraction (e.g. 0.02 = 2%)
	TargetPct  float64 // take profit as fraction
	PositionPct float64 // fraction of capital per trade
}

func NewSMACrossover(fast, slow int, stopPct, targetPct, positionPct float64) *SMACrossover {
	return &SMACrossover{
		FastPeriod:  fast,
		SlowPeriod:  slow,
		StopPct:     stopPct,
		TargetPct:   targetPct,
		PositionPct: positionPct,
	}
}

func (s *SMACrossover) Name() string {
	return "sma-crossover"
}

func (s *SMACrossover) OnCandle(candles []exchange.Candle, idx int) *Signal {
	if idx < s.SlowPeriod {
		return nil
	}

	fastSMA := sma(candles, idx, s.FastPeriod)
	slowSMA := sma(candles, idx, s.SlowPeriod)
	prevFastSMA := sma(candles, idx-1, s.FastPeriod)
	prevSlowSMA := sma(candles, idx-1, s.SlowPeriod)

	price := candles[idx].Close

	// bullish crossover: fast crosses above slow
	if prevFastSMA <= prevSlowSMA && fastSMA > slowSMA {
		return &Signal{
			Action:     ActionBuy,
			Entry:      price,
			StopLoss:   price * (1 - s.StopPct),
			TakeProfit: price * (1 + s.TargetPct),
			Size:       s.PositionPct,
			Reason:     "SMA bullish crossover",
		}
	}

	// bearish crossover: fast crosses below slow
	if prevFastSMA >= prevSlowSMA && fastSMA < slowSMA {
		return &Signal{
			Action:     ActionSell,
			Entry:      price,
			StopLoss:   price * (1 + s.StopPct),
			TakeProfit: price * (1 - s.TargetPct),
			Size:       s.PositionPct,
			Reason:     "SMA bearish crossover",
		}
	}

	return nil
}

// RSIMeanReversion buys oversold and sells overbought.
type RSIMeanReversion struct {
	Period      int
	Oversold    float64 // e.g. 30
	Overbought  float64 // e.g. 70
	StopPct     float64
	TargetPct   float64
	PositionPct float64
}

func NewRSIMeanReversion(period int, oversold, overbought, stopPct, targetPct, positionPct float64) *RSIMeanReversion {
	return &RSIMeanReversion{
		Period:      period,
		Oversold:    oversold,
		Overbought:  overbought,
		StopPct:     stopPct,
		TargetPct:   targetPct,
		PositionPct: positionPct,
	}
}

func (r *RSIMeanReversion) Name() string {
	return "rsi-mean-reversion"
}

func (r *RSIMeanReversion) OnCandle(candles []exchange.Candle, idx int) *Signal {
	if idx < r.Period+1 {
		return nil
	}

	rsiVal := rsi(candles, idx, r.Period)
	price := candles[idx].Close

	if rsiVal < r.Oversold {
		return &Signal{
			Action:     ActionBuy,
			Entry:      price,
			StopLoss:   price * (1 - r.StopPct),
			TakeProfit: price * (1 + r.TargetPct),
			Size:       r.PositionPct,
			Reason:     "RSI oversold",
		}
	}

	if rsiVal > r.Overbought {
		return &Signal{
			Action:     ActionSell,
			Entry:      price,
			StopLoss:   price * (1 + r.StopPct),
			TakeProfit: price * (1 - r.TargetPct),
			Size:       r.PositionPct,
			Reason:     "RSI overbought",
		}
	}

	return nil
}

// --- indicator helpers ---

func sma(candles []exchange.Candle, endIdx, period int) float64 {
	if endIdx < period-1 {
		return 0
	}
	sum := 0.0
	for i := endIdx - period + 1; i <= endIdx; i++ {
		sum += candles[i].Close
	}
	return sum / float64(period)
}

func rsi(candles []exchange.Candle, endIdx, period int) float64 {
	if endIdx < period {
		return 50
	}

	var gains, losses float64
	for i := endIdx - period + 1; i <= endIdx; i++ {
		change := candles[i].Close - candles[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses -= change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}
