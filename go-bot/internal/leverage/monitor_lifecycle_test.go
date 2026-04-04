// tests for leverage monitor: error handling, funding fees, start/stop
// lifecycle, cleanup, shouldNotify, multiple positions, and misc.
package leverage

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- mark price provider error ---

func TestMonitor_MarkPriceError_SkipsPosition(t *testing.T) {
	mon, lister, _, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	lister.add(pos)
	prices.err = errors.New("exchange down")

	mon.CheckPositions()

	if collector.count() != 0 {
		t.Errorf("events = %d, want 0 (price error should skip)", collector.count())
	}
}

// --- close error doesn't crash ---

func TestMonitor_CloseError_DoesNotCrash(t *testing.T) {
	mon, lister, closer, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.StopLoss = 48000
	lister.add(pos)
	closer.closeErr = errors.New("close failed")
	prices.prices["BTCUSDT"] = 47000

	mon.CheckPositions()

	if collector.count() != 0 {
		t.Errorf("events = %d, want 0 (close error should skip)", collector.count())
	}
}

func TestMonitor_AutoClose_CloseError(t *testing.T) {
	mon, lister, closer, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.StopLoss = 0
	pos.TakeProfit = 0
	pos.LiquidationPrice = 49500
	lister.add(pos)
	closer.closeErr = errors.New("close failed")
	prices.prices["BTCUSDT"] = 49600

	mon.CheckPositions()

	if collector.count() != 0 {
		t.Errorf("events = %d, want 0 (auto-close error should skip)", collector.count())
	}
}

// --- funding fee due ---

func TestMonitor_FundingFeeDue(t *testing.T) {
	mon, lister, _, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.StopLoss = 0
	pos.TakeProfit = 0
	pos.LiquidationPrice = 40000
	lister.add(pos)
	prices.prices["BTCUSDT"] = 50000

	mon.CheckPositions()

	if !collector.hasType(LevEventFundingFee) {
		t.Error("expected a funding fee event")
	}
	if !collector.hasType(LevEventPeriodicUpdate) {
		t.Error("expected a periodic update event alongside funding")
	}
}

func TestMonitor_FundingFeeNotDue(t *testing.T) {
	mon, lister, _, prices, funding := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.StopLoss = 0
	pos.TakeProfit = 0
	pos.LiquidationPrice = 40000
	lister.add(pos)
	prices.prices["BTCUSDT"] = 50000
	funding.RecordPayment("pos_1", 0.0001, 5000)

	mon.CheckPositions()

	if collector.hasType(LevEventFundingFee) {
		t.Error("should not emit funding fee event when not due")
	}
}

func TestMonitor_NilFunding_NoPanic(t *testing.T) {
	mon, lister, _, prices, _ := testMonitorSetup()
	mon.funding = nil
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.StopLoss = 0
	pos.TakeProfit = 0
	pos.LiquidationPrice = 40000
	lister.add(pos)
	prices.prices["BTCUSDT"] = 50000

	mon.CheckPositions()

	if collector.hasType(LevEventFundingFee) {
		t.Error("no funding event when tracker is nil")
	}
}

// --- start / stop lifecycle ---

func TestMonitor_StartStop(t *testing.T) {
	mon, _, _, _, _ := testMonitorSetup()

	if mon.IsRunning() {
		t.Fatal("monitor should not be running before Start")
	}
	ctx := context.Background()
	mon.Start(ctx)
	if !mon.IsRunning() {
		t.Fatal("monitor should be running after Start")
	}
	mon.Stop()
	if mon.IsRunning() {
		t.Fatal("monitor should not be running after Stop")
	}
}

func TestMonitor_DoubleStart(t *testing.T) {
	mon, _, _, _, _ := testMonitorSetup()
	ctx := context.Background()
	mon.Start(ctx)
	mon.Start(ctx)
	if !mon.IsRunning() {
		t.Fatal("monitor should still be running after double Start")
	}
	mon.Stop()
}

func TestMonitor_DoubleStop(t *testing.T) {
	mon, _, _, _, _ := testMonitorSetup()
	ctx := context.Background()
	mon.Start(ctx)
	mon.Stop()
	mon.Stop()
	if mon.IsRunning() {
		t.Fatal("monitor should not be running after Stop")
	}
}

