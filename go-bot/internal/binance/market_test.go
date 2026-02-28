// tests for binance market data parsing and symbol conversion
package binance

import (
	"encoding/json"
	"testing"
)

func TestToBinanceSymbol(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"BTC/USDT", "BTCUSDT"},
		{"ETH/BTC", "ETHBTC"},
		{"DOGE/USDT", "DOGEUSDT"},
		{"SOL/USDC", "SOLUSDC"},
		{"BTCUSDT", "BTCUSDT"},
	}

	for _, tt := range tests {
		got := toBinanceSymbol(tt.input)
		if got != tt.want {
			t.Errorf("toBinanceSymbol(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseBookEntry_Valid(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`"42000.50"`),
		json.RawMessage(`"1.250"`),
	}

	entry, err := parseBookEntry(raw)
	if err != nil {
		t.Fatalf("parseBookEntry() error: %v", err)
	}
	if entry.Price != 42000.50 {
		t.Errorf("Price = %v, want %v", entry.Price, 42000.50)
	}
	if entry.Quantity != 1.25 {
		t.Errorf("Quantity = %v, want %v", entry.Quantity, 1.25)
	}
}

func TestParseBookEntry_TooShort(t *testing.T) {
	raw := []json.RawMessage{json.RawMessage(`"42000"`)}
	_, err := parseBookEntry(raw)
	if err == nil {
		t.Fatal("expected error for too-short entry")
	}
}

func TestParseBookEntry_InvalidPrice(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`"not-a-number"`),
		json.RawMessage(`"1.0"`),
	}
	_, err := parseBookEntry(raw)
	if err == nil {
		t.Fatal("expected error for invalid price")
	}
}

func TestParseBookEntry_InvalidQuantity(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`"42000"`),
		json.RawMessage(`"not-a-number"`),
	}
	_, err := parseBookEntry(raw)
	if err == nil {
		t.Fatal("expected error for invalid quantity")
	}
}

func TestParseCandle_Valid(t *testing.T) {
	// typical binance kline: [openTime, open, high, low, close, volume, closeTime, ...]
	row := []json.RawMessage{
		json.RawMessage(`1704067200000`),  // open time
		json.RawMessage(`"42000.00"`),     // open
		json.RawMessage(`"42500.00"`),     // high
		json.RawMessage(`"41800.00"`),     // low
		json.RawMessage(`"42300.00"`),     // close
		json.RawMessage(`"1500.50"`),      // volume
		json.RawMessage(`1704070799999`),  // close time
	}

	candle, err := parseCandle(row)
	if err != nil {
		t.Fatalf("parseCandle() error: %v", err)
	}
	if candle.Open != 42000.00 {
		t.Errorf("Open = %v, want %v", candle.Open, 42000.00)
	}
	if candle.High != 42500.00 {
		t.Errorf("High = %v, want %v", candle.High, 42500.00)
	}
	if candle.Low != 41800.00 {
		t.Errorf("Low = %v, want %v", candle.Low, 41800.00)
	}
	if candle.Close != 42300.00 {
		t.Errorf("Close = %v, want %v", candle.Close, 42300.00)
	}
	if candle.Volume != 1500.50 {
		t.Errorf("Volume = %v, want %v", candle.Volume, 1500.50)
	}
	if candle.OpenTime.IsZero() {
		t.Error("OpenTime should not be zero")
	}
	if candle.CloseTime.IsZero() {
		t.Error("CloseTime should not be zero")
	}
	if !candle.CloseTime.After(candle.OpenTime) {
		t.Error("CloseTime should be after OpenTime")
	}
}

func TestParseCandle_TooShort(t *testing.T) {
	row := []json.RawMessage{
		json.RawMessage(`1704067200000`),
		json.RawMessage(`"42000.00"`),
	}
	_, err := parseCandle(row)
	if err == nil {
		t.Fatal("expected error for too-short candle row")
	}
}

func TestParseCandle_InvalidOpenTime(t *testing.T) {
	row := []json.RawMessage{
		json.RawMessage(`"not-a-number"`), // bad open time
		json.RawMessage(`"42000.00"`),
		json.RawMessage(`"42500.00"`),
		json.RawMessage(`"41800.00"`),
		json.RawMessage(`"42300.00"`),
		json.RawMessage(`"1500.50"`),
		json.RawMessage(`1704070799999`),
	}
	_, err := parseCandle(row)
	if err == nil {
		t.Fatal("expected error for invalid open time")
	}
}

func TestSignMarket_NonEmpty(t *testing.T) {
	// verify sign produces a non-empty hex string
	sig := sign("timestamp=1234567890", "test-secret")
	if sig == "" {
		t.Fatal("sign() returned empty string")
	}
	if len(sig) != 64 { // sha256 hex = 64 chars
		t.Errorf("sign() length = %d, want 64", len(sig))
	}
}

func TestSignMarket_DifferentSecrets(t *testing.T) {
	query := "timestamp=1234567890"
	sig1 := sign(query, "secret-1")
	sig2 := sign(query, "secret-2")
	if sig1 == sig2 {
		t.Error("different secrets should produce different signatures")
	}
}

func TestSignMarket_DifferentQueries(t *testing.T) {
	secret := "my-secret"
	sig1 := sign("timestamp=1111111111", secret)
	sig2 := sign("timestamp=2222222222", secret)
	if sig1 == sig2 {
		t.Error("different queries should produce different signatures")
	}
}
