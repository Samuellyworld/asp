package preferences

import (
	"context"
	"fmt"
	"testing"
)

// mock repository for preferences tests
type mockRepo struct {
	scanning     map[int]*Scanning
	notification map[int]*Notification
	trading      map[int]*Trading

	getScanErr    error
	updateScanErr error
	getNotifErr   error
	updateNotifErr error
	getTradeErr   error
	updateTradeErr error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		scanning:     make(map[int]*Scanning),
		notification: make(map[int]*Notification),
		trading:      make(map[int]*Trading),
	}
}

func (m *mockRepo) GetScanning(_ context.Context, userID int) (*Scanning, error) {
	if m.getScanErr != nil {
		return nil, m.getScanErr
	}
	return m.scanning[userID], nil
}

func (m *mockRepo) UpdateScanning(_ context.Context, s *Scanning) error {
	if m.updateScanErr != nil {
		return m.updateScanErr
	}
	m.scanning[s.UserID] = s
	return nil
}

func (m *mockRepo) GetNotification(_ context.Context, userID int) (*Notification, error) {
	if m.getNotifErr != nil {
		return nil, m.getNotifErr
	}
	return m.notification[userID], nil
}

func (m *mockRepo) UpdateNotification(_ context.Context, n *Notification) error {
	if m.updateNotifErr != nil {
		return m.updateNotifErr
	}
	m.notification[n.UserID] = n
	return nil
}

func (m *mockRepo) GetTrading(_ context.Context, userID int) (*Trading, error) {
	if m.getTradeErr != nil {
		return nil, m.getTradeErr
	}
	return m.trading[userID], nil
}

func (m *mockRepo) UpdateTrading(_ context.Context, t *Trading) error {
	if m.updateTradeErr != nil {
		return m.updateTradeErr
	}
	m.trading[t.UserID] = t
	return nil
}

// seed helpers
func (m *mockRepo) seedScanning(userID int) {
	m.scanning[userID] = &Scanning{
		UserID:            userID,
		MinConfidence:     60,
		ScanIntervalMins:  5,
		EnabledTimeframes: []string{"1h", "4h"},
		EnabledIndicators: []string{"rsi", "macd"},
		IsScanningEnabled: true,
	}
}

func (m *mockRepo) seedNotification(userID int) {
	m.notification[userID] = &Notification{
		UserID:                userID,
		MaxDailyNotifications: 50,
		Timezone:              "UTC",
		DailySummaryHour:      9,
		DailySummaryEnabled:   true,
	}
}

func (m *mockRepo) seedTrading(userID int) {
	m.trading[userID] = &Trading{
		UserID:               userID,
		DefaultPositionSize:  100,
		MaxPositionSize:      300,
		DefaultStopLossPct:   2.0,
		DefaultTakeProfitPct: 6.0,
		MaxLeverage:          10,
		RiskPerTradePct:      1.0,
		MarginMode:           "cross",
	}
}

func (m *mockRepo) seedAll(userID int) {
	m.seedScanning(userID)
	m.seedNotification(userID)
	m.seedTrading(userID)
}

// --- scanning tests ---

func TestGetScanning_Found(t *testing.T) {
	repo := newMockRepo()
	repo.seedScanning(1)
	svc := NewService(repo)

	prefs, err := svc.GetScanning(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetScanning() error: %v", err)
	}
	if prefs.MinConfidence != 60 {
		t.Errorf("MinConfidence = %d, want 60", prefs.MinConfidence)
	}
}

func TestGetScanning_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.GetScanning(context.Background(), 999)
	if err == nil {
		t.Fatal("GetScanning() expected error for missing prefs")
	}
}

func TestGetScanning_RepoError(t *testing.T) {
	repo := newMockRepo()
	repo.getScanErr = fmt.Errorf("db down")
	svc := NewService(repo)

	_, err := svc.GetScanning(context.Background(), 1)
	if err == nil {
		t.Fatal("GetScanning() expected error")
	}
}

