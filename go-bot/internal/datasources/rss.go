// rss sentiment provider — fetches crypto news from popular RSS feeds
// as a fallback when CryptoPanic and Reddit are unavailable.
package datasources

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// RSSProvider fetches news from crypto RSS feeds.
type RSSProvider struct {
	httpClient *http.Client
	feeds      []string
}

// NewRSSProvider creates an RSS news provider with default crypto feeds.
func NewRSSProvider() *RSSProvider {
	return &RSSProvider{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		feeds: []string{
			"https://cointelegraph.com/rss",
			"https://www.coindesk.com/arc/outboundfeeds/rss/",
		},
	}
}

// NewRSSProviderWithFeeds creates an RSS provider with custom feed URLs.
func NewRSSProviderWithFeeds(feeds []string) *RSSProvider {
	return &RSSProvider{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		feeds:      feeds,
	}
}

// rss feed structures
type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
	PubDate     string `xml:"pubDate"`
}

// GetNews fetches news articles from RSS feeds that mention the symbol.
func (r *RSSProvider) GetNews(ctx context.Context, symbol string, limit int) ([]NewsItem, error) {
	coin := extractCoinCode(symbol)
	coinLower := strings.ToLower(coin)

	var allItems []NewsItem

	for _, feedURL := range r.feeds {
		items, err := r.fetchFeed(ctx, feedURL, coin, coinLower)
		if err != nil {
			continue // skip failed feeds
		}
		allItems = append(allItems, items...)
	}

	// filter and limit
	var filtered []NewsItem
	for _, item := range allItems {
		if len(filtered) >= limit {
			break
		}
		filtered = append(filtered, item)
	}

	return filtered, nil
}

func (r *RSSProvider) fetchFeed(ctx context.Context, feedURL, coin, coinLower string) ([]NewsItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rss feed %s returned %d", feedURL, resp.StatusCode)
	}

	var feed rssFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("rss decode failed for %s: %w", feedURL, err)
	}

	var items []NewsItem
	for _, item := range feed.Channel.Items {
		titleLower := strings.ToLower(item.Title)
		descLower := strings.ToLower(item.Description)

		// filter: must mention the coin
		if !strings.Contains(titleLower, coinLower) && !strings.Contains(descLower, coinLower) {
			continue
		}

		pubTime := parseRSSTime(item.PubDate)
		sentiment := keywordSentiment(item.Title + " " + item.Description)

		items = append(items, NewsItem{
			Source:      "RSS/" + extractDomain(feedURL),
			Title:       item.Title,
			Content:     truncate(item.Description, 200),
			URL:         item.Link,
			Symbols:     []string{coin},
			Sentiment:   sentiment,
			Relevance:   0.7,
			PublishedAt: pubTime,
		})
	}

	return items, nil
}

func parseRSSTime(s string) time.Time {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"Mon, 02 Jan 2006 15:04:05 MST",
		"2006-01-02T15:04:05Z",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Now()
}

func extractDomain(url string) string {
	// simple domain extraction: "https://example.com/path" -> "example.com"
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "www.")
	if idx := strings.Index(url, "/"); idx > 0 {
		url = url[:idx]
	}
	return url
}
