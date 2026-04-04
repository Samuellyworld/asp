// package pipeline orchestrates the full analysis flow:
// binance market data - rust indicators - python ml - claude decision
package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/analysis"
	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/exchange"
	mlclient "github.com/trading-bot/go-bot/internal/ml-client"
	"github.com/trading-bot/go-bot/internal/regime"
)

// provides market data
type ExchangeProvider interface {
	GetPrice(ctx context.Context, symbol string) (*exchange.Ticker, error)
	GetCandles(ctx context.Context, symbol string, interval string, limit int) ([]exchange.Candle, error)
}

// provides technical indicators from rust
type IndicatorProvider interface {
	AnalyzeAll(ctx context.Context, candles []analysis.Candle, opts *analysis.AnalyzeOptions) (*analysis.AnalysisResult, error)
}

// provides ml predictions from python
type MLProvider interface {
	PredictPrice(ctx context.Context, req *mlclient.PricePredictionRequest) (*mlclient.PricePredictionResponse, error)
	AnalyzeSentiment(ctx context.Context, text string) (*mlclient.SentimentResponse, error)
	IsAvailable(ctx context.Context) bool
}

// provides ai decisions from claude
type AIProvider interface {
	Analyze(ctx context.Context, input *claude.AnalysisInput) (*claude.Decision, error)
}

// provides alternative data (order flow, on-chain, sentiment, funding)
type AltDataProvider interface {
	Fetch(ctx context.Context, symbol string) *claude.AltData
}

// holds the full analysis output from all services
type Result struct {
	Symbol     string
	Ticker     *exchange.Ticker
	Indicators *analysis.AnalysisResult
	Prediction *mlclient.PricePredictionResponse
	Sentiment  *mlclient.SentimentResponse
	AltData    *claude.AltData
	Decision   *claude.Decision
	Latency    time.Duration
	Errors     []string
}

// orchestrates the analysis pipeline
type Pipeline struct {
	exchange   ExchangeProvider
	indicators IndicatorProvider
	ml         MLProvider
	ai         AIProvider
	altData    AltDataProvider
	timeframe  string
	timeframes []string // for multi-timeframe analysis
}

// creates a new pipeline with all service clients
func New(ex ExchangeProvider, ind IndicatorProvider, ml MLProvider, ai AIProvider) *Pipeline {
	return &Pipeline{
		exchange:   ex,
		indicators: ind,
		ml:         ml,
		ai:         ai,
		timeframe:  "4h",
		timeframes: []string{"4h"},
	}
}

// SetAltData configures the alternative data provider.
func (p *Pipeline) SetAltData(provider AltDataProvider) {
	p.altData = provider
}

// SetTimeframes configures multi-timeframe analysis.
// The first timeframe is the primary decision timeframe.
func (p *Pipeline) SetTimeframes(timeframes []string) {
	if len(timeframes) == 0 {
		return
	}
	p.timeframes = timeframes
	p.timeframe = timeframes[0]
}

