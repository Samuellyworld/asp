// package analysis provides a grpc client for the rust technical indicators engine
package analysis

import (
	"context"
	"fmt"
	"time"

	pb "github.com/trading-bot/go-bot/internal/analysis/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the grpc connection to the rust engine
type Client struct {
	conn   *grpc.ClientConn
	client pb.TechnicalIndicatorsClient
}

// Candle represents ohlcv price data
type Candle struct {
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	Timestamp int64
}

// AnalysisResult holds the combined analysis from all indicators
type AnalysisResult struct {
	RSI           *RSIResult
	MACD          *MACDResult
	Bollinger     *BollingerResult
	EMA           *EMAResult
	Volume        *VolumeResult
	OverallSignal string
	BullishCount  int32
	BearishCount  int32
}

// RSIResult holds rsi calculation output
type RSIResult struct {
	Value  float64
	Signal string
	Series []float64
}

// MACDResult holds macd calculation output
type MACDResult struct {
	MACDLine        float64
	SignalLine      float64
	Histogram       float64
	Signal          string
	Crossover       bool
	MACDSeries      []float64
	SignalSeries    []float64
	HistogramSeries []float64
}

// BollingerResult holds bollinger bands output
type BollingerResult struct {
	Upper        float64
	Middle       float64
	Lower        float64
	Bandwidth    float64
	PercentB     float64
	Signal       string
	UpperSeries  []float64
	MiddleSeries []float64
	LowerSeries  []float64
}

// EMAResult holds ema output
type EMAResult struct {
	Value  float64
	Trend  string
	Series []float64
}

// VolumeResult holds volume spike detection output
type VolumeResult struct {
	IsSpike       bool
	CurrentVolume float64
	AverageVolume float64
	Ratio         float64
	Signal        string
}

// NewClient creates a new analysis client connected to the rust engine
func NewClient(addr string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rust engine at %s: %w", addr, err)
	}

	return &Client{
		conn:   conn,
		client: pb.NewTechnicalIndicatorsClient(conn),
	}, nil
}

// Close shuts down the grpc connection
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// converts local candles to proto candles
func toProtoCandles(candles []Candle) []*pb.Candle {
	result := make([]*pb.Candle, len(candles))
	for i, c := range candles {
		result[i] = &pb.Candle{
			Open:      c.Open,
			High:      c.High,
			Low:       c.Low,
			Close:     c.Close,
			Volume:    c.Volume,
			Timestamp: c.Timestamp,
		}
	}
	return result
}

// CalculateRSI computes the relative strength index
func (c *Client) CalculateRSI(ctx context.Context, candles []Candle, period int32) (*RSIResult, error) {
	resp, err := c.client.CalculateRSI(ctx, &pb.RSIRequest{
		Candles: toProtoCandles(candles),
		Period:  period,
	})
	if err != nil {
		return nil, fmt.Errorf("rsi calculation failed: %w", err)
	}
	return &RSIResult{
		Value:  resp.Value,
		Signal: resp.Signal,
		Series: resp.Series,
	}, nil
}

// CalculateMACD computes the macd indicator
func (c *Client) CalculateMACD(ctx context.Context, candles []Candle, fast, slow, signal int32) (*MACDResult, error) {
	resp, err := c.client.CalculateMACD(ctx, &pb.MACDRequest{
		Candles:      toProtoCandles(candles),
		FastPeriod:   fast,
		SlowPeriod:   slow,
		SignalPeriod: signal,
	})
	if err != nil {
		return nil, fmt.Errorf("macd calculation failed: %w", err)
	}
	return &MACDResult{
		MACDLine:        resp.MacdLine,
		SignalLine:      resp.SignalLine,
		Histogram:       resp.Histogram,
		Signal:          resp.Signal,
		Crossover:       resp.Crossover,
		MACDSeries:      resp.MacdSeries,
		SignalSeries:    resp.SignalSeries,
		HistogramSeries: resp.HistogramSeries,
	}, nil
}

// CalculateBollinger computes bollinger bands
func (c *Client) CalculateBollinger(ctx context.Context, candles []Candle, period int32, stdDev float64) (*BollingerResult, error) {
	resp, err := c.client.CalculateBollingerBands(ctx, &pb.BollingerRequest{
		Candles: toProtoCandles(candles),
		Period:  period,
		StdDev:  stdDev,
	})
	if err != nil {
		return nil, fmt.Errorf("bollinger calculation failed: %w", err)
	}
	return &BollingerResult{
		Upper:        resp.Upper,
		Middle:       resp.Middle,
		Lower:        resp.Lower,
		Bandwidth:    resp.Bandwidth,
		PercentB:     resp.PercentB,
		Signal:       resp.Signal,
		UpperSeries:  resp.UpperSeries,
		MiddleSeries: resp.MiddleSeries,
		LowerSeries:  resp.LowerSeries,
	}, nil
}

