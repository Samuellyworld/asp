package alerts

import (
	"context"
	"testing"
	"time"
)

type mockStore struct {
	alerts map[int]*Alert
	nextID int
}

func newMockStore() *mockStore {
	return &mockStore{alerts: make(map[int]*Alert), nextID: 1}
}

func (s *mockStore) Create(_ context.Context, a *Alert) (int, error) {
	id := s.nextID
	s.nextID++
	a.ID = id
	a.IsActive = true
	s.alerts[id] = a
	return id, nil
}

func (s *mockStore) ListActive(_ context.Context, userID int) ([]*Alert, error) {
	var result []*Alert
	for _, a := range s.alerts {
		if a.UserID == userID && a.IsActive {
			result = append(result, a)
		}
	}
	return result, nil
}

func (s *mockStore) ListAllActive(_ context.Context) ([]*Alert, error) {
	var result []*Alert
	for _, a := range s.alerts {
		if a.IsActive {
			result = append(result, a)
		}
	}
	return result, nil
}

func (s *mockStore) MarkTriggered(_ context.Context, alertID int) error {
	if a, ok := s.alerts[alertID]; ok {
		a.IsTriggered = true
		a.IsActive = false
		now := time.Now()
		a.TriggeredAt = &now
	}
	return nil
}

func (s *mockStore) Delete(_ context.Context, alertID int, userID int) error {
	if a, ok := s.alerts[alertID]; ok && a.UserID == userID {
		delete(s.alerts, alertID)
		return nil
	}
	return nil
}

type mockNotifier struct {
	notifications []notifRecord
}

type notifRecord struct {
	userID int
	alert  *Alert
	price  float64
}

func (n *mockNotifier) NotifyAlert(userID int, alert *Alert, price float64) error {
	n.notifications = append(n.notifications, notifRecord{userID, alert, price})
	return nil
}

func TestPriceAboveAlert(t *testing.T) {
	store := newMockStore()
	notifier := &mockNotifier{}
	mgr := NewManager(store, notifier)

	ctx := context.Background()
	mgr.Create(ctx, &Alert{
		UserID:         1,
		Symbol:         "BTCUSDT",
		AlertType:      TypePriceAbove,
		ConditionValue: 70000,
	})

	// price below threshold — no trigger
	mgr.OnPriceUpdate("BTCUSDT", 69999)
	if len(notifier.notifications) != 0 {
		t.Fatal("should not trigger below threshold")
	}

	// price at threshold — triggers
	mgr.OnPriceUpdate("BTCUSDT", 70000)
	if len(notifier.notifications) != 1 {
		t.Fatal("should trigger at threshold")
	}
	if notifier.notifications[0].price != 70000 {
		t.Fatalf("wrong price: %f", notifier.notifications[0].price)
	}

	// should not trigger again
	mgr.OnPriceUpdate("BTCUSDT", 71000)
	if len(notifier.notifications) != 1 {
		t.Fatal("should not trigger twice")
	}
}

func TestPriceBelowAlert(t *testing.T) {
	store := newMockStore()
	notifier := &mockNotifier{}
	mgr := NewManager(store, notifier)

	ctx := context.Background()
	mgr.Create(ctx, &Alert{
		UserID:         1,
		Symbol:         "ETHUSDT",
		AlertType:      TypePriceBelow,
		ConditionValue: 3000,
	})

	mgr.OnPriceUpdate("ETHUSDT", 3100)
	if len(notifier.notifications) != 0 {
		t.Fatal("should not trigger above threshold")
	}

	mgr.OnPriceUpdate("ETHUSDT", 2999)
	if len(notifier.notifications) != 1 {
		t.Fatal("should trigger below threshold")
	}
}

func TestMultipleAlertsPerSymbol(t *testing.T) {
	store := newMockStore()
	notifier := &mockNotifier{}
	mgr := NewManager(store, notifier)

	ctx := context.Background()
	mgr.Create(ctx, &Alert{UserID: 1, Symbol: "BTCUSDT", AlertType: TypePriceAbove, ConditionValue: 70000})
	mgr.Create(ctx, &Alert{UserID: 2, Symbol: "BTCUSDT", AlertType: TypePriceAbove, ConditionValue: 75000})

	mgr.OnPriceUpdate("BTCUSDT", 72000)
	if len(notifier.notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifier.notifications))
	}
	if notifier.notifications[0].alert.ConditionValue != 70000 {
		t.Fatal("wrong alert triggered")
	}

	mgr.OnPriceUpdate("BTCUSDT", 76000)
	if len(notifier.notifications) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(notifier.notifications))
	}
}

func TestDeleteAlert(t *testing.T) {
	store := newMockStore()
	notifier := &mockNotifier{}
	mgr := NewManager(store, notifier)

	ctx := context.Background()
	id, _ := mgr.Create(ctx, &Alert{
		UserID:         1,
		Symbol:         "BTCUSDT",
		AlertType:      TypePriceAbove,
		ConditionValue: 70000,
	})

	mgr.Delete(ctx, id, 1)

	mgr.OnPriceUpdate("BTCUSDT", 80000)
	if len(notifier.notifications) != 0 {
		t.Fatal("deleted alert should not trigger")
	}
}

func TestFormatAlertTriggered(t *testing.T) {
	a := &Alert{Symbol: "BTCUSDT", AlertType: TypePriceAbove, ConditionValue: 70000}
	msg := FormatAlertTriggered(a, 70500)
	if msg == "" {
		t.Fatal("expected non-empty message")
	}
}
