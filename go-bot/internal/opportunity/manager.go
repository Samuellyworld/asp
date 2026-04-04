// opportunity management for the approval flow.
// tracks pending trading opportunities with approve/reject/modify buttons
// and auto-expiry after a configurable timeout.
package opportunity

import (
	"fmt"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/pipeline"
)

// possible states for an opportunity
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusExpired  Status = "expired"
	StatusModified Status = "modified"
)

// a trading opportunity detected by the scanner
type Opportunity struct {
	ID        string
	UserID    int
	Symbol    string
	Action    claude.Action
	Result    *pipeline.Result
	Status    Status
	CreatedAt time.Time
	ResolvedAt *time.Time

	// modified trade plan (set when user modifies)
	ModifiedPlan *claude.TradePlan

	// leverage options (zero means spot trade)
	UseLeverage  bool
	Leverage     int
	PositionSide string // "LONG" or "SHORT" (empty for spot)

	// platform tracking for the notification
	Platform  string // "telegram" or "discord"
	MessageID int    // telegram message id for editing
	ChannelID string // discord channel id
}

// configuration for the opportunity manager
type Config struct {
	ExpiryDuration time.Duration
	CleanupInterval time.Duration
}

// returns sensible defaults
func DefaultConfig() Config {
	return Config{
		ExpiryDuration:  15 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}
}

// callback for when an opportunity state changes
type StateChangeCallback func(opp *Opportunity)

// manages pending opportunities with timeout and state tracking
type Manager struct {
	mu            sync.RWMutex
	opportunities map[string]*Opportunity
	config        Config
	nextID        int
	stopCh        chan struct{}
	running       bool

	// callbacks
	onExpire StateChangeCallback

	// for testing
	nowFunc func() time.Time
}

// creates a new opportunity manager
func NewManager(cfg Config) *Manager {
	return &Manager{
		opportunities: make(map[string]*Opportunity),
		config:        cfg,
		stopCh:        make(chan struct{}),
		nowFunc:       time.Now,
	}
}

// sets a callback for when opportunities expire
func (m *Manager) OnExpire(cb StateChangeCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onExpire = cb
}

// creates a new pending opportunity and returns its id
func (m *Manager) Create(userID int, symbol string, result *pipeline.Result, platform string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	id := fmt.Sprintf("opp_%d_%d", userID, m.nextID)

	opp := &Opportunity{
		ID:        id,
		UserID:    userID,
		Symbol:    symbol,
		Action:    result.Decision.Action,
		Result:    result,
		Status:    StatusPending,
		CreatedAt: m.now(),
		Platform:  platform,
	}

	m.opportunities[id] = opp
	return id
}

// retrieves an opportunity by id
func (m *Manager) Get(id string) *Opportunity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.opportunities[id]
}

// retrieves an opportunity by id only if it belongs to the given user
func (m *Manager) GetForUser(id string, userID int) *Opportunity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	opp, ok := m.opportunities[id]
	if !ok || opp.UserID != userID {
		return nil
	}
	return opp
}

// returns all pending opportunities for a user
func (m *Manager) PendingForUser(userID int) []*Opportunity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Opportunity
	for _, opp := range m.opportunities {
		if opp.UserID == userID && opp.Status == StatusPending {
			result = append(result, opp)
		}
	}
	return result
}

// marks an opportunity as approved. returns false if not pending.
func (m *Manager) Approve(id string, userID int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	opp, ok := m.opportunities[id]
	if !ok || opp.UserID != userID || opp.Status != StatusPending {
		return false
	}

	now := m.now()
	opp.Status = StatusApproved
	opp.ResolvedAt = &now
	return true
}

// marks an opportunity as rejected. returns false if not pending.
func (m *Manager) Reject(id string, userID int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	opp, ok := m.opportunities[id]
	if !ok || opp.UserID != userID || opp.Status != StatusPending {
		return false
	}

	now := m.now()
	opp.Status = StatusRejected
	opp.ResolvedAt = &now
	return true
}

// marks an opportunity as modified with an updated trade plan. returns false if not pending.
func (m *Manager) Modify(id string, userID int, plan *claude.TradePlan) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	opp, ok := m.opportunities[id]
	if !ok || opp.UserID != userID || opp.Status != StatusPending {
		return false
	}

	now := m.now()
	opp.Status = StatusModified
	opp.ResolvedAt = &now
	opp.ModifiedPlan = plan
	return true
}

// sets leverage parameters on a pending opportunity
func (m *Manager) SetLeverage(id string, userID int, leverage int, side string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	opp, ok := m.opportunities[id]
	if !ok || opp.UserID != userID || opp.Status != StatusPending {
		return false
	}

	opp.UseLeverage = true
	opp.Leverage = leverage
	opp.PositionSide = side
	return true
}

// sets the message tracking info for editing later
func (m *Manager) SetMessageID(id string, messageID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if opp, ok := m.opportunities[id]; ok {
		opp.MessageID = messageID
	}
}

// sets the discord channel id for the notification
func (m *Manager) SetChannelID(id string, channelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if opp, ok := m.opportunities[id]; ok {
		opp.ChannelID = channelID
	}
}

// starts the background expiry checker
func (m *Manager) StartExpiry() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	go m.expiryLoop()
}

// stops the background expiry checker
func (m *Manager) StopExpiry() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	m.running = false
	close(m.stopCh)
}

// background loop that checks for expired opportunities
func (m *Manager) expiryLoop() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.expireOld()
		}
	}
}

// expires opportunities that have exceeded the timeout
func (m *Manager) expireOld() {
	m.mu.Lock()

	cutoff := m.now().Add(-m.config.ExpiryDuration)
	var expired []*Opportunity

	for _, opp := range m.opportunities {
		if opp.Status == StatusPending && opp.CreatedAt.Before(cutoff) {
			now := m.now()
			opp.Status = StatusExpired
			opp.ResolvedAt = &now
			expired = append(expired, opp)
		}
	}

	cb := m.onExpire
	m.mu.Unlock()

	// fire callbacks outside the lock
	if cb != nil {
		for _, opp := range expired {
			cb(opp)
		}
	}
}

// removes resolved opportunities older than the given age
func (m *Manager) Cleanup(maxAge time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := m.now().Add(-maxAge)
	removed := 0

	for id, opp := range m.opportunities {
		if opp.Status != StatusPending && opp.ResolvedAt != nil && opp.ResolvedAt.Before(cutoff) {
			delete(m.opportunities, id)
			removed++
		}
	}

	return removed
}

// returns counts by status
func (m *Manager) Stats() map[Status]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	counts := make(map[Status]int)
	for _, opp := range m.opportunities {
		counts[opp.Status]++
	}
	return counts
}

// returns the total number of tracked opportunities
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.opportunities)
}

func (m *Manager) now() time.Time {
	return m.nowFunc()
}
