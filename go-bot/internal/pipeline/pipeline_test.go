package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/analysis"
	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/exchange"
	mlclient "github.com/trading-bot/go-bot/internal/ml-client"
)

// --- mock exchange ---

type mockExchange struct {
	ticker  *exchange.Ticker
	candles []exchange.Candle
	err     error
}

func (m *mockExchange) GetPrice(_ context.Context, _ string) (*exchange.Ticker, error) {
	return m.ticker, m.err
}

func (m *mockExchange) GetCandles(_ context.Context, _ string, _ string, _ int) ([]exchange.Candle, error) {
	return m.candles, m.err
}

// --- mock indicator provider ---

type mockIndicators struct {
	result *analysis.AnalysisResult
	err    error
}

func (m *mockIndicators) AnalyzeAll(_ context.Context, _ []analysis.Candle, _ *analysis.AnalyzeOptions) (*analysis.AnalysisResult, error) {
	return m.result, m.err
}

// --- mock ml provider ---

type mockML struct {
	prediction *mlclient.PricePredictionResponse
	sentiment  *mlclient.SentimentResponse
	available  bool
	predErr    error
	sentErr    error
}

func (m *mockML) PredictPrice(_ context.Context, _ *mlclient.PricePredictionRequest) (*mlclient.PricePredictionResponse, error) {
	return m.prediction, m.predErr
}

func (m *mockML) AnalyzeSentiment(_ context.Context, _ string) (*mlclient.SentimentResponse, error) {
	return m.sentiment, m.sentErr
}

func (m *mockML) IsAvailable(_ context.Context) bool {
	return m.available
}

// --- mock ai provider ---

type mockAI struct {
	decision *claude.Decision
	err      error
}

func (m *mockAI) Analyze(_ context.Context, _ *claude.AnalysisInput) (*claude.Decision, error) {
	return m.decision, m.err
}

// --- test data helpers ---

func testTicker() *exchange.Ticker {
	return &exchange.Ticker{
		Symbol:      "BTC/USDT",
		Price:       42450.00,
		PriceChange: -920.50,
		ChangePct:   -2.12,
		Volume:      680000,
		QuoteVolume: 28500000000,
	}
}

func testCandles(n int) []exchange.Candle {
	candles := make([]exchange.Candle, n)
	base := 42000.0
	now := time.Now()
	for i := 0; i < n; i++ {
		price := base + float64(i)*10
		candles[i] = exchange.Candle{
			OpenTime:  now.Add(time.Duration(-n+i) * 4 * time.Hour),
			Open:      price,
			High:      price + 200,
			Low:       price - 100,
			Close:     price + 50,
			Volume:    1000 + float64(i)*10,
			CloseTime: now.Add(time.Duration(-n+i+1) * 4 * time.Hour),
		}
	}
	return candles
}

func testIndicators() *analysis.AnalysisResult {
	return &analysis.AnalysisResult{
		RSI:           &analysis.RSIResult{Value: 32.5, Signal: "oversold"},
		MACD:          &analysis.MACDResult{MACDLine: -50, SignalLine: -80, Histogram: 30, Signal: "bullish"},
		Bollinger:     &analysis.BollingerResult{Upper: 44000, Middle: 42500, Lower: 41000, Signal: "lower_band"},
		EMA:           &analysis.EMAResult{Value: 42300, Trend: "bullish"},
		Volume:        &analysis.VolumeResult{IsSpike: false, CurrentVolume: 500, AverageVolume: 450, Ratio: 1.1, Signal: "normal"},
		OverallSignal: "bullish",
		BullishCount:  3,
		BearishCount:  1,
	}
}

func testPrediction() *mlclient.PricePredictionResponse {
	return &mlclient.PricePredictionResponse{
		Direction:      "up",
		Magnitude:      3.2,
		Confidence:     0.78,
		Timeframe:      "4h",
		PredictedPrice: 43800,
		CurrentPrice:   42450,
	}
}

func testSentiment() *mlclient.SentimentResponse {
	return &mlclient.SentimentResponse{
		Score:      0.82,
		Label:      "BULLISH",
		Confidence: 0.91,
	}
}

