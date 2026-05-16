package opsalert

import (
	"context"
	"strings"
	"testing"
	"time"
)

type recordingNotifier struct {
	alerts []Alert
}

func (r *recordingNotifier) Notify(_ context.Context, alert Alert) error {
	r.alerts = append(r.alerts, alert)
	return nil
}

func TestManagerDeduplicatesByKey(t *testing.T) {
	rec := &recordingNotifier{}
	m := NewManager(time.Hour, rec)
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }

	alert := Alert{Key: "same", Title: "Failure", Summary: "first"}
	if err := m.Notify(context.Background(), alert); err != nil {
		t.Fatalf("first notify: %v", err)
	}
	if err := m.Notify(context.Background(), alert); err != nil {
		t.Fatalf("second notify: %v", err)
	}
	if len(rec.alerts) != 1 {
		t.Fatalf("expected 1 alert after dedupe, got %d", len(rec.alerts))
	}

	now = now.Add(2 * time.Hour)
	if err := m.Notify(context.Background(), alert); err != nil {
		t.Fatalf("third notify: %v", err)
	}
	if len(rec.alerts) != 2 {
		t.Fatalf("expected alert after dedupe window, got %d", len(rec.alerts))
	}
}

func TestFormatTextIncludesOperationalFields(t *testing.T) {
	msg := FormatText(Alert{
		Severity:   SeverityCritical,
		Component:  "live_trading",
		Title:      "Missing stop loss",
		Summary:    "Position opened without protection.",
		UserID:     7,
		Symbol:     "BTCUSDT",
		PositionID: "live_42",
		Error:      "stop order rejected",
		Fields:     map[string]string{"exchange": "bybit"},
		OccurredAt: time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
	})
	for _, want := range []string{"CRITICAL", "Missing stop loss", "BTCUSDT", "live_42", "bybit"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q: %s", want, msg)
		}
	}
}

func TestBuildEmailIncludesHTMLAndPlainText(t *testing.T) {
	msg, err := buildEmail("bot@example.com", []string{"ops@example.com"}, "subject", Alert{
		Severity:  SeverityWarning,
		Component: "scanner",
		Title:     "Scanner stale",
		Summary:   "No scan cycle completed recently.",
	})
	if err != nil {
		t.Fatalf("buildEmail: %v", err)
	}
	text := string(msg)
	for _, want := range []string{"multipart/alternative", "Scanner stale", "text/html", "text/plain"} {
		if !strings.Contains(text, want) {
			t.Fatalf("email missing %q: %s", want, text)
		}
	}
}
