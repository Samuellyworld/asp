package pipeline

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/trading-bot/go-bot/internal/analysis"
)

// formats the pipeline result as a telegram message (markdown v1)
func FormatTelegramMessage(r *Result) string {
	var b strings.Builder

	if r.Decision == nil {
		b.WriteString("❌ Analysis failed for " + r.Symbol)
		return b.String()
	}

	// header
	emoji := decisionEmoji(string(r.Decision.Action))
	b.WriteString(fmt.Sprintf("%s *%s Analysis — %s*\n\n", emoji, r.Symbol, r.Decision.Action))

	// decision summary
	b.WriteString(fmt.Sprintf("*Decision:* %s | *Confidence:* %.0f%%\n", r.Decision.Action, r.Decision.Confidence))

	// trade plan (only for buy/sell)
	if r.Decision.Action != "HOLD" && r.Decision.Plan.Entry > 0 {
		b.WriteString(fmt.Sprintf("*Entry:* $%s | *SL:* $%s | *TP:* $%s\n",
			formatNum(r.Decision.Plan.Entry),
			formatNum(r.Decision.Plan.StopLoss),
			formatNum(r.Decision.Plan.TakeProfit)))
		b.WriteString(fmt.Sprintf("*Position:* $%s | *R/R:* 1:%.1f\n",
			formatNum(r.Decision.Plan.PositionSize),
			r.Decision.Plan.RiskReward))
	}

	b.WriteString("\n")

	// market data
	if r.Ticker != nil {
		b.WriteString(fmt.Sprintf("💰 *Price:* $%s (%s)\n",
			formatNum(r.Ticker.Price),
			formatChange(r.Ticker.ChangePct)))
	}

	// indicators
	if r.Indicators != nil {
		b.WriteString(formatIndicatorsSummary(r.Indicators))
	}

	// ml prediction
	if r.Prediction != nil {
		arrow := "→"
		if r.Prediction.Direction == "up" {
			arrow = "↑"
		} else if r.Prediction.Direction == "down" {
			arrow = "↓"
		}
		b.WriteString(fmt.Sprintf("🤖 *ML:* %s%.2f%% (%.0f%% conf, %s)\n",
			arrow, r.Prediction.Magnitude, r.Prediction.Confidence*100, r.Prediction.Timeframe))
	}

	// sentiment
	if r.Sentiment != nil {
		b.WriteString(fmt.Sprintf("💬 *Sentiment:* %s (%.2f, %.0f%% conf)\n",
			r.Sentiment.Label, r.Sentiment.Score, r.Sentiment.Confidence*100))
	}

	// reasoning
	if r.Decision.Reasoning != "" {
		b.WriteString(fmt.Sprintf("\n💡 %s\n", r.Decision.Reasoning))
	}

	// footer
	b.WriteString(fmt.Sprintf("\n⏱ %s", formatLatency(r.Latency)))

	return b.String()
}

// builds discord embed fields from the pipeline result
type DiscordField struct {
	Name   string
	Value  string
	Inline bool
}

// formats the pipeline result as discord embed fields
func FormatDiscordFields(r *Result) (title string, description string, fields []DiscordField, color int) {
	if r.Decision == nil {
		return "❌ Analysis Failed", "Could not analyze " + r.Symbol, nil, 0xFF0000
	}

	emoji := decisionEmoji(string(r.Decision.Action))
	title = fmt.Sprintf("%s %s — %s", emoji, r.Symbol, r.Decision.Action)
	color = decisionColor(string(r.Decision.Action))

	fields = append(fields, DiscordField{
		Name:   "Decision",
		Value:  fmt.Sprintf("%s (%.0f%% confidence)", r.Decision.Action, r.Decision.Confidence),
		Inline: true,
	})

	if r.Decision.Action != "HOLD" && r.Decision.Plan.Entry > 0 {
		fields = append(fields, DiscordField{
			Name: "Trade Plan",
			Value: fmt.Sprintf("Entry: $%s\nSL: $%s | TP: $%s\nSize: $%s | R/R: 1:%.1f",
				formatNum(r.Decision.Plan.Entry),
				formatNum(r.Decision.Plan.StopLoss),
				formatNum(r.Decision.Plan.TakeProfit),
				formatNum(r.Decision.Plan.PositionSize),
				r.Decision.Plan.RiskReward),
			Inline: true,
		})
	}

	if r.Ticker != nil {
		fields = append(fields, DiscordField{
			Name:   "Market",
			Value:  fmt.Sprintf("$%s (%s)", formatNum(r.Ticker.Price), formatChange(r.Ticker.ChangePct)),
			Inline: true,
		})
	}

	if r.Indicators != nil {
		fields = append(fields, DiscordField{
			Name:   "Indicators",
			Value:  formatIndicatorsSummary(r.Indicators),
			Inline: false,
		})
	}

	if r.Prediction != nil {
		fields = append(fields, DiscordField{
			Name:   "ML Prediction",
			Value:  fmt.Sprintf("%s %.2f%% (%.0f%% conf, %s)", r.Prediction.Direction, r.Prediction.Magnitude, r.Prediction.Confidence*100, r.Prediction.Timeframe),
			Inline: true,
		})
	}

	if r.Sentiment != nil {
		fields = append(fields, DiscordField{
			Name:   "Sentiment",
			Value:  fmt.Sprintf("%s (%.2f, %.0f%% conf)", r.Sentiment.Label, r.Sentiment.Score, r.Sentiment.Confidence*100),
			Inline: true,
		})
	}

	if r.Decision.Reasoning != "" {
		description = r.Decision.Reasoning
	}

	return title, description, fields, color
}

// formats a compact indicators summary
func formatIndicatorsSummary(ind *analysis.AnalysisResult) string {
	var parts []string
	if ind.RSI != nil {
		parts = append(parts, fmt.Sprintf("RSI: %.1f (%s)", ind.RSI.Value, ind.RSI.Signal))
	}
	if ind.MACD != nil {
		parts = append(parts, fmt.Sprintf("MACD: %s", ind.MACD.Signal))
	}
	if ind.Bollinger != nil {
		parts = append(parts, fmt.Sprintf("BB: %s", ind.Bollinger.Signal))
	}
	if ind.Volume != nil && ind.Volume.IsSpike {
		parts = append(parts, "Vol: SPIKE")
	}
	if len(parts) == 0 {
		return ""
	}
	return "📊 " + strings.Join(parts, " | ") + "\n"
}

func decisionEmoji(action string) string {
	switch action {
	case "BUY":
		return "🟢"
	case "SELL":
		return "🔴"
	default:
		return "⏸️"
	}
}

func decisionColor(action string) int {
	switch action {
	case "BUY":
		return 0x10B981
	case "SELL":
		return 0xEF4444
	default:
		return 0x6B7280
	}
}

func formatNum(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.2f", v)
	}
	if v >= 1 {
		return fmt.Sprintf("%.4f", v)
	}
	return fmt.Sprintf("%.6f", v)
}

func formatChange(pct float64) string {
	sign := "+"
	if pct < 0 {
		sign = ""
	}
	return fmt.Sprintf("%s%.2f%%", sign, pct)
}

func formatLatency(d time.Duration) string {
	ms := d.Milliseconds()
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000.0)
}

// returns absolute value
func abs(v float64) float64 {
	return math.Abs(v)
}
