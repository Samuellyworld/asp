package claude

import (
	"testing"
)

func TestParseDecisionBuy(t *testing.T) {
	text := `{"action":"BUY","confidence":85,"entry":42450,"stop_loss":41800,"take_profit":44200,"position_size":200,"reasoning":"Strong support."}`
	d, err := ParseDecision(text)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if d.Action != ActionBuy {
		t.Errorf("expected BUY, got %s", d.Action)
	}
	if d.Confidence != 85 {
		t.Errorf("expected confidence 85, got %.0f", d.Confidence)
	}
	if d.Plan.Entry != 42450 {
		t.Errorf("expected entry 42450, got %.0f", d.Plan.Entry)
	}
	if d.Plan.StopLoss != 41800 {
		t.Errorf("expected sl 41800, got %.0f", d.Plan.StopLoss)
	}
	if d.Plan.TakeProfit != 44200 {
		t.Errorf("expected tp 44200, got %.0f", d.Plan.TakeProfit)
	}
	if d.Plan.PositionSize != 200 {
		t.Errorf("expected position size 200, got %.0f", d.Plan.PositionSize)
	}
	if d.Reasoning != "Strong support." {
		t.Errorf("expected reasoning 'Strong support.', got %q", d.Reasoning)
	}
}

func TestParseDecisionSell(t *testing.T) {
	text := `{"action":"SELL","confidence":70,"entry":42450,"stop_loss":43200,"take_profit":40500,"position_size":150,"reasoning":"Bearish."}`
	d, err := ParseDecision(text)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if d.Action != ActionSell {
		t.Errorf("expected SELL, got %s", d.Action)
	}
}

func TestParseDecisionHold(t *testing.T) {
	text := `{"action":"HOLD","confidence":40,"entry":0,"stop_loss":0,"take_profit":0,"position_size":0,"reasoning":"Conflicting signals."}`
	d, err := ParseDecision(text)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if d.Action != ActionHold {
		t.Errorf("expected HOLD, got %s", d.Action)
	}
	if d.Plan.RiskReward != 0 {
		t.Errorf("expected r/r 0 for hold, got %.2f", d.Plan.RiskReward)
	}
}

func TestParseDecisionWithSurroundingText(t *testing.T) {
	text := `Here is my analysis:

{"action":"BUY","confidence":80,"entry":100,"stop_loss":95,"take_profit":115,"position_size":200,"reasoning":"Good setup."}

I hope this helps!`

	d, err := ParseDecision(text)
	if err != nil {
		t.Fatalf("should extract json from surrounding text: %v", err)
	}
	if d.Action != ActionBuy {
		t.Errorf("expected BUY, got %s", d.Action)
	}
	if d.Plan.Entry != 100 {
		t.Errorf("expected entry 100, got %.0f", d.Plan.Entry)
	}
}

func TestParseDecisionNoJSON(t *testing.T) {
	_, err := ParseDecision("no json here at all")
	if err == nil {
		t.Error("expected error when no json present")
	}
}

func TestParseDecisionInvalidJSON(t *testing.T) {
	_, err := ParseDecision("{invalid json}")
	if err == nil {
		t.Error("expected error for invalid json")
	}
}

func TestParseDecisionInvalidAction(t *testing.T) {
	text := `{"action":"WAIT","confidence":50,"entry":0,"stop_loss":0,"take_profit":0,"position_size":0,"reasoning":"waiting"}`
	_, err := ParseDecision(text)
	if err == nil {
		t.Error("expected error for invalid action")
	}
}

func TestParseDecisionCaseInsensitiveAction(t *testing.T) {
	text := `{"action":"buy","confidence":80,"entry":100,"stop_loss":95,"take_profit":110,"position_size":100,"reasoning":"test"}`
	d, err := ParseDecision(text)
	if err != nil {
		t.Fatalf("should handle lowercase action: %v", err)
	}
	if d.Action != ActionBuy {
		t.Errorf("expected BUY, got %s", d.Action)
	}
}

