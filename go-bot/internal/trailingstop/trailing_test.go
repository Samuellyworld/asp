package trailingstop

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.0001
}

func TestEnabled(t *testing.T) {
	ts := &TrailingStop{}
	if ts.Enabled() {
		t.Error("should not be enabled with zero trail percent")
	}

	ts.TrailPercent = 2.0
	if !ts.Enabled() {
		t.Error("should be enabled with trail percent set")
	}
}

func TestUpdateLong_BasicTrail(t *testing.T) {
	ts := &TrailingStop{TrailPercent: 2.0}

	// price at entry
	stop, updated := ts.UpdateLong(100, 100)
	if !updated {
		t.Error("expected first update to set stop")
	}
	if !almostEqual(stop, 98.0) {
		t.Errorf("expected stop 98.0, got %.4f", stop)
	}

	// price rises to 110 — stop should trail up
	stop, updated = ts.UpdateLong(100, 110)
	if !updated {
		t.Error("expected update when price rises")
	}
	if !almostEqual(stop, 107.8) {
		t.Errorf("expected stop 107.8, got %.4f", stop)
	}

	// price drops to 108 — stop should NOT move backward
	stop, updated = ts.UpdateLong(100, 108)
	if updated {
		t.Error("stop should not update on price drop")
	}
	if !almostEqual(stop, 107.8) {
		t.Errorf("expected stop to stay at 107.8, got %.4f", stop)
	}
}

func TestUpdateLong_WithActivation(t *testing.T) {
	ts := &TrailingStop{
		TrailPercent:  2.0,
		ActivationPct: 3.0,
	}

	// price at 101 — only 1% profit, not yet activated
	stop, updated := ts.UpdateLong(100, 101)
	if updated {
		t.Error("should not update before activation threshold")
	}
	if stop != 0 {
		t.Errorf("expected stop 0, got %.4f", stop)
	}
	if ts.Activated {
		t.Error("should not be activated yet")
	}

	// price at 103 — 3% profit, activation triggered
	stop, updated = ts.UpdateLong(100, 103)
	if !updated {
		t.Error("expected update at activation")
	}
	if !ts.Activated {
		t.Error("should be activated now")
	}
	if !almostEqual(stop, 100.94) {
		t.Errorf("expected stop ~100.94, got %.4f", stop)
	}

	// price continues to 106
	stop, _ = ts.UpdateLong(100, 106)
	if !almostEqual(stop, 103.88) {
		t.Errorf("expected stop ~103.88, got %.4f", stop)
	}
}

func TestUpdateShort_BasicTrail(t *testing.T) {
	ts := &TrailingStop{TrailPercent: 2.0}

	// price at entry
	stop, updated := ts.UpdateShort(100, 100)
	if !updated {
		t.Error("expected first update to set stop")
	}
	if !almostEqual(stop, 102.0) {
		t.Errorf("expected stop 102.0, got %.4f", stop)
	}

	// price drops to 90 — stop should trail down
	stop, updated = ts.UpdateShort(100, 90)
	if !updated {
		t.Error("expected update when price drops")
	}
	if !almostEqual(stop, 91.8) {
		t.Errorf("expected stop 91.8, got %.4f", stop)
	}

	// price rises to 95 — stop should NOT move upward
	stop, updated = ts.UpdateShort(100, 95)
	if updated {
		t.Error("stop should not update when price rises for short")
	}
	if !almostEqual(stop, 91.8) {
		t.Errorf("expected stop to stay at 91.8, got %.4f", stop)
	}
}

func TestUpdateShort_WithActivation(t *testing.T) {
	ts := &TrailingStop{
		TrailPercent:  2.0,
		ActivationPct: 3.0,
	}

	// price at 99 — only 1% profit for short, not activated
	stop, updated := ts.UpdateShort(100, 99)
	if updated {
		t.Error("should not update before activation threshold")
	}
	if stop != 0 {
		t.Errorf("expected stop 0, got %.4f", stop)
	}

	// price at 97 — 3% profit, activation triggered
	stop, updated = ts.UpdateShort(100, 97)
	if !updated {
		t.Error("expected update at activation")
	}
	if !ts.Activated {
		t.Error("should be activated now")
	}
	if !almostEqual(stop, 98.94) {
		t.Errorf("expected stop ~98.94, got %.4f", stop)
	}
}