func TestSetMinConfidence_Valid(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"lower bound", 0},
		{"mid value", 50},
		{"upper bound", 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedScanning(1)
			svc := NewService(repo)

			err := svc.SetMinConfidence(context.Background(), 1, tt.value)
			if err != nil {
				t.Fatalf("SetMinConfidence(%d) error: %v", tt.value, err)
			}
			if repo.scanning[1].MinConfidence != tt.value {
				t.Errorf("MinConfidence = %d, want %d", repo.scanning[1].MinConfidence, tt.value)
			}
		})
	}
}

func TestSetMinConfidence_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"negative", -1},
		{"too high", 101},
		{"way too high", 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedScanning(1)
			svc := NewService(repo)

			err := svc.SetMinConfidence(context.Background(), 1, tt.value)
			if err == nil {
				t.Fatalf("SetMinConfidence(%d) expected error", tt.value)
			}
		})
	}
}

func TestSetMinConfidence_PrefsNotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	err := svc.SetMinConfidence(context.Background(), 1, 50)
	if err == nil {
		t.Fatal("SetMinConfidence() expected error when prefs not found")
	}
}

func TestSetMinConfidence_UpdateError(t *testing.T) {
	repo := newMockRepo()
	repo.seedScanning(1)
	repo.updateScanErr = fmt.Errorf("write failed")
	svc := NewService(repo)

	err := svc.SetMinConfidence(context.Background(), 1, 50)
	if err == nil {
		t.Fatal("SetMinConfidence() expected error when update fails")
	}
}

func TestSetScanInterval_Valid(t *testing.T) {
	tests := []struct {
		name    string
		minutes int
	}{
		{"lower bound", 1},
		{"mid value", 30},
		{"upper bound", 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedScanning(1)
			svc := NewService(repo)

			err := svc.SetScanInterval(context.Background(), 1, tt.minutes)
			if err != nil {
				t.Fatalf("SetScanInterval(%d) error: %v", tt.minutes, err)
			}
			if repo.scanning[1].ScanIntervalMins != tt.minutes {
				t.Errorf("ScanIntervalMins = %d, want %d", repo.scanning[1].ScanIntervalMins, tt.minutes)
			}
		})
	}
}

func TestSetScanInterval_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		minutes int
	}{
		{"zero", 0},
		{"negative", -5},
		{"too high", 61},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedScanning(1)
			svc := NewService(repo)

			err := svc.SetScanInterval(context.Background(), 1, tt.minutes)
			if err == nil {
				t.Fatalf("SetScanInterval(%d) expected error", tt.minutes)
			}
		})
	}
}

func TestSetScanInterval_PrefsNotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	err := svc.SetScanInterval(context.Background(), 1, 5)
	if err == nil {
		t.Fatal("SetScanInterval() expected error when prefs not found")
	}
}

func TestToggleScanning(t *testing.T) {
	repo := newMockRepo()
	repo.seedScanning(1)
	svc := NewService(repo)

	// disable
	err := svc.ToggleScanning(context.Background(), 1, false)
	if err != nil {
		t.Fatalf("ToggleScanning(false) error: %v", err)
	}
	if repo.scanning[1].IsScanningEnabled {
		t.Error("expected scanning to be disabled")
	}

	// re-enable
	err = svc.ToggleScanning(context.Background(), 1, true)
	if err != nil {
		t.Fatalf("ToggleScanning(true) error: %v", err)
	}
	if !repo.scanning[1].IsScanningEnabled {
		t.Error("expected scanning to be enabled")
	}
}

func TestToggleScanning_PrefsNotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	err := svc.ToggleScanning(context.Background(), 1, true)
	if err == nil {
		t.Fatal("ToggleScanning() expected error when prefs not found")
	}
}

func TestToggleScanning_UpdateError(t *testing.T) {
	repo := newMockRepo()
	repo.seedScanning(1)
	repo.updateScanErr = fmt.Errorf("write failed")
	svc := NewService(repo)

	err := svc.ToggleScanning(context.Background(), 1, false)
	if err == nil {
		t.Fatal("ToggleScanning() expected error when update fails")
	}
}

// --- notification tests ---

