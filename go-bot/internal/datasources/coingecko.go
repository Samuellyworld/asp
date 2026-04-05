// coingecko market data provider — fetches market information, community stats,
// and developer activity from the CoinGecko API for on-chain insight.
package datasources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CoinGeckoProvider fetches market and community data from CoinGecko.
type CoinGeckoProvider struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string // optional, for pro tier
}

// NewCoinGeckoProvider creates a CoinGecko data provider.
// apiKey is optional (empty string for free tier).
func NewCoinGeckoProvider(apiKey string) *CoinGeckoProvider {
	base := "https://api.coingecko.com/api/v3"
	if apiKey != "" {
		base = "https://pro-api.coingecko.com/api/v3"
	}
	return &CoinGeckoProvider{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: base,
		apiKey:  apiKey,
	}
}

// CoinGeckoMarket holds market data from CoinGecko.
type CoinGeckoMarket struct {
	MarketCapRank          int     `json:"market_cap_rank"`
	MarketCap              float64 `json:"market_cap"`
	TotalVolume            float64 `json:"total_volume"`
	CirculatingSupply      float64 `json:"circulating_supply"`
	TotalSupply            float64 `json:"total_supply"`
	ATH                    float64 `json:"ath"`
	ATHChangePercentage    float64 `json:"ath_change_percentage"`
	CommunityScore         float64 `json:"community_score"`
	DeveloperScore         float64 `json:"developer_score"`
	TwitterFollowers       int     `json:"twitter_followers"`
	RedditSubscribers      int     `json:"reddit_subscribers"`
	RedditActiveAccounts   int     `json:"reddit_accounts_active_48h"`
	GithubForks            int     `json:"github_forks"`
	GithubStars            int     `json:"github_stars"`
	GithubCommits4Weeks    int     `json:"github_commit_count_4_weeks"`
}

// coingecko API response
type cgCoinResponse struct {
	ID            string `json:"id"`
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	MarketData    struct {
		CurrentPrice           map[string]float64 `json:"current_price"`
		MarketCap              map[string]float64 `json:"market_cap"`
		TotalVolume            map[string]float64 `json:"total_volume"`
		CirculatingSupply      float64            `json:"circulating_supply"`
		TotalSupply            float64            `json:"total_supply"`
		ATH                    map[string]float64 `json:"ath"`
		ATHChangePercentage    map[string]float64 `json:"ath_change_percentage"`
		MarketCapRank          int                `json:"market_cap_rank"`
	} `json:"market_data"`
	CommunityData struct {
		TwitterFollowers   int `json:"twitter_followers"`
		RedditSubscribers  int `json:"reddit_subscribers"`
		RedditActiveAccounts int `json:"reddit_accounts_active_48h"`
	} `json:"community_data"`
	DeveloperData struct {
		Forks               int `json:"forks"`
		Stars               int `json:"stars"`
		CommitCount4Weeks   int `json:"commit_count_4_weeks"`
	} `json:"developer_data"`
	CommunityScore float64 `json:"community_score"`
	DeveloperScore float64 `json:"developer_score"`
}

// GetMarketData fetches comprehensive market data for a coin.
func (cg *CoinGeckoProvider) GetMarketData(ctx context.Context, symbol string) (*CoinGeckoMarket, error) {
	coinID := symbolToCoinGeckoID(symbol)

	url := fmt.Sprintf("%s/coins/%s?localization=false&tickers=false&community_data=true&developer_data=true",
		cg.baseURL, coinID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if cg.apiKey != "" {
		req.Header.Set("x-cg-pro-api-key", cg.apiKey)
	}

	resp, err := cg.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("coingecko request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("coingecko returned %d", resp.StatusCode)
	}

	var data cgCoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("coingecko decode failed: %w", err)
	}

	result := &CoinGeckoMarket{
		MarketCapRank:       data.MarketData.MarketCapRank,
		MarketCap:           data.MarketData.MarketCap["usd"],
		TotalVolume:         data.MarketData.TotalVolume["usd"],
		CirculatingSupply:   data.MarketData.CirculatingSupply,
		TotalSupply:         data.MarketData.TotalSupply,
		ATH:                 data.MarketData.ATH["usd"],
		ATHChangePercentage: data.MarketData.ATHChangePercentage["usd"],
		CommunityScore:      data.CommunityScore,
		DeveloperScore:      data.DeveloperScore,
		TwitterFollowers:    data.CommunityData.TwitterFollowers,
		RedditSubscribers:   data.CommunityData.RedditSubscribers,
		RedditActiveAccounts: data.CommunityData.RedditActiveAccounts,
		GithubForks:         data.DeveloperData.Forks,
		GithubStars:         data.DeveloperData.Stars,
		GithubCommits4Weeks: data.DeveloperData.CommitCount4Weeks,
	}

	return result, nil
}

// GetMetrics implements OnChainProvider using CoinGecko data.
// Maps market + community data to on-chain-like metrics.
func (cg *CoinGeckoProvider) GetMetrics(ctx context.Context, symbol string) (*OnChainMetrics, error) {
	market, err := cg.GetMarketData(ctx, symbol)
	if err != nil {
		return nil, err
	}

	// map CoinGecko data to OnChainMetrics
	// active addresses approximated from reddit active accounts
	// NVT ratio = market cap / (24h volume)
	nvt := 0.0
	if market.TotalVolume > 0 {
		nvt = market.MarketCap / market.TotalVolume
	}

	return &OnChainMetrics{
		Symbol:             symbol,
		ActiveAddresses24h: int64(market.RedditActiveAccounts),
		NVTRatio:           nvt,
		FetchedAt:          time.Now(),
	}, nil
}

// SupportedSymbols returns all symbols (CoinGecko supports most coins).
func (cg *CoinGeckoProvider) SupportedSymbols() []string {
	return []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT", "AVAXUSDT"}
}

// symbolToCoinGeckoID maps trading symbols to CoinGecko IDs.
var coinGeckoIDs = map[string]string{
	"BTC":   "bitcoin",
	"ETH":   "ethereum",
	"BNB":   "binancecoin",
	"SOL":   "solana",
	"XRP":   "ripple",
	"DOGE":  "dogecoin",
	"ADA":   "cardano",
	"AVAX":  "avalanche-2",
	"DOT":   "polkadot",
	"LINK":  "chainlink",
	"MATIC": "matic-network",
	"UNI":   "uniswap",
	"ATOM":  "cosmos",
	"LTC":   "litecoin",
	"FTM":   "fantom",
	"NEAR":  "near",
	"OP":    "optimism",
	"ARB":   "arbitrum",
	"APT":   "aptos",
	"SUI":   "sui",
}

func symbolToCoinGeckoID(symbol string) string {
	coin := extractCoinCode(strings.ToUpper(symbol))
	if id, ok := coinGeckoIDs[coin]; ok {
		return id
	}
	return strings.ToLower(coin)
}
