package pipeline

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// mock candle store
type mockCandleStore struct {
	mu       sync.Mutex
	candles  []*CandleRecord
	upsertCb func([]*CandleRecord) error
}

func (m *mockCandleStore) UpsertBatch(_ context.Context, candles []*CandleRecord) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.upsertCb != nil {
		if err := m.upsertCb(candles); err != nil {
			return 0, err
		}
	}
	m.candles = append(m.candles, candles...)
	return len(candles), nil
}

func (m *mockCandleStore) LatestTime(_ context.Context, _, _ string) (time.Time, error) {
	return time.Time{}, nil
}

func (m *mockCandleStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.candles)
}

// mock symbol provider
type mockSymbolProvider struct {
	symbols []string
	err     error
}

func (m *mockSymbolProvider) ActiveSymbols(_ context.Context) ([]string, error) {
	return m.symbols, m.err
}

// mock candle fetcher
type mockCandleFetcher struct {
	mu      sync.Mutex
	candles map[string][]exchange.Candle
	calls   int
	err     error
}

func (m *mockCandleFetcher) GetCandles(_ context.Context, symbol, interval string, limit int) ([]exchange.Candle, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}

	key := symbol + "_" + interval
	if c, ok := m.candles[key]; ok {
		if len(c) > limit {
			return c[:limit], nil
		}
		return c, nil
	}
	return nil, nil
}

func (m *mockCandleFetcher) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func makeCandles(n int, symbol string) []exchange.Candle {
	candles := make([]exchange.Candle, n)
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		candles[i] = exchange.Candle{
			OpenTime: baseTime.Add(time.Duration(i) * 4 * time.Hour),
			Open:     42000 + float64(i),
			High:     42100 + float64(i),
			Low:      41900 + float64(i),
			Close:    42050 + float64(i),
			Volume:   1000 + float64(i*10),
		}
	}
	return candles
}

func TestDataIngestion_IngestStoresCandles(t *testing.T) {
	store := &mockCandleStore{}
	fetcher := &mockCandleFetcher{
		candles: map[string][]exchange.Candle{
			"BTC/USDT_4h": makeCandles(10, "BTC/USDT"),
			"BTC/USDT_1h": makeCandles(20, "BTC/USDT"),
		},
	}
	symbols := &mockSymbolProvider{symbols: []string{"BTC/USDT"}}

	cfg := DefaultIngestionConfig()
	cfg.BatchSize = 100

	d := NewDataIngestion(fetcher, store, symbols, cfg)
	d.ingest(context.Background())

	if store.count() != 30 { // 10 + 20
		t.Errorf("expected 30 candles stored, got %d", store.count())
	}
	if fetcher.callCount() != 2 { // 2 intervals
		t.Errorf("expected 2 fetch calls, got %d", fetcher.callCount())
	}

	runCount, totalStored, lastRun := d.Stats()
	if runCount != 1 {
		t.Errorf("expected 1 run, got %d", runCount)
	}
	if totalStored != 30 {
		t.Errorf("expected 30 total stored, got %d", totalStored)
	}
	if lastRun.IsZero() {
		t.Error("lastRun should not be zero")
	}
}

func TestDataIngestion_MultipleSymbols(t *testing.T) {
	store := &mockCandleStore{}
	fetcher := &mockCandleFetcher{
		candles: map[string][]exchange.Candle{
			"BTC/USDT_4h": makeCandles(5, "BTC/USDT"),
			"BTC/USDT_1h": makeCandles(5, "BTC/USDT"),
			"ETH/USDT_4h": makeCandles(5, "ETH/USDT"),
			"ETH/USDT_1h": makeCandles(5, "ETH/USDT"),
		},
	}
	symbols := &mockSymbolProvider{symbols: []string{"BTC/USDT", "ETH/USDT"}}

	cfg := DefaultIngestionConfig()
	d := NewDataIngestion(fetcher, store, symbols, cfg)
	d.ingest(context.Background())

	if store.count() != 20 { // 4 * 5
		t.Errorf("expected 20 candles, got %d", store.count())
	}
	if fetcher.callCount() != 4 { // 2 symbols * 2 intervals
		t.Errorf("expected 4 fetch calls, got %d", fetcher.callCount())
	}
}

func TestDataIngestion_FetchErrorContinues(t *testing.T) {
	store := &mockCandleStore{}
	fetcher := &mockCandleFetcher{
		err: fmt.Errorf("exchange down"),
	}
	symbols := &mockSymbolProvider{symbols: []string{"BTC/USDT"}}

	cfg := DefaultIngestionConfig()
	d := NewDataIngestion(fetcher, store, symbols, cfg)
	d.ingest(context.Background())

	if store.count() != 0 {
		t.Errorf("expected 0 candles on error, got %d", store.count())
	}
	// should still complete and increment run count
	runCount, _, _ := d.Stats()
	if runCount != 1 {
		t.Errorf("expected 1 run even with errors, got %d", runCount)
	}
}

