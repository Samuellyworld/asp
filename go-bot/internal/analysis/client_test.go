package analysis

import (
	"testing"
)

func TestNewClientInvalidAddress(t *testing.T) {
	// connection to non-existent server should fail
	_, err := NewClient("localhost:99999")
	if err == nil {
		t.Error("expected error connecting to invalid address")
	}
}

func TestToProtoCandles(t *testing.T) {
	candles := []Candle{
		{Open: 100, High: 105, Low: 95, Close: 102, Volume: 1000, Timestamp: 1},
		{Open: 102, High: 108, Low: 100, Close: 106, Volume: 1200, Timestamp: 2},
	}

	proto := toProtoCandles(candles)
	if len(proto) != 2 {
		t.Fatalf("expected 2 proto candles, got %d", len(proto))
	}
	if proto[0].Open != 100 {
		t.Errorf("expected open 100, got %f", proto[0].Open)
	}
	if proto[0].Close != 102 {
		t.Errorf("expected close 102, got %f", proto[0].Close)
	}
	if proto[1].Volume != 1200 {
		t.Errorf("expected volume 1200, got %f", proto[1].Volume)
	}
	if proto[1].Timestamp != 2 {
		t.Errorf("expected timestamp 2, got %d", proto[1].Timestamp)
	}
}

func TestToProtoCandlesEmpty(t *testing.T) {
	proto := toProtoCandles(nil)
	if len(proto) != 0 {
		t.Fatalf("expected 0 proto candles, got %d", len(proto))
	}
}

func TestDefaultAnalyzeOptions(t *testing.T) {
	opts := DefaultAnalyzeOptions()
	if opts.RSIPeriod != 14 {
		t.Errorf("expected rsi period 14, got %d", opts.RSIPeriod)
	}
	if opts.MACDFast != 12 {
		t.Errorf("expected macd fast 12, got %d", opts.MACDFast)
	}
	if opts.MACDSlow != 26 {
		t.Errorf("expected macd slow 26, got %d", opts.MACDSlow)
	}
	if opts.MACDSignal != 9 {
		t.Errorf("expected macd signal 9, got %d", opts.MACDSignal)
	}
	if opts.BBPeriod != 20 {
		t.Errorf("expected bb period 20, got %d", opts.BBPeriod)
	}
	if opts.BBStdDev != 2.0 {
		t.Errorf("expected bb std dev 2.0, got %f", opts.BBStdDev)
	}
	if opts.EMAPeriod != 21 {
		t.Errorf("expected ema period 21, got %d", opts.EMAPeriod)
	}
	if opts.VolumeLookback != 20 {
		t.Errorf("expected volume lookback 20, got %d", opts.VolumeLookback)
	}
	if opts.VolumeThreshold != 2.0 {
		t.Errorf("expected volume threshold 2.0, got %f", opts.VolumeThreshold)
	}
}

func TestClientClose(t *testing.T) {
	// closing a nil conn client should not panic
	c := &Client{}
	err := c.Close()
	if err != nil {
		t.Errorf("expected no error closing nil client, got %v", err)
	}
}

func TestCandleStruct(t *testing.T) {
	c := Candle{
		Open:      100.5,
		High:      105.0,
		Low:       98.0,
		Close:     103.0,
		Volume:    50000,
		Timestamp: 1700000000,
	}
	if c.Open != 100.5 {
		t.Errorf("unexpected open: %f", c.Open)
	}
	if c.Timestamp != 1700000000 {
		t.Errorf("unexpected timestamp: %d", c.Timestamp)
	}
}
