// package opsalert provides operational alert fanout for runtime failures.
package opsalert

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

type Alert struct {
	Key        string
	Severity   Severity
	Component  string
	Title      string
	Summary    string
	UserID     int
	Symbol     string
	PositionID string
	Error      string
	Fields     map[string]string
	OccurredAt time.Time
}

type Notifier interface {
	Notify(ctx context.Context, alert Alert) error
}

type NotifierFunc func(ctx context.Context, alert Alert) error

func (f NotifierFunc) Notify(ctx context.Context, alert Alert) error {
	return f(ctx, alert)
}

type Manager struct {
	mu          sync.Mutex
	notifiers   []Notifier
	lastSent    map[string]time.Time
	dedupWindow time.Duration
	now         func() time.Time
}

func NewManager(dedupWindow time.Duration, notifiers ...Notifier) *Manager {
	if dedupWindow <= 0 {
		dedupWindow = 30 * time.Minute
	}
	return &Manager{
		notifiers:   append([]Notifier(nil), notifiers...),
		lastSent:    make(map[string]time.Time),
		dedupWindow: dedupWindow,
		now:         time.Now,
	}
}

func (m *Manager) AddNotifier(n Notifier) {
	if m == nil || n == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifiers = append(m.notifiers, n)
}

func (m *Manager) Notify(ctx context.Context, alert Alert) error {
	if m == nil {
		return nil
	}
	alert = normalize(alert, m.now())
	if alert.Key == "" {
		return fmt.Errorf("ops alert missing key: %s", alert.Title)
	}

	m.mu.Lock()
	if last, ok := m.lastSent[alert.Key]; ok && m.now().Sub(last) < m.dedupWindow {
		m.mu.Unlock()
		return nil
	}
	m.lastSent[alert.Key] = m.now()
	notifiers := append([]Notifier(nil), m.notifiers...)
	m.mu.Unlock()

	slog.Log(ctx, slog.LevelWarn, "operational alert",
		"key", alert.Key,
		"severity", alert.Severity,
		"component", alert.Component,
		"title", alert.Title,
		"summary", alert.Summary,
		"symbol", alert.Symbol,
		"position", alert.PositionID,
	)

	var errs []error
	for _, n := range notifiers {
		if n == nil {
			continue
		}
		notifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := n.Notify(notifyCtx, alert)
		cancel()
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func normalize(alert Alert, now time.Time) Alert {
	if alert.OccurredAt.IsZero() {
		alert.OccurredAt = now
	}
	if alert.Severity == "" {
		alert.Severity = SeverityWarning
	}
	if alert.Key == "" {
		parts := []string{string(alert.Severity), alert.Component, alert.Title, alert.Symbol, alert.PositionID}
		alert.Key = strings.ToLower(strings.Join(parts, ":"))
	}
	return alert
}

func FormatText(alert Alert) string {
	alert = normalize(alert, time.Now())
	var b strings.Builder
	b.WriteString(strings.ToUpper(string(alert.Severity)))
	b.WriteString(": ")
	b.WriteString(alert.Title)
	if alert.Component != "" {
		b.WriteString("\nComponent: ")
		b.WriteString(alert.Component)
	}
	if alert.Summary != "" {
		b.WriteString("\n")
		b.WriteString(alert.Summary)
	}
	if alert.UserID > 0 {
		fmt.Fprintf(&b, "\nUser ID: %d", alert.UserID)
	}
	if alert.Symbol != "" {
		b.WriteString("\nSymbol: ")
		b.WriteString(alert.Symbol)
	}
	if alert.PositionID != "" {
		b.WriteString("\nPosition: ")
		b.WriteString(alert.PositionID)
	}
	if alert.Error != "" {
		b.WriteString("\nError: ")
		b.WriteString(alert.Error)
	}
	for _, k := range sortedKeys(alert.Fields) {
		b.WriteString("\n")
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(alert.Fields[k])
	}
	b.WriteString("\nTime: ")
	b.WriteString(alert.OccurredAt.UTC().Format(time.RFC3339))
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
