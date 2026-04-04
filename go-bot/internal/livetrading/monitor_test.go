// tests for live position monitor: order fill detection, periodic updates,
// cooldown enforcement, start/stop lifecycle, and cleanup.
package livetrading

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// --- mock price provider ---

type mockPrices struct {
	mu     sync.Mutex
	prices map[string]float64
	err    error
}

func (m *mockPrices) GetPrice(symbol string) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return 0, m.err
	}
	return m.prices[symbol], nil
}

// --- event collector ---

type eventCollector struct {
	mu     sync.Mutex
	events []Event
}

func (c *eventCollector) collect(e Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *eventCollector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func (c *eventCollector) get(i int) Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.events[i]
}

func (c *eventCollector) lastType() EventType {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.events) == 0 {
		return ""
	}
	return c.events[len(c.events)-1].Type
}

// --- helpers ---

func testMonitor(orders *mockOrders, keys *mockKeys, prices PriceProvider) (*Monitor, *Executor) {
	executor := NewExecutor(orders, keys, nil, nil)
	config := DefaultMonitorConfig()
	return NewMonitor(executor, orders, keys, prices, config), executor
}

func addTestPosition(executor *Executor, id string, symbol string, slOrderID, tpOrderID int64) *LivePosition {
	executor.mu.Lock()
	defer executor.mu.Unlock()

	pos := &LivePosition{
		ID:           id,
		UserID:       1,
		Symbol:       symbol,
		Side:         exchange.SideBuy,
		EntryPrice:   100.0,
		Quantity:     1.0,
		PositionSize: 100.0,
		StopLoss:     95.0,
		TakeProfit:   110.0,
		SLOrderID:    slOrderID,
		TPOrderID:    tpOrderID,
		Status:       "open",
		OpenedAt:     time.Now(),
	}
	executor.positions[id] = pos
	return pos
}

// --- default config tests ---

func TestDefaultMonitorConfig(t *testing.T) {
	cfg := DefaultMonitorConfig()

	if cfg.CheckInterval != 30*time.Second {
		t.Errorf("CheckInterval = %v, want 30s", cfg.CheckInterval)
	}
	if cfg.CooldownPeriod != 15*time.Minute {
		t.Errorf("CooldownPeriod = %v, want 15m", cfg.CooldownPeriod)
	}
	if cfg.SmallPositionInterval != 1*time.Hour {
		t.Errorf("SmallPositionInterval = %v, want 1h", cfg.SmallPositionInterval)
	}
	if cfg.MediumPositionInterval != 30*time.Minute {
		t.Errorf("MediumPositionInterval = %v, want 30m", cfg.MediumPositionInterval)
	}
	if cfg.LargePositionInterval != 15*time.Minute {
		t.Errorf("LargePositionInterval = %v, want 15m", cfg.LargePositionInterval)
	}
}

// --- order fill detection ---

func TestCheckPositions_SLFilled(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, executor := testMonitor(orders, keys, nil)

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	// add position with a SL order
	addTestPosition(executor, "pos_1", "BTCUSDT", 100, 200)

	// mark SL order as filled on the exchange
	orders.mu.Lock()
	orders.orders[100] = &exchange.Order{
		OrderID: 100,
		Status:  exchange.OrderStatusFilled,
	}
	// keep TP order as new
	orders.orders[200] = &exchange.Order{
		OrderID: 200,
		Status:  exchange.OrderStatusNew,
	}
	orders.mu.Unlock()

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	if collector.get(0).Type != EventSLHit {
		t.Errorf("event type = %s, want %s", collector.get(0).Type, EventSLHit)
	}
	if !collector.get(0).IsUrgent {
		t.Error("sl hit should be urgent")
	}
}

func TestCheckPositions_TPFilled(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, executor := testMonitor(orders, keys, nil)

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	addTestPosition(executor, "pos_1", "BTCUSDT", 100, 200)

	// SL not filled, TP filled
	orders.mu.Lock()
	orders.orders[100] = &exchange.Order{
		OrderID: 100,
		Status:  exchange.OrderStatusNew,
	}
	orders.orders[200] = &exchange.Order{
		OrderID: 200,
		Status:  exchange.OrderStatusFilled,
	}
	orders.mu.Unlock()

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	if collector.get(0).Type != EventTPHit {
		t.Errorf("event type = %s, want %s", collector.get(0).Type, EventTPHit)
	}
	if !collector.get(0).IsUrgent {
		t.Error("tp hit should be urgent")
	}
}

