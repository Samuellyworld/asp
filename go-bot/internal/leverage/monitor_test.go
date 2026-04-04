// tests for leverage monitor: config, constructor, tp/sl hit detection,
// tiered liquidation alerts (auto-close, critical, warning), periodic
// updates, cooldown enforcement, and alert escalation.
package leverage

import (
	"testing"
	"time"
)

// --- default config tests ---

func TestMonitor_DefaultConfig(t *testing.T) {
	cfg := DefaultMonitorConfig()

	if cfg.CheckInterval != 30*time.Second {
		t.Errorf("CheckInterval = %v, want 30s", cfg.CheckInterval)
	}
	if cfg.CooldownPeriod != 5*time.Minute {
		t.Errorf("CooldownPeriod = %v, want 5m", cfg.CooldownPeriod)
	}
	if cfg.WarningCooldown != 2*time.Minute {
		t.Errorf("WarningCooldown = %v, want 2m", cfg.WarningCooldown)
	}
	if cfg.CriticalCooldown != 30*time.Second {
		t.Errorf("CriticalCooldown = %v, want 30s", cfg.CriticalCooldown)
	}
}

// --- NewMonitor ---

func TestNewMonitor_Fields(t *testing.T) {
	mon, lister, closer, prices, funding := testMonitorSetup()

	if mon.lister != lister {
		t.Error("lister not set")
	}
	if mon.closer != closer {
		t.Error("closer not set")
	}
	if mon.prices != prices {
		t.Error("prices not set")
	}
	if mon.funding != funding {
		t.Error("funding not set")
	}
	if mon.lastNotified == nil {
		t.Error("lastNotified should be initialized")
	}
	if mon.lastAlertLevel == nil {
		t.Error("lastAlertLevel should be initialized")
	}
}

// --- tp hit ---

func TestMonitor_TPHit_Long(t *testing.T) {
	mon, lister, closer, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.TakeProfit = 55000
	lister.add(pos)

	closedPos := &LeveragePosition{
		ID: "pos_1", Symbol: "BTCUSDT", Side: SideLong,
		ClosePrice: 55500, CloseReason: "take_profit", PnL: 55,
	}
	closer.prepareClose(closedPos)
	prices.prices["BTCUSDT"] = 55500

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	e := collector.get(0)
	if e.Type != LevEventTPHit {
		t.Errorf("event type = %s, want %s", e.Type, LevEventTPHit)
	}
	if !e.IsUrgent {
		t.Error("tp hit should be urgent")
	}
	if e.Position.CloseReason != "take_profit" {
		t.Errorf("close reason = %s, want take_profit", e.Position.CloseReason)
	}
}

func TestMonitor_TPHit_Short(t *testing.T) {
	mon, lister, closer, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testShortPosition("pos_1", "ETHUSDT")
	pos.TakeProfit = 45000
	lister.add(pos)
	closer.prepareClose(&LeveragePosition{ID: "pos_1"})
	prices.prices["ETHUSDT"] = 44000

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	if collector.get(0).Type != LevEventTPHit {
		t.Errorf("event type = %s, want %s", collector.get(0).Type, LevEventTPHit)
	}
}

// --- sl hit ---

func TestMonitor_SLHit_Long(t *testing.T) {
	mon, lister, closer, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.StopLoss = 48000
	lister.add(pos)
	closer.prepareClose(&LeveragePosition{ID: "pos_1"})
	prices.prices["BTCUSDT"] = 47500

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	e := collector.get(0)
	if e.Type != LevEventSLHit {
		t.Errorf("event type = %s, want %s", e.Type, LevEventSLHit)
	}
	if !e.IsUrgent {
		t.Error("sl hit should be urgent")
	}
}

func TestMonitor_SLHit_Short(t *testing.T) {
	mon, lister, closer, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testShortPosition("pos_1", "ETHUSDT")
	pos.StopLoss = 52000
	lister.add(pos)
	closer.prepareClose(&LeveragePosition{ID: "pos_1"})
	prices.prices["ETHUSDT"] = 52500

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	if collector.get(0).Type != LevEventSLHit {
		t.Errorf("event type = %s, want %s", collector.get(0).Type, LevEventSLHit)
	}
}

// --- auto-close (< 2% from liquidation) ---

func TestMonitor_AutoClose_Long(t *testing.T) {
	mon, lister, closer, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.StopLoss = 0
	pos.TakeProfit = 0
	pos.LiquidationPrice = 49500
	lister.add(pos)
	closer.prepareClose(&LeveragePosition{ID: "pos_1", Symbol: "BTCUSDT"})
	prices.prices["BTCUSDT"] = 49800

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	e := collector.get(0)
	if e.Type != LevEventAutoClose {
		t.Errorf("event type = %s, want %s", e.Type, LevEventAutoClose)
	}
	if !e.IsUrgent {
		t.Error("auto-close should be urgent")
	}
	if e.AlertLevel != AlertAutoClose {
		t.Errorf("alert level = %v, want %v", e.AlertLevel, AlertAutoClose)
	}
}

func TestMonitor_AutoClose_Short(t *testing.T) {
	mon, lister, closer, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testShortPosition("pos_1", "ETHUSDT")
	pos.StopLoss = 0
	pos.TakeProfit = 0
	pos.LiquidationPrice = 50500
	lister.add(pos)
	closer.prepareClose(&LeveragePosition{ID: "pos_1", Symbol: "ETHUSDT"})
	prices.prices["ETHUSDT"] = 50200

	mon.CheckPositions()

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	if collector.get(0).Type != LevEventAutoClose {
		t.Errorf("event type = %s, want %s", collector.get(0).Type, LevEventAutoClose)
	}
}

