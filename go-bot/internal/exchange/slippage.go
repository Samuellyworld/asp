// slippage tracking — compares expected fill prices from AI decisions
// with actual execution prices to measure and report slippage.
// tracks per-symbol slippage statistics for self-learning feedback.
package exchange

import (
	"math"
	"sync"
	"time"
)

// SlippageRecord captures one slippage observation.
type SlippageRecord struct {
	Symbol        string
	Side          string  // "BUY" or "SELL"
	ExpectedPrice float64 // the price from the AI decision
	ActualPrice   float64 // the price we actually got filled at
	SlippageBps   float64 // slippage in basis points
	Quantity      float64
	IsPaper       bool
	RecordedAt    time.Time
}

// SlippageStats holds aggregate slippage statistics for a symbol.
type SlippageStats struct {
	Symbol        string
	TradeCount    int
	AvgSlippageBps float64
	MaxSlippageBps float64
	TotalSlipUSD  float64
}

// SlippageTracker accumulates slippage records and provides statistics.
type SlippageTracker struct {
	mu      sync.Mutex
	records []SlippageRecord
	maxSize int
}

// NewSlippageTracker creates a tracker that retains at most maxSize recent records.
func NewSlippageTracker(maxSize int) *SlippageTracker {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &SlippageTracker{
		records: make([]SlippageRecord, 0, 256),
		maxSize: maxSize,
	}
}

// Record records a slippage observation. expectedPrice is the AI decision price,
// actualPrice is what we got filled at.
func (s *SlippageTracker) Record(symbol, side string, expectedPrice, actualPrice, quantity float64, isPaper bool) SlippageRecord {
	slippageBps := CalculateSlippageBps(expectedPrice, actualPrice, side)

	rec := SlippageRecord{
		Symbol:        symbol,
		Side:          side,
		ExpectedPrice: expectedPrice,
		ActualPrice:   actualPrice,
		SlippageBps:   slippageBps,
		Quantity:      quantity,
		IsPaper:       isPaper,
		RecordedAt:    time.Now(),
	}

	s.mu.Lock()
	s.records = append(s.records, rec)
	if len(s.records) > s.maxSize {
		s.records = s.records[len(s.records)-s.maxSize:]
	}
	s.mu.Unlock()

	return rec
}

// StatsForSymbol returns aggregate slippage statistics for a symbol.
func (s *SlippageTracker) StatsForSymbol(symbol string) SlippageStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := SlippageStats{Symbol: symbol}
	var totalBps float64

	for _, r := range s.records {
		if r.Symbol != symbol {
			continue
		}
		stats.TradeCount++
		totalBps += r.SlippageBps
		if math.Abs(r.SlippageBps) > math.Abs(stats.MaxSlippageBps) {
			stats.MaxSlippageBps = r.SlippageBps
		}
		priceDiff := math.Abs(r.ActualPrice - r.ExpectedPrice)
		stats.TotalSlipUSD += priceDiff * r.Quantity
	}

	if stats.TradeCount > 0 {
		stats.AvgSlippageBps = totalBps / float64(stats.TradeCount)
	}

	return stats
}

// AllStats returns slippage stats for all symbols that have records.
func (s *SlippageTracker) AllStats() []SlippageStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	symbolMap := make(map[string][]SlippageRecord)
	for _, r := range s.records {
		symbolMap[r.Symbol] = append(symbolMap[r.Symbol], r)
	}

	result := make([]SlippageStats, 0, len(symbolMap))
	for symbol, records := range symbolMap {
		stats := SlippageStats{Symbol: symbol, TradeCount: len(records)}
		var totalBps float64
		for _, r := range records {
			totalBps += r.SlippageBps
			if math.Abs(r.SlippageBps) > math.Abs(stats.MaxSlippageBps) {
				stats.MaxSlippageBps = r.SlippageBps
			}
			priceDiff := math.Abs(r.ActualPrice - r.ExpectedPrice)
			stats.TotalSlipUSD += priceDiff * r.Quantity
		}
		stats.AvgSlippageBps = totalBps / float64(stats.TradeCount)
		result = append(result, stats)
	}

	return result
}

// RecentRecords returns up to n most recent slippage records.
func (s *SlippageTracker) RecentRecords(n int) []SlippageRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	if n <= 0 || n > len(s.records) {
		n = len(s.records)
	}
	start := len(s.records) - n
	out := make([]SlippageRecord, n)
	copy(out, s.records[start:])
	return out
}

// CalculateSlippageBps computes slippage in basis points.
// Positive = unfavorable (paid more for BUY, got less for SELL).
// Negative = favorable (got a better price than expected).
func CalculateSlippageBps(expected, actual float64, side string) float64 {
	if expected == 0 {
		return 0
	}
	rawBps := (actual - expected) / expected * 10000
	if side == "BUY" {
		return rawBps // positive = unfavorable for buys
	}
	return -rawBps // for sells, higher actual price is favorable (negative slippage = good)
}