func TestCheckPositions_NoFill_NoEvent(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, executor := testMonitor(orders, keys, nil)

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	addTestPosition(executor, "pos_1", "BTCUSDT", 100, 200)

	// both orders still open
	orders.mu.Lock()
	orders.orders[100] = &exchange.Order{OrderID: 100, Status: exchange.OrderStatusNew}
	orders.orders[200] = &exchange.Order{OrderID: 200, Status: exchange.OrderStatusNew}
	orders.mu.Unlock()

	mon.CheckPositions()

	// no fills and no price provider, so no events
	if collector.count() != 0 {
		t.Errorf("events = %d, want 0", collector.count())
	}
}

func TestCheckPositions_SLPrioritizedOverTP(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, executor := testMonitor(orders, keys, nil)

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	addTestPosition(executor, "pos_1", "BTCUSDT", 100, 200)

	// both filled — SL should be checked first and position closed
	orders.mu.Lock()
	orders.orders[100] = &exchange.Order{OrderID: 100, Status: exchange.OrderStatusFilled}
	orders.orders[200] = &exchange.Order{OrderID: 200, Status: exchange.OrderStatusFilled}
	orders.mu.Unlock()

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1 (SL takes precedence)", collector.count())
	}
	if collector.get(0).Type != EventSLHit {
		t.Errorf("event type = %s, want %s", collector.get(0).Type, EventSLHit)
	}
}

func TestCheckPositions_NoSLOrder(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, executor := testMonitor(orders, keys, nil)

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	// position with no SL order id, only TP
	addTestPosition(executor, "pos_1", "BTCUSDT", 0, 200)

	orders.mu.Lock()
	orders.orders[200] = &exchange.Order{OrderID: 200, Status: exchange.OrderStatusFilled}
	orders.mu.Unlock()

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	if collector.get(0).Type != EventTPHit {
		t.Errorf("event type = %s, want %s", collector.get(0).Type, EventTPHit)
	}
}

func TestCheckPositions_KeyDecryptError_SkipsPosition(t *testing.T) {
	orders := newMockOrders()
	keys := &mockKeys{keys: map[int][2]string{}} // no keys for user 1
	mon, executor := testMonitor(orders, keys, nil)

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	addTestPosition(executor, "pos_1", "BTCUSDT", 100, 200)

	mon.CheckPositions()

	// should silently skip without crashing
	if collector.count() != 0 {
		t.Errorf("events = %d, want 0 (key decrypt failed)", collector.count())
	}
}

// --- periodic update tests ---

func TestCheckPositions_PeriodicUpdate(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 105.0}}
	mon, executor := testMonitor(orders, keys, prices)

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	// position with no SL/TP orders (won't trigger fills)
	addTestPosition(executor, "pos_1", "BTCUSDT", 0, 0)

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	if collector.get(0).Type != EventPeriodicUpdate {
		t.Errorf("event type = %s, want %s", collector.get(0).Type, EventPeriodicUpdate)
	}
	if collector.get(0).IsUrgent {
		t.Error("periodic update should not be urgent")
	}
}

func TestCheckPositions_PeriodicUpdate_CooldownEnforced(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 105.0}}
	mon, executor := testMonitor(orders, keys, prices)

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	addTestPosition(executor, "pos_1", "BTCUSDT", 0, 0)

	// first check fires periodic update
	mon.CheckPositions()
	if collector.count() != 1 {
		t.Fatalf("first check: events = %d, want 1", collector.count())
	}

	// second check immediately after — cooldown should suppress
	mon.CheckPositions()
	if collector.count() != 1 {
		t.Errorf("second check: events = %d, want 1 (cooldown should block)", collector.count())
	}
}

