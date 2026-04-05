// reddit sentiment provider — fetches crypto-related posts from Reddit
// and derives sentiment from upvotes, comments, and title keywords.
package datasources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// RedditProvider fetches posts from crypto subreddits for sentiment analysis.
type RedditProvider struct {
	httpClient  *http.Client
	subreddits  []string
	userAgent   string
}

// NewRedditProvider creates a Reddit sentiment provider.
// Uses the public JSON API (no auth required, rate limited to ~10 req/min).
func NewRedditProvider() *RedditProvider {
	return &RedditProvider{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		subreddits: []string{"cryptocurrency", "bitcoin", "CryptoMarkets"},
		userAgent:  "TradingBot/1.0 (crypto analysis)",
	}
}

// reddit JSON API structures
type redditListing struct {
	Data struct {
		Children []redditChild `json:"children"`
	} `json:"data"`
}

type redditChild struct {
	Data redditPost `json:"data"`
}

type redditPost struct {
	Title        string  `json:"title"`
	Selftext     string  `json:"selftext"`
	Score        int     `json:"score"`
	NumComments  int     `json:"num_comments"`
	Permalink    string  `json:"permalink"`
	Subreddit    string  `json:"subreddit"`
	CreatedUTC   float64 `json:"created_utc"`
	UpvoteRatio  float64 `json:"upvote_ratio"`
}

// GetNews fetches recent Reddit posts mentioning the given symbol.
func (r *RedditProvider) GetNews(ctx context.Context, symbol string, limit int) ([]NewsItem, error) {
	coin := extractCoinCode(symbol)
	coinLower := strings.ToLower(coin)

	var allItems []NewsItem

	for _, sub := range r.subreddits {
		items, err := r.fetchSubreddit(ctx, sub, coin, coinLower, limit)
		if err != nil {
			continue // skip failed subreddits
		}
		allItems = append(allItems, items...)
		if len(allItems) >= limit {
			break
		}
	}

	if len(allItems) > limit {
		allItems = allItems[:limit]
	}

	return allItems, nil
}

func (r *RedditProvider) fetchSubreddit(ctx context.Context, subreddit, coin, coinLower string, limit int) ([]NewsItem, error) {
	url := fmt.Sprintf("https://www.reddit.com/r/%s/hot.json?limit=50", subreddit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", r.userAgent)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit returned %d for r/%s", resp.StatusCode, subreddit)
	}

	var listing redditListing
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return nil, fmt.Errorf("reddit decode failed: %w", err)
	}

	var items []NewsItem
	for _, child := range listing.Data.Children {
		post := child.Data
		titleLower := strings.ToLower(post.Title)
		textLower := strings.ToLower(post.Selftext)

		// filter: must mention the coin
		if !strings.Contains(titleLower, coinLower) && !strings.Contains(textLower, coinLower) {
			continue
		}

		sentiment := deriveRedditSentiment(post)
		relevance := deriveRedditRelevance(post, coinLower)

		items = append(items, NewsItem{
			Source:      "Reddit/r/" + subreddit,
			Title:       post.Title,
			Content:     truncate(post.Selftext, 200),
			URL:         "https://reddit.com" + post.Permalink,
			Symbols:     []string{coin},
			Sentiment:   sentiment,
			Relevance:   relevance,
			PublishedAt: time.Unix(int64(post.CreatedUTC), 0),
		})

		if len(items) >= limit {
			break
		}
	}

	return items, nil
}

// deriveRedditSentiment produces a -1 to 1 score from Reddit post metrics.
func deriveRedditSentiment(post redditPost) float64 {
	// upvote ratio centers around 0.5 for neutral
	ratioScore := (post.UpvoteRatio - 0.5) * 2 // maps 0.0-1.0 to -1.0 to 1.0

	// high engagement amplifies sentiment direction
	engagementWeight := 1.0
	if post.Score > 100 {
		engagementWeight = 1.2
	}
	if post.Score > 500 {
		engagementWeight = 1.5
	}

	// keyword-based sentiment from title
	titleScore := keywordSentiment(post.Title)

	// blend: 40% ratio, 30% title keywords, 30% engagement-weighted ratio
	blended := ratioScore*0.4 + titleScore*0.3 + ratioScore*engagementWeight*0.3

	// clamp to [-1, 1]
	if blended > 1 {
		return 1
	}
	if blended < -1 {
		return -1
	}
	return blended
}

func deriveRedditRelevance(post redditPost, coinLower string) float64 {
	relevance := 0.5

	titleLower := strings.ToLower(post.Title)
	if strings.Contains(titleLower, coinLower) {
		relevance += 0.3 // mentioned in title = more relevant
	}
	if post.NumComments > 50 {
		relevance += 0.1 // high discussion
	}
	if post.Score > 100 {
		relevance += 0.1 // popular post
	}

	if relevance > 1 {
		return 1
	}
	return relevance
}

// keywordSentiment does simple keyword-based sentiment on text.
func keywordSentiment(text string) float64 {
	lower := strings.ToLower(text)

	bullish := []string{"bullish", "moon", "pump", "rally", "breakout", "ath", "adoption", "surge", "gains"}
	bearish := []string{"bearish", "crash", "dump", "plunge", "sell", "fear", "scam", "hack", "down"}

	var score float64
	for _, kw := range bullish {
		if strings.Contains(lower, kw) {
			score += 0.3
		}
	}
	for _, kw := range bearish {
		if strings.Contains(lower, kw) {
			score -= 0.3
		}
	}

	if score > 1 {
		return 1
	}
	if score < -1 {
		return -1
	}
	return score
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
