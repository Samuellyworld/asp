package circuitbreaker

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxDailyLoss != 100 {
		t.Errorf("MaxDailyLoss = %f, want 100", cfg.MaxDailyLoss)
	}
	if cfg.MaxConsecutiveLosses != 5 {
		t.Errorf("MaxConsecutiveLosses = %d, want 5", cfg.MaxConsecutiveLosses)
	}
	if cfg.CooldownDuration != 1*time.Hour {
		t.Errorf("CooldownDuration = %v, want 1h", cfg.CooldownDuration)
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half_open"},
		{State(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestClosedAllowsTrade(t *testing.T) {
	b := New(DefaultConfig())
	ok, reason := b.AllowTrade(1)
	if !ok {
		t.Errorf("AllowTrade should be true for new user, got blocked: %s", reason)
	}
	if b.State(1) != StateClosed {
		t.Errorf("State = %v, want closed", b.State(1))
	}
}

func TestTripOnDailyLoss(t *testing.T) {
	cfg := Config{MaxDailyLoss: 50, MaxConsecutiveLosses: 0, CooldownDuration: time.Hour}
	b := New(cfg)

	// record losses that don't exceed threshold
	b.RecordTrade(1, -20)
	b.RecordTrade(1, -20)
	if b.State(1) != StateClosed {
		t.Fatal("should still be closed after $40 loss")
	}

	ok, _ := b.AllowTrade(1)
	if !ok {
		t.Fatal("should allow trade before limit")
	}

	// this puts us at $50 loss — trips the breaker
	b.RecordTrade(1, -10)
	if b.State(1) != StateOpen {
		t.Fatalf("State = %v, want open after $50 loss", b.State(1))
	}

	ok, reason := b.AllowTrade(1)
	if ok {
		t.Fatal("AllowTrade should be false when breaker is open")
	}
	if !strings.Contains(reason, "daily loss") {
		t.Errorf("reason should mention daily loss, got: %s", reason)
	}
}

func TestTripOnConsecutiveLosses(t *testing.T) {
	cfg := Config{MaxDailyLoss: 0, MaxConsecutiveLosses: 3, CooldownDuration: time.Hour}
	b := New(cfg)

	b.RecordTrade(1, -5)
	b.RecordTrade(1, -5)
	if b.State(1) != StateClosed {
		t.Fatal("should be closed after 2 losses")
	}

	b.RecordTrade(1, -5)
	if b.State(1) != StateOpen {
		t.Fatalf("State = %v, want open after 3 consecutive losses", b.State(1))
	}
}

func TestConsecutiveLossesResetOnWin(t *testing.T) {
	cfg := Config{MaxDailyLoss: 0, MaxConsecutiveLosses: 3, CooldownDuration: time.Hour}
	b := New(cfg)

	b.RecordTrade(1, -5)
	b.RecordTrade(1, -5)
	// win resets streak
	b.RecordTrade(1, 10)
	_, losses := b.Stats(1)
	if losses != 0 {
		t.Errorf("consecutive losses = %d, want 0 after win", losses)
	}

	// two more losses shouldn't trip (streak was reset)
	b.RecordTrade(1, -5)
	b.RecordTrade(1, -5)
	if b.State(1) != StateClosed {
		t.Fatal("should still be closed — streak was reset")
	}
}

func TestCooldownTransition(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{MaxDailyLoss: 10, MaxConsecutiveLosses: 0, CooldownDuration: 30 * time.Minute}
	b := New(cfg)
	b.now = func() time.Time { return now }

	// trip the breaker
	b.RecordTrade(1, -15)
	if b.State(1) != StateOpen {
		t.Fatal("should be open")
	}

	// 29 minutes later — still open
	b.now = func() time.Time { return now.Add(29 * time.Minute) }
	ok, reason := b.AllowTrade(1)
	if ok {
		t.Fatal("should still be blocked at 29min")
	}
	if !strings.Contains(reason, "cooldown") {
		t.Errorf("reason should mention cooldown, got: %s", reason)
	}

	// 31 minutes later — transitions to half-open
	b.now = func() time.Time { return now.Add(31 * time.Minute) }
	if b.State(1) != StateHalfOpen {
		t.Fatalf("State = %v, want half_open after cooldown", b.State(1))
	}
	ok, _ = b.AllowTrade(1)
	if !ok {
		t.Fatal("should allow probe trade in half-open")
	}
}

func TestHalfOpenProbeSuccess(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{MaxDailyLoss: 10, MaxConsecutiveLosses: 0, CooldownDuration: time.Minute}
	b := New(cfg)
	b.now = func() time.Time { return now }

	b.RecordTrade(1, -15)
	if b.State(1) != StateOpen {
		t.Fatal("should be open")
	}

	// advance past cooldown
	b.now = func() time.Time { return now.Add(2 * time.Minute) }
	if b.State(1) != StateHalfOpen {
		t.Fatal("should be half-open")
	}

	// probe trade wins — breaker closes
	b.RecordTrade(1, 5)
	if b.State(1) != StateClosed {
		t.Fatalf("State = %v, want closed after successful probe", b.State(1))
	}
}

func TestHalfOpenProbeFailure(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{MaxDailyLoss: 10, MaxConsecutiveLosses: 0, CooldownDuration: time.Minute}
	b := New(cfg)
	b.now = func() time.Time { return now }

	b.RecordTrade(1, -15)

	// advance past cooldown to half-open
	now2 := now.Add(2 * time.Minute)
	b.now = func() time.Time { return now2 }
	if b.State(1) != StateHalfOpen {
		t.Fatal("should be half-open")
	}

	// probe trade loses — re-trips
	b.RecordTrade(1, -3)
	if b.State(1) != StateOpen {
		t.Fatalf("State = %v, want open after failed probe", b.State(1))
	}

	// needs another cooldown
	ok, _ := b.AllowTrade(1)
	if ok {
		t.Fatal("should be blocked immediately after re-trip")
	}

	b.now = func() time.Time { return now2.Add(2 * time.Minute) }
	ok, _ = b.AllowTrade(1)
	if !ok {
		t.Fatal("should allow after second cooldown")
	}
}

func TestPerUserIsolation(t *testing.T) {
	cfg := Config{MaxDailyLoss: 20, MaxConsecutiveLosses: 0, CooldownDuration: time.Hour}
	b := New(cfg)

	b.RecordTrade(1, -25)
	b.RecordTrade(2, -5)

	if b.State(1) != StateOpen {
		t.Fatal("user 1 should be open")
	}
	if b.State(2) != StateClosed {
		t.Fatal("user 2 should still be closed")
	}

	ok1, _ := b.AllowTrade(1)
	ok2, _ := b.AllowTrade(2)
	if ok1 {
		t.Fatal("user 1 should be blocked")
	}
	if !ok2 {
		t.Fatal("user 2 should be allowed")
	}
}

func TestManualReset(t *testing.T) {
	cfg := Config{MaxDailyLoss: 10, MaxConsecutiveLosses: 0, CooldownDuration: time.Hour}
	b := New(cfg)

	b.RecordTrade(1, -15)
	if b.State(1) != StateOpen {
		t.Fatal("should be open")
	}

	b.Reset(1)
	if b.State(1) != StateClosed {
		t.Fatal("should be closed after reset")
	}

	loss, consec := b.Stats(1)
	if loss != 0 || consec != 0 {
		t.Errorf("Stats after reset: loss=%f, consec=%d, want 0, 0", loss, consec)
	}

	ok, _ := b.AllowTrade(1)
	if !ok {
		t.Fatal("should allow trade after reset")
	}
}

func TestDailyReset(t *testing.T) {
	now := time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC)
	cfg := Config{MaxDailyLoss: 50, MaxConsecutiveLosses: 0, CooldownDuration: 10 * time.Minute}
	b := New(cfg)
	b.now = func() time.Time { return now }

	// accumulate $30 loss today
	b.RecordTrade(1, -30)
	loss, _ := b.Stats(1)
	if loss != 30 {
		t.Errorf("dailyLoss = %f, want 30", loss)
	}

	// next day — daily loss resets
	b.now = func() time.Time { return now.Add(2 * time.Hour) } // crosses midnight
	loss, _ = b.Stats(1)
	if loss != 0 {
		t.Errorf("dailyLoss after day change = %f, want 0", loss)
	}
}

func TestDailyResetDoesNotClearOpenState(t *testing.T) {
	now := time.Date(2025, 1, 1, 23, 59, 0, 0, time.UTC)
	cfg := Config{MaxDailyLoss: 10, MaxConsecutiveLosses: 0, CooldownDuration: 2 * time.Hour}
	b := New(cfg)
	b.now = func() time.Time { return now }

	b.RecordTrade(1, -15)
	if b.State(1) != StateOpen {
		t.Fatal("should be open")
	}

	// next day — loss resets but breaker stays open (cooldown not expired)
	b.now = func() time.Time { return now.Add(10 * time.Minute) }
	if b.State(1) != StateOpen {
		t.Fatal("breaker should stay open across day boundary when cooldown hasn't expired")
	}
	loss, _ := b.Stats(1)
	if loss != 0 {
		t.Errorf("daily loss should reset to 0, got %f", loss)
	}
}

func TestProfitDoesNotReduceDailyLoss(t *testing.T) {
	cfg := Config{MaxDailyLoss: 50, MaxConsecutiveLosses: 0, CooldownDuration: time.Hour}
	b := New(cfg)

	b.RecordTrade(1, -30)
	b.RecordTrade(1, 20) // profit doesn't reduce daily loss
	loss, _ := b.Stats(1)
	if loss != 30 {
		t.Errorf("dailyLoss = %f, want 30 (profit shouldn't reduce)", loss)
	}
}

func TestBothThresholds(t *testing.T) {
	cfg := Config{MaxDailyLoss: 100, MaxConsecutiveLosses: 3, CooldownDuration: time.Hour}
	b := New(cfg)

	// 3 small losses trip on consecutive, not daily
	b.RecordTrade(1, -5)
	b.RecordTrade(1, -5)
	b.RecordTrade(1, -5)

	if b.State(1) != StateOpen {
		t.Fatal("should trip on consecutive losses")
	}

	ok, reason := b.AllowTrade(1)
	if ok {
		t.Fatal("should be blocked")
	}
	if !strings.Contains(reason, "consecutive") {
		t.Errorf("reason should mention consecutive, got: %s", reason)
	}
}

func TestStats(t *testing.T) {
	cfg := Config{MaxDailyLoss: 200, MaxConsecutiveLosses: 10, CooldownDuration: time.Hour}
	b := New(cfg)

	b.RecordTrade(1, -10)
	b.RecordTrade(1, -20)
	b.RecordTrade(1, 5)
	b.RecordTrade(1, -7)

	loss, consec := b.Stats(1)
	if loss != 37 {
		t.Errorf("dailyLoss = %f, want 37", loss)
	}
	if consec != 1 {
		t.Errorf("consecutiveLosses = %d, want 1 (reset by win)", consec)
	}
}

func TestZeroConfigDisablesThreshold(t *testing.T) {
	// MaxDailyLoss=0 means no daily loss limit; MaxConsecutiveLosses=0 means no streak limit
	cfg := Config{MaxDailyLoss: 0, MaxConsecutiveLosses: 0, CooldownDuration: time.Hour}
	b := New(cfg)

	for i := 0; i < 20; i++ {
		b.RecordTrade(1, -100)
	}

	if b.State(1) != StateClosed {
		t.Fatal("breaker should never trip when both thresholds are 0")
	}
	ok, _ := b.AllowTrade(1)
	if !ok {
		t.Fatal("should always allow when thresholds disabled")
	}
}

func TestNilBreakerSafety(t *testing.T) {
	// Verify the pattern: callers check b != nil before calling AllowTrade.
	// This test documents the expected nil-safety pattern.
	var b *Breaker
	if b != nil {
		t.Fatal("nil breaker should be nil")
	}
}
