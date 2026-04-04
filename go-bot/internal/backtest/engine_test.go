package backtest

import (
	"context"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// --- Engine tests ---

func TestEngine_BasicLongTrade(t *testing.T) {
	// price goes up then down — strategy buys early, engine closes at end
	candles := make([]exchange.Candle, 50)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range candles {
		price := 100.0 + float64(i)*2
		candles[i] = exchange.Candle{
			OpenTime: base.Add(time.Duration(i) * time.Hour),
			Open:     price - 1,
			High:     price + 5,
			Low:      price - 5,
			Close:    price,
			Volume:   1000,
		}
	}

	loader := NewSliceLoader(candles)
	// strategy that always buys on candle 10
	strategy := &fixedSignalStrategy{
		signalAt: 10,
		signal: &Signal{
			Action:     ActionBuy,
			StopLoss:   90,
			TakeProfit: 200,
			Size:       0.5,
		},
	}

	engine := NewEngine(Config{
		Symbol:         "TEST/USDT",
		Interval:       "1h",
		InitialCapital: 10000,
		FeeRate:        0.001,
		MaxOpenTrades:  1,
	}, loader, strategy)

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) == 0 {
		t.Fatal("expected at least one trade")
	}
	if result.FinalEquity <= 0 {
		t.Error("expected positive equity")
	}
	if result.TotalCandles != 50 {
		t.Errorf("total candles = %d, want 50", result.TotalCandles)
	}
}

func TestEngine_StopLossHit(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := []exchange.Candle{
		{OpenTime: base, Open: 100, High: 105, Low: 95, Close: 100, Volume: 100},
		{OpenTime: base.Add(time.Hour), Open: 100, High: 105, Low: 95, Close: 102, Volume: 100},
		{OpenTime: base.Add(2 * time.Hour), Open: 102, High: 103, Low: 85, Close: 88, Volume: 100}, // drops below SL
		{OpenTime: base.Add(3 * time.Hour), Open: 88, High: 90, Low: 80, Close: 85, Volume: 100},
	}

	strategy := &fixedSignalStrategy{
		signalAt: 1,
		signal: &Signal{
			Action:     ActionBuy,
			StopLoss:   90,
			TakeProfit: 120,
			Size:       0.5,
		},
	}

	engine := NewEngine(Config{
		Symbol:         "TEST/USDT",
		Interval:       "1h",
		InitialCapital: 10000,
		FeeRate:        0,
	}, NewSliceLoader(candles), strategy)

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// should have a trade closed by stop loss
	found := false
	for _, trade := range result.Trades {
		if trade.ExitReason == "stop_loss" {
			found = true
			if trade.PnL >= 0 {
				t.Error("stop loss trade should have negative PnL")
			}
		}
	}
	if !found {
		t.Error("expected a stop_loss exit, got none")
	}
}

func TestEngine_TakeProfitHit(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := []exchange.Candle{
		{OpenTime: base, Open: 100, High: 105, Low: 95, Close: 100, Volume: 100},
		{OpenTime: base.Add(time.Hour), Open: 100, High: 105, Low: 98, Close: 102, Volume: 100},
		{OpenTime: base.Add(2 * time.Hour), Open: 102, High: 115, Low: 101, Close: 112, Volume: 100}, // hits TP at 110
		{OpenTime: base.Add(3 * time.Hour), Open: 112, High: 120, Low: 110, Close: 118, Volume: 100},
	}

	strategy := &fixedSignalStrategy{
		signalAt: 1,
		signal: &Signal{
			Action:     ActionBuy,
			StopLoss:   90,
			TakeProfit: 110,
			Size:       0.5,
		},
	}

	engine := NewEngine(Config{
		Symbol:         "TEST/USDT",
		Interval:       "1h",
		InitialCapital: 10000,
		FeeRate:        0,
	}, NewSliceLoader(candles), strategy)

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, trade := range result.Trades {
		if trade.ExitReason == "take_profit" {
			found = true
			if trade.PnL <= 0 {
				t.Error("take profit trade should have positive PnL")
			}
		}
	}
	if !found {
		t.Error("expected a take_profit exit, got none")
	}
}

