package leverage

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestNewFundingTracker(t *testing.T) {
	tracker := NewFundingTracker()
	if tracker == nil {
		t.Fatal("NewFundingTracker() returned nil")
	}
	if tracker.payments == nil {
		t.Fatal("payments map should be initialized")
	}
	if len(tracker.payments) != 0 {
		t.Errorf("payments map should be empty, got %d entries", len(tracker.payments))
	}
}

func TestRecordPayment_Single(t *testing.T) {
	tracker := NewFundingTracker()
	tracker.RecordPayment("pos-1", 0.0001, 10000.0)

	payments := tracker.Payments("pos-1")
	if len(payments) != 1 {
		t.Fatalf("expected 1 payment, got %d", len(payments))
	}

	p := payments[0]
	if p.PositionID != "pos-1" {
		t.Errorf("PositionID = %q, want %q", p.PositionID, "pos-1")
	}
	if p.Rate != 0.0001 {
		t.Errorf("Rate = %v, want %v", p.Rate, 0.0001)
	}

	wantAmount := 0.0001 * 10000.0
	if math.Abs(p.Amount-wantAmount) > 1e-10 {
		t.Errorf("Amount = %v, want %v", p.Amount, wantAmount)
	}
	if p.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestRecordPayment_MultipleForOnePosition(t *testing.T) {
	tracker := NewFundingTracker()
	tracker.RecordPayment("pos-1", 0.0001, 10000.0)
	tracker.RecordPayment("pos-1", -0.0002, 10000.0)
	tracker.RecordPayment("pos-1", 0.0003, 10000.0)

	payments := tracker.Payments("pos-1")
	if len(payments) != 3 {
		t.Fatalf("expected 3 payments, got %d", len(payments))
	}

	// verify each rate was recorded correctly
	wantRates := []float64{0.0001, -0.0002, 0.0003}
	for i, want := range wantRates {
		if payments[i].Rate != want {
			t.Errorf("payment[%d].Rate = %v, want %v", i, payments[i].Rate, want)
		}
	}
}

func TestCumulativeFees_Single(t *testing.T) {
	tracker := NewFundingTracker()
	tracker.RecordPayment("pos-1", 0.0001, 10000.0)

	got := tracker.CumulativeFees("pos-1")
	want := 1.0 // 0.0001 * 10000
	if math.Abs(got-want) > 1e-10 {
		t.Errorf("CumulativeFees() = %v, want %v", got, want)
	}
}

func TestCumulativeFees_Multiple(t *testing.T) {
	tracker := NewFundingTracker()
	tracker.RecordPayment("pos-1", 0.0001, 10000.0)  // +1.0
	tracker.RecordPayment("pos-1", -0.0002, 10000.0)  // -2.0
	tracker.RecordPayment("pos-1", 0.00015, 10000.0)  // +1.5

	got := tracker.CumulativeFees("pos-1")
	want := 0.5 // 1.0 - 2.0 + 1.5
	if math.Abs(got-want) > 1e-10 {
		t.Errorf("CumulativeFees() = %v, want %v", got, want)
	}
}

func TestCumulativeFees_EmptyPosition(t *testing.T) {
	tracker := NewFundingTracker()
	got := tracker.CumulativeFees("nonexistent")
	if got != 0 {
		t.Errorf("CumulativeFees() for nonexistent position = %v, want 0", got)
	}
}

func TestPayments_ReturnsCopy(t *testing.T) {
	tracker := NewFundingTracker()
	tracker.RecordPayment("pos-1", 0.0001, 10000.0)

	payments := tracker.Payments("pos-1")
	// modify returned slice
	payments[0].Rate = 999.0

	// original should be unchanged
	original := tracker.Payments("pos-1")
	if original[0].Rate == 999.0 {
		t.Error("Payments() should return a copy, not the original slice")
	}
}

func TestPayments_NilForUnknownPosition(t *testing.T) {
	tracker := NewFundingTracker()
	payments := tracker.Payments("nonexistent")
	if payments != nil {
		t.Errorf("Payments() for unknown position should return nil, got %v", payments)
	}
}

func TestIsFundingDue_NoPayments(t *testing.T) {
	tracker := NewFundingTracker()
	if !tracker.IsFundingDue("pos-1") {
		t.Error("IsFundingDue() should return true when no payments exist")
	}
}

func TestIsFundingDue_RecentPayment(t *testing.T) {
	tracker := NewFundingTracker()

	// record a payment just now; funding should not be due again
	// unless we happen to be exactly at a funding boundary
	tracker.RecordPayment("pos-1", 0.0001, 10000.0)

	// since we just recorded, the most recent funding time should be <= now
	// and the payment timestamp should also be ~now, so funding should not be due
	if tracker.IsFundingDue("pos-1") {
		t.Error("IsFundingDue() should return false right after a payment")
	}
}

func TestIsFundingDue_OldPayment(t *testing.T) {
	tracker := NewFundingTracker()

	// manually insert a payment from 9 hours ago to simulate an old payment
	nineHoursAgo := time.Now().UTC().Add(-9 * time.Hour)
	tracker.mu.Lock()
	tracker.payments["pos-1"] = []FundingPayment{
		{
			PositionID: "pos-1",
			Rate:       0.0001,
			Amount:     1.0,
			Timestamp:  nineHoursAgo,
		},
	}
	tracker.mu.Unlock()

	// 9 hours ago means at least one 8-hour funding window has passed
	if !tracker.IsFundingDue("pos-1") {
		t.Error("IsFundingDue() should return true when a funding window has passed since last payment")
	}
}

func TestMostRecentFundingTime(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		wantHour int
	}{
		{
			name:     "early morning",
			input:    time.Date(2025, 1, 15, 3, 30, 0, 0, time.UTC),
			wantHour: 0,
		},
		{
			name:     "exactly at 08:00",
			input:    time.Date(2025, 1, 15, 8, 0, 0, 0, time.UTC),
			wantHour: 8,
		},
		{
			name:     "mid morning",
			input:    time.Date(2025, 1, 15, 10, 45, 0, 0, time.UTC),
			wantHour: 8,
		},
		{
			name:     "exactly at 16:00",
			input:    time.Date(2025, 1, 15, 16, 0, 0, 0, time.UTC),
			wantHour: 16,
		},
		{
			name:     "late evening",
			input:    time.Date(2025, 1, 15, 23, 59, 0, 0, time.UTC),
			wantHour: 16,
		},
		{
			name:     "midnight exactly",
			input:    time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
			wantHour: 0,
		},
		{
			name:     "just before 08:00",
			input:    time.Date(2025, 1, 15, 7, 59, 59, 0, time.UTC),
			wantHour: 0,
		},
		{
			name:     "just before 16:00",
			input:    time.Date(2025, 1, 15, 15, 59, 59, 0, time.UTC),
			wantHour: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mostRecentFundingTime(tt.input)
			if got.Hour() != tt.wantHour {
				t.Errorf("mostRecentFundingTime(%v).Hour() = %d, want %d", tt.input, got.Hour(), tt.wantHour)
			}
			if got.Minute() != 0 || got.Second() != 0 {
				t.Errorf("mostRecentFundingTime() should have zero minutes/seconds, got %v", got)
			}
			if got.Year() != tt.input.Year() || got.Month() != tt.input.Month() || got.Day() != tt.input.Day() {
				t.Errorf("mostRecentFundingTime() date mismatch: got %v, input %v", got, tt.input)
			}
		})
	}
}