func TestGetNotification_Found(t *testing.T) {
	repo := newMockRepo()
	repo.seedNotification(1)
	svc := NewService(repo)

	prefs, err := svc.GetNotification(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetNotification() error: %v", err)
	}
	if prefs.MaxDailyNotifications != 50 {
		t.Errorf("MaxDailyNotifications = %d, want 50", prefs.MaxDailyNotifications)
	}
}

func TestGetNotification_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.GetNotification(context.Background(), 999)
	if err == nil {
		t.Fatal("GetNotification() expected error for missing prefs")
	}
}

func TestGetNotification_RepoError(t *testing.T) {
	repo := newMockRepo()
	repo.getNotifErr = fmt.Errorf("db error")
	svc := NewService(repo)

	_, err := svc.GetNotification(context.Background(), 1)
	if err == nil {
		t.Fatal("GetNotification() expected error")
	}
}

func TestSetMaxDailyNotifications_Valid(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"lower bound", 1},
		{"mid value", 50},
		{"upper bound", 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedNotification(1)
			svc := NewService(repo)

			err := svc.SetMaxDailyNotifications(context.Background(), 1, tt.value)
			if err != nil {
				t.Fatalf("SetMaxDailyNotifications(%d) error: %v", tt.value, err)
			}
			if repo.notification[1].MaxDailyNotifications != tt.value {
				t.Errorf("MaxDailyNotifications = %d, want %d", repo.notification[1].MaxDailyNotifications, tt.value)
			}
		})
	}
}

func TestSetMaxDailyNotifications_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too high", 101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedNotification(1)
			svc := NewService(repo)

			err := svc.SetMaxDailyNotifications(context.Background(), 1, tt.value)
			if err == nil {
				t.Fatalf("SetMaxDailyNotifications(%d) expected error", tt.value)
			}
		})
	}
}

func TestSetTimezone_Valid(t *testing.T) {
	repo := newMockRepo()
	repo.seedNotification(1)
	svc := NewService(repo)

	err := svc.SetTimezone(context.Background(), 1, "America/New_York")
	if err != nil {
		t.Fatalf("SetTimezone() error: %v", err)
	}
	if repo.notification[1].Timezone != "America/New_York" {
		t.Errorf("Timezone = %q, want %q", repo.notification[1].Timezone, "America/New_York")
	}
}

func TestSetTimezone_Empty(t *testing.T) {
	repo := newMockRepo()
	repo.seedNotification(1)
	svc := NewService(repo)

	err := svc.SetTimezone(context.Background(), 1, "")
	if err == nil {
		t.Fatal("SetTimezone() expected error for empty timezone")
	}
}

func TestSetTimezone_Whitespace(t *testing.T) {
	repo := newMockRepo()
	repo.seedNotification(1)
	svc := NewService(repo)

	err := svc.SetTimezone(context.Background(), 1, "   ")
	if err == nil {
		t.Fatal("SetTimezone() expected error for whitespace-only timezone")
	}
}

func TestSetTimezone_PrefsNotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	err := svc.SetTimezone(context.Background(), 1, "UTC")
	if err == nil {
		t.Fatal("SetTimezone() expected error when prefs not found")
	}
}

func TestSetTimezone_UpdateError(t *testing.T) {
	repo := newMockRepo()
	repo.seedNotification(1)
	repo.updateNotifErr = fmt.Errorf("write failed")
	svc := NewService(repo)

	err := svc.SetTimezone(context.Background(), 1, "UTC")
	if err == nil {
		t.Fatal("SetTimezone() expected error when update fails")
	}
}

func TestSetDailySummaryHour_Valid(t *testing.T) {
	tests := []struct {
		name string
		hour int
	}{
		{"midnight", 0},
		{"noon", 12},
		{"last hour", 23},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedNotification(1)
			svc := NewService(repo)

			err := svc.SetDailySummaryHour(context.Background(), 1, tt.hour)
			if err != nil {
				t.Fatalf("SetDailySummaryHour(%d) error: %v", tt.hour, err)
			}
			if repo.notification[1].DailySummaryHour != tt.hour {
				t.Errorf("DailySummaryHour = %d, want %d", repo.notification[1].DailySummaryHour, tt.hour)
			}
		})
	}
}