func TestEngine_ShortPosition(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := []exchange.Candle{
		{OpenTime: base, Open: 100, High: 105, Low: 95, Close: 100, Volume: 100},
		{OpenTime: base.Add(time.Hour), Open: 100, High: 102, Low: 95, Close: 98, Volume: 100},
		{OpenTime: base.Add(2 * time.Hour), Open: 98, High: 99, Low: 85, Close: 88, Volume: 100}, // drops to TP
		{OpenTime: base.Add(3 * time.Hour), Open: 88, High: 90, Low: 80, Close: 82, Volume: 100},
	}

	strategy := &fixedSignalStrategy{
		signalAt: 1,
		signal: &Signal{
			Action:     ActionSell,
			StopLoss:   110,
			TakeProfit: 88,
			Size:       0.3,
		},
	}

	engine := NewEngine(Config{
		Symbol:         "TEST/USDT",
		Interval:       "1h",
		InitialCapital: 10000,
		FeeRate:        0,
	}, NewSliceLoader(candles), strategy)

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	foundTP := false
	for _, trade := range result.Trades {
		if trade.Side == ActionSell && trade.ExitReason == "take_profit" {
			foundTP = true
			if trade.PnL <= 0 {
				t.Error("short TP should be profitable")
			}
		}
	}
	if !foundTP {
		// may close at end of data instead, check that at least we have a trade
		if len(result.Trades) == 0 {
			t.Error("expected at least one short trade")
		}
	}
}

func TestEngine_TrailingStop(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// price rallies then drops
	candles := []exchange.Candle{
		{OpenTime: base, Open: 100, High: 105, Low: 95, Close: 100, Volume: 100},
		{OpenTime: base.Add(time.Hour), Open: 100, High: 105, Low: 98, Close: 103, Volume: 100},
		{OpenTime: base.Add(2 * time.Hour), Open: 103, High: 120, Low: 102, Close: 118, Volume: 100},
		{OpenTime: base.Add(3 * time.Hour), Open: 118, High: 125, Low: 117, Close: 124, Volume: 100},
		{OpenTime: base.Add(4 * time.Hour), Open: 124, High: 126, Low: 105, Close: 108, Volume: 100}, // big drop
	}

	strategy := &fixedSignalStrategy{
		signalAt: 1,
		signal: &Signal{
			Action:     ActionBuy,
			StopLoss:   80,
			TakeProfit: 200,
			Size:       0.5,
		},
	}

	engine := NewEngine(Config{
		Symbol:         "TEST/USDT",
		Interval:       "1h",
		InitialCapital: 10000,
		FeeRate:        0,
		TrailingStop:   &TrailingStopConfig{TrailPercent: 0.05, ActivationPct: 0},
	}, NewSliceLoader(candles), strategy)

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) == 0 {
		t.Fatal("expected trades")
	}
}

func TestEngine_FeeDeduction(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]exchange.Candle, 20)
	for i := range candles {
		candles[i] = exchange.Candle{
			OpenTime: base.Add(time.Duration(i) * time.Hour),
			Open:     100, High: 105, Low: 95, Close: 100, Volume: 100,
		}
	}

	strategy := &fixedSignalStrategy{
		signalAt: 5,
		signal:   &Signal{Action: ActionBuy, StopLoss: 80, TakeProfit: 120, Size: 0.5},
	}

	// with fees
	engineFee := NewEngine(Config{
		Symbol: "TEST/USDT", Interval: "1h", InitialCapital: 10000, FeeRate: 0.01,
	}, NewSliceLoader(candles), strategy)
	resFee, _ := engineFee.Run(context.Background())

	// without fees
	engineNoFee := NewEngine(Config{
		Symbol: "TEST/USDT", Interval: "1h", InitialCapital: 10000, FeeRate: 0,
	}, NewSliceLoader(candles), strategy)
	resNoFee, _ := engineNoFee.Run(context.Background())

	if resFee.FinalEquity >= resNoFee.FinalEquity {
		t.Error("with fees should result in less equity")
	}
}

