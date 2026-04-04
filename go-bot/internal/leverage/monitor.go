// leverage position monitor. checks mark prices at 30s intervals for tp/sl
// hits, tiered liquidation alerts with escalating severity, auto-close at
// 2% from liquidation, and funding fee detection.
package leverage

import (
	"context"
	"sync"
	"time"
)

// event types for leverage position monitoring
type LevEventType string

const (
	LevEventOpened         LevEventType = "leverage_opened"
	LevEventTPHit          LevEventType = "tp_hit"
	LevEventSLHit          LevEventType = "sl_hit"
	LevEventTrailingStop   LevEventType = "trailing_stop"
	LevEventLiqWarning     LevEventType = "liquidation_warning"
	LevEventLiqCritical    LevEventType = "liquidation_critical"
	LevEventAutoClose      LevEventType = "auto_close"
	LevEventFundingFee     LevEventType = "funding_fee"
	LevEventPeriodicUpdate LevEventType = "periodic_update"
	LevEventClosed         LevEventType = "closed"
)

// a monitoring event emitted when a leverage position state changes
type LevEvent struct {
	Type          LevEventType
	Position      *LeveragePosition
	DistancePct   float64    // distance to liquidation
	AlertLevel    AlertLevel
	FundingRate   float64 // for funding events
	FundingAmount float64
	IsUrgent      bool
}

// provides current mark price for a symbol
type MarkPriceProvider interface {
	GetMarkPrice(symbol string) (float64, error)
}

// can close a position (paper or live executor)
type PositionCloser interface {
	Close(posID string, reason string) (*LeveragePosition, error)
}

// provides list of open positions
type PositionLister interface {
	AllOpen() []*LeveragePosition
}

// updates mark price on a position under the executor's mutex
type MarkPriceUpdater interface {
	UpdateMarkPrice(posID string, price float64)
}

// monitor configuration with per-alert-level cooldowns
type MonitorConfig struct {
	CheckInterval    time.Duration // 30s default
	CooldownPeriod   time.Duration // 5min for normal updates
	WarningCooldown  time.Duration // 2min for warnings
	CriticalCooldown time.Duration // 30s for critical
}

// returns sensible defaults for leverage monitoring
func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		CheckInterval:    30 * time.Second,
		CooldownPeriod:   5 * time.Minute,
		WarningCooldown:  2 * time.Minute,
		CriticalCooldown: 30 * time.Second,
	}
}

// background goroutine that monitors leverage positions for mark price
// changes, liquidation proximity, tp/sl hits, and funding fee events
type Monitor struct {
	lister       PositionLister
	closer       PositionCloser
	priceUpdater MarkPriceUpdater
	prices       MarkPriceProvider
	funding      *FundingTracker
	config       MonitorConfig
	OnEvent      func(LevEvent)

	mu             sync.Mutex
	lastNotified   map[string]time.Time
	lastAlertLevel map[string]AlertLevel

	cancel  context.CancelFunc
	done    chan struct{}
	running bool
}

