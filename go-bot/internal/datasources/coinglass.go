// coinglass on-chain / derivatives data provider — fetches open interest,
// liquidation data, and funding rates from the CoinGlass API.
package datasources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// CoinGlassProvider fetches derivatives data from CoinGlass.
type CoinGlassProvider struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewCoinGlassProvider creates a CoinGlass data provider.
func NewCoinGlassProvider(apiKey string) *CoinGlassProvider {
	return &CoinGlassProvider{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: "https://open-api.coinglass.com/public/v2",
	}
}

// CoinGlassOI holds open interest data from CoinGlass.
type CoinGlassOI struct {
	OpenInterest       float64 `json:"open_interest"`
	OpenInterestChange float64 `json:"oi_change_pct_24h"`
}

// CoinGlassLiquidations holds liquidation data.
type CoinGlassLiquidations struct {
	LongLiquidations  float64 `json:"long_liquidations_24h"`
	ShortLiquidations float64 `json:"short_liquidations_24h"`
	TotalLiquidations float64 `json:"total_liquidations_24h"`
}

// CoinGlassDerivatives holds combined derivatives metrics.
type CoinGlassDerivatives struct {
	Symbol             string
	OpenInterest       float64
	OIChange24h        float64
	LongLiquidations   float64
	ShortLiquidations  float64
	TotalLiquidations  float64
	LongShortRatio     float64
	FetchedAt          time.Time
}

// coinglass API response wrappers
type cgOIResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		Symbol             string  `json:"symbol"`
		OpenInterest       float64 `json:"openInterest"`
		OpenInterestAmount float64 `json:"openInterestAmount"`
		H24Change          float64 `json:"h24Change"`
	} `json:"data"`
}

type cgLiqResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		Symbol           string  `json:"symbol"`
		LongVolUSD       float64 `json:"longVolUsd"`
		ShortVolUSD      float64 `json:"shortVolUsd"`
		TotalVolUSD      float64 `json:"totalVolUsd"`
	} `json:"data"`
}

type cgLSRResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		Symbol    string  `json:"symbol"`
		LongRate  float64 `json:"longRate"`
		ShortRate float64 `json:"shortRate"`
	} `json:"data"`
}

// GetDerivatives fetches combined derivatives metrics for a symbol.
func (cg *CoinGlassProvider) GetDerivatives(ctx context.Context, symbol string) (*CoinGlassDerivatives, error) {
	coin := extractCoinCode(symbol)

	result := &CoinGlassDerivatives{
		Symbol:    symbol,
		FetchedAt: time.Now(),
	}

	// fetch open interest
	oi, err := cg.fetchOI(ctx, coin)
	if err == nil && oi != nil {
		result.OpenInterest = oi.OpenInterest
		result.OIChange24h = oi.OpenInterestChange
	}

	// fetch liquidations
	liq, err := cg.fetchLiquidations(ctx, coin)
	if err == nil && liq != nil {
		result.LongLiquidations = liq.LongLiquidations
		result.ShortLiquidations = liq.ShortLiquidations
		result.TotalLiquidations = liq.TotalLiquidations
	}

	// fetch long/short ratio
	lsr, err := cg.fetchLongShortRatio(ctx, coin)
	if err == nil {
		result.LongShortRatio = lsr
	}

	return result, nil
}

func (cg *CoinGlassProvider) doRequest(ctx context.Context, path string, target interface{}) error {
	url := cg.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("coinglassSecret", cg.apiKey)

	resp, err := cg.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("coinglass returned %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func (cg *CoinGlassProvider) fetchOI(ctx context.Context, coin string) (*CoinGlassOI, error) {
	var resp cgOIResponse
	if err := cg.doRequest(ctx, fmt.Sprintf("/open_interest?symbol=%s", coin), &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no OI data for %s", coin)
	}

	return &CoinGlassOI{
		OpenInterest:       resp.Data[0].OpenInterest,
		OpenInterestChange: resp.Data[0].H24Change,
	}, nil
}

func (cg *CoinGlassProvider) fetchLiquidations(ctx context.Context, coin string) (*CoinGlassLiquidations, error) {
	var resp cgLiqResponse
	if err := cg.doRequest(ctx, fmt.Sprintf("/liquidation_chart?symbol=%s&timeType=2", coin), &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no liquidation data for %s", coin)
	}

	return &CoinGlassLiquidations{
		LongLiquidations:  resp.Data[0].LongVolUSD,
		ShortLiquidations: resp.Data[0].ShortVolUSD,
		TotalLiquidations: resp.Data[0].TotalVolUSD,
	}, nil
}

func (cg *CoinGlassProvider) fetchLongShortRatio(ctx context.Context, coin string) (float64, error) {
	var resp cgLSRResponse
	if err := cg.doRequest(ctx, fmt.Sprintf("/long_short?symbol=%s&timeType=2", coin), &resp); err != nil {
		return 0, err
	}
	if len(resp.Data) == 0 || resp.Data[0].ShortRate == 0 {
		return 0, fmt.Errorf("no LSR data for %s", coin)
	}

	return resp.Data[0].LongRate / resp.Data[0].ShortRate, nil
}

// GetMetrics implements OnChainProvider using CoinGlass derivatives data
// to provide exchange-level insights (OI as proxy for exchange activity).
func (cg *CoinGlassProvider) GetMetrics(ctx context.Context, symbol string) (*OnChainMetrics, error) {
	deriv, err := cg.GetDerivatives(ctx, symbol)
	if err != nil {
		return nil, err
	}

	// map derivatives data to on-chain metrics
	// OI change is a proxy for flow direction (increasing OI = new money entering)
	netFlow := 0.0
	if deriv.OIChange24h < 0 {
		netFlow = deriv.OIChange24h * 100 // negative OI change = reducing positions
	}

	return &OnChainMetrics{
		Symbol:             symbol,
		NetFlow:            netFlow,
		WhaleTransactions:  int(deriv.TotalLiquidations / 100000), // proxy
		ExchangeInflow:     deriv.LongLiquidations,
		ExchangeOutflow:    deriv.ShortLiquidations,
		FetchedAt:          deriv.FetchedAt,
	}, nil
}

// SupportedSymbols returns commonly supported derivative symbols.
func (cg *CoinGlassProvider) SupportedSymbols() []string {
	return []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT", "AVAXUSDT"}
}
