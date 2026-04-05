// cryptopanic news sentiment provider — fetches crypto news from the CryptoPanic API
// and extracts basic sentiment signals from titles and vote counts.
package datasources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CryptoPanicProvider fetches crypto news articles and derives sentiment.
type CryptoPanicProvider struct {
	apiToken   string
	httpClient *http.Client
	baseURL    string
}

// NewCryptoPanicProvider creates a CryptoPanic news provider.
// apiToken is the free API key from cryptopanic.com/developers/api/
func NewCryptoPanicProvider(apiToken string) *CryptoPanicProvider {
	return &CryptoPanicProvider{
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: "https://cryptopanic.com/api/free/v1",
	}
}

// cryptopanic API response structures
type cpResponse struct {
	Results []cpPost `json:"results"`
}

type cpPost struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Source      cpSource  `json:"source"`
	PublishedAt string    `json:"published_at"`
	Currencies  []cpCoin  `json:"currencies"`
	Votes       cpVotes   `json:"votes"`
	Kind        string    `json:"kind"`
}

type cpSource struct {
	Title string `json:"title"`
}

type cpCoin struct {
	Code  string `json:"code"`
	Title string `json:"title"`
}

type cpVotes struct {
	Positive    int `json:"positive"`
	Negative    int `json:"negative"`
	Important   int `json:"important"`
	Liked       int `json:"liked"`
	Disliked    int `json:"disliked"`
	Lol         int `json:"lol"`
	Toxic       int `json:"toxic"`
	Saved       int `json:"saved"`
	Comments    int `json:"comments"`
}

// GetNews fetches recent news articles for a given symbol from CryptoPanic.
func (cp *CryptoPanicProvider) GetNews(ctx context.Context, symbol string, limit int) ([]NewsItem, error) {
	// convert "BTC/USDT" or "BTCUSDT" to "BTC"
	coin := extractCoinCode(symbol)

	url := fmt.Sprintf("%s/posts/?auth_token=%s&currencies=%s&kind=news&public=true",
		cp.baseURL, cp.apiToken, coin)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := cp.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cryptopanic request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cryptopanic returned %d", resp.StatusCode)
	}

	var cpResp cpResponse
	if err := json.NewDecoder(resp.Body).Decode(&cpResp); err != nil {
		return nil, fmt.Errorf("cryptopanic decode failed: %w", err)
	}

	items := make([]NewsItem, 0, limit)
	for _, post := range cpResp.Results {
		if len(items) >= limit {
			break
		}

		sentiment := deriveCPSentiment(post.Votes)
		pubTime, _ := time.Parse(time.RFC3339, post.PublishedAt)

		symbols := make([]string, len(post.Currencies))
		for j, c := range post.Currencies {
			symbols[j] = c.Code
		}

		items = append(items, NewsItem{
			Source:      "CryptoPanic/" + post.Source.Title,
			Title:       post.Title,
			URL:         post.URL,
			Symbols:     symbols,
			Sentiment:   sentiment,
			Relevance:   1.0,
			PublishedAt: pubTime,
		})
	}

	return items, nil
}

// deriveCPSentiment calculates a -1 to 1 sentiment score from CryptoPanic votes.
func deriveCPSentiment(v cpVotes) float64 {
	positive := float64(v.Positive + v.Liked + v.Saved)
	negative := float64(v.Negative + v.Disliked + v.Toxic)
	total := positive + negative

	if total == 0 {
		return 0
	}

	return (positive - negative) / total
}

// extractCoinCode converts "BTC/USDT", "BTCUSDT" to "BTC"
func extractCoinCode(symbol string) string {
	s := strings.ToUpper(symbol)
	s = strings.ReplaceAll(s, "/", "")
	// common quote currencies
	for _, quote := range []string{"USDT", "BUSD", "USDC", "USD", "BTC", "ETH", "BNB"} {
		if strings.HasSuffix(s, quote) && len(s) > len(quote) {
			return s[:len(s)-len(quote)]
		}
	}
	return s
}
