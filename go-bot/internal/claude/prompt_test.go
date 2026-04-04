package claude

import (
	"strings"
	"testing"
)

func TestBuildSystemPrompt(t *testing.T) {
	prompt := buildSystemPrompt()
	if prompt == "" {
		t.Error("expected non-empty system prompt")
	}
	if !strings.Contains(prompt, "BUY") {
		t.Error("system prompt should mention BUY action")
	}
	if !strings.Contains(prompt, "SELL") {
		t.Error("system prompt should mention SELL action")
	}
	if !strings.Contains(prompt, "HOLD") {
		t.Error("system prompt should mention HOLD action")
	}
	if !strings.Contains(prompt, "JSON") {
		t.Error("system prompt should mention JSON format")
	}
	if !strings.Contains(prompt, "stop_loss") {
		t.Error("system prompt should mention stop_loss field")
	}
	if !strings.Contains(prompt, "risk/reward") {
		t.Error("system prompt should mention risk/reward")
	}
}

func TestBuildSystemPromptFeeAwareness(t *testing.T) {
	prompt := buildSystemPrompt()
	checks := []string{
		"0.10%",
		"0.02%",
		"0.04%",
		"funding rate",
		"trading costs",
		"scalps",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("system prompt should contain %q for fee awareness", check)
		}
	}
}

func TestBuildUserPromptWithAllData(t *testing.T) {
	input := &AnalysisInput{
		Market: MarketData{
			Symbol:    "BTC/USDT",
			Price:     42450,
			Volume24h: 28500000000,
			Change24h: -2.1,
		},
		Indicators: &Indicators{
			RSI:         28.5,
			MACDValue:   -50,
			MACDSignal:  -80,
			MACDHist:    30,
			BBUpper:     44000,
			BBMiddle:    42500,
			BBLower:     41000,
			EMA12:       42300,
			EMA26:       42100,
			VolumeSpike: true,
		},
		Prediction: &MLPrediction{
			Direction:  "up",
			Magnitude:  3.2,
			Confidence: 0.78,
			Timeframe:  "4h",
		},
		Sentiment: &Sentiment{
			Score:      0.82,
			Label:      "BULLISH",
			Confidence: 0.91,
		},
	}

	prompt := buildUserPrompt(input)

	checks := []string{
		"BTC/USDT",
		"42450",
		"Technical Indicators",
		"RSI(14): 28.5 (oversold)",
		"bullish",
		"ML Price Prediction",
		"Direction: up",
		"3.20%",
		"Sentiment",
		"BULLISH",
		"SPIKE",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("prompt should contain %q", check)
		}
	}
}

func TestBuildUserPromptMarketOnly(t *testing.T) {
	input := &AnalysisInput{
		Market: MarketData{
			Symbol:    "ETH/USDT",
			Price:     2200,
			Volume24h: 15000000000,
			Change24h: 1.5,
		},
	}

	prompt := buildUserPrompt(input)

	if !strings.Contains(prompt, "ETH/USDT") {
		t.Error("should contain symbol")
	}
	if !strings.Contains(prompt, "2200") {
		t.Error("should contain price")
	}
	if strings.Contains(prompt, "Technical Indicators") {
		t.Error("should not contain indicators section")
	}
	if strings.Contains(prompt, "ML Price Prediction") {
		t.Error("should not contain prediction section")
	}
	if strings.Contains(prompt, "Sentiment") {
		t.Error("should not contain sentiment section")
	}
}

func TestFormatIndicatorsOversold(t *testing.T) {
	ind := &Indicators{RSI: 25, MACDHist: -10, EMA12: 100, EMA26: 200}
	result := formatIndicators(ind)
	if !strings.Contains(result, "oversold") {
		t.Error("rsi 25 should be labeled oversold")
	}
	if !strings.Contains(result, "bearish") {
		t.Error("negative macd histogram should be bearish")
	}
}

