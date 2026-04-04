package database

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAIDecisionRecord_Fields(t *testing.T) {
	approved := true
	rec := &AIDecisionRecord{
		UserID:          1,
		Symbol:          "BTC/USDT",
		Decision:        "BUY",
		Confidence:      85,
		EntryPrice:      42000,
		StopLoss:        41500,
		TakeProfit:      43000,
		PositionSizeUSD: 500,
		RiskRewardRatio: 2.0,
		Reasoning:       "strong bullish momentum",
		WasApproved:     &approved,
		WasExecuted:     false,
		FilterReason:    "none",
		CreatedAt:       time.Now(),
	}

	if rec.Decision != "BUY" {
		t.Errorf("expected BUY, got %s", rec.Decision)
	}
	if *rec.WasApproved != true {
		t.Error("expected approved")
	}
	if rec.FilterReason != "none" {
		t.Errorf("expected none filter, got %s", rec.FilterReason)
	}
}

func TestAIDecisionRecord_FilterReasons(t *testing.T) {
	reasons := []string{"none", "hold", "low_confidence", "duplicate",
		"daily_limit", "safety_blocked", "expired", "user_rejected"}

	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			rec := &AIDecisionRecord{FilterReason: reason}
			if rec.FilterReason != reason {
				t.Errorf("expected %s, got %s", reason, rec.FilterReason)
			}
		})
	}
}

func TestAIDecisionRecord_JSONBFields(t *testing.T) {
	rec := &AIDecisionRecord{
		IndicatorsData: map[string]interface{}{
			"rsi":            65.5,
			"overall_signal": "BULLISH",
		},
		MLPrediction: map[string]interface{}{
			"predicted_price": 43000.0,
			"confidence":      0.82,
		},
		SentimentData: map[string]interface{}{
			"label": "positive",
			"score": 0.75,
		},
	}

	// verify indicators serialize correctly
	indJSON, err := json.Marshal(rec.IndicatorsData)
	if err != nil {
		t.Fatalf("failed to marshal indicators: %v", err)
	}
	var ind map[string]interface{}
	if err := json.Unmarshal(indJSON, &ind); err != nil {
		t.Fatalf("failed to unmarshal indicators: %v", err)
	}
	if ind["overall_signal"] != "BULLISH" {
		t.Errorf("expected BULLISH signal, got %v", ind["overall_signal"])
	}

	// verify ML prediction
	mlJSON, err := json.Marshal(rec.MLPrediction)
	if err != nil {
		t.Fatalf("failed to marshal ml prediction: %v", err)
	}
	if len(mlJSON) == 0 {
		t.Error("expected non-empty ML JSON")
	}

	// verify sentiment
	sentJSON, err := json.Marshal(rec.SentimentData)
	if err != nil {
		t.Fatalf("failed to marshal sentiment: %v", err)
	}
	if len(sentJSON) == 0 {
		t.Error("expected non-empty sentiment JSON")
	}
}

func TestAIDecisionRecord_NullableApproved(t *testing.T) {
	// pending (nil)
	pending := &AIDecisionRecord{Decision: "BUY"}
	if pending.WasApproved != nil {
		t.Error("pending should have nil WasApproved")
	}

	// approved
	approved := true
	approvedRec := &AIDecisionRecord{Decision: "BUY", WasApproved: &approved}
	if *approvedRec.WasApproved != true {
		t.Error("expected true")
	}

	// rejected
	rejected := false
	rejectedRec := &AIDecisionRecord{Decision: "BUY", WasApproved: &rejected}
	if *rejectedRec.WasApproved != false {
		t.Error("expected false")
	}
}

func TestNewAIDecisionRepository(t *testing.T) {
	repo := NewAIDecisionRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}
