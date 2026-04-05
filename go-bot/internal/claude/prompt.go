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
- Consider ALL available signals: indicators, ML predictions, sentiment, market regime, and alternative data
- Keep reasoning concise (under 100 words)
- When order flow data is available, use buy/sell ratio and depth imbalance for entry timing
- When on-chain data shows high exchange inflows (positive net flow), be more cautious about buying
- When fear/greed index is extreme (< 20 or > 80), consider contrarian positions
- When higher-timeframe context is provided, use it for confirmation:
  * Only take longs if at least one HTF trend is "up" or "neutral"
  * Only take shorts if at least one HTF trend is "down" or "neutral"
  * If HTF and primary timeframe disagree, reduce confidence by 15-20
  * HTF overbought/oversold adds weight to reversal signals
- When market regime is provided, adapt strategy accordingly:
  * Trending: favor trend-following entries, wider stops
  * Ranging: favor mean-reversion at support/resistance
  * Volatile: reduce position size, use wider stops
  * Quiet: watch for breakout setups, wait for confirmation
- IMPORTANT: Account for trading costs when sizing positions and setting targets.
  Typical spot fees are 0.10% maker / 0.10% taker (round-trip ~0.20%).
  Futures fees are 0.02% maker / 0.04% taker plus 8h funding rate.
  Only recommend trades where expected profit clearly exceeds total costs.
  Avoid small scalps that fees would eat up.
- When ATR/ADX/Stochastic data is available, incorporate it:
  * High ATR (>3%) = volatile market — reduce size or widen stops
  * ADX > 25 = trending — favor trend-following strategies
  * ADX < 15 = no trend — favor mean-reversion or wait
  * Stochastic oversold + bullish cross = potential long entry
  * Stochastic overbought + bearish cross = potential short entry