func TestCheckPositions_NoPriceProvider_SkipsPeriodicUpdate(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, executor := testMonitor(orders, keys, nil) // nil price provider

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	addTestPosition(executor, "pos_1", "BTCUSDT", 0, 0)

	mon.CheckPositions()

	if collector.count() != 0 {
		t.Errorf("events = %d, want 0 (no price provider)", collector.count())
	}
}

func TestCheckPositions_FillSkipsPeriodicUpdate(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 105.0}}
	mon, executor := testMonitor(orders, keys, prices)

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	addTestPosition(executor, "pos_1", "BTCUSDT", 100, 0)

	// SL filled — should emit SL hit but not periodic update
	orders.mu.Lock()
	orders.orders[100] = &exchange.Order{OrderID: 100, Status: exchange.OrderStatusFilled}
	orders.mu.Unlock()

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	if collector.get(0).Type != EventSLHit {
		t.Errorf("event type = %s, want %s (fill should skip periodic)", collector.get(0).Type, EventSLHit)
	}
}

// --- periodic interval by position size ---

func TestPeriodicInterval_Small(t *testing.T) {
	mon := &Monitor{config: DefaultMonitorConfig()}
	interval := mon.periodicInterval(50.0) // < $100

	if interval != 1*time.Hour {
		t.Errorf("small interval = %v, want 1h", interval)
	}
}

func TestPeriodicInterval_Medium(t *testing.T) {
	mon := &Monitor{config: DefaultMonitorConfig()}
	interval := mon.periodicInterval(250.0) // $100-$500

	if interval != 30*time.Minute {
		t.Errorf("medium interval = %v, want 30m", interval)
	}
}

func TestPeriodicInterval_Large(t *testing.T) {
	mon := &Monitor{config: DefaultMonitorConfig()}
	interval := mon.periodicInterval(1000.0) // > $500

	if interval != 15*time.Minute {
		t.Errorf("large interval = %v, want 15m", interval)
	}
}

func TestPeriodicInterval_Boundaries(t *testing.T) {
	mon := &Monitor{config: DefaultMonitorConfig()}

	tests := []struct {
		size     float64
		expected time.Duration
	}{
		{99.99, 1 * time.Hour},
		{100.0, 30 * time.Minute},
		{499.99, 30 * time.Minute},
		{500.0, 15 * time.Minute},
	}

	for _, tt := range tests {
		got := mon.periodicInterval(tt.size)
		if got != tt.expected {
			t.Errorf("periodicInterval(%.2f) = %v, want %v", tt.size, got, tt.expected)
		}
	}
}

// --- start / stop lifecycle ---

func TestMonitor_StartStop(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, _ := testMonitor(orders, keys, nil)

	if mon.IsRunning() {
		t.Fatal("monitor should not be running before Start")
	}

	ctx := context.Background()
	mon.Start(ctx)

	if !mon.IsRunning() {
		t.Fatal("monitor should be running after Start")
	}

	mon.Stop()

	if mon.IsRunning() {
		t.Fatal("monitor should not be running after Stop")
	}
}

func TestMonitor_DoubleStart(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, _ := testMonitor(orders, keys, nil)

	ctx := context.Background()
	mon.Start(ctx)
	mon.Start(ctx) // should be a no-op

	if !mon.IsRunning() {
		t.Fatal("monitor should still be running after double Start")
	}

	mon.Stop()
}

func TestMonitor_DoubleStop(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, _ := testMonitor(orders, keys, nil)

	ctx := context.Background()
	mon.Start(ctx)
	mon.Stop()
	mon.Stop() // should be a no-op without panic

	if mon.IsRunning() {
		t.Fatal("monitor should not be running after Stop")
	}
}

func TestMonitor_ContextCancellation(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, _ := testMonitor(orders, keys, nil)

	ctx, cancel := context.WithCancel(context.Background())
	mon.Start(ctx)

	if !mon.IsRunning() {
		t.Fatal("monitor should be running")
	}

	cancel()
	// wait for the goroutine to finish
	<-mon.done

	if mon.IsRunning() {
		t.Fatal("monitor should stop when context is canceled")
	}
}

