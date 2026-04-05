package datasources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRSSGetNews(t *testing.T) {
	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Bitcoin surges past $60,000</title>
      <description>Bitcoin rallied to new highs as BTC demand increases</description>
      <link>https://example.com/btc-article</link>
      <pubDate>Mon, 15 Jan 2025 12:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Ethereum 2.0 update released</title>
      <description>New ETH staking features launched</description>
      <link>https://example.com/eth-article</link>
      <pubDate>Mon, 15 Jan 2025 11:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Market overview for today</title>
      <description>Stocks and bonds performance summary</description>
      <link>https://example.com/market-overview</link>
      <pubDate>Mon, 15 Jan 2025 10:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(rssXML))
	}))
	defer srv.Close()

	rp := NewRSSProviderWithFeeds([]string{srv.URL})

	items, err := rp.GetNews(context.Background(), "BTCUSDT", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// should find BTC-related articles only
	if len(items) != 1 {
		t.Fatalf("expected 1 BTC article, got %d", len(items))
	}
	if items[0].Title != "Bitcoin surges past $60,000" {
		t.Errorf("unexpected title: %s", items[0].Title)
	}
	if items[0].URL != "https://example.com/btc-article" {
		t.Errorf("unexpected URL: %s", items[0].URL)
	}
	if len(items[0].Symbols) == 0 || items[0].Symbols[0] != "BTC" {
		t.Errorf("expected symbol BTC, got %v", items[0].Symbols)
	}
}

func TestRSSGetNewsETH(t *testing.T) {
	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Ethereum sees massive adoption</title>
      <description>ETH network activity hits all-time high</description>
      <link>https://example.com/eth-1</link>
      <pubDate>2025-01-15T12:00:00Z</pubDate>
    </item>
    <item>
      <title>US Fed rate decision</title>
      <description>Federal reserve keeps rates unchanged</description>
      <link>https://example.com/fed</link>
      <pubDate>2025-01-15T11:00:00Z</pubDate>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssXML))
	}))
	defer srv.Close()

	rp := NewRSSProviderWithFeeds([]string{srv.URL})
	items, err := rp.GetNews(context.Background(), "ETHUSDT", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 ETH article, got %d", len(items))
	}
}

func TestRSSServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	rp := NewRSSProviderWithFeeds([]string{srv.URL})
	items, err := rp.GetNews(context.Background(), "BTCUSDT", 10)
	// RSS provider continues on error — returns empty, no error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items on error, got %d", len(items))
	}
}

func TestRSSInvalidXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("this is not xml"))
	}))
	defer srv.Close()

	rp := NewRSSProviderWithFeeds([]string{srv.URL})
	items, err := rp.GetNews(context.Background(), "BTCUSDT", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items for invalid XML, got %d", len(items))
	}
}

func TestRSSMultipleFeeds(t *testing.T) {
	rssXML1 := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <item><title>BTC breaking news</title><description>Bitcoin update</description>
  <link>https://feed1.com/1</link><pubDate>Mon, 15 Jan 2025 12:00:00 +0000</pubDate></item>
</channel></rss>`

	rssXML2 := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <item><title>Another btc article</title><description>More bitcoin coverage</description>
  <link>https://feed2.com/1</link><pubDate>Mon, 15 Jan 2025 11:00:00 +0000</pubDate></item>
</channel></rss>`

	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssXML1))
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(rssXML2))
	}))
	defer srv2.Close()

	rp := NewRSSProviderWithFeeds([]string{srv1.URL, srv2.URL})
	items, err := rp.GetNews(context.Background(), "BTCUSDT", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items from 2 feeds, got %d", len(items))
	}
}

func TestParseRSSTime(t *testing.T) {
	cases := []struct {
		input string
		valid bool
	}{
		{"Mon, 15 Jan 2025 12:00:00 +0000", true},
		{"2025-01-15T12:00:00Z", true},
		{"not a date", true}, // returns time.Now() as fallback
	}
	for _, tc := range cases {
		result := parseRSSTime(tc.input)
		if result.IsZero() {
			t.Errorf("parseRSSTime(%s) returned zero time", tc.input)
		}
	}
}

func TestExtractDomain(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"https://cointelegraph.com/rss", "cointelegraph.com"},
		{"https://www.coindesk.com/feed/", "coindesk.com"},
		{"http://example.com", "example.com"},
	}
	for _, tc := range cases {
		got := extractDomain(tc.input)
		if got != tc.expected {
			t.Errorf("extractDomain(%s) = %s, want %s", tc.input, got, tc.expected)
		}
	}
}