func TestDataIngestion_SymbolProviderError(t *testing.T) {
	store := &mockCandleStore{}
	fetcher := &mockCandleFetcher{}
	symbols := &mockSymbolProvider{err: fmt.Errorf("db down")}

	cfg := DefaultIngestionConfig()
	d := NewDataIngestion(fetcher, store, symbols, cfg)
	d.ingest(context.Background())

	// should not crash, should not fetch
	if fetcher.callCount() != 0 {
		t.Errorf("should not fetch when symbol provider errors, got %d calls", fetcher.callCount())
	}
}

func TestDataIngestion_EmptyResults(t *testing.T) {
	store := &mockCandleStore{}
	fetcher := &mockCandleFetcher{
		candles: map[string][]exchange.Candle{}, // no results
	}
	symbols := &mockSymbolProvider{symbols: []string{"BTC/USDT"}}

	cfg := DefaultIngestionConfig()
	d := NewDataIngestion(fetcher, store, symbols, cfg)
	d.ingest(context.Background())

	if store.count() != 0 {
		t.Errorf("expected 0 candles, got %d", store.count())
	}
}

func TestDataIngestion_StoreError(t *testing.T) {
	store := &mockCandleStore{
		upsertCb: func(_ []*CandleRecord) error {
			return fmt.Errorf("disk full")
		},
	}
	fetcher := &mockCandleFetcher{
		candles: map[string][]exchange.Candle{
			"BTC/USDT_4h": makeCandles(5, "BTC/USDT"),
		},
	}
	symbols := &mockSymbolProvider{symbols: []string{"BTC/USDT"}}

	cfg := DefaultIngestionConfig()
	cfg.Intervals = []string{"4h"}
	d := NewDataIngestion(fetcher, store, symbols, cfg)
	d.ingest(context.Background())

	// should not crash; stored count is 0 since store returned error
	_, totalStored, _ := d.Stats()
	if totalStored != 0 {
		t.Errorf("expected 0 total stored on error, got %d", totalStored)
	}
}

func TestDataIngestion_StartStop(t *testing.T) {
	store := &mockCandleStore{}
	fetcher := &mockCandleFetcher{
		candles: map[string][]exchange.Candle{
			"BTC/USDT_4h": makeCandles(3, "BTC/USDT"),
			"BTC/USDT_1h": makeCandles(3, "BTC/USDT"),
		},
	}
	symbols := &mockSymbolProvider{symbols: []string{"BTC/USDT"}}

	cfg := DefaultIngestionConfig()
	cfg.PollInterval = 50 * time.Millisecond // fast for testing

	ctx := context.Background()
	d := NewDataIngestion(fetcher, store, symbols, cfg)
	d.Start(ctx)

	// wait for at least 1 ingestion
	time.Sleep(200 * time.Millisecond)
	d.Stop()

	runCount, _, _ := d.Stats()
	if runCount < 1 {
		t.Errorf("expected at least 1 run, got %d", runCount)
	}
}

func TestDataIngestion_CandleMapping(t *testing.T) {
	store := &mockCandleStore{}
	candle := exchange.Candle{
		OpenTime: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Open:     42000,
		High:     42500,
		Low:      41800,
		Close:    42300,
		Volume:   1500,
	}
	fetcher := &mockCandleFetcher{
		candles: map[string][]exchange.Candle{
			"BTC/USDT_4h": {candle},
		},
	}
	symbols := &mockSymbolProvider{symbols: []string{"BTC/USDT"}}

	cfg := DefaultIngestionConfig()
	cfg.Intervals = []string{"4h"}
	d := NewDataIngestion(fetcher, store, symbols, cfg)
	d.ingest(context.Background())

	if store.count() != 1 {
		t.Fatalf("expected 1 candle, got %d", store.count())
	}

	rec := store.candles[0]
	if rec.Symbol != "BTC/USDT" {
		t.Errorf("expected BTC/USDT, got %s", rec.Symbol)
	}
	if rec.Interval != "4h" {
		t.Errorf("expected 4h interval, got %s", rec.Interval)
	}
	if rec.Open != 42000 {
		t.Errorf("expected open 42000, got %f", rec.Open)
	}
	if rec.High != 42500 {
		t.Errorf("expected high 42500, got %f", rec.High)
	}
	if rec.Close != 42300 {
		t.Errorf("expected close 42300, got %f", rec.Close)
	}
	if rec.Volume != 1500 {
		t.Errorf("expected volume 1500, got %f", rec.Volume)
	}
}

func TestDefaultIngestionConfig(t *testing.T) {
	cfg := DefaultIngestionConfig()
	if len(cfg.Intervals) != 2 {
		t.Errorf("expected 2 intervals, got %d", len(cfg.Intervals))
	}
	if cfg.BatchSize != 500 {
		t.Errorf("expected batch size 500, got %d", cfg.BatchSize)
	}
	if cfg.PollInterval != 5*time.Minute {
		t.Errorf("expected 5m poll interval, got %v", cfg.PollInterval)
	}
}
