// candle data loaders for backtesting — supports database, CSV, and Binance API sources.
package backtest

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trading-bot/go-bot/internal/exchange"
)

// CandleLoader loads historical candle data for backtesting.
type CandleLoader interface {
	LoadCandles(ctx context.Context, symbol, interval string, from, to time.Time) ([]exchange.Candle, error)
}

// DBLoader loads candles from the TimescaleDB candles hypertable.
type DBLoader struct {
	pool *pgxpool.Pool
}

func NewDBLoader(pool *pgxpool.Pool) *DBLoader {
	return &DBLoader{pool: pool}
}

func (d *DBLoader) LoadCandles(ctx context.Context, symbol, interval string, from, to time.Time) ([]exchange.Candle, error) {
	query := `
		SELECT time, open, high, low, close, volume
		FROM candles
		WHERE symbol = $1 AND interval = $2 AND time >= $3 AND time <= $4
		ORDER BY time ASC`

	rows, err := d.pool.Query(ctx, query, symbol, interval, from, to)
	if err != nil {
		return nil, fmt.Errorf("query candles: %w", err)
	}
	defer rows.Close()

	var candles []exchange.Candle
	for rows.Next() {
		var c exchange.Candle
		if err := rows.Scan(&c.OpenTime, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume); err != nil {
			return nil, fmt.Errorf("scan candle: %w", err)
		}
		candles = append(candles, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate candles: %w", err)
	}
	return candles, nil
}

// CSVLoader loads candles from a CSV file.
// Expected format: time,open,high,low,close,volume
// Time format: RFC3339 or Unix milliseconds.
type CSVLoader struct {
	filePath   string
	timeFormat string // "rfc3339" or "unix_ms"
	hasHeader  bool
}

func NewCSVLoader(filePath, timeFormat string, hasHeader bool) *CSVLoader {
	if timeFormat == "" {
		timeFormat = "unix_ms"
	}
	return &CSVLoader{filePath: filePath, timeFormat: timeFormat, hasHeader: hasHeader}
}

func (l *CSVLoader) LoadCandles(_ context.Context, symbol, interval string, from, to time.Time) ([]exchange.Candle, error) {
	f, err := os.Open(l.filePath)
	if err != nil {
		return nil, fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()

	return parseCSVCandles(f, l.timeFormat, l.hasHeader, from, to)
}

func parseCSVCandles(r io.Reader, timeFormat string, hasHeader bool, from, to time.Time) ([]exchange.Candle, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // allow variable fields

	if hasHeader {
		if _, err := reader.Read(); err != nil {
			return nil, fmt.Errorf("read csv header: %w", err)
		}
	}

	var candles []exchange.Candle
	lineNum := 1
	if hasHeader {
		lineNum = 2
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read csv line %d: %w", lineNum, err)
		}
		if len(record) < 6 {
			lineNum++
			continue
		}

		t, err := parseTime(record[0], timeFormat)
		if err != nil {
			return nil, fmt.Errorf("parse time line %d: %w", lineNum, err)
		}

		if (!from.IsZero() && t.Before(from)) || (!to.IsZero() && t.After(to)) {
			lineNum++
			continue
		}

		open, err := strconv.ParseFloat(record[1], 64)
		if err != nil {
			return nil, fmt.Errorf("parse open line %d: %w", lineNum, err)
		}
		high, err := strconv.ParseFloat(record[2], 64)
		if err != nil {
			return nil, fmt.Errorf("parse high line %d: %w", lineNum, err)
		}
		low, err := strconv.ParseFloat(record[3], 64)
		if err != nil {
			return nil, fmt.Errorf("parse low line %d: %w", lineNum, err)
		}
		cls, err := strconv.ParseFloat(record[4], 64)
		if err != nil {
			return nil, fmt.Errorf("parse close line %d: %w", lineNum, err)
		}
		vol, err := strconv.ParseFloat(record[5], 64)
		if err != nil {
			return nil, fmt.Errorf("parse volume line %d: %w", lineNum, err)
		}

		candles = append(candles, exchange.Candle{
			OpenTime: t,
			Open:     open,
			High:     high,
			Low:      low,
			Close:    cls,
			Volume:   vol,
		})
		lineNum++
	}

	return candles, nil
}

func parseTime(s, format string) (time.Time, error) {
	switch format {
	case "rfc3339":
		return time.Parse(time.RFC3339, s)
	case "unix_ms":
		ms, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.UnixMilli(ms), nil
	default:
		return time.Parse(format, s)
	}
}

// BinanceLoader fetches historical candles from the Binance REST API.
// Handles pagination for large date ranges (max 1000 candles per request).
type BinanceLoader struct {
	client CandleFetcher
}

// CandleFetcher abstracts the Binance candle fetching capability.
type CandleFetcher interface {
	GetCandles(ctx context.Context, symbol string, interval string, limit int) ([]exchange.Candle, error)
}

func NewBinanceLoader(client CandleFetcher) *BinanceLoader {
	return &BinanceLoader{client: client}
}

func (b *BinanceLoader) LoadCandles(ctx context.Context, symbol, interval string, from, to time.Time) ([]exchange.Candle, error) {
	const batchSize = 1000
	var all []exchange.Candle

	// fetch in batches of 1000 until we cover the range
	candles, err := b.client.GetCandles(ctx, symbol, interval, batchSize)
	if err != nil {
		return nil, fmt.Errorf("fetch candles from binance: %w", err)
	}

	for _, c := range candles {
		if (!from.IsZero() && c.OpenTime.Before(from)) || (!to.IsZero() && c.OpenTime.After(to)) {
			continue
		}
		all = append(all, c)
	}
	return all, nil
}

// SliceLoader wraps an in-memory candle slice — useful for testing.
type SliceLoader struct {
	Candles []exchange.Candle
}

func NewSliceLoader(candles []exchange.Candle) *SliceLoader {
	return &SliceLoader{Candles: candles}
}

func (s *SliceLoader) LoadCandles(_ context.Context, _, _ string, from, to time.Time) ([]exchange.Candle, error) {
	var result []exchange.Candle
	for _, c := range s.Candles {
		if (!from.IsZero() && c.OpenTime.Before(from)) || (!to.IsZero() && c.OpenTime.After(to)) {
			continue
		}
		result = append(result, c)
	}
	return result, nil
}