// runs the full analysis pipeline for a symbol
func (p *Pipeline) Analyze(ctx context.Context, symbol string) (*Result, error) {
	start := time.Now()
	result := &Result{Symbol: symbol}

	// step 1: fetch market data from binance
	ticker, candles, err := p.fetchMarketData(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("market data fetch failed: %w", err)
	}
	result.Ticker = ticker

	// step 2+3: run rust indicators and python ml in parallel
	analysisCandles := exchangeToAnalysisCandles(candles)
	mlCandles := exchangeToMLCandles(candles)

	var (
		indicators *analysis.AnalysisResult
		prediction *mlclient.PricePredictionResponse
		sentiment  *mlclient.SentimentResponse
		altData    *claude.AltData
		htfCtx     []claude.HTFSnapshot
		indErr     error
		predErr    error
		sentErr    error
		wg         sync.WaitGroup
	)

	wg.Add(5)

	// rust indicators
	go func() {
		defer wg.Done()
		indicators, indErr = p.indicators.AnalyzeAll(ctx, analysisCandles, nil)
	}()

	// ml price prediction
	go func() {
		defer wg.Done()
		if p.ml != nil && p.ml.IsAvailable(ctx) {
			prediction, predErr = p.ml.PredictPrice(ctx, &mlclient.PricePredictionRequest{
				Symbol:    symbol,
				Candles:   mlCandles,
				Timeframe: p.timeframe,
			})
		}
	}()

	// ml sentiment
	go func() {
		defer wg.Done()
		if p.ml != nil && p.ml.IsAvailable(ctx) {
			sentimentText := fmt.Sprintf("%s price %.2f 24h change %.2f%%", symbol, ticker.Price, ticker.ChangePct)
			sentiment, sentErr = p.ml.AnalyzeSentiment(ctx, sentimentText)
		}
	}()

	// alternative data (order flow, on-chain, sentiment, funding)
	go func() {
		defer wg.Done()
		if p.altData != nil {
			altData = p.altData.Fetch(ctx, symbol)
		}
	}()

	// higher-timeframe context (skip primary timeframe, fetch others)
	go func() {
		defer wg.Done()
		if len(p.timeframes) > 1 {
			htfCtx = p.fetchHTFContext(ctx, symbol)
		}
	}()

	wg.Wait()

	// collect results, log errors but continue
	if indErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("indicators: %v", indErr))
	} else {
		result.Indicators = indicators
	}

	if predErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("prediction: %v", predErr))
	} else {
		result.Prediction = prediction
	}

	if sentErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("sentiment: %v", sentErr))
	} else {
		result.Sentiment = sentiment
	}

	result.AltData = altData

	// step 4: feed everything to claude
	aiInput := buildAIInput(symbol, ticker, candles, indicators, prediction, sentiment, altData)
	aiInput.HTFContext = htfCtx
	decision, err := p.ai.Analyze(ctx, aiInput)
	if err != nil {
		return nil, fmt.Errorf("ai analysis failed: %w", err)
	}
	result.Decision = decision
	result.Latency = time.Since(start)

	return result, nil
}

// fetches ticker and candles from the exchange
func (p *Pipeline) fetchMarketData(ctx context.Context, symbol string) (*exchange.Ticker, []exchange.Candle, error) {
	ticker, err := p.exchange.GetPrice(ctx, symbol)
	if err != nil {
		return nil, nil, fmt.Errorf("price fetch failed: %w", err)
	}

	candles, err := p.exchange.GetCandles(ctx, symbol, p.timeframe, 100)
	if err != nil {
		return nil, nil, fmt.Errorf("candle fetch failed: %w", err)
	}

	return ticker, candles, nil
}

