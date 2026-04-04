// news sentiment aggregator — collects fear/greed index and basic
// crypto news headlines for sentiment analysis via the ML service.
package datasources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// HTTPSentimentAggregator combines fear/greed index + news for sentiment.
type HTTPSentimentAggregator struct {
	httpClient *http.Client
	mlAnalyze  func(ctx context.Context, text string) (float64, string, float64, error) // score, label, confidence
}

// NewHTTPSentimentAggregator creates a sentiment aggregator.
// mlAnalyze is called to analyze combined text — pass nil to skip ML analysis.
func NewHTTPSentimentAggregator(mlAnalyze func(ctx context.Context, text string) (float64, string, float64, error)) *HTTPSentimentAggregator {
	return &HTTPSentimentAggregator{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		mlAnalyze:  mlAnalyze,
	}
}

// alternative.me fear and greed API response
type fearGreedResponse struct {
	Data []struct {
		Value               string `json:"value"`
		ValueClassification string `json:"value_classification"`
		Timestamp           string `json:"timestamp"`
	} `json:"data"`
}

// GetSentiment combines fear/greed index with ML text analysis.
func (h *HTTPSentimentAggregator) GetSentiment(ctx context.Context, symbol string) (*AggregatedSentiment, error) {
	result := &AggregatedSentiment{
		Symbol:    symbol,
		FetchedAt: time.Now(),
	}

	// fetch fear & greed index (crypto market-wide)
	fg, err := h.fetchFearGreed(ctx)
	if err == nil && fg > 0 {
		result.FearGreedIndex = fg
	}

	// convert fear/greed to directional score: 0-100 -> -1 to +1
	if result.FearGreedIndex > 0 {
		result.OverallScore = (float64(result.FearGreedIndex) - 50) / 50 // 0=-1, 50=0, 100=+1
	}

	// classify
	switch {
	case result.OverallScore > 0.2:
		result.OverallLabel = "BULLISH"
	case result.OverallScore < -0.2:
		result.OverallLabel = "BEARISH"
	default:
		result.OverallLabel = "NEUTRAL"
	}

	// if ML analyzer is available, analyze a summary text
	if h.mlAnalyze != nil {
		summaryText := buildSentimentText(symbol, result.FearGreedIndex)
		score, label, confidence, err := h.mlAnalyze(ctx, summaryText)
		if err == nil {
			// blend ML score with fear/greed (70% ML, 30% fear/greed)
			if result.FearGreedIndex > 0 {
				result.OverallScore = score*0.7 + result.OverallScore*0.3
			} else {
				result.OverallScore = score
			}
			result.OverallLabel = label
			result.SocialCount = 1 // ML analyzed at least one input
			_ = confidence
		}
	}

	return result, nil
}

func (h *HTTPSentimentAggregator) fetchFearGreed(ctx context.Context) (int, error) {
	url := "https://api.alternative.me/fng/?limit=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("fear/greed API returned %d", resp.StatusCode)
	}

	var fg fearGreedResponse
	if err := json.NewDecoder(resp.Body).Decode(&fg); err != nil {
		return 0, err
	}

	if len(fg.Data) == 0 {
		return 0, fmt.Errorf("no fear/greed data")
	}

	var val int
	fmt.Sscanf(fg.Data[0].Value, "%d", &val)
	return val, nil
}

func buildSentimentText(symbol string, fearGreed int) string {
	parts := []string{
		fmt.Sprintf("Crypto market analysis for %s.", symbol),
	}
	if fearGreed > 0 {
		var mood string
		switch {
		case fearGreed >= 75:
			mood = "extreme greed"
		case fearGreed >= 55:
			mood = "greed"
		case fearGreed >= 45:
			mood = "neutral"
		case fearGreed >= 25:
			mood = "fear"
		default:
			mood = "extreme fear"
		}
		parts = append(parts, fmt.Sprintf("The Fear & Greed Index is at %d (%s).", fearGreed, mood))
	}
	return strings.Join(parts, " ")
}