// CalculateEMA computes the exponential moving average
func (c *Client) CalculateEMA(ctx context.Context, candles []Candle, period int32) (*EMAResult, error) {
	resp, err := c.client.CalculateEMA(ctx, &pb.EMARequest{
		Candles: toProtoCandles(candles),
		Period:  period,
	})
	if err != nil {
		return nil, fmt.Errorf("ema calculation failed: %w", err)
	}
	return &EMAResult{
		Value:  resp.Value,
		Trend:  resp.Trend,
		Series: resp.Series,
	}, nil
}

// DetectVolumeSpike checks for volume anomalies
func (c *Client) DetectVolumeSpike(ctx context.Context, candles []Candle, lookback int32, threshold float64) (*VolumeResult, error) {
	resp, err := c.client.DetectVolumeSpike(ctx, &pb.VolumeRequest{
		Candles:   toProtoCandles(candles),
		Lookback:  lookback,
		Threshold: threshold,
	})
	if err != nil {
		return nil, fmt.Errorf("volume analysis failed: %w", err)
	}
	return &VolumeResult{
		IsSpike:       resp.IsSpike,
		CurrentVolume: resp.CurrentVolume,
		AverageVolume: resp.AverageVolume,
		Ratio:         resp.Ratio,
		Signal:        resp.Signal,
	}, nil
}

// AnalyzeAll runs all indicators and returns a combined analysis
func (c *Client) AnalyzeAll(ctx context.Context, candles []Candle, opts *AnalyzeOptions) (*AnalysisResult, error) {
	if opts == nil {
		opts = DefaultAnalyzeOptions()
	}

	resp, err := c.client.AnalyzeAll(ctx, &pb.AnalyzeAllRequest{
		Candles:          toProtoCandles(candles),
		RsiPeriod:        opts.RSIPeriod,
		MacdFast:         opts.MACDFast,
		MacdSlow:         opts.MACDSlow,
		MacdSignal:       opts.MACDSignal,
		BbPeriod:         opts.BBPeriod,
		BbStdDev:         opts.BBStdDev,
		EmaPeriod:        opts.EMAPeriod,
		VolumeLookback:   opts.VolumeLookback,
		VolumeThreshold:  opts.VolumeThreshold,
	})
	if err != nil {
		return nil, fmt.Errorf("full analysis failed: %w", err)
	}

	result := &AnalysisResult{
		OverallSignal: resp.OverallSignal,
		BullishCount:  resp.BullishCount,
		BearishCount:  resp.BearishCount,
	}

	if resp.Rsi != nil {
		result.RSI = &RSIResult{
			Value:  resp.Rsi.Value,
			Signal: resp.Rsi.Signal,
			Series: resp.Rsi.Series,
		}
	}
	if resp.Macd != nil {
		result.MACD = &MACDResult{
			MACDLine:        resp.Macd.MacdLine,
			SignalLine:      resp.Macd.SignalLine,
			Histogram:       resp.Macd.Histogram,
			Signal:          resp.Macd.Signal,
			Crossover:       resp.Macd.Crossover,
			MACDSeries:      resp.Macd.MacdSeries,
			SignalSeries:    resp.Macd.SignalSeries,
			HistogramSeries: resp.Macd.HistogramSeries,
		}
	}
	if resp.Bollinger != nil {
		result.Bollinger = &BollingerResult{
			Upper:        resp.Bollinger.Upper,
			Middle:       resp.Bollinger.Middle,
			Lower:        resp.Bollinger.Lower,
			Bandwidth:    resp.Bollinger.Bandwidth,
			PercentB:     resp.Bollinger.PercentB,
			Signal:       resp.Bollinger.Signal,
			UpperSeries:  resp.Bollinger.UpperSeries,
			MiddleSeries: resp.Bollinger.MiddleSeries,
			LowerSeries:  resp.Bollinger.LowerSeries,
		}
	}
	if resp.Ema != nil {
		result.EMA = &EMAResult{
			Value:  resp.Ema.Value,
			Trend:  resp.Ema.Trend,
			Series: resp.Ema.Series,
		}
	}
	if resp.Volume != nil {
		result.Volume = &VolumeResult{
			IsSpike:       resp.Volume.IsSpike,
			CurrentVolume: resp.Volume.CurrentVolume,
			AverageVolume: resp.Volume.AverageVolume,
			Ratio:         resp.Volume.Ratio,
			Signal:        resp.Volume.Signal,
		}
	}

	return result, nil
}

// AnalyzeOptions holds configuration for the full analysis
type AnalyzeOptions struct {
	RSIPeriod       int32
	MACDFast        int32
	MACDSlow        int32
	MACDSignal      int32
	BBPeriod        int32
	BBStdDev        float64
	EMAPeriod       int32
	VolumeLookback  int32
	VolumeThreshold float64
}

// DefaultAnalyzeOptions returns standard indicator parameters
func DefaultAnalyzeOptions() *AnalyzeOptions {
	return &AnalyzeOptions{
		RSIPeriod:       14,
		MACDFast:        12,
		MACDSlow:        26,
		MACDSignal:      9,
		BBPeriod:        20,
		BBStdDev:        2.0,
		EMAPeriod:       21,
		VolumeLookback:  20,
		VolumeThreshold: 2.0,
	}
}
