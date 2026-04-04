// data ingestion service — periodically fetches OHLCV candles from binance
// and persists them to the TimescaleDB candles hypertable.
// runs as a background goroutine alongside the scanner.
package pipeline

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// CandleRecord matches database.CandleRecord without importing database (interface boundary).
type CandleRecord struct {
	Time        time.Time
	Symbol      string
	Interval    string
	Open        float64
	High        float64
	Low         float64
	Close       float64
	Volume      float64
	QuoteVolume float64
	TradeCount  int
}

// CandleStore persists candle records (implemented by database.CandleRepository).
type CandleStore interface {
	UpsertBatch(ctx context.Context, candles []*CandleRecord) (int, error)
	LatestTime(ctx context.Context, symbol, interval string) (time.Time, error)
}

// SymbolProvider returns the list of symbols to ingest candles for.
type SymbolProvider interface {
	ActiveSymbols(ctx context.Context) ([]string, error)
}

// CandleFetcher fetches candles from an exchange.
type CandleFetcher interface {
	GetCandles(ctx context.Context, symbol string, interval string, limit int) ([]exchange.Candle, error)
}

// DataIngestionConfig configures the data ingestion loop.
type DataIngestionConfig struct {
	Intervals    []string      // candle intervals to ingest, e.g. ["4h", "1h"]
	BatchSize    int           // max candles per fetch (binance limit: 1000)
	PollInterval time.Duration // how often to poll for new candles
}

// DefaultIngestionConfig returns sensible defaults.
func DefaultIngestionConfig() DataIngestionConfig {
	return DataIngestionConfig{
		Intervals:    []string{"4h", "1h"},
		BatchSize:    500,
		PollInterval: 5 * time.Minute,
	}
}

// DataIngestion fetches candles from an exchange and stores them.
type DataIngestion struct {
	fetcher  CandleFetcher
	store    CandleStore
	symbols  SymbolProvider
	config   DataIngestionConfig

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc

	// stats
	lastRunAt   time.Time
	totalStored int
	runCount    int
}

// NewDataIngestion creates a new ingestion service.
func NewDataIngestion(fetcher CandleFetcher, store CandleStore, symbols SymbolProvider, cfg DataIngestionConfig) *DataIngestion {
	return &DataIngestion{
		fetcher: fetcher,
		store:   store,
		symbols: symbols,
		config:  cfg,
	}
}

// Start begins the background candle ingestion loop.
func (d *DataIngestion) Start(ctx context.Context) {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return
	}
	d.running = true
	ctx, d.cancel = context.WithCancel(ctx)
	d.mu.Unlock()

	go d.loop(ctx)
}

// Stop halts the ingestion loop.
func (d *DataIngestion) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		d.cancel()
	}
	d.running = false
}

// Stats returns ingestion statistics.
func (d *DataIngestion) Stats() (runCount, totalStored int, lastRun time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.runCount, d.totalStored, d.lastRunAt
}

func (d *DataIngestion) loop(ctx context.Context) {
	// run immediately on start
	d.ingest(ctx)

	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.ingest(ctx)
		}
	}
}

func (d *DataIngestion) ingest(ctx context.Context) {
	symbols, err := d.symbols.ActiveSymbols(ctx)
	if err != nil {
		slog.Error("data ingestion: failed to get symbols", "error", err)
		return
	}

	totalStored := 0
	for _, symbol := range symbols {
		for _, interval := range d.config.Intervals {
			n, err := d.ingestSymbol(ctx, symbol, interval)
			if err != nil {
				slog.Error("data ingestion: failed",
					"symbol", symbol, "interval", interval, "error", err)
				continue
			}
			totalStored += n
		}
	}

	d.mu.Lock()
	d.runCount++
	d.totalStored += totalStored
	d.lastRunAt = time.Now()
	d.mu.Unlock()

	if totalStored > 0 {
		slog.Info("data ingestion: complete",
			"symbols", len(symbols), "candles_stored", totalStored)
	}
}

func (d *DataIngestion) ingestSymbol(ctx context.Context, symbol, interval string) (int, error) {
	candles, err := d.fetcher.GetCandles(ctx, symbol, interval, d.config.BatchSize)
	if err != nil {
		return 0, err
	}

	if len(candles) == 0 {
		return 0, nil
	}

	records := make([]*CandleRecord, len(candles))
	for i, c := range candles {
		records[i] = &CandleRecord{
			Time:     c.OpenTime,
			Symbol:   symbol,
			Interval: interval,
			Open:     c.Open,
			High:     c.High,
			Low:      c.Low,
			Close:    c.Close,
			Volume:   c.Volume,
		}
	}

	return d.store.UpsertBatch(ctx, records)
}
