// price and indicator alert system. evaluates alert conditions against
// live prices from the websocket feed and triggers notifications.
package alerts

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// alert types matching the DB schema CHECK constraint
const (
	TypePriceAbove    = "PRICE_ABOVE"
	TypePriceBelow    = "PRICE_BELOW"
	TypeRSIOverbought = "RSI_OVERBOUGHT"
	TypeRSIOversold   = "RSI_OVERSOLD"
	TypeMACDCross     = "MACD_CROSS"
	TypeVolumeSpike   = "VOLUME_SPIKE"
	TypeCustom        = "CUSTOM"
)

// represents a user-defined alert
type Alert struct {
	ID             int
	UserID         int
	Symbol         string
	AlertType      string
	ConditionValue float64
	IsActive       bool
	IsTriggered    bool
	TriggeredAt    *time.Time
	CreatedAt      time.Time
}

// persists alerts to the database
type Store interface {
	Create(ctx context.Context, a *Alert) (int, error)
	ListActive(ctx context.Context, userID int) ([]*Alert, error)
	ListAllActive(ctx context.Context) ([]*Alert, error)
	MarkTriggered(ctx context.Context, alertID int) error
	Delete(ctx context.Context, alertID int, userID int) error
}

// sends alert notifications to the user
type Notifier interface {
	NotifyAlert(userID int, alert *Alert, currentPrice float64) error
}

// manages price + indicator alerts with real-time evaluation
type Manager struct {
	store    Store
	notifier Notifier

	mu     sync.RWMutex
	active map[string][]*Alert // symbol -> active alerts
	loaded bool
}

func NewManager(store Store, notifier Notifier) *Manager {
	return &Manager{
		store:    store,
		notifier: notifier,
		active:   make(map[string][]*Alert),
	}
}

// loads all active alerts into memory for fast evaluation
func (m *Manager) LoadActive(ctx context.Context) error {
	alerts, err := m.store.ListAllActive(ctx)
	if err != nil {
		return fmt.Errorf("failed to load active alerts: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.active = make(map[string][]*Alert)
	for _, a := range alerts {
		m.active[a.Symbol] = append(m.active[a.Symbol], a)
	}
	m.loaded = true

	slog.Info("alerts loaded", "count", len(alerts))
	return nil
}

// creates a new alert and adds it to the in-memory cache
func (m *Manager) Create(ctx context.Context, a *Alert) (int, error) {
	id, err := m.store.Create(ctx, a)
	if err != nil {
		return 0, err
	}
	a.ID = id

	m.mu.Lock()
	m.active[a.Symbol] = append(m.active[a.Symbol], a)
	m.mu.Unlock()

	return id, nil
}

// deletes an alert
func (m *Manager) Delete(ctx context.Context, alertID int, userID int) error {
	if err := m.store.Delete(ctx, alertID, userID); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for symbol, alerts := range m.active {
		for i, a := range alerts {
			if a.ID == alertID {
				m.active[symbol] = append(alerts[:i], alerts[i+1:]...)
				break
			}
		}
	}
	return nil
}

// called on every price update from the websocket feed.
// evaluates PRICE_ABOVE and PRICE_BELOW conditions.
func (m *Manager) OnPriceUpdate(symbol string, price float64) {
	m.mu.RLock()
	alerts := m.active[symbol]
	if len(alerts) == 0 {
		m.mu.RUnlock()
		return
	}
	// copy to avoid holding lock during notification
	toCheck := make([]*Alert, len(alerts))
	copy(toCheck, alerts)
	m.mu.RUnlock()

	var triggered []*Alert
	for _, a := range toCheck {
		if !a.IsActive || a.IsTriggered {
			continue
		}

		hit := false
		switch a.AlertType {
		case TypePriceAbove:
			hit = price >= a.ConditionValue
		case TypePriceBelow:
			hit = price <= a.ConditionValue
		}

		if hit {
			triggered = append(triggered, a)
		}
	}

	for _, a := range triggered {
		a.IsTriggered = true
		a.IsActive = false
		now := time.Now()
		a.TriggeredAt = &now

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := m.store.MarkTriggered(ctx, a.ID); err != nil {
			slog.Error("failed to mark alert triggered", "alert_id", a.ID, "error", err)
		}
		cancel()

		if m.notifier != nil {
			if err := m.notifier.NotifyAlert(a.UserID, a, price); err != nil {
				slog.Error("failed to send alert notification", "alert_id", a.ID, "error", err)
			}
		}

		// remove from active cache
		m.mu.Lock()
		alerts := m.active[symbol]
		for i, ca := range alerts {
			if ca.ID == a.ID {
				m.active[symbol] = append(alerts[:i], alerts[i+1:]...)
				break
			}
		}
		m.mu.Unlock()

		slog.Info("alert triggered", "alert_id", a.ID, "type", a.AlertType, "symbol", symbol, "price", price, "condition", a.ConditionValue)
	}
}

// lists active alerts for a user
func (m *Manager) ListUser(ctx context.Context, userID int) ([]*Alert, error) {
	return m.store.ListActive(ctx, userID)
}

// formats an alert trigger message
func FormatAlertTriggered(a *Alert, price float64) string {
	switch a.AlertType {
	case TypePriceAbove:
		return fmt.Sprintf("🔔 Alert: %s above $%.2f\nCurrent: $%.2f", a.Symbol, a.ConditionValue, price)
	case TypePriceBelow:
		return fmt.Sprintf("🔔 Alert: %s below $%.2f\nCurrent: $%.2f", a.Symbol, a.ConditionValue, price)
	default:
		return fmt.Sprintf("🔔 Alert triggered: %s %s (condition: %.2f, price: %.2f)", a.Symbol, a.AlertType, a.ConditionValue, price)
	}
}