func TestIsHitLong(t *testing.T) {
	ts := &TrailingStop{TrailPercent: 2.0}

	// not enabled yet (no stop price)
	if ts.IsHitLong(95) {
		t.Error("should not be hit with no stop price")
	}

	// set stop at 98
	ts.UpdateLong(100, 100)

	// price above stop
	if ts.IsHitLong(99) {
		t.Error("should not be hit at 99")
	}

	// price at stop
	if !ts.IsHitLong(98) {
		t.Error("should be hit at 98")
	}

	// price below stop
	if !ts.IsHitLong(97) {
		t.Error("should be hit at 97")
	}
}

func TestIsHitShort(t *testing.T) {
	ts := &TrailingStop{TrailPercent: 2.0}

	// set stop at 102
	ts.UpdateShort(100, 100)

	// price below stop
	if ts.IsHitShort(101) {
		t.Error("should not be hit at 101")
	}

	// price at stop
	if !ts.IsHitShort(102) {
		t.Error("should be hit at 102")
	}

	// price above stop
	if !ts.IsHitShort(103) {
		t.Error("should be hit at 103")
	}
}

func TestDisabled_NoUpdates(t *testing.T) {
	ts := &TrailingStop{} // trail percent = 0

	stop, updated := ts.UpdateLong(100, 110)
	if updated {
		t.Error("disabled trailing stop should not update")
	}
	if stop != 0 {
		t.Errorf("expected stop 0, got %.4f", stop)
	}
}

func TestDisabled_NeverHit(t *testing.T) {
	ts := &TrailingStop{}

	if ts.IsHitLong(0) {
		t.Error("disabled trailing stop should never be hit")
	}
	if ts.IsHitShort(1000000) {
		t.Error("disabled trailing stop should never be hit")
	}
}

func TestUpdateLong_ZeroEntry(t *testing.T) {
	ts := &TrailingStop{TrailPercent: 2.0}
	_, updated := ts.UpdateLong(0, 100)
	if updated {
		t.Error("should not update with zero entry price")
	}
}

func TestUpdateShort_ZeroEntry(t *testing.T) {
	ts := &TrailingStop{TrailPercent: 2.0}
	_, updated := ts.UpdateShort(0, 100)
	if updated {
		t.Error("should not update with zero entry price")
	}
}

func TestUpdateLong_MonotonicallyIncreasing(t *testing.T) {
	ts := &TrailingStop{TrailPercent: 1.5}

	// simulate a rising then falling price series
	prices := []float64{100, 102, 105, 108, 106, 104, 110, 107}
	var lastStop float64

	for _, p := range prices {
		stop, _ := ts.UpdateLong(100, p)
		if stop < lastStop {
			t.Errorf("stop moved backward from %.4f to %.4f at price %.2f", lastStop, stop, p)
		}
		lastStop = stop
	}

	// stop should reflect the highest price (110)
	expectedStop := 110 * (1 - 1.5/100) // 108.35
	if !almostEqual(lastStop, expectedStop) {
		t.Errorf("expected final stop %.4f, got %.4f", expectedStop, lastStop)
	}
}

func TestUpdateShort_MonotonicallyDecreasing(t *testing.T) {
	ts := &TrailingStop{TrailPercent: 1.5}

	// simulate a falling then rising price series
	prices := []float64{100, 98, 95, 92, 94, 96, 90, 93}
	var lastStop float64

	for _, p := range prices {
		stop, _ := ts.UpdateShort(100, p)
		if lastStop != 0 && stop > lastStop {
			t.Errorf("stop moved upward from %.4f to %.4f at price %.2f", lastStop, stop, p)
		}
		lastStop = stop
	}

	// stop should reflect the lowest price (90)
	expectedStop := 90 * (1 + 1.5/100) // 91.35
	if !almostEqual(lastStop, expectedStop) {
		t.Errorf("expected final stop %.4f, got %.4f", expectedStop, lastStop)
	}
}

func TestActivation_OnlyTriggersOnce(t *testing.T) {
	ts := &TrailingStop{
		TrailPercent:  2.0,
		ActivationPct: 5.0,
	}

	// slowly approach activation
	ts.UpdateLong(100, 103) // 3% — not activated
	ts.UpdateLong(100, 104) // 4% — not activated

	if ts.Activated {
		t.Error("should not be activated below threshold")
	}

	ts.UpdateLong(100, 105) // 5% — activated
	if !ts.Activated {
		t.Error("should be activated at threshold")
	}

	// high water mark should be set at activation
	if !almostEqual(ts.HighWaterMark, 105) {
		t.Errorf("expected HWM 105, got %.4f", ts.HighWaterMark)
	}

	// further updates should continue to work
	ts.UpdateLong(100, 110)
	if !almostEqual(ts.HighWaterMark, 110) {
		t.Errorf("expected HWM 110, got %.4f", ts.HighWaterMark)
	}
}