func TestMonitor_ContextCancellation(t *testing.T) {
	mon, _, _, _, _ := testMonitorSetup()
	ctx, cancel := context.WithCancel(context.Background())
	mon.Start(ctx)
	if !mon.IsRunning() {
		t.Fatal("monitor should be running")
	}
	cancel()
	<-mon.done
	if mon.IsRunning() {
		t.Fatal("monitor should stop when context is canceled")
	}
}

// --- emit behavior ---

func TestMonitor_Emit_NilOnEvent(t *testing.T) {
	mon := &Monitor{}
	mon.emit(LevEvent{Type: LevEventPeriodicUpdate})
}

// --- cleanup ---

func TestMonitor_Cleanup_RemovesClosedPositions(t *testing.T) {
	mon, lister, _, _, _ := testMonitorSetup()
	pos1 := testLongPosition("pos_1", "BTCUSDT")
	pos2 := testLongPosition("pos_2", "ETHUSDT")
	lister.add(pos1)
	lister.add(pos2)
	mon.recordNotification("pos_1", AlertNone)
	mon.recordNotification("pos_2", AlertWarning)
	lister.remove("pos_2")

	mon.Cleanup()

	mon.mu.Lock()
	defer mon.mu.Unlock()
	if _, exists := mon.lastNotified["pos_1"]; !exists {
		t.Error("pos_1 tracking should be preserved (still open)")
	}
	if _, exists := mon.lastNotified["pos_2"]; exists {
		t.Error("pos_2 tracking should be removed (no longer open)")
	}
	if _, exists := mon.lastAlertLevel["pos_2"]; exists {
		t.Error("pos_2 alert level should be removed")
	}
}

func TestMonitor_Cleanup_NoOpenPositions(t *testing.T) {
	mon, _, _, _, _ := testMonitorSetup()
	mon.recordNotification("pos_1", AlertNone)
	mon.recordNotification("pos_2", AlertWarning)

	mon.Cleanup()

	mon.mu.Lock()
	defer mon.mu.Unlock()
	if len(mon.lastNotified) != 0 {
		t.Errorf("lastNotified count = %d, want 0", len(mon.lastNotified))
	}
	if len(mon.lastAlertLevel) != 0 {
		t.Errorf("lastAlertLevel count = %d, want 0", len(mon.lastAlertLevel))
	}
}

// --- cooldownForLevel ---

func TestMonitor_CooldownForLevel(t *testing.T) {
	cfg := DefaultMonitorConfig()
	mon := &Monitor{config: cfg}
	tests := []struct {
		level AlertLevel
		want  time.Duration
	}{
		{AlertCritical, 30 * time.Second},
		{AlertWarning, 2 * time.Minute},
		{AlertNone, 5 * time.Minute},
		{AlertAutoClose, 5 * time.Minute},
	}
	for _, tc := range tests {
		got := mon.cooldownForLevel(tc.level)
		if got != tc.want {
			t.Errorf("cooldownForLevel(%v) = %v, want %v", tc.level, got, tc.want)
		}
	}
}

// --- shouldNotify ---

func TestMonitor_ShouldNotify_FirstTime(t *testing.T) {
	mon, _, _, _, _ := testMonitorSetup()
	if !mon.shouldNotify("pos_1", AlertNone) {
		t.Error("should notify on first check (no prior notification)")
	}
}

func TestMonitor_ShouldNotify_WithinCooldown(t *testing.T) {
	mon, _, _, _, _ := testMonitorSetup()
	mon.recordNotification("pos_1", AlertNone)
	if mon.shouldNotify("pos_1", AlertNone) {
		t.Error("should not notify within cooldown period")
	}
}

func TestMonitor_ShouldNotify_Escalation(t *testing.T) {
	mon, _, _, _, _ := testMonitorSetup()
	mon.config.WarningCooldown = 1 * time.Hour
	mon.config.CriticalCooldown = 1 * time.Hour
	mon.recordNotification("pos_1", AlertWarning)
	if !mon.shouldNotify("pos_1", AlertCritical) {
		t.Error("escalation from warning to critical should bypass cooldown")
	}
}

func TestMonitor_ShouldNotify_SameLevel_WithinCooldown(t *testing.T) {
	mon, _, _, _, _ := testMonitorSetup()
	mon.config.WarningCooldown = 1 * time.Hour
	mon.recordNotification("pos_1", AlertWarning)
	if mon.shouldNotify("pos_1", AlertWarning) {
		t.Error("same alert level within cooldown should not notify")
	}
}

