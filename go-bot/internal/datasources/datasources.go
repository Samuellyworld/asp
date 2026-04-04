// package datasources provides interfaces and implementations for alternative
// market data feeds: on-chain metrics, order flow, news sentiment, and
// funding rate arbitrage signals.
package datasources

import (
	"context"
	"time"
)

// --- on-chain data ---

// OnChainMetrics holds blockchain-derived signals for a crypto asset.
type OnChainMetrics struct {
	Symbol             string    `json:"symbol"`
	ActiveAddresses24h int64     `json:"active_addresses_24h"`
	TransactionCount   int64     `json:"transaction_count"`
	ExchangeInflow     float64   `json:"exchange_inflow"`   // coins flowing into exchanges (sell pressure)
	ExchangeOutflow    float64   `json:"exchange_outflow"`  // coins leaving exchanges (accumulation)
	NetFlow            float64   `json:"net_flow"`           // inflow - outflow (positive = bearish)
	WhaleTransactions  int       `json:"whale_transactions"` // large transfers (>$100k)
	NVTRatio           float64   `json:"nvt_ratio"`          // network value to transactions
	FetchedAt          time.Time `json:"fetched_at"`
}

// OnChainProvider fetches on-chain data for crypto assets.
type OnChainProvider interface {
	GetMetrics(ctx context.Context, symbol string) (*OnChainMetrics, error)
	SupportedSymbols() []string
}

// --- order flow / market microstructure ---

// OrderFlowSnapshot captures buy/sell pressure and market depth.
type OrderFlowSnapshot struct {
	Symbol          string    `json:"symbol"`
	BuyVolume       float64   `json:"buy_volume"`       // taker buy volume (aggressor buys)
	SellVolume      float64   `json:"sell_volume"`      // taker sell volume (aggressor sells)
	BuySellRatio    float64   `json:"buy_sell_ratio"`   // >1 = more buyers
	LargeBuyOrders  int       `json:"large_buy_orders"` // orders > $50k
	LargeSellOrders int       `json:"large_sell_orders"`
	BidDepthUSD     float64   `json:"bid_depth_usd"`    // total bid liquidity within 1%
	AskDepthUSD     float64   `json:"ask_depth_usd"`    // total ask liquidity within 1%
	DepthImbalance  float64   `json:"depth_imbalance"`  // (bid-ask)/(bid+ask), positive = buy wall
	SpreadBps       float64   `json:"spread_bps"`       // current spread in basis points
	FetchedAt       time.Time `json:"fetched_at"`
}

// OrderFlowProvider fetches real-time order flow data.
type OrderFlowProvider interface {
	GetSnapshot(ctx context.Context, symbol string) (*OrderFlowSnapshot, error)
}

// --- news and social sentiment ---

// NewsItem represents a single news article or social media post.
type NewsItem struct {
	Source    string    `json:"source"`    // "twitter", "reddit", "coindesk", etc.
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	URL       string    `json:"url"`
	Symbols   []string  `json:"symbols"`   // mentioned symbols
	Sentiment float64   `json:"sentiment"` // -1 to 1
	Relevance float64   `json:"relevance"` // 0 to 1
	PublishedAt time.Time `json:"published_at"`
}

// AggregatedSentiment combines sentiment from multiple sources for a symbol.
type AggregatedSentiment struct {
	Symbol          string    `json:"symbol"`
	OverallScore    float64   `json:"overall_score"`    // -1 to 1
	OverallLabel    string    `json:"overall_label"`    // BULLISH, BEARISH, NEUTRAL
	NewsCount       int       `json:"news_count"`       // articles analyzed
	SocialCount     int       `json:"social_count"`     // social posts analyzed
	SocialVolume    float64   `json:"social_volume"`    // relative volume (0-100, spikes indicate hype)
	FearGreedIndex  int       `json:"fear_greed_index"` // 0=extreme fear, 100=extreme greed
	TrendingScore   float64   `json:"trending_score"`   // how much this symbol is trending (0-100)
	TopSources      []NewsItem `json:"top_sources"`     // most relevant items
	FetchedAt       time.Time  `json:"fetched_at"`
}

// SentimentAggregator collects news/social data and produces aggregate sentiment.
type SentimentAggregator interface {
	GetSentiment(ctx context.Context, symbol string) (*AggregatedSentiment, error)
}

// NewsProvider fetches recent news articles.
type NewsProvider interface {
	GetNews(ctx context.Context, symbol string, limit int) ([]NewsItem, error)
}