func TestSetDailySummaryHour_Invalid(t *testing.T) {
	tests := []struct {
		name string
		hour int
	}{
		{"negative", -1},
		{"24", 24},
		{"too high", 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedNotification(1)
			svc := NewService(repo)

			err := svc.SetDailySummaryHour(context.Background(), 1, tt.hour)
			if err == nil {
				t.Fatalf("SetDailySummaryHour(%d) expected error", tt.hour)
			}
		})
	}
}

// --- trading tests ---

func TestGetTrading_Found(t *testing.T) {
	repo := newMockRepo()
	repo.seedTrading(1)
	svc := NewService(repo)

	prefs, err := svc.GetTrading(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetTrading() error: %v", err)
	}
	if prefs.DefaultPositionSize != 100 {
		t.Errorf("DefaultPositionSize = %f, want 100", prefs.DefaultPositionSize)
	}
}

func TestGetTrading_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	_, err := svc.GetTrading(context.Background(), 999)
	if err == nil {
		t.Fatal("GetTrading() expected error for missing prefs")
	}
}

func TestGetTrading_RepoError(t *testing.T) {
	repo := newMockRepo()
	repo.getTradeErr = fmt.Errorf("db error")
	svc := NewService(repo)

	_, err := svc.GetTrading(context.Background(), 1)
	if err == nil {
		t.Fatal("GetTrading() expected error")
	}
}

func TestSetPositionSize_Valid(t *testing.T) {
	tests := []struct {
		name        string
		defaultSize float64
		maxSize     float64
	}{
		{"small position", 10, 30},
		{"equal default and max", 100, 100},
		{"large position", 1000, 5000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedTrading(1)
			svc := NewService(repo)

			err := svc.SetPositionSize(context.Background(), 1, tt.defaultSize, tt.maxSize)
			if err != nil {
				t.Fatalf("SetPositionSize(%f, %f) error: %v", tt.defaultSize, tt.maxSize, err)
			}
			if repo.trading[1].DefaultPositionSize != tt.defaultSize {
				t.Errorf("DefaultPositionSize = %f, want %f", repo.trading[1].DefaultPositionSize, tt.defaultSize)
			}
			if repo.trading[1].MaxPositionSize != tt.maxSize {
				t.Errorf("MaxPositionSize = %f, want %f", repo.trading[1].MaxPositionSize, tt.maxSize)
			}
		})
	}
}

func TestSetPositionSize_Invalid(t *testing.T) {
	tests := []struct {
		name        string
		defaultSize float64
		maxSize     float64
	}{
		{"zero default", 0, 100},
		{"negative default", -10, 100},
		{"max less than default", 100, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedTrading(1)
			svc := NewService(repo)

			err := svc.SetPositionSize(context.Background(), 1, tt.defaultSize, tt.maxSize)
			if err == nil {
				t.Fatalf("SetPositionSize(%f, %f) expected error", tt.defaultSize, tt.maxSize)
			}
		})
	}
}

func TestSetPositionSize_PrefsNotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	err := svc.SetPositionSize(context.Background(), 1, 100, 300)
	if err == nil {
		t.Fatal("SetPositionSize() expected error when prefs not found")
	}
}

func TestSetPositionSize_UpdateError(t *testing.T) {
	repo := newMockRepo()
	repo.seedTrading(1)
	repo.updateTradeErr = fmt.Errorf("write failed")
	svc := NewService(repo)

	err := svc.SetPositionSize(context.Background(), 1, 100, 300)
	if err == nil {
		t.Fatal("SetPositionSize() expected error when update fails")
	}
}

