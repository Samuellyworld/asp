package claude

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// raw json shape from claude's response
type rawDecision struct {
	Action       string  `json:"action"`
	Confidence   float64 `json:"confidence"`
	Entry        float64 `json:"entry"`
	StopLoss     float64 `json:"stop_loss"`
	TakeProfit   float64 `json:"take_profit"`
	PositionSize float64 `json:"position_size"`
	Reasoning    string  `json:"reasoning"`
}

// extracts a structured decision from claude's text response
func ParseDecision(text string) (*Decision, error) {
	jsonStr := extractJSON(text)
	if jsonStr == "" {
		return nil, fmt.Errorf("no json found in response")
	}

	var raw rawDecision
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	action, err := validateAction(raw.Action)
	if err != nil {
		return nil, err
	}

	confidence := clamp(raw.Confidence, 0, 100)

	riskReward := calculateRiskReward(raw.Entry, raw.StopLoss, raw.TakeProfit, action)

	decision := &Decision{
		Action:     action,
		Confidence: confidence,
		Plan: TradePlan{
			Entry:        raw.Entry,
			StopLoss:     raw.StopLoss,
			TakeProfit:   raw.TakeProfit,
			PositionSize: raw.PositionSize,
			RiskReward:   riskReward,
		},
		Reasoning: raw.Reasoning,
	}

	return decision, nil
}

// finds the first json object in the text
func extractJSON(text string) string {
	start := strings.Index(text, "{")
	if start == -1 {
		return ""
	}

	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}

	return ""
}

// validates and normalizes the action string
func validateAction(s string) (Action, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "BUY":
		return ActionBuy, nil
	case "SELL":
		return ActionSell, nil
	case "HOLD":
		return ActionHold, nil
	default:
		return "", fmt.Errorf("invalid action: %q", s)
	}
}

// calculates risk/reward ratio from entry, sl, tp
func calculateRiskReward(entry, sl, tp float64, action Action) float64 {
	if entry == 0 || sl == 0 || tp == 0 {
		return 0
	}

	var risk, reward float64
	switch action {
	case ActionBuy:
		risk = entry - sl
		reward = tp - entry
	case ActionSell:
		risk = sl - entry
		reward = entry - tp
	default:
		return 0
	}

	if risk <= 0 {
		return 0
	}

	rr := reward / risk
	return math.Round(rr*100) / 100
}

// limits v to the range [min, max]
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