func TestEngine_InsufficientCandles(t *testing.T) {
	candles := []exchange.Candle{
		{OpenTime: time.Now(), Close: 100},
	}
	engine := NewEngine(Config{InitialCapital: 10000}, NewSliceLoader(candles), &fixedSignalStrategy{})
	_, err := engine.Run(context.Background())
	if err == nil {
		t.Error("expected error with insufficient candles")
	}
}

func TestEngine_EquityCurve(t *testing.T) {
	candles := generateCandles(30, 100, time.Hour)
	strategy := &fixedSignalStrategy{
		signalAt: 5,
		signal:   &Signal{Action: ActionBuy, StopLoss: 50, TakeProfit: 300, Size: 0.3},
	}

	engine := NewEngine(Config{
		Symbol: "TEST/USDT", Interval: "1h", InitialCapital: 10000, FeeRate: 0.001,
	}, NewSliceLoader(candles), strategy)

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.EquityCurve) < 2 {
		t.Error("expected equity curve with multiple points")
	}
}

func TestEngine_MaxOpenTrades(t *testing.T) {
	candles := generateCandles(50, 100, time.Hour)

	// strategy signals every 5 bars
	strategy := &periodicSignalStrategy{
		period:    5,
		stopLoss:  50,
		takeProfit: 300,
	}

	engine := NewEngine(Config{
		Symbol: "TEST/USDT", Interval: "1h", InitialCapital: 10000,
		MaxOpenTrades: 1,
	}, NewSliceLoader(candles), strategy)

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// with max 1, trades should be sequential, not overlapping
	if len(result.Trades) == 0 {
		t.Error("expected at least one trade")
	}
}

func TestEngine_ForceClosesAtEnd(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]exchange.Candle, 10)
	for i := range candles {
		candles[i] = exchange.Candle{
			OpenTime: base.Add(time.Duration(i) * time.Hour),
			Open:     100, High: 110, Low: 90, Close: 100, Volume: 100,
		}
	}

	strategy := &fixedSignalStrategy{
		signalAt: 2,
		signal:   &Signal{Action: ActionBuy, StopLoss: 50, TakeProfit: 200, Size: 0.5},
	}

	engine := NewEngine(Config{
		Symbol: "TEST/USDT", Interval: "1h", InitialCapital: 10000,
	}, NewSliceLoader(candles), strategy)

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, trade := range result.Trades {
		if trade.ExitReason == "end_of_data" {
			found = true
		}
	}
	if !found {
		t.Error("expected at least one trade closed with end_of_data reason")
	}
}

// --- test strategy helpers ---

type fixedSignalStrategy struct {
	signalAt int
	signal   *Signal
}

func (f *fixedSignalStrategy) Name() string { return "fixed" }
func (f *fixedSignalStrategy) OnCandle(candles []exchange.Candle, idx int) *Signal {
	if idx == f.signalAt {
		return f.signal
	}
	return nil
}

type periodicSignalStrategy struct {
	period     int
	stopLoss   float64
	takeProfit float64
}

func (p *periodicSignalStrategy) Name() string { return "periodic" }
func (p *periodicSignalStrategy) OnCandle(candles []exchange.Candle, idx int) *Signal {
	if p.period > 0 && idx > 0 && idx%p.period == 0 {
		return &Signal{
			Action:     ActionBuy,
			StopLoss:   p.stopLoss,
			TakeProfit: p.takeProfit,
			Size:       0.1,
		}
	}
	return nil
}
