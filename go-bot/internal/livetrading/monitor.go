// live position monitor. checks exchange order status for sl/tp fills,
// fetches current prices for periodic updates, and enforces cooldowns.
// runs at 30s intervals (faster than paper trading's 60s).
package livetrading

import (
	"context"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// event types for live position monitoring
type EventType string

const (
	EventTradeExecuted  EventType = "trade_executed"
	EventTPHit          EventType = "tp_hit"
	EventSLHit          EventType = "sl_hit"
	EventManualClose    EventType = "manual_close"
	EventPeriodicUpdate EventType = "periodic_update"
)

// a monitoring event emitted when a position state changes
type Event struct {
	Type     EventType
	Position *LivePosition
	IsUrgent bool
}

// monitor configuration
type MonitorConfig struct {
	CheckInterval          time.Duration
	CooldownPeriod         time.Duration
	SmallPositionInterval  time.Duration // < $100
	MediumPositionInterval time.Duration // $100-$500
	LargePositionInterval  time.Duration // > $500
}

func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		CheckInterval:          30 * time.Second,
		CooldownPeriod:         15 * time.Minute,
		SmallPositionInterval:  1 * time.Hour,
		MediumPositionInterval: 30 * time.Minute,
		LargePositionInterval:  15 * time.Minute,
	}
}

// provides current prices for position monitoring
type PriceProvider interface {
	GetPrice(symbol string) (float64, error)
}

// background goroutine that monitors live positions for order fills and price events
type Monitor struct {
	executor *Executor
	orders   exchange.OrderExecutor
	keys     KeyDecryptor
	prices   PriceProvider
	config   MonitorConfig
	OnEvent  func(Event)

	mu            sync.Mutex
	lastNotified  map[string]time.Time
	lastEventType map[string]EventType

	cancel  context.CancelFunc
	done    chan struct{}
	running bool
}

func NewMonitor(executor *Executor, orders exchange.OrderExecutor, keys KeyDecryptor, prices PriceProvider, config MonitorConfig) *Monitor {
	return &Monitor{
		executor:      executor,
		orders:        orders,
		keys:          keys,
		prices:        prices,
		config:        config,
		lastNotified:  make(map[string]time.Time),
		lastEventType: make(map[string]EventType),
	}
}

// starts the background monitoring goroutine
func (m *Monitor) Start(ctx context.Context) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	ctx, m.cancel = context.WithCancel(ctx)
	m.done = make(chan struct{})
	m.mu.Unlock()

	go m.run(ctx)
}

// stops the monitoring goroutine and waits for it to finish
func (m *Monitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.cancel()
	m.mu.Unlock()
	<-m.done
}

func (m *Monitor) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *Monitor) run(ctx context.Context) {
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		close(m.done)
	}()

	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.CheckPositions()
		}
	}
}

// checks all open positions for order fills and price changes.
// exported for testing — also called automatically by the monitor loop.
func (m *Monitor) CheckPositions() {
	positions := m.executor.AllOpen()

	for _, pos := range positions {
		// check if sl/tp orders have been filled on the exchange
		if m.checkOrderFills(pos) {
			continue
		}

		// check current price for periodic updates
		if m.prices != nil {
			m.checkPriceUpdate(pos)
		}
	}
}

// checks if sl or tp orders have been filled on the exchange.
// returns true if the position was closed (caller should skip further checks).
func (m *Monitor) checkOrderFills(pos *LivePosition) bool {
	apiKey, apiSecret, err := m.keys.DecryptKeys(pos.UserID)
	if err != nil {
		return false
	}

	// check stop loss order
	if pos.SLOrderID > 0 && m.orders != nil {
		slOrder, err := m.orders.GetOrder(pos.Symbol, pos.SLOrderID, apiKey, apiSecret)
		if err == nil && slOrder.Status == exchange.OrderStatusFilled {
			closed, err := m.executor.Close(pos.ID, "stop_loss")
			if err != nil {
				// close failed — don't skip this position; retry on next cycle
				return false
			}
			m.emit(Event{Type: EventSLHit, Position: closed, IsUrgent: true})
			return true
		}
	}

	// check take profit order
	if pos.TPOrderID > 0 && m.orders != nil {
		tpOrder, err := m.orders.GetOrder(pos.Symbol, pos.TPOrderID, apiKey, apiSecret)
		if err == nil && tpOrder.Status == exchange.OrderStatusFilled {
			closed, err := m.executor.Close(pos.ID, "take_profit")
			if err != nil {
				// close failed — don't skip this position; retry on next cycle
				return false
			}
			m.emit(Event{Type: EventTPHit, Position: closed, IsUrgent: true})
			return true
		}
	}

	return false
}

// fetches current price and emits periodic update if cooldown has elapsed
func (m *Monitor) checkPriceUpdate(pos *LivePosition) {
	if !m.shouldSendPeriodic(pos) {
		return
	}

	m.emit(Event{Type: EventPeriodicUpdate, Position: pos, IsUrgent: false})
	m.recordNotification(pos.ID, EventPeriodicUpdate)
}

// determines if a periodic update should be sent based on position size and cooldown
func (m *Monitor) shouldSendPeriodic(pos *LivePosition) bool {
	interval := m.periodicInterval(pos.PositionSize)

	m.mu.Lock()
	lastTime, exists := m.lastNotified[pos.ID]
	m.mu.Unlock()

	if !exists {
		return true
	}

	if time.Since(lastTime) < m.config.CooldownPeriod {
		return false
	}

	return time.Since(lastTime) >= interval
}

// returns the update interval based on position size category
func (m *Monitor) periodicInterval(size float64) time.Duration {
	if size >= 500 {
		return m.config.LargePositionInterval
	}
	if size >= 100 {
		return m.config.MediumPositionInterval
	}
	return m.config.SmallPositionInterval
}

func (m *Monitor) recordNotification(posID string, eventType EventType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastNotified[posID] = time.Now()
	m.lastEventType[posID] = eventType
}

func (m *Monitor) emit(event Event) {
	if m.OnEvent == nil {
		return
	}

	// urgent events always fire (sl/tp fills)
	if event.IsUrgent {
		m.OnEvent(event)
		return
	}

	m.OnEvent(event)
}

// removes tracking state for positions that are no longer open
func (m *Monitor) Cleanup() {
	openIDs := make(map[string]bool)
	for _, pos := range m.executor.AllOpen() {
		openIDs[pos.ID] = true
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for id := range m.lastNotified {
		if !openIDs[id] {
			delete(m.lastNotified, id)
			delete(m.lastEventType, id)
		}
	}
}