func TestFormatIndicatorsOverbought(t *testing.T) {
	ind := &Indicators{RSI: 75, MACDHist: 10, EMA12: 200, EMA26: 100}
	result := formatIndicators(ind)
	if !strings.Contains(result, "overbought") {
		t.Error("rsi 75 should be labeled overbought")
	}
	if !strings.Contains(result, "bullish") {
		t.Error("positive macd histogram should be bullish")
	}
}

func TestFormatIndicatorsNeutral(t *testing.T) {
	ind := &Indicators{RSI: 50, MACDHist: 0, EMA12: 100, EMA26: 100}
	result := formatIndicators(ind)
	if !strings.Contains(result, "neutral") {
		t.Error("rsi 50 should be neutral")
	}
}

func TestFormatIndicatorsVolumeNormal(t *testing.T) {
	ind := &Indicators{RSI: 50, VolumeSpike: false, EMA12: 100, EMA26: 100}
	result := formatIndicators(ind)
	if !strings.Contains(result, "normal") {
		t.Error("no volume spike should show normal")
	}
}

func TestFormatPrediction(t *testing.T) {
	pred := &MLPrediction{
		Direction:  "up",
		Magnitude:  3.2,
		Confidence: 0.78,
		Timeframe:  "4h",
	}
	result := formatPrediction(pred)
	if !strings.Contains(result, "up") {
		t.Error("should contain direction")
	}
	if !strings.Contains(result, "3.20%") {
		t.Error("should contain magnitude")
	}
	if !strings.Contains(result, "78%") {
		t.Error("should contain confidence as percentage")
	}
	if !strings.Contains(result, "4h") {
		t.Error("should contain timeframe")
	}
}

func TestFormatSentiment(t *testing.T) {
	sent := &Sentiment{Score: 0.82, Label: "BULLISH", Confidence: 0.91}
	result := formatSentiment(sent)
	if !strings.Contains(result, "BULLISH") {
		t.Error("should contain label")
	}
	if !strings.Contains(result, "0.82") {
		t.Error("should contain score")
	}
	if !strings.Contains(result, "91%") {
		t.Error("should contain confidence as percentage")
	}
}

func TestBuildUserPromptPartialData(t *testing.T) {
	// only indicators, no ml/sentiment
	input := &AnalysisInput{
		Market: MarketData{Symbol: "SOL/USDT", Price: 100},
		Indicators: &Indicators{
			RSI: 45, EMA12: 99, EMA26: 101,
		},
	}
	prompt := buildUserPrompt(input)
	if !strings.Contains(prompt, "Technical Indicators") {
		t.Error("should contain indicators")
	}
	if strings.Contains(prompt, "ML Price Prediction") {
		t.Error("should not contain prediction")
	}
	if strings.Contains(prompt, "Sentiment") {
		t.Error("should not contain sentiment")
	}
}

func TestEMACrossoverDirection(t *testing.T) {
	bullish := &Indicators{RSI: 50, EMA12: 200, EMA26: 100}
	result := formatIndicators(bullish)
	if !strings.Contains(result, "bullish crossover") {
		t.Error("ema12 > ema26 should show bullish crossover")
	}

	bearish := &Indicators{RSI: 50, EMA12: 100, EMA26: 200}
	result = formatIndicators(bearish)
	if !strings.Contains(result, "bearish crossover") {
		t.Error("ema12 < ema26 should show bearish crossover")
	}
}

func TestFormatTradingCostsFullFields(t *testing.T) {
	costs := &TradingCosts{
		SpotMakerFeePct: 0.10,
		SpotTakerFeePct: 0.10,
		FuturesMakerPct: 0.02,
		FuturesTakerPct: 0.04,
		FundingRatePct:  0.0100,
		EstSlippageBps:  3.5,
		AvgRoundTripCost: 0.245,
	}
	result := formatTradingCosts(costs)

	checks := []string{
		"Spot fees: 0.10% maker / 0.10% taker",
		"Futures fees: 0.02% maker / 0.04% taker",
		"funding rate: 0.0100%",
		"slippage: 3.5 bps",
		"round-trip cost: 0.245%",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("formatTradingCosts should contain %q, got:\n%s", check, result)
		}
	}
}