func TestSetStopLoss_Valid(t *testing.T) {
	tests := []struct {
		name string
		pct  float64
	}{
		{"small", 0.1},
		{"typical", 2.0},
		{"upper bound", 50.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedTrading(1)
			svc := NewService(repo)

			err := svc.SetStopLoss(context.Background(), 1, tt.pct)
			if err != nil {
				t.Fatalf("SetStopLoss(%f) error: %v", tt.pct, err)
			}
			if repo.trading[1].DefaultStopLossPct != tt.pct {
				t.Errorf("DefaultStopLossPct = %f, want %f", repo.trading[1].DefaultStopLossPct, tt.pct)
			}
		})
	}
}

func TestSetStopLoss_Invalid(t *testing.T) {
	tests := []struct {
		name string
		pct  float64
	}{
		{"zero", 0},
		{"negative", -1},
		{"too high", 50.1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedTrading(1)
			svc := NewService(repo)

			err := svc.SetStopLoss(context.Background(), 1, tt.pct)
			if err == nil {
				t.Fatalf("SetStopLoss(%f) expected error", tt.pct)
			}
		})
	}
}

func TestSetStopLoss_PrefsNotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	err := svc.SetStopLoss(context.Background(), 1, 2.0)
	if err == nil {
		t.Fatal("SetStopLoss() expected error when prefs not found")
	}
}

func TestSetTakeProfit_Valid(t *testing.T) {
	tests := []struct {
		name string
		pct  float64
	}{
		{"small", 0.1},
		{"typical", 6.0},
		{"upper bound", 100.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedTrading(1)
			svc := NewService(repo)

			err := svc.SetTakeProfit(context.Background(), 1, tt.pct)
			if err != nil {
				t.Fatalf("SetTakeProfit(%f) error: %v", tt.pct, err)
			}
		})
	}
}

func TestSetTakeProfit_Invalid(t *testing.T) {
	tests := []struct {
		name string
		pct  float64
	}{
		{"zero", 0},
		{"negative", -1},
		{"too high", 100.1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedTrading(1)
			svc := NewService(repo)

			err := svc.SetTakeProfit(context.Background(), 1, tt.pct)
			if err == nil {
				t.Fatalf("SetTakeProfit(%f) expected error", tt.pct)
			}
		})
	}
}

func TestSetMaxLeverage_Valid(t *testing.T) {
	tests := []struct {
		name     string
		leverage int
	}{
		{"no leverage", 1},
		{"typical", 10},
		{"max leverage", 125},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedTrading(1)
			svc := NewService(repo)

			err := svc.SetMaxLeverage(context.Background(), 1, tt.leverage)
			if err != nil {
				t.Fatalf("SetMaxLeverage(%d) error: %v", tt.leverage, err)
			}
			if repo.trading[1].MaxLeverage != tt.leverage {
				t.Errorf("MaxLeverage = %d, want %d", repo.trading[1].MaxLeverage, tt.leverage)
			}
		})
	}
}

func TestSetMaxLeverage_Invalid(t *testing.T) {
	tests := []struct {
		name     string
		leverage int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too high", 126},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedTrading(1)
			svc := NewService(repo)

			err := svc.SetMaxLeverage(context.Background(), 1, tt.leverage)
			if err == nil {
				t.Fatalf("SetMaxLeverage(%d) expected error", tt.leverage)
			}
		})
	}
}

func TestSetRiskPerTrade_Valid(t *testing.T) {
	tests := []struct {
		name string
		pct  float64
	}{
		{"small", 0.1},
		{"typical", 1.0},
		{"upper bound", 25.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedTrading(1)
			svc := NewService(repo)

			err := svc.SetRiskPerTrade(context.Background(), 1, tt.pct)
			if err != nil {
				t.Fatalf("SetRiskPerTrade(%f) error: %v", tt.pct, err)
			}
			if repo.trading[1].RiskPerTradePct != tt.pct {
				t.Errorf("RiskPerTradePct = %f, want %f", repo.trading[1].RiskPerTradePct, tt.pct)
			}
		})
	}
}

func TestSetRiskPerTrade_Invalid(t *testing.T) {
	tests := []struct {
		name string
		pct  float64
	}{
		{"zero", 0},
		{"negative", -1},
		{"too high", 25.1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.seedTrading(1)
			svc := NewService(repo)

			err := svc.SetRiskPerTrade(context.Background(), 1, tt.pct)
			if err == nil {
				t.Fatalf("SetRiskPerTrade(%f) expected error", tt.pct)
			}
		})
	}
}

