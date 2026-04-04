// position monitor goroutine. periodically checks prices against open positions,
// detects sl/tp hits, milestone breaches, and sends periodic updates
// with cooldown enforcement and duplicate suppression.
package papertrading

import (
	"context"
	"sync"
	"time"
)

// notification event types
type EventType string

const (
	EventTradeExecuted  EventType = "trade_executed"
	EventTPHit          EventType = "tp_hit"
	EventSLHit          EventType = "sl_hit"
	EventTrailingStop   EventType = "trailing_stop"
	EventManualClose    EventType = "manual_close"
	EventMilestone      EventType = "milestone"
	EventPeriodicUpdate EventType = "periodic_update"
	EventDailySummary   EventType = "daily_summary"
)

// a notification event emitted by the monitor
type Event struct {
	Type      EventType
	Position  *Position
	Milestone float64
	Summary   *DailySummary
	IsUrgent  bool
}

// monitor configuration
type MonitorConfig struct {
	CheckInterval          time.Duration
	CooldownPeriod         time.Duration
	SmallPositionInterval  time.Duration // position < $100
	MediumPositionInterval time.Duration // position $100-$500
	LargePositionInterval  time.Duration // position > $500
}

func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		CheckInterval:          60 * time.Second,
		CooldownPeriod:         15 * time.Minute,
		SmallPositionInterval:  1 * time.Hour,
		MediumPositionInterval: 30 * time.Minute,
		LargePositionInterval:  15 * time.Minute,
	}
}

// background goroutine that monitors open positions for price events
type Monitor struct {
	executor *Executor
	prices   PriceProvider
	config   MonitorConfig
	OnEvent  func(Event)

	mu            sync.Mutex
	lastNotified  map[string]time.Time  // posID -> last notification time
	lastEventType map[string]EventType  // posID -> last event type for dedup
	lastEventKey  map[string]string     // posID -> dedup key for last event

	cancel  context.CancelFunc
	done    chan struct{}
	running bool
}

func NewMonitor(executor *Executor, prices PriceProvider, config MonitorConfig) *Monitor {
	return &Monitor{
		executor:      executor,
		prices:        prices,
		config:        config,
		lastNotified:  make(map[string]time.Time),
		lastEventType: make(map[string]EventType),
		lastEventKey:  make(map[string]string),
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

// iterates all open positions and checks for price events.
// exported for testing — also called automatically by the monitor loop.
func (m *Monitor) CheckPositions() {
	positions := m.executor.AllOpen()

	for _, pos := range positions {
		price, err := m.prices.GetPrice(pos.Symbol)
		if err != nil {
			continue
		}

		m.executor.UpdatePrice(pos.ID, price)

		// stop loss hit — urgent, bypasses cooldown
		if pos.IsSLHit() {
			closed, err := m.executor.Close(pos.ID, CloseSL, price)
			if err == nil {
				m.emit(Event{Type: EventSLHit, Position: closed, IsUrgent: true})
			}
			continue
		}

		// take profit hit — urgent, bypasses cooldown
		if pos.IsTPHit() {
			closed, err := m.executor.Close(pos.ID, CloseTP, price)
			if err == nil {
				m.emit(Event{Type: EventTPHit, Position: closed, IsUrgent: true})
			}
			continue
		}

		// update trailing stop and check if hit — urgent
		pos.UpdateTrailingStop()
		if pos.IsTrailingStopHit() {
			closed, err := m.executor.Close(pos.ID, CloseTrailingStop, price)
			if err == nil {
				m.emit(Event{Type: EventTrailingStop, Position: closed, IsUrgent: true})
			}
			continue
		}

		// milestone detection — recorded individually
		milestones := pos.NewMilestones()
		for _, milestone := range milestones {
			pos.HitMilestones[milestone] = true
			m.emit(Event{Type: EventMilestone, Position: pos, Milestone: milestone, IsUrgent: false})
			m.recordNotification(pos.ID, EventMilestone)
		}

		// periodic updates — respects cooldown and position size intervals
		if m.shouldSendPeriodic(pos) {
			m.emit(Event{Type: EventPeriodicUpdate, Position: pos, IsUrgent: false})
			m.recordNotification(pos.ID, EventPeriodicUpdate)
		}
	}
}

// determines if a periodic update should be sent based on position size and cooldown
func (m *Monitor) shouldSendPeriodic(pos *Position) bool {
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

// checks whether the cooldown period has elapsed for non-urgent events
func (m *Monitor) CanNotify(posID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	lastTime, exists := m.lastNotified[posID]
	if !exists {
		return true
	}

	return time.Since(lastTime) >= m.config.CooldownPeriod
}

func (m *Monitor) recordNotification(posID string, eventType EventType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastNotified[posID] = time.Now()
	m.lastEventType[posID] = eventType
}

// checks if the exact same event type was just sent within the cooldown window
func (m *Monitor) isDuplicate(posID string, eventType EventType) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	lastType, exists := m.lastEventType[posID]
	if !exists {
		return false
	}

	lastTime := m.lastNotified[posID]
	return lastType == eventType && time.Since(lastTime) < m.config.CooldownPeriod
}

func (m *Monitor) emit(event Event) {
	if m.OnEvent == nil {
		return
	}

	// urgent events always fire (sl/tp hits, trade executed)
	if event.IsUrgent {
		m.OnEvent(event)
		return
	}

	// milestones bypass cooldown but each threshold fires only once
	if event.Type == EventMilestone {
		m.OnEvent(event)
		return
	}

	// non-urgent events respect cooldown and dedup
	if !m.CanNotify(event.Position.ID) {
		return
	}

	if m.isDuplicate(event.Position.ID, event.Type) {
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
			delete(m.lastEventKey, id)
		}
	}
}