func TestFormatTradingCostsMinimalFields(t *testing.T) {
	costs := &TradingCosts{
		SpotMakerFeePct: 0.10,
		SpotTakerFeePct: 0.10,
		FuturesMakerPct: 0.02,
		FuturesTakerPct: 0.04,
	}
	result := formatTradingCosts(costs)

	if !strings.Contains(result, "Spot fees") {
		t.Error("should always include spot fees")
	}
	if strings.Contains(result, "funding rate") {
		t.Error("should not include funding rate when zero")
	}
	if strings.Contains(result, "slippage") {
		t.Error("should not include slippage when zero")
	}
	if strings.Contains(result, "round-trip") {
		t.Error("should not include round-trip cost when zero")
	}
}

func TestBuildUserPromptWithCosts(t *testing.T) {
	input := &AnalysisInput{
		Market: MarketData{Symbol: "BTC/USDT", Price: 42000},
		Costs:  DefaultTradingCosts(),
	}
	prompt := buildUserPrompt(input)

	if !strings.Contains(prompt, "Trading Costs") {
		t.Error("prompt should contain Trading Costs section")
	}
	if !strings.Contains(prompt, "Spot fees") {
		t.Error("prompt should contain spot fees info")
	}
}

func TestBuildUserPromptWithoutCosts(t *testing.T) {
	input := &AnalysisInput{
		Market: MarketData{Symbol: "BTC/USDT", Price: 42000},
	}
	prompt := buildUserPrompt(input)

	if strings.Contains(prompt, "Trading Costs") {
		t.Error("prompt should not contain Trading Costs section when costs is nil")
	}
}

func TestDefaultTradingCosts(t *testing.T) {
	costs := DefaultTradingCosts()
	if costs.SpotMakerFeePct != 0.10 {
		t.Errorf("expected spot maker 0.10, got %f", costs.SpotMakerFeePct)
	}
	if costs.SpotTakerFeePct != 0.10 {
		t.Errorf("expected spot taker 0.10, got %f", costs.SpotTakerFeePct)
	}
	if costs.FuturesMakerPct != 0.02 {
		t.Errorf("expected futures maker 0.02, got %f", costs.FuturesMakerPct)
	}
	if costs.FuturesTakerPct != 0.04 {
		t.Errorf("expected futures taker 0.04, got %f", costs.FuturesTakerPct)
	}
	if costs.FundingRatePct != 0 {
		t.Error("default funding rate should be 0")
	}
	if costs.EstSlippageBps != 0 {
		t.Error("default slippage should be 0")
	}
}

func TestBuildSystemPromptAltDataRules(t *testing.T) {
	prompt := buildSystemPrompt()
	checks := []string{
		"order flow",
		"on-chain",
		"fear/greed",
		"higher-timeframe",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("system prompt should mention %q", check)
		}
	}
}

func TestFormatAltDataOrderFlow(t *testing.T) {
	alt := &AltData{
		OrderFlow: &OrderFlowData{
			BuySellRatio:    1.5,
			DepthImbalance:  0.3,
			LargeBuyOrders:  5,
			LargeSellOrders: 2,
			SpreadBps:       1.2,
		},
	}
	result := formatAltData(alt)
	if !strings.Contains(result, "buyer dominated") {
		t.Error("1.5 ratio should show buyer dominated")
	}
	if !strings.Contains(result, "buy wall") {
		t.Error("0.3 imbalance should show buy wall")
	}
	if !strings.Contains(result, "Large Buy Orders: 5") {
		t.Error("should show large buy orders")
	}
}

func TestFormatAltDataSellerDominated(t *testing.T) {
	alt := &AltData{
		OrderFlow: &OrderFlowData{
			BuySellRatio:   0.7,
			DepthImbalance: -0.3,
		},
	}
	result := formatAltData(alt)
	if !strings.Contains(result, "seller dominated") {
		t.Error("0.7 ratio should show seller dominated")
	}
	if !strings.Contains(result, "sell wall") {
		t.Error("-0.3 imbalance should show sell wall")
	}
}

