// package claude provides the ai decision-making layer using the claude api
package claude

import "time"

// action represents the trading decision
type Action string

const (
	ActionBuy  Action = "BUY"
	ActionSell Action = "SELL"
	ActionHold Action = "HOLD"
)

// market data fed into the analysis pipeline
type MarketData struct {
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Volume24h float64 `json:"volume_24h"`
	Change24h float64 `json:"change_24h"`
}

// technical indicators from the rust engine
type Indicators struct {
	RSI        float64 `json:"rsi"`
	MACDValue  float64 `json:"macd_value"`
	MACDSignal float64 `json:"macd_signal"`
	MACDHist   float64 `json:"macd_histogram"`
	BBUpper    float64 `json:"bb_upper"`
	BBMiddle   float64 `json:"bb_middle"`
	BBLower    float64 `json:"bb_lower"`
	EMA12      float64 `json:"ema_12"`
	EMA26      float64 `json:"ema_26"`
	VolumeSpike bool   `json:"volume_spike"`
}

// ml predictions from the python service
type MLPrediction struct {
	Direction  string  `json:"direction"`
	Magnitude  float64 `json:"magnitude"`
	Confidence float64 `json:"confidence"`
	Timeframe  string  `json:"timeframe"`
}

// sentiment data from the python service
type Sentiment struct {
	Score      float64 `json:"score"`
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
}

// trading cost context for fee-aware decision making
type TradingCosts struct {
	SpotMakerFeePct  float64 `json:"spot_maker_fee_pct"`  // e.g. 0.10 for 0.10%
	SpotTakerFeePct  float64 `json:"spot_taker_fee_pct"`  // e.g. 0.10
	FuturesMakerPct  float64 `json:"futures_maker_pct"`   // e.g. 0.02
	FuturesTakerPct  float64 `json:"futures_taker_pct"`   // e.g. 0.04
	FundingRatePct   float64 `json:"funding_rate_pct"`    // current 8h funding rate
	EstSlippageBps   float64 `json:"est_slippage_bps"`    // avg slippage in bps
	AvgRoundTripCost float64 `json:"avg_round_trip_cost"` // total cost estimate in %
}

// DefaultTradingCosts returns Binance standard fee tier.
func DefaultTradingCosts() *TradingCosts {
	return &TradingCosts{
		SpotMakerFeePct: 0.10,
		SpotTakerFeePct: 0.10,
		FuturesMakerPct: 0.02,
		FuturesTakerPct: 0.04,
	}
}

// bundles all context for claude to analyze
type AnalysisInput struct {
	Market     MarketData    `json:"market"`
	Indicators *Indicators   `json:"indicators,omitempty"`
	Prediction *MLPrediction `json:"prediction,omitempty"`
	Sentiment  *Sentiment    `json:"sentiment,omitempty"`
	Costs      *TradingCosts `json:"costs,omitempty"`
}

// the trade plan extracted from claude's response
type TradePlan struct {
	Entry        float64 `json:"entry"`
	StopLoss     float64 `json:"stop_loss"`
	TakeProfit   float64 `json:"take_profit"`
	PositionSize float64 `json:"position_size"`
	RiskReward   float64 `json:"risk_reward"`
}

// the structured decision from claude
type Decision struct {
	Action     Action    `json:"action"`
	Confidence float64   `json:"confidence"`
	Plan       TradePlan `json:"plan"`
	Reasoning  string    `json:"reasoning"`
	Timestamp  time.Time `json:"timestamp"`
	Latency    time.Duration `json:"latency"`
}

// claude api message format
type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claude api request body
type apiRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    string       `json:"system"`
	Messages  []apiMessage `json:"messages"`
}

// a single content block in the api response
type apiContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// claude api response body
type apiResponse struct {
	ID      string            `json:"id"`
	Content []apiContentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// claude api error response
type apiError struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}
