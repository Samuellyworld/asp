package claude

import (
	"fmt"
	"strings"
)

// builds the system prompt that tells claude how to respond
func buildSystemPrompt() string {
	return `You are a crypto trading analyst. Given market data, technical indicators, and ML predictions, you must provide a structured trading decision.

You MUST respond with EXACTLY this JSON format and nothing else:
{
  "action": "BUY" | "SELL" | "HOLD",
  "confidence": <number 0-100>,
  "entry": <price>,
  "stop_loss": <price>,
  "take_profit": <price>,
  "position_size": <usd amount>,
  "reasoning": "<one paragraph explaining your decision>"
}

Rules:
- Only recommend BUY or SELL if confidence >= 60
- Stop loss should limit risk to 1-3% of position
- Take profit should give at least 1:2 risk/reward ratio
- Position size should be proportional to confidence (higher confidence = larger position, max $500)
- If data is insufficient or conflicting, choose HOLD
- Be conservative — false positives are worse than missed opportunities
- Consider ALL available signals: indicators, ML predictions, and sentiment
- Keep reasoning concise (under 100 words)
- IMPORTANT: Account for trading costs when sizing positions and setting targets.
  Typical spot fees are 0.10% maker / 0.10% taker (round-trip ~0.20%).
  Futures fees are 0.02% maker / 0.04% taker plus 8h funding rate.
  Only recommend trades where expected profit clearly exceeds total costs.
  Avoid small scalps that fees would eat up.`
}

// builds the user prompt with all available analysis data
func buildUserPrompt(input *AnalysisInput) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Analyze %s for a trading decision.\n\n", input.Market.Symbol))

	// market data
	b.WriteString("## Market Data\n")
	b.WriteString(fmt.Sprintf("- Price: $%.2f\n", input.Market.Price))
	b.WriteString(fmt.Sprintf("- 24h Volume: $%.0f\n", input.Market.Volume24h))
	b.WriteString(fmt.Sprintf("- 24h Change: %.2f%%\n\n", input.Market.Change24h))

	// technical indicators
	if input.Indicators != nil {
		b.WriteString("## Technical Indicators\n")
		b.WriteString(formatIndicators(input.Indicators))
		b.WriteString("\n")
	}

	// ml prediction
	if input.Prediction != nil {
		b.WriteString("## ML Price Prediction\n")
		b.WriteString(formatPrediction(input.Prediction))
		b.WriteString("\n")
	}

	// sentiment
	if input.Sentiment != nil {
		b.WriteString("## Sentiment Analysis\n")
		b.WriteString(formatSentiment(input.Sentiment))
		b.WriteString("\n")
	}

	// trading costs
	if input.Costs != nil {
		b.WriteString("## Trading Costs\n")
		b.WriteString(formatTradingCosts(input.Costs))
		b.WriteString("\n")
	}

	b.WriteString("Provide your trading decision in the required JSON format.")
	return b.String()
}

// formats technical indicators for the prompt
func formatIndicators(ind *Indicators) string {
	var b strings.Builder

	// rsi interpretation
	rsiLabel := "neutral"
	if ind.RSI < 30 {
		rsiLabel = "oversold"
	} else if ind.RSI > 70 {
		rsiLabel = "overbought"
	}
	b.WriteString(fmt.Sprintf("- RSI(14): %.1f (%s)\n", ind.RSI, rsiLabel))

	// macd
	macdSignal := "neutral"
	if ind.MACDHist > 0 {
		macdSignal = "bullish"
	} else if ind.MACDHist < 0 {
		macdSignal = "bearish"
	}
	b.WriteString(fmt.Sprintf("- MACD: value=%.4f, signal=%.4f, histogram=%.4f (%s)\n",
		ind.MACDValue, ind.MACDSignal, ind.MACDHist, macdSignal))

	// bollinger bands
	b.WriteString(fmt.Sprintf("- Bollinger Bands: upper=%.2f, middle=%.2f, lower=%.2f\n",
		ind.BBUpper, ind.BBMiddle, ind.BBLower))

	// ema
	emaSignal := "bearish"
	if ind.EMA12 > ind.EMA26 {
		emaSignal = "bullish"
	}
	b.WriteString(fmt.Sprintf("- EMA: 12=%.2f, 26=%.2f (%s crossover)\n", ind.EMA12, ind.EMA26, emaSignal))

	// volume
	if ind.VolumeSpike {
		b.WriteString("- Volume: SPIKE detected (unusual activity)\n")
	} else {
		b.WriteString("- Volume: normal\n")
	}

	return b.String()
}

// formats ml prediction for the prompt
func formatPrediction(pred *MLPrediction) string {
	return fmt.Sprintf("- Direction: %s\n- Magnitude: %.2f%%\n- Confidence: %.0f%%\n- Timeframe: %s\n",
		pred.Direction, pred.Magnitude, pred.Confidence*100, pred.Timeframe)
}

// formats sentiment data for the prompt
func formatSentiment(sent *Sentiment) string {
	return fmt.Sprintf("- Label: %s\n- Score: %.2f\n- Confidence: %.0f%%\n",
		sent.Label, sent.Score, sent.Confidence*100)
}

// formats trading cost context for the prompt
func formatTradingCosts(costs *TradingCosts) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- Spot fees: %.2f%% maker / %.2f%% taker\n", costs.SpotMakerFeePct, costs.SpotTakerFeePct))
	b.WriteString(fmt.Sprintf("- Futures fees: %.2f%% maker / %.2f%% taker\n", costs.FuturesMakerPct, costs.FuturesTakerPct))
	if costs.FundingRatePct != 0 {
		b.WriteString(fmt.Sprintf("- Current 8h funding rate: %.4f%%\n", costs.FundingRatePct))
	}
	if costs.EstSlippageBps != 0 {
		b.WriteString(fmt.Sprintf("- Estimated slippage: %.1f bps\n", costs.EstSlippageBps))
	}
	if costs.AvgRoundTripCost != 0 {
		b.WriteString(fmt.Sprintf("- Estimated round-trip cost: %.3f%%\n", costs.AvgRoundTripCost))
	}
	return b.String()
}