func TestFormatAltDataFundingCrowdedLongs(t *testing.T) {
	alt := &AltData{
		FundingRate: &FundingData{
			Rate:       0.002,
			Annualized: 21.9,
		},
	}
	result := formatAltData(alt)
	if !strings.Contains(result, "crowded longs") {
		t.Error("high positive funding should signal crowded longs")
	}
}

func TestFormatAltDataFundingCrowdedShorts(t *testing.T) {
	alt := &AltData{
		FundingRate: &FundingData{
			Rate:       -0.002,
			Annualized: -21.9,
		},
	}
	result := formatAltData(alt)
	if !strings.Contains(result, "crowded shorts") {
		t.Error("negative funding should signal crowded shorts")
	}
}

func TestFormatAltDataSentiment(t *testing.T) {
	alt := &AltData{
		Sentiment: &SentimentData{
			OverallScore:   0.5,
			OverallLabel:   "BULLISH",
			FearGreedIndex: 72,
		},
	}
	result := formatAltData(alt)
	if !strings.Contains(result, "BULLISH") {
		t.Error("should show overall label")
	}
	if !strings.Contains(result, "72/100") {
		t.Error("should show fear/greed index")
	}
}

func TestFormatAltDataOnChainPositiveNetFlow(t *testing.T) {
	alt := &AltData{
		OnChain: &OnChainData{
			NetFlow:           500,
			WhaleTransactions: 10,
			ActiveAddresses:   100000,
		},
	}
	result := formatAltData(alt)
	if !strings.Contains(result, "potential sell pressure") {
		t.Error("positive net flow should indicate sell pressure")
	}
	if !strings.Contains(result, "Whale Transactions") {
		t.Error("should show whale transactions")
	}
}

func TestFormatHTFContext(t *testing.T) {
	htf := []HTFSnapshot{
		{
			Timeframe:  "1d",
			TrendDir:   "up",
			RSI:        55,
			MACDHist:   0.5,
			BBPosition: 0.6,
			EMASlope:   0.5,
		},
		{
			Timeframe:  "1w",
			TrendDir:   "down",
			RSI:        72,
			MACDHist:   -0.2,
			BBPosition: 0.8,
			EMASlope:   -0.3,
		},
	}
	result := formatHTFContext(htf)
	if !strings.Contains(result, "1d Timeframe") {
		t.Error("should contain 1d timeframe header")
	}
	if !strings.Contains(result, "1w Timeframe") {
		t.Error("should contain 1w timeframe header")
	}
	if !strings.Contains(result, "Trend: up") {
		t.Error("should show up trend")
	}
	if !strings.Contains(result, "overbought") {
		t.Error("RSI 72 should show overbought")
	}
	if !strings.Contains(result, "rising") {
		t.Error("positive EMA slope should show rising")
	}
	if !strings.Contains(result, "falling") {
		t.Error("negative EMA slope should show falling")
	}
}

func TestFormatHTFContextEmpty(t *testing.T) {
	result := formatHTFContext(nil)
	if result != "" {
		t.Error("expected empty string for nil HTF context")
	}
}

func TestBuildUserPromptWithAltData(t *testing.T) {
	input := &AnalysisInput{
		Market: MarketData{Symbol: "BTC/USDT", Price: 42000},
		AltData: &AltData{
			OrderFlow: &OrderFlowData{BuySellRatio: 1.1, DepthImbalance: 0.05},
		},
	}
	prompt := buildUserPrompt(input)
	if !strings.Contains(prompt, "Alternative Data") {
		t.Error("prompt should contain Alternative Data section")
	}
}

func TestBuildUserPromptWithHTFContext(t *testing.T) {
	input := &AnalysisInput{
		Market: MarketData{Symbol: "BTC/USDT", Price: 42000},
		HTFContext: []HTFSnapshot{
			{Timeframe: "1d", TrendDir: "up", RSI: 55},
		},
	}
	prompt := buildUserPrompt(input)
	if !strings.Contains(prompt, "Higher Timeframe Context") {
		t.Error("prompt should contain Higher Timeframe Context section")
	}
}