- SELF-LEARNING: When trade history is provided, analyze your past decisions:
  * Identify patterns in winning vs losing trades
  * Adjust confidence based on recent accuracy (lower if losing streak)
  * Avoid market conditions that led to consecutive losses
  * If win rate < 40%, increase HOLD bias until conditions improve`
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

	// market regime
	if input.Regime != nil {
		b.WriteString("## Market Regime\n")
		b.WriteString(formatRegime(input.Regime))
		b.WriteString("\n")
	}

	// alternative data sources
	if input.AltData != nil {
		b.WriteString("## Alternative Data\n")
		b.WriteString(formatAltData(input.AltData))
		b.WriteString("\n")
	}

	// higher-timeframe context
	if len(input.HTFContext) > 0 {
		b.WriteString("## Higher Timeframe Context\n")
		b.WriteString(formatHTFContext(input.HTFContext))
		b.WriteString("\n")
	}

	// trade history for self-learning
	if len(input.TradeHistory) > 0 {
		b.WriteString("## Recent Trade History (Your Past Decisions)\n")
		b.WriteString(formatTradeHistory(input.TradeHistory))
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

	// atr
	if ind.ATR > 0 {
		b.WriteString(fmt.Sprintf("- ATR(14): %.4f (%.2f%% of price, %s)\n", ind.ATR, ind.ATRPercent, ind.ATRSignal))
	}

	// adx
	if ind.ADX > 0 {
		b.WriteString(fmt.Sprintf("- ADX(14): %.1f (%s, trend direction: %s)\n", ind.ADX, ind.ADXSignal, ind.ADXTrendDir))
	}

	// stochastic
	if ind.StochK > 0 || ind.StochD > 0 {
		b.WriteString(fmt.Sprintf("- Stochastic: %%K=%.1f, %%D=%.1f (%s)\n", ind.StochK, ind.StochD, ind.StochSignal))
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

// formats market regime data for the prompt
func formatRegime(r *RegimeInfo) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- Regime: %s\n", r.Regime))
	b.WriteString(fmt.Sprintf("- ADX (trend strength): %.1f\n", r.ADX))
	b.WriteString(fmt.Sprintf("- ATR%%  (volatility): %.2f%%\n", r.ATRPercent))
	b.WriteString(fmt.Sprintf("- Trend direction: %s\n", r.TrendDir))
	b.WriteString(fmt.Sprintf("- Classification confidence: %.0f%%\n", r.Confidence))
	b.WriteString(fmt.Sprintf("- Assessment: %s\n", r.Description))
	return b.String()
}

// formats alternative data for the prompt
func formatAltData(alt *AltData) string {
	var b strings.Builder

	if alt.OrderFlow != nil {
		b.WriteString("### Order Flow\n")
		b.WriteString(fmt.Sprintf("- Buy/Sell Ratio: %.2f", alt.OrderFlow.BuySellRatio))
		if alt.OrderFlow.BuySellRatio > 1.2 {
			b.WriteString(" (buyer dominated)")
		} else if alt.OrderFlow.BuySellRatio < 0.8 {
			b.WriteString(" (seller dominated)")
		} else {
			b.WriteString(" (balanced)")
		}
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("- Depth Imbalance: %.2f", alt.OrderFlow.DepthImbalance))
		if alt.OrderFlow.DepthImbalance > 0.2 {
			b.WriteString(" (buy wall)")
		} else if alt.OrderFlow.DepthImbalance < -0.2 {
			b.WriteString(" (sell wall)")
		}
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("- Large Buy Orders: %d, Large Sell Orders: %d\n",
			alt.OrderFlow.LargeBuyOrders, alt.OrderFlow.LargeSellOrders))
		b.WriteString(fmt.Sprintf("- Spread: %.1f bps\n", alt.OrderFlow.SpreadBps))
	}

	if alt.OnChain != nil {
		b.WriteString("### On-Chain Metrics\n")
		b.WriteString(fmt.Sprintf("- Net Exchange Flow: %.2f", alt.OnChain.NetFlow))
		if alt.OnChain.NetFlow > 0 {
			b.WriteString(" (coins entering exchanges = potential sell pressure)")
		} else if alt.OnChain.NetFlow < 0 {
			b.WriteString(" (coins leaving exchanges = accumulation)")
		}
		b.WriteString("\n")
		if alt.OnChain.WhaleTransactions > 0 {
			b.WriteString(fmt.Sprintf("- Whale Transactions (>$100k): %d\n", alt.OnChain.WhaleTransactions))
		}
		if alt.OnChain.ActiveAddresses > 0 {
			b.WriteString(fmt.Sprintf("- Active Addresses (24h): %d\n", alt.OnChain.ActiveAddresses))
		}
	}

	if alt.FundingRate != nil {
		b.WriteString("### Funding Rate\n")
		b.WriteString(fmt.Sprintf("- Current Rate: %.4f%%\n", alt.FundingRate.Rate*100))
		b.WriteString(fmt.Sprintf("- Annualized: %.1f%%\n", alt.FundingRate.Annualized))
		if alt.FundingRate.Rate > 0.001 {
			b.WriteString("- Signal: High positive funding = crowded longs (bearish contrarian)\n")
		} else if alt.FundingRate.Rate < -0.001 {
			b.WriteString("- Signal: Negative funding = crowded shorts (bullish contrarian)\n")
		}
	}

	if alt.Sentiment != nil {
		b.WriteString("### Market Sentiment\n")
		b.WriteString(fmt.Sprintf("- Overall: %s (score: %.2f)\n", alt.Sentiment.OverallLabel, alt.Sentiment.OverallScore))
		b.WriteString(fmt.Sprintf("- Fear & Greed Index: %d/100\n", alt.Sentiment.FearGreedIndex))
	}

	return b.String()
}

// formats higher-timeframe context for the prompt
func formatHTFContext(htf []HTFSnapshot) string {
	var b strings.Builder
	for _, snap := range htf {
		b.WriteString(fmt.Sprintf("### %s Timeframe\n", snap.Timeframe))
		if snap.TrendDir != "" {
			b.WriteString(fmt.Sprintf("- Trend: %s\n", snap.TrendDir))
		}
		if snap.RSI > 0 {
			label := "neutral"
			if snap.RSI < 30 {
				label = "oversold"
			} else if snap.RSI > 70 {
				label = "overbought"
			}
			b.WriteString(fmt.Sprintf("- RSI: %.1f (%s)\n", snap.RSI, label))
		}
		b.WriteString(fmt.Sprintf("- MACD Histogram: %.4f\n", snap.MACDHist))
		b.WriteString(fmt.Sprintf("- BB Position: %.2f\n", snap.BBPosition))
		if snap.EMASlope != 0 {
			dir := "flat"
			if snap.EMASlope > 0.1 {
				dir = "rising"
			} else if snap.EMASlope < -0.1 {
				dir = "falling"
			}
			b.WriteString(fmt.Sprintf("- EMA Slope: %.2f%% (%s)\n", snap.EMASlope, dir))
		}
	}
	return b.String()
}

// formats recent trade outcomes for self-learning feedback
func formatTradeHistory(trades []TradeOutcome) string {
	var b strings.Builder

	var wins, losses int
	var totalPnL float64
	for _, t := range trades {
		if t.PnLPct > 0 {
			wins++
		} else if t.PnLPct < 0 {
			losses++
		}
		totalPnL += t.PnLPct
	}

	b.WriteString(fmt.Sprintf("Summary: %d trades, %d wins, %d losses, avg P&L: %.2f%%\n",
		len(trades), wins, losses, totalPnL/float64(len(trades))))
	b.WriteString("Learn from these outcomes — avoid repeating losing patterns.\n\n")

	for _, t := range trades {
		result := "LOSS"
		if t.Correct {
			result = "WIN"
		}
		b.WriteString(fmt.Sprintf("- %s %s @ $%.2f → $%.2f (%.2f%%, %s, confidence was %.0f%%)\n",
			t.Action, t.Symbol, t.EntryPrice, t.ExitPrice, t.PnLPct, result, t.Confidence))
	}
	return b.String()
}
