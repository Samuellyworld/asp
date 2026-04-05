package datasources

import "testing"

func TestExtractCoinCode(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"BTCUSDT", "BTC"},
		{"BTC/USDT", "BTC"},
		{"ETHUSDT", "ETH"},
		{"SOLUSDT", "SOL"},
		{"DOGEUSDT", "DOGE"},
		{"BTCBUSD", "BTC"},
		{"ETHBTC", "ETH"},
		{"BTC", "BTC"},
		{"btcusdt", "BTC"},
	}
	for _, tc := range cases {
		got := extractCoinCode(tc.input)
		if got != tc.expected {
			t.Errorf("extractCoinCode(%s) = %s, want %s", tc.input, got, tc.expected)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello world", 5, "hello..."},
		{"hello", 10, "hello"},
		{"", 5, ""},
		{"abc", 3, "abc"},
	}
	for _, tc := range cases {
		got := truncate(tc.input, tc.maxLen)
		if got != tc.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.expected)
		}
	}
}

func TestJoinStrings(t *testing.T) {
	cases := []struct {
		input    []string
		sep      string
		expected string
	}{
		{[]string{"a", "b", "c"}, ", ", "a, b, c"},
		{[]string{"hello"}, ".", "hello"},
		{nil, ".", ""},
	}
	for _, tc := range cases {
		got := joinStrings(tc.input, tc.sep)
		if got != tc.expected {
			t.Errorf("joinStrings(%v, %q) = %q, want %q", tc.input, tc.sep, got, tc.expected)
		}
	}
}