func testDecision() *claude.Decision {
	return &claude.Decision{
		Action:     claude.ActionBuy,
		Confidence: 85,
		Plan: claude.TradePlan{
			Entry:        42450,
			StopLoss:     41800,
			TakeProfit:   44200,
			PositionSize: 200,
			RiskReward:   2.69,
		},
		Reasoning: "Strong confluence at support with bullish indicators and positive ML prediction.",
		Timestamp: time.Now(),
		Latency:   1400 * time.Millisecond,
	}
}

// --- pipeline tests ---

func TestFullPipeline(t *testing.T) {
	ex := &mockExchange{ticker: testTicker(), candles: testCandles(100)}
	ind := &mockIndicators{result: testIndicators()}
	ml := &mockML{prediction: testPrediction(), sentiment: testSentiment(), available: true}
	ai := &mockAI{decision: testDecision()}

	p := New(ex, ind, ml, ai)
	result, err := p.Analyze(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	if result.Symbol != "BTC/USDT" {
		t.Errorf("expected symbol BTC/USDT, got %s", result.Symbol)
	}
	if result.Ticker == nil {
		t.Fatal("expected ticker")
	}
	if result.Indicators == nil {
		t.Fatal("expected indicators")
	}
	if result.Prediction == nil {
		t.Fatal("expected prediction")
	}
	if result.Sentiment == nil {
		t.Fatal("expected sentiment")
	}
	if result.Decision == nil {
		t.Fatal("expected decision")
	}
	if result.Decision.Action != claude.ActionBuy {
		t.Errorf("expected BUY, got %s", result.Decision.Action)
	}
	if result.Latency <= 0 {
		t.Error("expected positive latency")
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}
}

func TestPipelineWithoutML(t *testing.T) {
	ex := &mockExchange{ticker: testTicker(), candles: testCandles(100)}
	ind := &mockIndicators{result: testIndicators()}
	ml := &mockML{available: false}
	ai := &mockAI{decision: testDecision()}

	p := New(ex, ind, ml, ai)
	result, err := p.Analyze(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	if result.Prediction != nil {
		t.Error("expected nil prediction when ml unavailable")
	}
	if result.Sentiment != nil {
		t.Error("expected nil sentiment when ml unavailable")
	}
	if result.Decision == nil {
		t.Fatal("expected decision even without ml")
	}
}

func TestPipelineWithNilML(t *testing.T) {
	ex := &mockExchange{ticker: testTicker(), candles: testCandles(100)}
	ind := &mockIndicators{result: testIndicators()}
	ai := &mockAI{decision: testDecision()}

	p := New(ex, ind, nil, ai)
	result, err := p.Analyze(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
	if result.Decision == nil {
		t.Fatal("expected decision with nil ml provider")
	}
}

func TestPipelineIndicatorError(t *testing.T) {
	ex := &mockExchange{ticker: testTicker(), candles: testCandles(100)}
	ind := &mockIndicators{err: context.DeadlineExceeded}
	ml := &mockML{prediction: testPrediction(), sentiment: testSentiment(), available: true}
	ai := &mockAI{decision: testDecision()}

	p := New(ex, ind, ml, ai)
	result, err := p.Analyze(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatalf("pipeline should not fail on indicator error: %v", err)
	}
	if result.Indicators != nil {
		t.Error("expected nil indicators on error")
	}
	if len(result.Errors) == 0 {
		t.Error("expected error logged for indicators")
	}
}

func TestPipelineMLPredictionError(t *testing.T) {
	ex := &mockExchange{ticker: testTicker(), candles: testCandles(100)}
	ind := &mockIndicators{result: testIndicators()}
	ml := &mockML{predErr: context.DeadlineExceeded, available: true}
	ai := &mockAI{decision: testDecision()}

	p := New(ex, ind, ml, ai)
	result, err := p.Analyze(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatalf("pipeline should not fail on ml error: %v", err)
	}
	if result.Prediction != nil {
		t.Error("expected nil prediction on error")
	}
	if len(result.Errors) == 0 {
		t.Error("expected error logged for prediction")
	}
}

func TestPipelineExchangeError(t *testing.T) {
	ex := &mockExchange{err: context.DeadlineExceeded}
	ind := &mockIndicators{result: testIndicators()}
	ml := &mockML{available: true}
	ai := &mockAI{decision: testDecision()}

	p := New(ex, ind, ml, ai)
	_, err := p.Analyze(context.Background(), "BTC/USDT")
	if err == nil {
		t.Error("expected error when exchange fails")
	}
}

func TestPipelineAIError(t *testing.T) {
	ex := &mockExchange{ticker: testTicker(), candles: testCandles(100)}
	ind := &mockIndicators{result: testIndicators()}
	ml := &mockML{prediction: testPrediction(), sentiment: testSentiment(), available: true}
	ai := &mockAI{err: context.DeadlineExceeded}

	p := New(ex, ind, ml, ai)
	_, err := p.Analyze(context.Background(), "BTC/USDT")
	if err == nil {
		t.Error("expected error when ai fails")
	}
}

func TestPipelineSellDecision(t *testing.T) {
	decision := testDecision()
	decision.Action = claude.ActionSell
	decision.Confidence = 72

	ex := &mockExchange{ticker: testTicker(), candles: testCandles(100)}
	ind := &mockIndicators{result: testIndicators()}
	ml := &mockML{prediction: testPrediction(), sentiment: testSentiment(), available: true}
	ai := &mockAI{decision: decision}

	p := New(ex, ind, ml, ai)
	result, err := p.Analyze(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
	if result.Decision.Action != claude.ActionSell {
		t.Errorf("expected SELL, got %s", result.Decision.Action)
	}
}

func TestPipelineHoldDecision(t *testing.T) {
	decision := &claude.Decision{
		Action:     claude.ActionHold,
		Confidence: 40,
		Reasoning:  "Conflicting signals.",
		Timestamp:  time.Now(),
	}

	ex := &mockExchange{ticker: testTicker(), candles: testCandles(100)}
	ind := &mockIndicators{result: testIndicators()}
	ml := &mockML{available: false}
	ai := &mockAI{decision: decision}

	p := New(ex, ind, ml, ai)
	result, err := p.Analyze(context.Background(), "ETH/USDT")
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
	if result.Decision.Action != claude.ActionHold {
		t.Errorf("expected HOLD, got %s", result.Decision.Action)
	}
}

// --- candle conversion tests ---

func TestExchangeToAnalysisCandles(t *testing.T) {
	candles := testCandles(5)
	result := exchangeToAnalysisCandles(candles)
	if len(result) != 5 {
		t.Fatalf("expected 5 candles, got %d", len(result))
	}
	if result[0].Open != candles[0].Open {
		t.Errorf("expected open %.2f, got %.2f", candles[0].Open, result[0].Open)
	}
	if result[0].Close != candles[0].Close {
		t.Errorf("expected close %.2f, got %.2f", candles[0].Close, result[0].Close)
	}
}

func TestExchangeToMLCandles(t *testing.T) {
	candles := testCandles(5)
	result := exchangeToMLCandles(candles)
	if len(result) != 5 {
		t.Fatalf("expected 5 candles, got %d", len(result))
	}
	if result[0].Volume != candles[0].Volume {
		t.Errorf("expected volume %.2f, got %.2f", candles[0].Volume, result[0].Volume)
	}
}

// --- ai input builder tests ---

func TestBuildAIInputFull(t *testing.T) {
	ticker := testTicker()
	ind := testIndicators()
	pred := testPrediction()
	sent := testSentiment()

	input := buildAIInput("BTC/USDT", ticker, nil, ind, pred, sent)

	if input.Market.Symbol != "BTC/USDT" {
		t.Errorf("expected symbol BTC/USDT, got %s", input.Market.Symbol)
	}
	if input.Market.Price != 42450 {
		t.Errorf("expected price 42450, got %.2f", input.Market.Price)
	}
	if input.Indicators == nil {
		t.Fatal("expected indicators")
	}
	if input.Indicators.RSI != 32.5 {
		t.Errorf("expected rsi 32.5, got %.1f", input.Indicators.RSI)
	}
	if input.Prediction == nil {
		t.Fatal("expected prediction")
	}
	if input.Prediction.Direction != "up" {
		t.Errorf("expected direction up, got %s", input.Prediction.Direction)
	}
	if input.Sentiment == nil {
		t.Fatal("expected sentiment")
	}
	if input.Sentiment.Label != "BULLISH" {
		t.Errorf("expected label BULLISH, got %s", input.Sentiment.Label)
	}
}

func TestBuildAIInputMinimal(t *testing.T) {
	ticker := testTicker()
	input := buildAIInput("ETH/USDT", ticker, nil, nil, nil, nil)

	if input.Market.Symbol != "ETH/USDT" {
		t.Errorf("expected symbol ETH/USDT, got %s", input.Market.Symbol)
	}
	if input.Indicators != nil {
		t.Error("expected nil indicators")
	}
	if input.Prediction != nil {
		t.Error("expected nil prediction")
	}
	if input.Sentiment != nil {
		t.Error("expected nil sentiment")
	}
}

// --- format tests ---

func TestFormatTelegramBuyMessage(t *testing.T) {
	result := &Result{
		Symbol:     "BTC/USDT",
		Ticker:     testTicker(),
		Indicators: testIndicators(),
		Prediction: testPrediction(),
		Sentiment:  testSentiment(),
		Decision:   testDecision(),
		Latency:    1400 * time.Millisecond,
	}

	msg := FormatTelegramMessage(result)

	checks := []string{
		"BTC/USDT",
		"BUY",
		"85%",
		"42450",
		"41800",
		"44200",
		"R/R",
		"RSI: 32.5",
		"MACD: bullish",
		"ML:",
		"3.20%",
		"Sentiment:",
		"confluence",
	}
	for _, check := range checks {
		if !strings.Contains(msg, check) {
			t.Errorf("telegram message should contain %q, got:\n%s", check, msg)
		}
	}
}

func TestFormatTelegramHoldMessage(t *testing.T) {
	result := &Result{
		Symbol: "ETH/USDT",
		Ticker: testTicker(),
		Decision: &claude.Decision{
			Action:     claude.ActionHold,
			Confidence: 40,
			Reasoning:  "Conflicting signals.",
		},
		Latency: 800 * time.Millisecond,
	}

	msg := FormatTelegramMessage(result)
	if !strings.Contains(msg, "HOLD") {
		t.Error("should contain HOLD")
	}
	if strings.Contains(msg, "Entry") {
		t.Error("hold message should not contain trade plan")
	}
}

func TestFormatTelegramNilDecision(t *testing.T) {
	result := &Result{Symbol: "BTC/USDT"}
	msg := FormatTelegramMessage(result)
	if !strings.Contains(msg, "failed") {
		t.Error("should show failure message")
	}
}

func TestFormatDiscordBuyFields(t *testing.T) {
	result := &Result{
		Symbol:     "BTC/USDT",
		Ticker:     testTicker(),
		Indicators: testIndicators(),
		Prediction: testPrediction(),
		Sentiment:  testSentiment(),
		Decision:   testDecision(),
		Latency:    1400 * time.Millisecond,
	}

	title, desc, fields, color := FormatDiscordFields(result)

	if !strings.Contains(title, "BTC/USDT") {
		t.Error("title should contain symbol")
	}
	if !strings.Contains(title, "BUY") {
		t.Error("title should contain action")
	}
	if desc == "" {
		t.Error("should have reasoning as description")
	}
	if color != 0x10B981 {
		t.Errorf("expected green color for buy, got %x", color)
	}
	if len(fields) < 4 {
		t.Errorf("expected at least 4 fields, got %d", len(fields))
	}

	// check field names exist
	fieldNames := make(map[string]bool)
	for _, f := range fields {
		fieldNames[f.Name] = true
	}
	expected := []string{"Decision", "Trade Plan", "Market", "Indicators", "ML Prediction", "Sentiment"}
	for _, name := range expected {
		if !fieldNames[name] {
			t.Errorf("missing discord field %q", name)
		}
	}
}

func TestFormatDiscordHoldFields(t *testing.T) {
	result := &Result{
		Symbol: "ETH/USDT",
		Ticker: testTicker(),
		Decision: &claude.Decision{
			Action:     claude.ActionHold,
			Confidence: 40,
			Reasoning:  "Insufficient data.",
		},
	}

	title, _, fields, color := FormatDiscordFields(result)
	if !strings.Contains(title, "HOLD") {
		t.Error("title should contain HOLD")
	}
	if color != 0x6B7280 {
		t.Errorf("expected gray color for hold, got %x", color)
	}

	for _, f := range fields {
		if f.Name == "Trade Plan" {
			t.Error("hold should not have trade plan field")
		}
	}
}

func TestFormatDiscordSellFields(t *testing.T) {
	decision := testDecision()
	decision.Action = claude.ActionSell
	result := &Result{
		Symbol:   "BTC/USDT",
		Ticker:   testTicker(),
		Decision: decision,
	}

	_, _, _, color := FormatDiscordFields(result)
	if color != 0xEF4444 {
		t.Errorf("expected red color for sell, got %x", color)
	}
}

func TestFormatDiscordNilDecision(t *testing.T) {
	result := &Result{Symbol: "BTC/USDT"}
	title, _, _, color := FormatDiscordFields(result)
	if !strings.Contains(title, "Failed") {
		t.Error("should show failure in title")
	}
	if color != 0xFF0000 {
		t.Errorf("expected red for failure, got %x", color)
	}
}

// --- helper tests ---

func TestDecisionEmoji(t *testing.T) {
	if decisionEmoji("BUY") != "🟢" {
		t.Error("buy should be green")
	}
	if decisionEmoji("SELL") != "🔴" {
		t.Error("sell should be red")
	}
	if decisionEmoji("HOLD") != "⏸️" {
		t.Error("hold should be pause")
	}
}

func TestDecisionColor(t *testing.T) {
	if decisionColor("BUY") != 0x10B981 {
		t.Error("buy should be green")
	}
	if decisionColor("SELL") != 0xEF4444 {
		t.Error("sell should be red")
	}
	if decisionColor("HOLD") != 0x6B7280 {
		t.Error("hold should be gray")
	}
}

func TestFormatNum(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{42450.00, "42450.00"},
		{5.1234, "5.1234"},
		{0.000123, "0.000123"},
	}
	for _, tt := range tests {
		got := formatNum(tt.input)
		if got != tt.want {
			t.Errorf("formatNum(%.6f) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestFormatChange(t *testing.T) {
	if formatChange(2.5) != "+2.50%" {
		t.Errorf("expected +2.50%%, got %s", formatChange(2.5))
	}
	if formatChange(-1.3) != "-1.30%" {
		t.Errorf("expected -1.30%%, got %s", formatChange(-1.3))
	}
	if formatChange(0) != "+0.00%" {
		t.Errorf("expected +0.00%%, got %s", formatChange(0))
	}
}

func TestFormatLatency(t *testing.T) {
	if formatLatency(500*time.Millisecond) != "500ms" {
		t.Errorf("expected 500ms, got %s", formatLatency(500*time.Millisecond))
	}
	if formatLatency(1400*time.Millisecond) != "1.4s" {
		t.Errorf("expected 1.4s, got %s", formatLatency(1400*time.Millisecond))
	}
}

func TestFormatIndicatorsSummary(t *testing.T) {
	ind := testIndicators()
	summary := formatIndicatorsSummary(ind)
	if !strings.Contains(summary, "RSI: 32.5") {
		t.Error("should contain rsi")
	}
	if !strings.Contains(summary, "MACD: bullish") {
		t.Error("should contain macd")
	}
	if !strings.Contains(summary, "BB: lower_band") {
		t.Error("should contain bollinger")
	}
}

func TestFormatIndicatorsSummaryWithSpike(t *testing.T) {
	ind := testIndicators()
	ind.Volume.IsSpike = true
	summary := formatIndicatorsSummary(ind)
	if !strings.Contains(summary, "Vol: SPIKE") {
		t.Error("should contain volume spike")
	}
}

func TestFormatIndicatorsSummaryEmpty(t *testing.T) {
	ind := &analysis.AnalysisResult{}
	summary := formatIndicatorsSummary(ind)
	if summary != "" {
		t.Errorf("expected empty string for no indicators, got %q", summary)
	}
}