func TestCleanup(t *testing.T) {
	tracker := NewFundingTracker()
	tracker.RecordPayment("pos-1", 0.0001, 10000.0)
	tracker.RecordPayment("pos-2", 0.0002, 5000.0)

	tracker.Cleanup("pos-1")

	if payments := tracker.Payments("pos-1"); payments != nil {
		t.Errorf("Payments() after Cleanup() should return nil, got %v", payments)
	}
	if fees := tracker.CumulativeFees("pos-1"); fees != 0 {
		t.Errorf("CumulativeFees() after Cleanup() should return 0, got %v", fees)
	}

	// pos-2 should be unaffected
	if payments := tracker.Payments("pos-2"); len(payments) != 1 {
		t.Errorf("Cleanup() should not affect other positions, got %d payments for pos-2", len(payments))
	}
}

func TestCleanup_NonexistentPosition(t *testing.T) {
	tracker := NewFundingTracker()
	// should not panic
	tracker.Cleanup("nonexistent")
}

func TestTotalFees_Empty(t *testing.T) {
	tracker := NewFundingTracker()
	got := tracker.TotalFees()
	if got != 0 {
		t.Errorf("TotalFees() on empty tracker = %v, want 0", got)
	}
}

func TestTotalFees_SinglePosition(t *testing.T) {
	tracker := NewFundingTracker()
	tracker.RecordPayment("pos-1", 0.0001, 10000.0) // 1.0

	got := tracker.TotalFees()
	want := 1.0
	if math.Abs(got-want) > 1e-10 {
		t.Errorf("TotalFees() = %v, want %v", got, want)
	}
}

