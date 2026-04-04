package database

import (
	"testing"
	"time"
)

func TestCandleRecord_Fields(t *testing.T) {
	rec := &CandleRecord{
		Time:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Symbol:      "BTC/USDT",
		Interval:    "4h",
		Open:        42000,
		High:        42500,
		Low:         41800,
		Close:       42300,
		Volume:      1500,
		QuoteVolume: 63000000,
		TradeCount:  5000,
	}

	if rec.Symbol != "BTC/USDT" {
		t.Errorf("expected BTC/USDT, got %s", rec.Symbol)
	}
	if rec.Interval != "4h" {
		t.Errorf("expected 4h, got %s", rec.Interval)
	}
	if rec.High < rec.Low {
		t.Error("high should be >= low")
	}
}

func TestCandleRecord_ValidIntervals(t *testing.T) {
	valid := []string{"1m", "5m", "15m", "1h", "4h", "1d"}
	for _, interval := range valid {
		t.Run(interval, func(t *testing.T) {
			rec := &CandleRecord{Interval: interval}
			if rec.Interval != interval {
				t.Errorf("expected %s, got %s", interval, rec.Interval)
			}
		})
	}
}

func TestNewCandleRepository(t *testing.T) {
	repo := NewCandleRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

func TestFundingRateRecord_Fields(t *testing.T) {
	rec := &FundingRateRecord{
		Time:      time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC),
		Symbol:    "BTCUSDT",
		Rate:      0.0001,
		MarkPrice: 42000,
	}

	if rec.Rate != 0.0001 {
		t.Errorf("expected rate 0.0001, got %f", rec.Rate)
	}
	if rec.MarkPrice != 42000 {
		t.Errorf("expected mark price 42000, got %f", rec.MarkPrice)
	}
}

func TestNewFundingRateRepository(t *testing.T) {
	repo := NewFundingRateRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}
