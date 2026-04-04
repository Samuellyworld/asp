package backtest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// --- SliceLoader tests ---

func TestSliceLoader_LoadCandles(t *testing.T) {
	candles := generateCandles(100, 50000, time.Hour)
	loader := NewSliceLoader(candles)

	got, err := loader.LoadCandles(context.Background(), "BTC/USDT", "1h", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 100 {
		t.Fatalf("expected 100 candles, got %d", len(got))
	}
}

func TestSliceLoader_FiltersByDateRange(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]exchange.Candle, 30)
	for i := range candles {
		candles[i] = exchange.Candle{
			OpenTime: start.Add(time.Duration(i) * 24 * time.Hour),
			Open:     100, High: 110, Low: 90, Close: 105, Volume: 1000,
		}
	}

	loader := NewSliceLoader(candles)

	from := start.Add(10 * 24 * time.Hour)
	to := start.Add(19 * 24 * time.Hour)
	got, err := loader.LoadCandles(context.Background(), "ETH/USDT", "1d", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 {
		t.Fatalf("expected 10 candles in range, got %d", len(got))
	}
	if got[0].OpenTime != from {
		t.Errorf("first candle time = %v, want %v", got[0].OpenTime, from)
	}
}

// --- CSVLoader tests ---

func TestCSVLoader_ParsesUnixMS(t *testing.T) {
	csv := "time,open,high,low,close,volume\n" +
		"1704067200000,42000.0,42500.0,41500.0,42200.0,100.0\n" +
		"1704070800000,42200.0,42800.0,42100.0,42700.0,120.0\n"

	candles, err := parseCSVCandles(strings.NewReader(csv), "unix_ms", true, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(candles) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(candles))
	}
	if candles[0].Open != 42000.0 {
		t.Errorf("open = %f, want 42000", candles[0].Open)
	}
	if candles[1].Close != 42700.0 {
		t.Errorf("close = %f, want 42700", candles[1].Close)
	}
}

func TestCSVLoader_ParsesRFC3339(t *testing.T) {
	csv := "2024-01-01T00:00:00Z,42000.0,42500.0,41500.0,42200.0,100.0\n"

	candles, err := parseCSVCandles(strings.NewReader(csv), "rfc3339", false, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(candles) != 1 {
		t.Fatalf("expected 1 candle, got %d", len(candles))
	}
	if candles[0].Close != 42200.0 {
		t.Errorf("close = %f, want 42200", candles[0].Close)
	}
}

func TestCSVLoader_FiltersDateRange(t *testing.T) {
	csv := "time,open,high,low,close,volume\n" +
		"1704067200000,100,110,90,105,100\n" + // Jan 1
		"1704153600000,105,115,95,110,100\n" + // Jan 2
		"1704240000000,110,120,100,115,100\n" // Jan 3

	from := time.UnixMilli(1704153600000) // Jan 2
	to := time.UnixMilli(1704153600000)   // Jan 2

	candles, err := parseCSVCandles(strings.NewReader(csv), "unix_ms", true, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(candles) != 1 {
		t.Fatalf("expected 1 candle, got %d", len(candles))
	}
}

func TestCSVLoader_InvalidData(t *testing.T) {
	csv := "time,open,high,low,close,volume\n" +
		"not_a_number,100,110,90,105,100\n"

	_, err := parseCSVCandles(strings.NewReader(csv), "unix_ms", true, time.Time{}, time.Time{})
	if err == nil {
		t.Fatal("expected error for invalid time")
	}
}

func TestCSVLoader_SkipsShortRows(t *testing.T) {
	csv := "time,open,high,low,close,volume\n" +
		"1704067200000,100\n" + // too short, skipped
		"1704153600000,105,115,95,110,100\n"

	candles, err := parseCSVCandles(strings.NewReader(csv), "unix_ms", true, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(candles) != 1 {
		t.Fatalf("expected 1 candle (short row skipped), got %d", len(candles))
	}
}

// --- BinanceLoader tests ---

type mockFetcher struct {
	candles []exchange.Candle
	err     error
}

func (m *mockFetcher) GetCandles(_ context.Context, _, _ string, _ int) ([]exchange.Candle, error) {
	return m.candles, m.err
}

func TestBinanceLoader_FiltersDate(t *testing.T) {
	candles := generateCandles(50, 40000, time.Hour)
	from := candles[10].OpenTime
	to := candles[39].OpenTime

	loader := NewBinanceLoader(&mockFetcher{candles: candles})
	got, err := loader.LoadCandles(context.Background(), "BTC/USDT", "1h", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 30 {
		t.Fatalf("expected 30 candles, got %d", len(got))
	}
}

func TestBinanceLoader_Error(t *testing.T) {
	loader := NewBinanceLoader(&mockFetcher{err: context.DeadlineExceeded})
	_, err := loader.LoadCandles(context.Background(), "BTC/USDT", "1h", time.Time{}, time.Time{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- helpers ---

func generateCandles(n int, startPrice float64, interval time.Duration) []exchange.Candle {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	candles := make([]exchange.Candle, n)
	price := startPrice

	for i := range candles {
		// oscillating price pattern for strategy testing
		delta := float64(i%7-3) * 100
		open := price
		close := price + delta
		high := open + 200
		low := open - 200
		if close > high {
			high = close + 50
		}
		if close < low {
			low = close - 50
		}

		candles[i] = exchange.Candle{
			OpenTime: base.Add(time.Duration(i) * interval),
			Open:     open,
			High:     high,
			Low:      low,
			Close:    close,
			Volume:   1000 + float64(i)*10,
		}
		price = close
	}
	return candles
}