func TestTotalFees_MultiplePositions(t *testing.T) {
	tracker := NewFundingTracker()
	tracker.RecordPayment("pos-1", 0.0001, 10000.0) // 1.0
	tracker.RecordPayment("pos-2", 0.0002, 5000.0)  // 1.0
	tracker.RecordPayment("pos-1", 0.0003, 10000.0)  // 3.0

	got := tracker.TotalFees()
	want := 5.0 // 1.0 + 1.0 + 3.0
	if math.Abs(got-want) > 1e-10 {
		t.Errorf("TotalFees() = %v, want %v", got, want)
	}
}

func TestTotalFees_AfterCleanup(t *testing.T) {
	tracker := NewFundingTracker()
	tracker.RecordPayment("pos-1", 0.0001, 10000.0) // 1.0
	tracker.RecordPayment("pos-2", 0.0002, 5000.0)  // 1.0

	tracker.Cleanup("pos-1")

	got := tracker.TotalFees()
	want := 1.0 // only pos-2 remains
	if math.Abs(got-want) > 1e-10 {
		t.Errorf("TotalFees() after Cleanup = %v, want %v", got, want)
	}
}

func TestNegativeFundingRate(t *testing.T) {
	tracker := NewFundingTracker()
	tracker.RecordPayment("pos-1", -0.0005, 20000.0)

	got := tracker.CumulativeFees("pos-1")
	want := -10.0 // -0.0005 * 20000
	if math.Abs(got-want) > 1e-10 {
		t.Errorf("CumulativeFees() with negative rate = %v, want %v", got, want)
	}
}

func TestConcurrentAccess(t *testing.T) {
	tracker := NewFundingTracker()
	var wg sync.WaitGroup

	// concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tracker.RecordPayment("pos-1", 0.0001, float64(n))
		}(i)
	}

	// concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tracker.CumulativeFees("pos-1")
			_ = tracker.Payments("pos-1")
			_ = tracker.TotalFees()
			_ = tracker.IsFundingDue("pos-1")
		}()
	}

	wg.Wait()

	payments := tracker.Payments("pos-1")
	if len(payments) != 100 {
		t.Errorf("expected 100 payments after concurrent writes, got %d", len(payments))
	}
}

func TestZeroNotional(t *testing.T) {
	tracker := NewFundingTracker()
	tracker.RecordPayment("pos-1", 0.0001, 0.0)

	got := tracker.CumulativeFees("pos-1")
	if got != 0 {
		t.Errorf("CumulativeFees() with zero notional = %v, want 0", got)
	}
}

func TestZeroRate(t *testing.T) {
	tracker := NewFundingTracker()
	tracker.RecordPayment("pos-1", 0.0, 10000.0)

	got := tracker.CumulativeFees("pos-1")
	if got != 0 {
		t.Errorf("CumulativeFees() with zero rate = %v, want 0", got)
	}
}
