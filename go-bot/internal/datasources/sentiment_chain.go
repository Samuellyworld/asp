// sentiment chain — tries multiple news sources in priority order
// and aggregates sentiment from whichever sources respond successfully.
package datasources

import (
	"context"
	"log/slog"
	"time"
)

// SentimentChain implements SentimentAggregator by querying multiple
// NewsProviders in priority order and combining their results.
type SentimentChain struct {
	providers []NewsProvider
	mlAnalyze func(ctx context.Context, text string) (float64, string, float64, error)
	fearGreed func(ctx context.Context) (int, error)
}

// NewSentimentChain creates a sentiment chain with the given providers.
// Providers are queried in order; results are aggregated from all that respond.
// mlAnalyze is optional — used to refine sentiment via ML if available.
// fearGreed is optional — used to blend fear/greed index into overall score.
func NewSentimentChain(
	providers []NewsProvider,
	mlAnalyze func(ctx context.Context, text string) (float64, string, float64, error),
	fearGreed func(ctx context.Context) (int, error),
) *SentimentChain {
	return &SentimentChain{
		providers: providers,
		mlAnalyze: mlAnalyze,
		fearGreed: fearGreed,
	}
}

// GetSentiment aggregates sentiment across all configured news sources.
func (sc *SentimentChain) GetSentiment(ctx context.Context, symbol string) (*AggregatedSentiment, error) {
	result := &AggregatedSentiment{
		Symbol:    symbol,
		FetchedAt: time.Now(),
	}

	// collect news from all providers
	var allItems []NewsItem
	for _, provider := range sc.providers {
		items, err := provider.GetNews(ctx, symbol, 20)
		if err != nil {
			slog.Debug("sentiment provider failed", "error", err)
			continue
		}
		allItems = append(allItems, items...)
	}

	result.NewsCount = len(allItems)

	// compute average sentiment from news items
	if len(allItems) > 0 {
		var totalSentiment float64
		for _, item := range allItems {
			totalSentiment += item.Sentiment * item.Relevance
		}
		result.OverallScore = totalSentiment / float64(len(allItems))

		// keep top 5 most relevant items
		sorted := sortByRelevance(allItems)
		if len(sorted) > 5 {
			sorted = sorted[:5]
		}
		result.TopSources = sorted
	}

	// blend with fear/greed index if available
	if sc.fearGreed != nil {
		fg, err := sc.fearGreed(ctx)
		if err == nil && fg > 0 {
			result.FearGreedIndex = fg
			fgScore := (float64(fg) - 50) / 50

			if len(allItems) > 0 {
				// blend: 60% news, 40% fear/greed
				result.OverallScore = result.OverallScore*0.6 + fgScore*0.4
			} else {
				result.OverallScore = fgScore
			}
		}
	}

	// optionally refine with ML sentiment
	if sc.mlAnalyze != nil && len(allItems) > 0 {
		// combine top titles into a single text for ML analysis
		var titles []string
		for i, item := range allItems {
			if i >= 5 {
				break
			}
			titles = append(titles, item.Title)
		}
		combinedText := symbol + ": " + joinStrings(titles, ". ")

		score, label, _, err := sc.mlAnalyze(ctx, combinedText)
		if err == nil {
			// blend: 50% news aggregate, 50% ML
			result.OverallScore = result.OverallScore*0.5 + score*0.5
			result.OverallLabel = label
			result.SocialCount = 1
		}
	}

	// classify if not already set by ML
	if result.OverallLabel == "" {
		switch {
		case result.OverallScore > 0.2:
			result.OverallLabel = "BULLISH"
		case result.OverallScore < -0.2:
			result.OverallLabel = "BEARISH"
		default:
			result.OverallLabel = "NEUTRAL"
		}
	}

	return result, nil
}

// sortByRelevance sorts news items by relevance (highest first).
func sortByRelevance(items []NewsItem) []NewsItem {
	sorted := make([]NewsItem, len(items))
	copy(sorted, items)

	// simple insertion sort — list is small
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j].Relevance < key.Relevance {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}

	return sorted
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
}