// --- emit behavior ---

func TestEmit_NilOnEvent(t *testing.T) {
	mon := &Monitor{}
	// should not panic when OnEvent is nil
	mon.emit(Event{Type: EventPeriodicUpdate})
}

func TestEmit_UrgentBypassesCooldown(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, executor := testMonitor(orders, keys, nil)

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	addTestPosition(executor, "pos_1", "BTCUSDT", 100, 200)

	// mark SL as filled — urgent events always fire
	orders.mu.Lock()
	orders.orders[100] = &exchange.Order{OrderID: 100, Status: exchange.OrderStatusFilled}
	orders.orders[200] = &exchange.Order{OrderID: 200, Status: exchange.OrderStatusNew}
	orders.mu.Unlock()

	// simulate that we already notified recently
	mon.recordNotification("pos_1", EventPeriodicUpdate)

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1 (urgent should always fire)", collector.count())
	}
	if collector.get(0).Type != EventSLHit {
		t.Errorf("event type = %s, want %s", collector.get(0).Type, EventSLHit)
	}
}

// --- cleanup ---

func TestCleanup_RemovesClosedPositions(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, executor := testMonitor(orders, keys, nil)

	// add two positions
	addTestPosition(executor, "pos_1", "BTCUSDT", 0, 0)
	addTestPosition(executor, "pos_2", "ETHUSDT", 0, 0)

	// record notifications for both
	mon.recordNotification("pos_1", EventPeriodicUpdate)
	mon.recordNotification("pos_2", EventPeriodicUpdate)

	// remove pos_2 from open positions (simulate close)
	executor.mu.Lock()
	delete(executor.positions, "pos_2")
	executor.mu.Unlock()

	mon.Cleanup()

	mon.mu.Lock()
	defer mon.mu.Unlock()

	if _, exists := mon.lastNotified["pos_1"]; !exists {
		t.Error("pos_1 tracking should be preserved (still open)")
	}
	if _, exists := mon.lastNotified["pos_2"]; exists {
		t.Error("pos_2 tracking should be removed (no longer open)")
	}
	if _, exists := mon.lastEventType["pos_2"]; exists {
		t.Error("pos_2 event type should be removed")
	}
}

func TestCleanup_NoOpenPositions(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	mon, _ := testMonitor(orders, keys, nil)

	mon.recordNotification("pos_1", EventPeriodicUpdate)
	mon.recordNotification("pos_2", EventPeriodicUpdate)

	mon.Cleanup()

	mon.mu.Lock()
	defer mon.mu.Unlock()

	if len(mon.lastNotified) != 0 {
		t.Errorf("lastNotified count = %d, want 0", len(mon.lastNotified))
	}
}

// --- multiple positions ---

func TestCheckPositions_MultiplePositions(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	prices := &mockPrices{prices: map[string]float64{
		"BTCUSDT": 105.0,
		"ETHUSDT": 3500.0,
	}}
	mon, executor := testMonitor(orders, keys, prices)

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	// pos_1 has SL filled
	addTestPosition(executor, "pos_1", "BTCUSDT", 100, 200)
	orders.mu.Lock()
	orders.orders[100] = &exchange.Order{OrderID: 100, Status: exchange.OrderStatusFilled}
	orders.orders[200] = &exchange.Order{OrderID: 200, Status: exchange.OrderStatusNew}
	orders.mu.Unlock()

	// pos_2 has no fills, should get periodic update
	addTestPosition(executor, "pos_2", "ETHUSDT", 0, 0)

	mon.CheckPositions()

	if collector.count() != 2 {
		t.Fatalf("events = %d, want 2", collector.count())
	}

	// verify we got one SL hit and one periodic update
	types := map[EventType]int{}
	for i := 0; i < collector.count(); i++ {
		types[collector.get(i).Type]++
	}

	if types[EventSLHit] != 1 {
		t.Errorf("SL hit events = %d, want 1", types[EventSLHit])
	}
	if types[EventPeriodicUpdate] != 1 {
		t.Errorf("periodic events = %d, want 1", types[EventPeriodicUpdate])
	}
}