func TestMonitor_ShouldNotify_Deescalation(t *testing.T) {
	mon, _, _, _, _ := testMonitorSetup()
	mon.config.CooldownPeriod = 1 * time.Hour
	mon.recordNotification("pos_1", AlertCritical)
	if mon.shouldNotify("pos_1", AlertNone) {
		t.Error("deescalation should not bypass cooldown")
	}
}

// --- multiple positions ---

func TestMonitor_MultiplePositions(t *testing.T) {
	mon, lister, closer, prices, _ := testMonitorSetup()
	mon.funding = nil
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos1 := testLongPosition("pos_1", "BTCUSDT")
	pos1.TakeProfit = 55000
	pos1.StopLoss = 0
	pos1.LiquidationPrice = 40000
	lister.add(pos1)
	closer.prepareClose(&LeveragePosition{ID: "pos_1"})
	prices.prices["BTCUSDT"] = 56000

	pos2 := testLongPosition("pos_2", "ETHUSDT")
	pos2.StopLoss = 0
	pos2.TakeProfit = 0
	pos2.LiquidationPrice = 2000
	lister.add(pos2)
	prices.prices["ETHUSDT"] = 3500

	mon.CheckPositions()

	if collector.count() != 2 {
		t.Fatalf("events = %d, want 2", collector.count())
	}
	types := map[LevEventType]int{}
	for i := 0; i < collector.count(); i++ {
		types[collector.get(i).Type]++
	}
	if types[LevEventTPHit] != 1 {
		t.Errorf("tp hit events = %d, want 1", types[LevEventTPHit])
	}
	if types[LevEventPeriodicUpdate] != 1 {
		t.Errorf("periodic events = %d, want 1", types[LevEventPeriodicUpdate])
	}
}

func TestMonitor_NoOpenPositions(t *testing.T) {
	mon, _, _, _, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect
	mon.CheckPositions()
	if collector.count() != 0 {
		t.Errorf("events = %d, want 0", collector.count())
	}
}

// --- recordNotification ---

func TestMonitor_RecordNotification(t *testing.T) {
	mon, _, _, _, _ := testMonitorSetup()
	before := time.Now()
	mon.recordNotification("pos_1", AlertCritical)
	after := time.Now()

	mon.mu.Lock()
	defer mon.mu.Unlock()
	ts, ok := mon.lastNotified["pos_1"]
	if !ok {
		t.Fatal("pos_1 should be recorded in lastNotified")
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp out of range: %v not between %v and %v", ts, before, after)
	}
	if mon.lastAlertLevel["pos_1"] != AlertCritical {
		t.Errorf("alert level = %v, want %v", mon.lastAlertLevel["pos_1"], AlertCritical)
	}
}

// --- mark price update on position ---

func TestMonitor_UpdatesMarkPrice(t *testing.T) {
	mon, lister, _, prices, _ := testMonitorSetup()
	mon.funding = nil
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.StopLoss = 0
	pos.TakeProfit = 0
	pos.LiquidationPrice = 40000
	pos.MarkPrice = 50000
	lister.add(pos)
	prices.prices["BTCUSDT"] = 51500

	mon.CheckPositions()

	if pos.MarkPrice != 51500 {
		t.Errorf("MarkPrice = %.2f, want 51500", pos.MarkPrice)
	}
}

// --- tp takes priority over sl ---

func TestMonitor_TPCheckedBeforeSL(t *testing.T) {
	mon, lister, closer, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.TakeProfit = 55000
	pos.StopLoss = 48000
	lister.add(pos)
	closer.prepareClose(&LeveragePosition{ID: "pos_1"})
	prices.prices["BTCUSDT"] = 56000

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	if collector.get(0).Type != LevEventTPHit {
		t.Errorf("event type = %s, want %s", collector.get(0).Type, LevEventTPHit)
	}
}

// --- event type constants ---

func TestLevEventType_Constants(t *testing.T) {
	tests := []struct {
		got  LevEventType
		want string
	}{
		{LevEventOpened, "leverage_opened"},
		{LevEventTPHit, "tp_hit"},
		{LevEventSLHit, "sl_hit"},
		{LevEventLiqWarning, "liquidation_warning"},
		{LevEventLiqCritical, "liquidation_critical"},
		{LevEventAutoClose, "auto_close"},
		{LevEventFundingFee, "funding_fee"},
		{LevEventPeriodicUpdate, "periodic_update"},
		{LevEventClosed, "closed"},
	}
	for _, tc := range tests {
		if string(tc.got) != tc.want {
			t.Errorf("event type = %q, want %q", tc.got, tc.want)
		}
	}
}
