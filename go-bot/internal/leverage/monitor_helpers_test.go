// shared mocks and test helpers for leverage monitor tests.
package leverage

import (
	"context"
	"errors"
	"sync"
	"time"
)

// --- mock position lister ---

type mockLister struct {
	mu        sync.Mutex
	positions []*LeveragePosition
}

func (m *mockLister) AllOpen() []*LeveragePosition {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*LeveragePosition, len(m.positions))
	copy(out, m.positions)
	return out
}

func (m *mockLister) add(pos *LeveragePosition) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.positions = append(m.positions, pos)
}

func (m *mockLister) remove(posID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, p := range m.positions {
		if p.ID == posID {
			m.positions = append(m.positions[:i], m.positions[i+1:]...)
			return
		}
	}
}

// --- mock position closer ---

type mockCloser struct {
	mu       sync.Mutex
	closed   map[string]*LeveragePosition
	reasons  map[string]string
	closeErr error
}

func newMockCloser() *mockCloser {
	return &mockCloser{
		closed:  make(map[string]*LeveragePosition),
		reasons: make(map[string]string),
	}
}

func (m *mockCloser) Close(posID, reason string) (*LeveragePosition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closeErr != nil {
		return nil, m.closeErr
	}
	pos, ok := m.closed[posID]
	if !ok {
		pos = &LeveragePosition{
			ID:          posID,
			Status:      "closed",
			CloseReason: reason,
		}
	}
	pos.Status = "closed"
	pos.CloseReason = reason
	m.reasons[posID] = reason
	return pos, nil
}

func (m *mockCloser) prepareClose(pos *LeveragePosition) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed[pos.ID] = pos
}

// --- mock mark price provider ---

type mockMarkPrices struct {
	mu     sync.Mutex
	prices map[string]float64
	err    error
}

func (m *mockMarkPrices) GetMarkPrice(ctx context.Context, symbol string) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return 0, m.err
	}
	price, ok := m.prices[symbol]
	if !ok {
		return 0, errors.New("symbol not found")
	}
	return price, nil
}

// --- event collector ---

type levEventCollector struct {
	mu     sync.Mutex
	events []LevEvent
}

func (c *levEventCollector) collect(e LevEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *levEventCollector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func (c *levEventCollector) get(i int) LevEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.events[i]
}

func (c *levEventCollector) hasType(t LevEventType) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.events {
		if e.Type == t {
			return true
		}
	}
	return false
}

// --- test helpers ---

func testLongPosition(id, symbol string) *LeveragePosition {
	return &LeveragePosition{
		ID:               id,
		UserID:           1,
		Symbol:           symbol,
		Side:             SideLong,
		Leverage:         10,
		EntryPrice:       50000,
		MarkPrice:        50000,
		Quantity:         0.1,
		Margin:           500,
		NotionalValue:    5000,
		LiquidationPrice: 45200,
		StopLoss:         48000,
		TakeProfit:       55000,
		MarginType:       "isolated",
		IsPaper:          true,
		Status:           "open",
		OpenedAt:         time.Now(),
	}
}

func testShortPosition(id, symbol string) *LeveragePosition {
	return &LeveragePosition{
		ID:               id,
		UserID:           1,
		Symbol:           symbol,
		Side:             SideShort,
		Leverage:         10,
		EntryPrice:       50000,
		MarkPrice:        50000,
		Quantity:         0.1,
		Margin:           500,
		NotionalValue:    5000,
		LiquidationPrice: 54800,
		StopLoss:         52000,
		TakeProfit:       45000,
		MarginType:       "isolated",
		IsPaper:          true,
		Status:           "open",
		OpenedAt:         time.Now(),
	}
}

func testMonitorSetup() (*Monitor, *mockLister, *mockCloser, *mockMarkPrices, *FundingTracker) {
	lister := &mockLister{}
	closer := newMockCloser()
	prices := &mockMarkPrices{prices: make(map[string]float64)}
	funding := NewFundingTracker()
	config := DefaultMonitorConfig()
	mon := NewMonitor(lister, closer, prices, funding, config)
	return mon, lister, closer, prices, funding
}