func TestCheckPositions_NoOpenPositions(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	prices := &mockPrices{prices: map[string]float64{"BTCUSDT": 105.0}}
	mon, _ := testMonitor(orders, keys, prices)

	collector := &eventCollector{}
	mon.OnEvent = collector.collect

	// no positions — should be a no-op
	mon.CheckPositions()

	if collector.count() != 0 {
		t.Errorf("events = %d, want 0", collector.count())
	}
}

// --- record notification ---

func TestRecordNotification(t *testing.T) {
	mon := &Monitor{
		lastNotified:  make(map[string]time.Time),
		lastEventType: make(map[string]EventType),
	}

	before := time.Now()
	mon.recordNotification("pos_1", EventTPHit)
	after := time.Now()

	mon.mu.Lock()
	defer mon.mu.Unlock()

	ts, ok := mon.lastNotified["pos_1"]
	if !ok {
		t.Fatal("pos_1 should be recorded in lastNotified")
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp out of range: %v not between %v and %v", ts, before, after)
	}

	if mon.lastEventType["pos_1"] != EventTPHit {
		t.Errorf("event type = %s, want %s", mon.lastEventType["pos_1"], EventTPHit)
	}
}

// --- shouldSendPeriodic ---

func TestShouldSendPeriodic_FirstTime(t *testing.T) {
	mon := &Monitor{
		config:       DefaultMonitorConfig(),
		lastNotified: make(map[string]time.Time),
	}

	pos := &LivePosition{ID: "pos_1", PositionSize: 250.0}

	if !mon.shouldSendPeriodic(pos) {
		t.Error("should send periodic on first check (no prior notification)")
	}
}

func TestShouldSendPeriodic_WithinCooldown(t *testing.T) {
	mon := &Monitor{
		config:       DefaultMonitorConfig(),
		lastNotified: make(map[string]time.Time),
	}

	// notified just now — within 15m cooldown
	mon.lastNotified["pos_1"] = time.Now()
	pos := &LivePosition{ID: "pos_1", PositionSize: 250.0}

	if mon.shouldSendPeriodic(pos) {
		t.Error("should not send periodic within cooldown period")
	}
}

func TestShouldSendPeriodic_AfterCooldownBeforeInterval(t *testing.T) {
	mon := &Monitor{
		config:       DefaultMonitorConfig(),
		lastNotified: make(map[string]time.Time),
	}

	// notified 20 minutes ago — past cooldown (15m) but before medium interval (30m)
	mon.lastNotified["pos_1"] = time.Now().Add(-20 * time.Minute)
	pos := &LivePosition{ID: "pos_1", PositionSize: 250.0}

	if mon.shouldSendPeriodic(pos) {
		t.Error("should not send periodic before interval has elapsed")
	}
}

func TestShouldSendPeriodic_AfterInterval(t *testing.T) {
	mon := &Monitor{
		config:       DefaultMonitorConfig(),
		lastNotified: make(map[string]time.Time),
	}

	// notified 35 minutes ago — past both cooldown (15m) and medium interval (30m)
	mon.lastNotified["pos_1"] = time.Now().Add(-35 * time.Minute)
	pos := &LivePosition{ID: "pos_1", PositionSize: 250.0}

	if !mon.shouldSendPeriodic(pos) {
		t.Error("should send periodic after interval has elapsed")
	}
}

// --- NewMonitor ---

func TestNewMonitor_Fields(t *testing.T) {
	orders := newMockOrders()
	keys := newMockKeys()
	prices := &mockPrices{prices: map[string]float64{}}
	executor := NewExecutor(orders, keys, nil, nil)
	config := DefaultMonitorConfig()

	mon := NewMonitor(executor, orders, keys, prices, config)

	if mon.executor != executor {
		t.Error("executor not set")
	}
	if mon.orders != orders {
		t.Error("orders not set")
	}
	if mon.prices != prices {
		t.Error("prices not set")
	}
	if mon.lastNotified == nil {
		t.Error("lastNotified should be initialized")
	}
	if mon.lastEventType == nil {
		t.Error("lastEventType should be initialized")
	}
}