// fetchHTFContext fetches candles for higher timeframes and runs indicators
// to provide multi-timeframe confirmation signals
func (p *Pipeline) fetchHTFContext(ctx context.Context, symbol string) []claude.HTFSnapshot {
	var snapshots []claude.HTFSnapshot
	for _, tf := range p.timeframes[1:] {
		candles, err := p.exchange.GetCandles(ctx, symbol, tf, 50)
		if err != nil || len(candles) < 28 {
			continue
		}
		ac := exchangeToAnalysisCandles(candles)
		ind, err := p.indicators.AnalyzeAll(ctx, ac, nil)
		if err != nil {
			continue
		}
		snap := claude.HTFSnapshot{Timeframe: tf}
		if ind.RSI != nil {
			snap.RSI = ind.RSI.Value
		}
		if ind.MACD != nil {
			snap.MACDHist = ind.MACD.Histogram
		}
		if ind.Bollinger != nil && ind.Bollinger.Upper > ind.Bollinger.Lower {
			lastClose := candles[len(candles)-1].Close
			snap.BBPosition = (lastClose - ind.Bollinger.Lower) / (ind.Bollinger.Upper - ind.Bollinger.Lower)
		}
		if ind.EMA != nil && len(candles) >= 2 {
			prevClose := candles[len(candles)-2].Close
			currClose := candles[len(candles)-1].Close
			snap.EMASlope = (currClose - prevClose) / prevClose * 100
		}
		// determine trend direction from indicators
		if ind.MACD != nil && ind.EMA != nil {
			if ind.MACD.Histogram > 0 && snap.EMASlope > 0 {
				snap.TrendDir = "up"
			} else if ind.MACD.Histogram < 0 && snap.EMASlope < 0 {
				snap.TrendDir = "down"
			} else {
				snap.TrendDir = "neutral"
			}
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots
}

// converts exchange candles to analysis candles for the rust engine
func exchangeToAnalysisCandles(candles []exchange.Candle) []analysis.Candle {
	result := make([]analysis.Candle, len(candles))
	for i, c := range candles {
		result[i] = analysis.Candle{
			Open:      c.Open,
			High:      c.High,
			Low:       c.Low,
			Close:     c.Close,
			Volume:    c.Volume,
			Timestamp: c.OpenTime.Unix(),
		}
	}
	return result
}

// converts exchange candles to ml candles for the python service
func exchangeToMLCandles(candles []exchange.Candle) []mlclient.Candle {
	result := make([]mlclient.Candle, len(candles))
	for i, c := range candles {
		result[i] = mlclient.Candle{
			Open:      c.Open,
			High:      c.High,
			Low:       c.Low,
			Close:     c.Close,
			Volume:    c.Volume,
			Timestamp: c.OpenTime.Unix(),
		}
	}
	return result
}

// converts exchange candles to regime candles for market classification
func exchangeToRegimeCandles(candles []exchange.Candle) []regime.Candle {
	result := make([]regime.Candle, len(candles))
	for i, c := range candles {
		result[i] = regime.Candle{
			Open:   c.Open,
			High:   c.High,
			Low:    c.Low,
			Close:  c.Close,
			Volume: c.Volume,
		}
	}
	return result
}

// assembles the ai input from all available analysis data
func buildAIInput(
	symbol string,
	ticker *exchange.Ticker,
	candles []exchange.Candle,
	indicators *analysis.AnalysisResult,
	prediction *mlclient.PricePredictionResponse,
	sentiment *mlclient.SentimentResponse,
	altData *claude.AltData,
) *claude.AnalysisInput {
	input := &claude.AnalysisInput{
		Market: claude.MarketData{
			Symbol:    symbol,
			Price:     ticker.Price,
			Volume24h: ticker.QuoteVolume,
			Change24h: ticker.ChangePct,
		},
	}

	if indicators != nil {
		ind := &claude.Indicators{}
		if indicators.RSI != nil {
			ind.RSI = indicators.RSI.Value
		}
		if indicators.MACD != nil {
			ind.MACDValue = indicators.MACD.MACDLine
			ind.MACDSignal = indicators.MACD.SignalLine
			ind.MACDHist = indicators.MACD.Histogram
		}
		if indicators.Bollinger != nil {
			ind.BBUpper = indicators.Bollinger.Upper
			ind.BBMiddle = indicators.Bollinger.Middle
			ind.BBLower = indicators.Bollinger.Lower
		}
		if indicators.EMA != nil {
			ind.EMA12 = indicators.EMA.Value
			ind.EMA26 = indicators.EMA.Value * 0.98 // approximation when only one ema available
		}
		if indicators.Volume != nil {
			ind.VolumeSpike = indicators.Volume.IsSpike
		}
		input.Indicators = ind
	}

	if prediction != nil {
		input.Prediction = &claude.MLPrediction{
			Direction:  prediction.Direction,
			Magnitude:  prediction.Magnitude,
			Confidence: prediction.Confidence,
			Timeframe:  prediction.Timeframe,
		}
	}

	if sentiment != nil {
		input.Sentiment = &claude.Sentiment{
			Score:      sentiment.Score,
			Label:      sentiment.Label,
			Confidence: sentiment.Confidence,
		}
	}

	input.Costs = claude.DefaultTradingCosts()

	// run regime detection on candle data
	if len(candles) >= 28 {
		regimeCandles := exchangeToRegimeCandles(candles)
		det := regime.Detect(regimeCandles, ticker.Price)
		input.Regime = &claude.RegimeInfo{
			Regime:      string(det.Regime),
			ADX:         det.ADX,
			ATRPercent:  det.ATRPercent,
			TrendDir:    det.TrendDir,
			Confidence:  det.Confidence,
			Description: det.Description,
		}
	}

	input.AltData = altData

	return input
}