// creates a new leverage position monitor
func NewMonitor(
	lister PositionLister,
	closer PositionCloser,
	prices MarkPriceProvider,
	funding *FundingTracker,
	config MonitorConfig,
	opts ...MonitorOption,
) *Monitor {
	m := &Monitor{
		lister:         lister,
		closer:         closer,
		prices:         prices,
		funding:        funding,
		config:         config,
		lastNotified:   make(map[string]time.Time),
		lastAlertLevel: make(map[string]AlertLevel),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// MonitorOption configures optional monitor dependencies
type MonitorOption func(*Monitor)

// WithMarkPriceUpdater sets the mark price updater for thread-safe position updates
func WithMarkPriceUpdater(u MarkPriceUpdater) MonitorOption {
	return func(m *Monitor) {
		m.priceUpdater = u
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

// returns whether the monitor is currently running
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

// checks all open positions for mark price changes, tp/sl hits,
// liquidation proximity, and funding fees.
// exported for testing — also called automatically by the monitor loop.
func (m *Monitor) CheckPositions() {
	positions := m.lister.AllOpen()

	for _, pos := range positions {
		m.checkPosition(pos)
	}
}

// checks a single position for all conditions
func (m *Monitor) checkPosition(pos *LeveragePosition) {
	// fetch mark price
	markPrice, err := m.prices.GetMarkPrice(pos.Symbol)
	if err != nil {
		return
	}

	// update position mark price through executor's mutex if available
	if m.priceUpdater != nil {
		m.priceUpdater.UpdateMarkPrice(pos.ID, markPrice)
	} else {
		pos.MarkPrice = markPrice
	}

	// check tp/sl hits first (highest priority)
	if pos.IsTPHit() {
		closed, err := m.closer.Close(pos.ID, "take_profit")
		if err != nil {
			return
		}
		m.emit(LevEvent{
			Type:     LevEventTPHit,
			Position: closed,
			IsUrgent: true,
		})
		return
	}

	if pos.IsSLHit() {
		closed, err := m.closer.Close(pos.ID, "stop_loss")
		if err != nil {
			return
		}
		m.emit(LevEvent{
			Type:     LevEventSLHit,
			Position: closed,
			IsUrgent: true,
		})
		return
	}

	// update trailing stop and check if hit
	pos.UpdateTrailingStop()
	if pos.IsTrailingStopHit() {
		closed, err := m.closer.Close(pos.ID, "trailing_stop")
		if err != nil {
			return
		}
		m.emit(LevEvent{
			Type:     LevEventTrailingStop,
			Position: closed,
			IsUrgent: true,
		})
		return
	}

	// calculate liquidation distance
	distancePct := DistanceToLiquidation(markPrice, pos.LiquidationPrice, string(pos.Side))
	level := ClassifyLiquidationRisk(distancePct)

	// tiered alert logic
	switch level {
	case AlertAutoClose:
		closed, err := m.closer.Close(pos.ID, "auto_close")
		if err != nil {
			return
		}
		m.emit(LevEvent{
			Type:        LevEventAutoClose,
			Position:    closed,
			DistancePct: distancePct,
			AlertLevel:  AlertAutoClose,
			IsUrgent:    true,
		})
		return

	case AlertCritical:
		if m.shouldNotify(pos.ID, AlertCritical) {
			m.emit(LevEvent{
				Type:        LevEventLiqCritical,
				Position:    pos,
				DistancePct: distancePct,
				AlertLevel:  AlertCritical,
				IsUrgent:    true,
			})
			m.recordNotification(pos.ID, AlertCritical)
		}

	case AlertWarning:
		if m.shouldNotify(pos.ID, AlertWarning) {
			m.emit(LevEvent{
				Type:        LevEventLiqWarning,
				Position:    pos,
				DistancePct: distancePct,
				AlertLevel:  AlertWarning,
				IsUrgent:    true,
			})
			m.recordNotification(pos.ID, AlertWarning)
		}

	case AlertNone:
		if m.shouldNotify(pos.ID, AlertNone) {
			m.emit(LevEvent{
				Type:        LevEventPeriodicUpdate,
				Position:    pos,
				DistancePct: distancePct,
				AlertLevel:  AlertNone,
				IsUrgent:    false,
			})
			m.recordNotification(pos.ID, AlertNone)
		}
	}

	// check funding fees
	if m.funding != nil && m.funding.IsFundingDue(pos.ID) {
		m.emit(LevEvent{
			Type:     LevEventFundingFee,
			Position: pos,
			IsUrgent: false,
		})
	}
}

// determines whether a notification should be sent for a position based
// on cooldown and alert level escalation
func (m *Monitor) shouldNotify(posID string, level AlertLevel) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	lastTime, exists := m.lastNotified[posID]
	if !exists {
		return true
	}

	// escalation: if alert level increased since last notification, always notify
	prevLevel, hasLevel := m.lastAlertLevel[posID]
	if hasLevel && level > prevLevel {
		return true
	}

	cooldown := m.cooldownForLevel(level)
	return time.Since(lastTime) >= cooldown
}

// records a notification timestamp and alert level for a position
func (m *Monitor) recordNotification(posID string, level AlertLevel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastNotified[posID] = time.Now()
	m.lastAlertLevel[posID] = level
}

// returns the appropriate cooldown duration for an alert level
func (m *Monitor) cooldownForLevel(level AlertLevel) time.Duration {
	switch level {
	case AlertCritical:
		return m.config.CriticalCooldown
	case AlertWarning:
		return m.config.WarningCooldown
	default:
		return m.config.CooldownPeriod
	}
}

// sends an event to the registered handler
func (m *Monitor) emit(event LevEvent) {
	if m.OnEvent == nil {
		return
	}
	m.OnEvent(event)
}

// removes tracking state for positions that are no longer open
func (m *Monitor) Cleanup() {
	openIDs := make(map[string]bool)
	for _, pos := range m.lister.AllOpen() {
		openIDs[pos.ID] = true
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for id := range m.lastNotified {
		if !openIDs[id] {
			delete(m.lastNotified, id)
			delete(m.lastAlertLevel, id)
		}
	}
}