// --- funding rate arbitrage ---

// FundingRateData holds cross-exchange funding rate data for arbitrage signals.
type FundingRateData struct {
	Symbol      string             `json:"symbol"`
	Rates       map[string]float64 `json:"rates"`        // exchange -> rate (e.g. "binance": 0.01)
	MaxRate     float64            `json:"max_rate"`
	MinRate     float64            `json:"min_rate"`
	Spread      float64            `json:"spread"`       // max - min (arbitrage opportunity)
	Annualized  float64            `json:"annualized"`   // spread * 3 * 365 (8h funding periods)
	NextFunding time.Time          `json:"next_funding"` // time of next funding event
	FetchedAt   time.Time          `json:"fetched_at"`
}

// FundingRateProvider collects funding rates across exchanges.
type FundingRateProvider interface {
	GetFundingRates(ctx context.Context, symbol string) (*FundingRateData, error)
}

// --- composite data source ---

// AlternativeData bundles all alternative data sources for pipeline consumption.
type AlternativeData struct {
	OnChain     *OnChainMetrics      `json:"on_chain,omitempty"`
	OrderFlow   *OrderFlowSnapshot   `json:"order_flow,omitempty"`
	Sentiment   *AggregatedSentiment `json:"sentiment,omitempty"`
	FundingRate *FundingRateData     `json:"funding_rate,omitempty"`
}

// Aggregator collects alt data from all configured providers.
type Aggregator struct {
	onChain     OnChainProvider
	orderFlow   OrderFlowProvider
	sentiment   SentimentAggregator
	fundingRate FundingRateProvider
}

// NewAggregator creates an alternative data aggregator.
// All providers are optional (nil = skip that data source).
func NewAggregator(opts ...AggregatorOption) *Aggregator {
	a := &Aggregator{}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// AggregatorOption configures the aggregator.
type AggregatorOption func(*Aggregator)

func WithOnChain(p OnChainProvider) AggregatorOption     { return func(a *Aggregator) { a.onChain = p } }
func WithOrderFlow(p OrderFlowProvider) AggregatorOption  { return func(a *Aggregator) { a.orderFlow = p } }
func WithSentiment(p SentimentAggregator) AggregatorOption { return func(a *Aggregator) { a.sentiment = p } }
func WithFundingRate(p FundingRateProvider) AggregatorOption { return func(a *Aggregator) { a.fundingRate = p } }

// Fetch gathers all available alternative data for a symbol.
// Runs providers in parallel, never fails entirely (partial results returned).
func (a *Aggregator) Fetch(ctx context.Context, symbol string) *AlternativeData {
	data := &AlternativeData{}

	type result struct {
		kind string
		val  interface{}
	}

	ch := make(chan result, 4)
	count := 0

	if a.onChain != nil {
		count++
		go func() {
			m, err := a.onChain.GetMetrics(ctx, symbol)
			if err == nil {
				ch <- result{"onchain", m}
			} else {
				ch <- result{"onchain", nil}
			}
		}()
	}

	if a.orderFlow != nil {
		count++
		go func() {
			s, err := a.orderFlow.GetSnapshot(ctx, symbol)
			if err == nil {
				ch <- result{"orderflow", s}
			} else {
				ch <- result{"orderflow", nil}
			}
		}()
	}

	if a.sentiment != nil {
		count++
		go func() {
			s, err := a.sentiment.GetSentiment(ctx, symbol)
			if err == nil {
				ch <- result{"sentiment", s}
			} else {
				ch <- result{"sentiment", nil}
			}
		}()
	}

	if a.fundingRate != nil {
		count++
		go func() {
			f, err := a.fundingRate.GetFundingRates(ctx, symbol)
			if err == nil {
				ch <- result{"funding", f}
			} else {
				ch <- result{"funding", nil}
			}
		}()
	}

	for i := 0; i < count; i++ {
		r := <-ch
		switch r.kind {
		case "onchain":
			if v, ok := r.val.(*OnChainMetrics); ok {
				data.OnChain = v
			}
		case "orderflow":
			if v, ok := r.val.(*OrderFlowSnapshot); ok {
				data.OrderFlow = v
			}
		case "sentiment":
			if v, ok := r.val.(*AggregatedSentiment); ok {
				data.Sentiment = v
			}
		case "funding":
			if v, ok := r.val.(*FundingRateData); ok {
				data.FundingRate = v
			}
		}
	}

	return data
}