func TestParseDecisionConfidenceClamped(t *testing.T) {
	text := `{"action":"BUY","confidence":150,"entry":100,"stop_loss":95,"take_profit":110,"position_size":100,"reasoning":"test"}`
	d, err := ParseDecision(text)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if d.Confidence != 100 {
		t.Errorf("confidence should be clamped to 100, got %.0f", d.Confidence)
	}
}

func TestParseDecisionNegativeConfidence(t *testing.T) {
	text := `{"action":"HOLD","confidence":-10,"entry":0,"stop_loss":0,"take_profit":0,"position_size":0,"reasoning":"test"}`
	d, err := ParseDecision(text)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if d.Confidence != 0 {
		t.Errorf("negative confidence should be clamped to 0, got %.0f", d.Confidence)
	}
}

func TestCalculateRiskRewardBuy(t *testing.T) {
	// buy at 100, sl at 95 (risk=5), tp at 115 (reward=15) → r/r = 3.0
	rr := calculateRiskReward(100, 95, 115, ActionBuy)
	if rr != 3.0 {
		t.Errorf("expected r/r 3.0, got %.2f", rr)
	}
}

func TestCalculateRiskRewardSell(t *testing.T) {
	// sell at 100, sl at 105 (risk=5), tp at 85 (reward=15) → r/r = 3.0
	rr := calculateRiskReward(100, 105, 85, ActionSell)
	if rr != 3.0 {
		t.Errorf("expected r/r 3.0, got %.2f", rr)
	}
}

func TestCalculateRiskRewardZeroRisk(t *testing.T) {
	rr := calculateRiskReward(100, 100, 115, ActionBuy)
	if rr != 0 {
		t.Errorf("expected r/r 0 when sl equals entry, got %.2f", rr)
	}
}

func TestCalculateRiskRewardHold(t *testing.T) {
	rr := calculateRiskReward(100, 95, 115, ActionHold)
	if rr != 0 {
		t.Errorf("expected r/r 0 for hold, got %.2f", rr)
	}
}

func TestCalculateRiskRewardZeroValues(t *testing.T) {
	rr := calculateRiskReward(0, 0, 0, ActionBuy)
	if rr != 0 {
		t.Errorf("expected r/r 0 for zero values, got %.2f", rr)
	}
}

func TestExtractJSONNested(t *testing.T) {
	text := `some text {"outer": {"inner": "value"}} more text`
	result := extractJSON(text)
	if result != `{"outer": {"inner": "value"}}` {
		t.Errorf("expected nested json, got %q", result)
	}
}

func TestExtractJSONNone(t *testing.T) {
	result := extractJSON("no braces here")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestExtractJSONUnclosed(t *testing.T) {
	result := extractJSON("{unclosed")
	if result != "" {
		t.Errorf("expected empty for unclosed json, got %q", result)
	}
}

func TestValidateAction(t *testing.T) {
	tests := []struct {
		input string
		want  Action
		err   bool
	}{
		{"BUY", ActionBuy, false},
		{"buy", ActionBuy, false},
		{"  Buy  ", ActionBuy, false},
		{"SELL", ActionSell, false},
		{"sell", ActionSell, false},
		{"HOLD", ActionHold, false},
		{"hold", ActionHold, false},
		{"WAIT", "", true},
		{"", "", true},
		{"BUYY", "", true},
	}

	for _, tt := range tests {
		action, err := validateAction(tt.input)
		if tt.err && err == nil {
			t.Errorf("validateAction(%q): expected error", tt.input)
		}
		if !tt.err && err != nil {
			t.Errorf("validateAction(%q): unexpected error: %v", tt.input, err)
		}
		if !tt.err && action != tt.want {
			t.Errorf("validateAction(%q): expected %s, got %s", tt.input, tt.want, action)
		}
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		v, min, max, want float64
	}{
		{50, 0, 100, 50},
		{-10, 0, 100, 0},
		{150, 0, 100, 100},
		{0, 0, 100, 0},
		{100, 0, 100, 100},
	}
	for _, tt := range tests {
		got := clamp(tt.v, tt.min, tt.max)
		if got != tt.want {
			t.Errorf("clamp(%.0f, %.0f, %.0f) = %.0f, want %.0f", tt.v, tt.min, tt.max, got, tt.want)
		}
	}
}