// --- critical alert (2-5% from liquidation) ---

func TestMonitor_CriticalAlert(t *testing.T) {
	mon, lister, _, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.StopLoss = 0
	pos.TakeProfit = 0
	pos.LiquidationPrice = 48000
	lister.add(pos)
	prices.prices["BTCUSDT"] = 49500

	mon.CheckPositions()

	if collector.count() < 1 {
		t.Fatalf("events = %d, want >= 1", collector.count())
	}
	found := false
	for i := 0; i < collector.count(); i++ {
		e := collector.get(i)
		if e.Type == LevEventLiqCritical {
			found = true
			if !e.IsUrgent {
				t.Error("critical alert should be urgent")
			}
			if e.AlertLevel != AlertCritical {
				t.Errorf("alert level = %v, want %v", e.AlertLevel, AlertCritical)
			}
			if e.DistancePct < 2 || e.DistancePct >= 5 {
				t.Errorf("distance = %.2f%%, want between 2%% and 5%%", e.DistancePct)
			}
		}
	}
	if !found {
		t.Error("expected a critical alert event")
	}
}

// --- warning alert (5-10% from liquidation) ---

func TestMonitor_WarningAlert(t *testing.T) {
	mon, lister, _, prices, _ := testMonitorSetup()
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.StopLoss = 0
	pos.TakeProfit = 0
	pos.LiquidationPrice = 46500
	lister.add(pos)
	prices.prices["BTCUSDT"] = 50000

	mon.CheckPositions()

	if collector.count() < 1 {
		t.Fatalf("events = %d, want >= 1", collector.count())
	}
	found := false
	for i := 0; i < collector.count(); i++ {
		e := collector.get(i)
		if e.Type == LevEventLiqWarning {
			found = true
			if !e.IsUrgent {
				t.Error("warning alert should be urgent")
			}
			if e.AlertLevel != AlertWarning {
				t.Errorf("alert level = %v, want %v", e.AlertLevel, AlertWarning)
			}
		}
	}
	if !found {
		t.Error("expected a warning alert event")
	}
}

// --- periodic update (> 10% from liquidation) ---

func TestMonitor_PeriodicUpdate(t *testing.T) {
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

	if collector.count() != 1 {
		t.Fatalf("events = %d, want 1", collector.count())
	}
	e := collector.get(0)
	if e.Type != LevEventPeriodicUpdate {
		t.Errorf("event type = %s, want %s", e.Type, LevEventPeriodicUpdate)
	}
	if e.IsUrgent {
		t.Error("periodic update should not be urgent")
	}
	if e.AlertLevel != AlertNone {
		t.Errorf("alert level = %v, want %v", e.AlertLevel, AlertNone)
	}
}

// --- cooldown enforcement ---

func TestMonitor_CooldownPrevents_DuplicateAlerts(t *testing.T) {
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
	if collector.count() != 1 {
		t.Fatalf("first check: events = %d, want 1", collector.count())
	}

	mon.CheckPositions()
	if collector.count() != 1 {
		t.Errorf("second check: events = %d, want 1 (cooldown should block)", collector.count())
	}
}

func TestMonitor_WarningCooldownShorterThanPeriodic(t *testing.T) {
	mon, lister, _, prices, _ := testMonitorSetup()
	mon.funding = nil
	mon.config.WarningCooldown = 10 * time.Millisecond
	mon.config.CooldownPeriod = 1 * time.Hour
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.StopLoss = 0
	pos.TakeProfit = 0
	pos.LiquidationPrice = 46500
	lister.add(pos)
	prices.prices["BTCUSDT"] = 50000

	mon.CheckPositions()
	if collector.count() != 1 {
		t.Fatalf("first check: events = %d, want 1", collector.count())
	}

	time.Sleep(15 * time.Millisecond)

	mon.CheckPositions()
	if collector.count() != 2 {
		t.Errorf("second check: events = %d, want 2 (warning cooldown elapsed)", collector.count())
	}
}

// --- alert level escalation bypasses cooldown ---

func TestMonitor_AlertEscalation_BypassesCooldown(t *testing.T) {
	mon, lister, _, prices, _ := testMonitorSetup()
	mon.funding = nil
	mon.config.CooldownPeriod = 1 * time.Hour
	mon.config.WarningCooldown = 1 * time.Hour
	mon.config.CriticalCooldown = 1 * time.Hour
	collector := &levEventCollector{}
	mon.OnEvent = collector.collect

	pos := testLongPosition("pos_1", "BTCUSDT")
	pos.StopLoss = 0
	pos.TakeProfit = 0
	pos.LiquidationPrice = 46500
	lister.add(pos)

	prices.prices["BTCUSDT"] = 50000
	mon.CheckPositions()
	if collector.count() != 1 {
		t.Fatalf("first check: events = %d, want 1", collector.count())
	}
	if collector.get(0).Type != LevEventLiqWarning {
		t.Errorf("first event = %s, want %s", collector.get(0).Type, LevEventLiqWarning)
	}

	prices.mu.Lock()
	prices.prices["BTCUSDT"] = 48000
	prices.mu.Unlock()

	mon.CheckPositions()
	if collector.count() != 2 {
		t.Fatalf("second check: events = %d, want 2 (escalation should bypass)", collector.count())
	}
	if collector.get(1).Type != LevEventLiqCritical {
		t.Errorf("second event = %s, want %s", collector.get(1).Type, LevEventLiqCritical)
	}
}
