package datasources

import (
	"context"
	"testing"
)

func TestSentimentChainNoProviders(t *testing.T) {
	sc := NewSentimentChain(nil, nil, nil)
	result, err := sc.GetSentiment(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Symbol != "BTCUSDT" {
		t.Errorf("expected BTCUSDT, got %s", result.Symbol)
	}
	if result.OverallLabel != "NEUTRAL" {
		t.Errorf("expected NEUTRAL with no providers, got %s", result.OverallLabel)
	}
}

func TestSentimentChainWithNewsProvider(t *testing.T) {
	mockNews := &mockNewsProvider{
		items: []NewsItem{
			{Title: "BTC surges", Sentiment: 0.8, Relevance: 1.0, Source: "test"},
			{Title: "BTC bullish", Sentiment: 0.6, Relevance: 0.9, Source: "test"},
		},
	}

	sc := NewSentimentChain([]NewsProvider{mockNews}, nil, nil)
	result, err := sc.GetSentiment(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NewsCount != 2 {
		t.Errorf("expected 2 news items, got %d", result.NewsCount)
	}
	if result.OverallScore <= 0 {
		t.Errorf("expected positive overall score, got %f", result.OverallScore)
	}
	if result.OverallLabel != "BULLISH" {
		t.Errorf("expected BULLISH, got %s", result.OverallLabel)
	}
}

func TestSentimentChainWithFearGreed(t *testing.T) {
	mockNews := &mockNewsProvider{
		items: []NewsItem{
			{Title: "BTC article", Sentiment: 0.3, Relevance: 1.0},
		},
	}
	fearGreed := func(ctx context.Context) (int, error) {
		return 75, nil // greed
	}

	sc := NewSentimentChain([]NewsProvider{mockNews}, nil, fearGreed)
	result, err := sc.GetSentiment(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FearGreedIndex != 75 {
		t.Errorf("expected fear/greed 75, got %d", result.FearGreedIndex)
	}
	// score should be blended between news (0.3) and fear/greed (0.5)
	if result.OverallScore <= 0 {
		t.Errorf("expected positive blended score, got %f", result.OverallScore)
	}
}

func TestSentimentChainWithMLAnalyze(t *testing.T) {
	mockNews := &mockNewsProvider{
		items: []NewsItem{
			{Title: "BTC positive", Sentiment: 0.5, Relevance: 1.0},
		},
	}
	mlAnalyze := func(ctx context.Context, text string) (float64, string, float64, error) {
		return 0.7, "BULLISH", 0.85, nil
	}

	sc := NewSentimentChain([]NewsProvider{mockNews}, mlAnalyze, nil)
	result, err := sc.GetSentiment(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OverallLabel != "BULLISH" {
		t.Errorf("expected BULLISH from ML, got %s", result.OverallLabel)
	}
	if result.SocialCount != 1 {
		t.Errorf("expected social count 1 (ML analyzed), got %d", result.SocialCount)
	}
}

func TestSentimentChainMultipleProviders(t *testing.T) {
	provider1 := &mockNewsProvider{
		items: []NewsItem{
			{Title: "Source 1 BTC", Sentiment: 0.4, Relevance: 1.0},
		},
	}
	provider2 := &mockNewsProvider{
		items: []NewsItem{
			{Title: "Source 2 BTC", Sentiment: 0.6, Relevance: 0.8},
		},
	}

	sc := NewSentimentChain([]NewsProvider{provider1, provider2}, nil, nil)
	result, err := sc.GetSentiment(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NewsCount != 2 {
		t.Errorf("expected 2 total news items, got %d", result.NewsCount)
	}
}

func TestSentimentChainBearish(t *testing.T) {
	mockNews := &mockNewsProvider{
		items: []NewsItem{
			{Title: "BTC crash", Sentiment: -0.8, Relevance: 1.0},
			{Title: "BTC dump", Sentiment: -0.6, Relevance: 0.9},
		},
	}

	sc := NewSentimentChain([]NewsProvider{mockNews}, nil, nil)
	result, err := sc.GetSentiment(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OverallLabel != "BEARISH" {
		t.Errorf("expected BEARISH, got %s", result.OverallLabel)
	}
	if result.OverallScore >= 0 {
		t.Errorf("expected negative score, got %f", result.OverallScore)
	}
}

func TestSortByRelevance(t *testing.T) {
	items := []NewsItem{
		{Title: "low", Relevance: 0.3},
		{Title: "high", Relevance: 0.9},
		{Title: "mid", Relevance: 0.6},
	}
	sorted := sortByRelevance(items)
	if sorted[0].Title != "high" {
		t.Errorf("expected 'high' first, got %s", sorted[0].Title)
	}
	if sorted[2].Title != "low" {
		t.Errorf("expected 'low' last, got %s", sorted[2].Title)
	}
}

// mock news provider for testing
type mockNewsProvider struct {
	items []NewsItem
	err   error
}

func (m *mockNewsProvider) GetNews(_ context.Context, _ string, limit int) ([]NewsItem, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.items) > limit {
		return m.items[:limit], nil
	}
	return m.items, nil
}
