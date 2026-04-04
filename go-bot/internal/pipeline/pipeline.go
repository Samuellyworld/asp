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

// holds the full analysis output from all services
type Result struct {
	Symbol     string
	Ticker     *exchange.Ticker
	Indicators *analysis.AnalysisResult
	Prediction *mlclient.PricePredictionResponse
	Sentiment  *mlclient.SentimentResponse
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
	timeframe  string
}

// creates a new pipeline with all service clients
func New(ex ExchangeProvider, ind IndicatorProvider, ml MLProvider, ai AIProvider) *Pipeline {
	return &Pipeline{
		exchange:   ex,
		indicators: ind,
		ml:         ml,
		ai:         ai,
		timeframe:  "4h",
	}
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
		indErr     error
		predErr    error
		sentErr    error
		wg         sync.WaitGroup
	)

	wg.Add(3)

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

	// step 4: feed everything to claude
	aiInput := buildAIInput(symbol, ticker, indicators, prediction, sentiment)
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

// assembles the ai input from all available analysis data
func buildAIInput(
	symbol string,
	ticker *exchange.Ticker,
	indicators *analysis.AnalysisResult,
	prediction *mlclient.PricePredictionResponse,
	sentiment *mlclient.SentimentResponse,
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

	return input
}
