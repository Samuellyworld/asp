package database

import (
	"testing"
	"time"
)

func TestTradeRecord_Fields(t *testing.T) {
	rec := &TradeRecord{
		UserID:          1,
		Symbol:          "BTC/USDT",
		Side:            "BUY",
		TradeType:       "SPOT",
		Quantity:        0.5,
		Price:           42000,
		Fee:             0.42,
		FeeCurrency:     "USDT",
		IsPaper:         true,
		ExecutedAt:      time.Now(),
	}

	if rec.Side != "BUY" {
		t.Errorf("expected BUY, got %s", rec.Side)
	}
	if rec.TradeType != "SPOT" {
		t.Errorf("expected SPOT, got %s", rec.TradeType)
	}
	if rec.IsPaper != true {
		t.Error("expected paper trade")
	}
}

func TestTradeRecord_NullablePositionID(t *testing.T) {
	rec := &TradeRecord{
		UserID:     1,
		Symbol:     "ETH/USDT",
		Side:       "SELL",
		TradeType:  "SPOT",
		Quantity:   2.0,
		Price:      3000,
		ExecutedAt: time.Now(),
	}

	if rec.PositionID != nil {
		t.Error("position ID should be nil when not linked")
	}

	posID := 42
	rec.PositionID = &posID
	if *rec.PositionID != 42 {
		t.Errorf("expected position ID 42, got %d", *rec.PositionID)
	}
}

func TestTradeRecord_FuturesTypes(t *testing.T) {
	cases := []struct {
		name      string
		tradeType string
	}{
		{"spot", "SPOT"},
		{"futures long", "FUTURES_LONG"},
		{"futures short", "FUTURES_SHORT"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &TradeRecord{
				TradeType:  tc.tradeType,
				ExecutedAt: time.Now(),
			}
			if rec.TradeType != tc.tradeType {
				t.Errorf("expected %s, got %s", tc.tradeType, rec.TradeType)
			}
		})
	}
}

func TestNewTradeRepository(t *testing.T) {
	repo := NewTradeRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}
