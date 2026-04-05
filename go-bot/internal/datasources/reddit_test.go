package datasources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedditGetNews(t *testing.T) {
	listing := redditListing{}
	listing.Data.Children = []redditChild{
		{Data: redditPost{
			Title:       "Bitcoin is going to the moon!",
			Selftext:    "BTC looking very bullish today",
			Score:       500,
			NumComments: 100,
			Permalink:   "/r/cryptocurrency/comments/abc123",
			Subreddit:   "cryptocurrency",
			CreatedUTC:  1705312800,
			UpvoteRatio: 0.92,
		}},
		{Data: redditPost{
			Title:       "Random post about Ethereum",
			Selftext:    "ETH is cool",
			Score:       50,
			NumComments: 10,
			Permalink:   "/r/cryptocurrency/comments/def456",
			Subreddit:   "cryptocurrency",
			CreatedUTC:  1705309200,
			UpvoteRatio: 0.75,
		}},
		{Data: redditPost{
			Title:       "Another BTC discussion",
			Selftext:    "What do you think about btc?",
			Score:       200,
			NumComments: 60,
			Permalink:   "/r/cryptocurrency/comments/ghi789",
			Subreddit:   "cryptocurrency",
			CreatedUTC:  1705305600,
			UpvoteRatio: 0.85,
		}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listing)
	}))
	defer srv.Close()

	rp := NewRedditProvider()
	// override subreddit URLs to hit our test server
	rp.subreddits = []string{"test"}
	// patch fetch to use test URL
	origFetch := rp.httpClient

	// create a custom provider that hits the test server
	rp2 := &RedditProvider{
		httpClient: origFetch,
		subreddits: []string{"test"},
		userAgent:  "test/1.0",
	}

	// we need to mock the URL — let's create a provider with a custom transport
	transport := &testTransport{handler: srv}
	rp2.httpClient = &http.Client{Transport: transport}

	items, err := rp2.GetNews(context.Background(), "BTCUSDT", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// should find posts mentioning "btc" (case insensitive)
	if len(items) < 2 {
		t.Errorf("expected at least 2 BTC-related posts, got %d", len(items))
	}

	for _, item := range items {
		if item.Source == "" {
			t.Error("expected non-empty source")
		}
		if item.Sentiment < -1 || item.Sentiment > 1 {
			t.Errorf("sentiment out of range: %f", item.Sentiment)
		}
	}
}

func TestDeriveRedditSentiment(t *testing.T) {
	cases := []struct {
		name     string
		post     redditPost
		wantSign int // 1=positive, -1=negative, 0=neutral-ish
	}{
		{
			"highly upvoted bullish",
			redditPost{Title: "Bitcoin to the moon! Bullish breakout", UpvoteRatio: 0.95, Score: 600},
			1,
		},
		{
			"bearish post",
			redditPost{Title: "Crash incoming, sell everything", UpvoteRatio: 0.3, Score: 10},
			-1,
		},
		{
			"neutral post",
			redditPost{Title: "Weekly discussion thread", UpvoteRatio: 0.5, Score: 50},
			0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			score := deriveRedditSentiment(tc.post)
			if tc.wantSign > 0 && score <= 0 {
				t.Errorf("expected positive sentiment, got %f", score)
			}
			if tc.wantSign < 0 && score >= 0 {
				t.Errorf("expected negative sentiment, got %f", score)
			}
			if score < -1 || score > 1 {
				t.Errorf("score out of [-1,1] range: %f", score)
			}
		})
	}
}

func TestDeriveRedditRelevance(t *testing.T) {
	post := redditPost{
		Title:       "Bitcoin price prediction for 2025",
		NumComments: 100,
		Score:       200,
	}
	rel := deriveRedditRelevance(post, "bitcoin")
	if rel <= 0.5 {
		t.Errorf("expected high relevance for post mentioning coin in title, got %f", rel)
	}
	if rel > 1.0 {
		t.Errorf("relevance should be capped at 1, got %f", rel)
	}
}

func TestKeywordSentiment(t *testing.T) {
	bullish := keywordSentiment("This is super bullish, pump incoming, moon soon")
	if bullish <= 0 {
		t.Errorf("expected positive keyword sentiment, got %f", bullish)
	}

	bearish := keywordSentiment("Crash dump sell fear scam")
	if bearish >= 0 {
		t.Errorf("expected negative keyword sentiment, got %f", bearish)
	}

	neutral := keywordSentiment("just a random post here")
	if neutral != 0 {
		t.Errorf("expected zero sentiment for unrelated text, got %f", neutral)
	}
}

// testTransport redirects all requests to the test server
type testTransport struct {
	handler *httptest.Server
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.handler.Listener.Addr().String()
	return http.DefaultTransport.RoundTrip(req)
}