func TestSetRiskPerTrade_PrefsNotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo)

	err := svc.SetRiskPerTrade(context.Background(), 1, 1.0)
	if err == nil {
		t.Fatal("SetRiskPerTrade() expected error when prefs not found")
	}
}

// --- cross-user isolation ---

func TestPreferencesIsolatedByUser(t *testing.T) {
	repo := newMockRepo()
	repo.seedAll(1)
	repo.seedAll(2)
	svc := NewService(repo)

	// change user 1's confidence
	_ = svc.SetMinConfidence(context.Background(), 1, 80)

	// user 2 should be unaffected
	prefs, _ := svc.GetScanning(context.Background(), 2)
	if prefs.MinConfidence != 60 {
		t.Errorf("user 2 MinConfidence = %d, want 60 (unchanged)", prefs.MinConfidence)
	}
}

// --- full update flow ---

func TestFullScanningUpdateFlow(t *testing.T) {
	repo := newMockRepo()
	repo.seedScanning(1)
	svc := NewService(repo)

	// set confidence
	if err := svc.SetMinConfidence(context.Background(), 1, 80); err != nil {
		t.Fatalf("SetMinConfidence: %v", err)
	}

	// set interval
	if err := svc.SetScanInterval(context.Background(), 1, 15); err != nil {
		t.Fatalf("SetScanInterval: %v", err)
	}

	// disable scanning
	if err := svc.ToggleScanning(context.Background(), 1, false); err != nil {
		t.Fatalf("ToggleScanning: %v", err)
	}

	// verify all changes persisted
	prefs, err := svc.GetScanning(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetScanning: %v", err)
	}
	if prefs.MinConfidence != 80 {
		t.Errorf("MinConfidence = %d, want 80", prefs.MinConfidence)
	}
	if prefs.ScanIntervalMins != 15 {
		t.Errorf("ScanIntervalMins = %d, want 15", prefs.ScanIntervalMins)
	}
	if prefs.IsScanningEnabled {
		t.Error("expected scanning to be disabled")
	}
}

func TestFullTradingUpdateFlow(t *testing.T) {
	repo := newMockRepo()
	repo.seedTrading(1)
	svc := NewService(repo)

	if err := svc.SetPositionSize(context.Background(), 1, 200, 600); err != nil {
		t.Fatalf("SetPositionSize: %v", err)
	}
	if err := svc.SetStopLoss(context.Background(), 1, 3.0); err != nil {
		t.Fatalf("SetStopLoss: %v", err)
	}
	if err := svc.SetTakeProfit(context.Background(), 1, 9.0); err != nil {
		t.Fatalf("SetTakeProfit: %v", err)
	}
	if err := svc.SetMaxLeverage(context.Background(), 1, 20); err != nil {
		t.Fatalf("SetMaxLeverage: %v", err)
	}
	if err := svc.SetRiskPerTrade(context.Background(), 1, 2.0); err != nil {
		t.Fatalf("SetRiskPerTrade: %v", err)
	}

	prefs, err := svc.GetTrading(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetTrading: %v", err)
	}
	if prefs.DefaultPositionSize != 200 {
		t.Errorf("DefaultPositionSize = %f, want 200", prefs.DefaultPositionSize)
	}
	if prefs.DefaultStopLossPct != 3.0 {
		t.Errorf("DefaultStopLossPct = %f, want 3.0", prefs.DefaultStopLossPct)
	}
	if prefs.DefaultTakeProfitPct != 9.0 {
		t.Errorf("DefaultTakeProfitPct = %f, want 9.0", prefs.DefaultTakeProfitPct)
	}
	if prefs.MaxLeverage != 20 {
		t.Errorf("MaxLeverage = %d, want 20", prefs.MaxLeverage)
	}
	if prefs.RiskPerTradePct != 2.0 {
		t.Errorf("RiskPerTradePct = %f, want 2.0", prefs.RiskPerTradePct)
	}
}
