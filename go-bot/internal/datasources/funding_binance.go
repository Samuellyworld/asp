// funding rate provider — collects funding rates from Binance futures
// and provides arbitrage signal data.
package datasources

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"
)

// BinanceFundingRate implements FundingRateProvider using Binance futures API.
type BinanceFundingRate struct {
	futuresURL string
	httpClient *http.Client
}

// NewBinanceFundingRate creates a funding rate provider for Binance.
func NewBinanceFundingRate(futuresURL string) *BinanceFundingRate {
	return &BinanceFundingRate{
		futuresURL: futuresURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type binanceFundingRateResponse struct {
	Symbol          string `json:"symbol"`
	FundingRate     string `json:"lastFundingRate"`
	FundingTime     int64  `json:"nextFundingTime"`
	MarkPrice       string `json:"markPrice"`
	IndexPrice      string `json:"indexPrice"`
}

// GetFundingRates fetches funding rate from Binance futures.
func (b *BinanceFundingRate) GetFundingRates(ctx context.Context, symbol string) (*FundingRateData, error) {
	sym := normalizeBinanceSymbol(symbol)

	url := fmt.Sprintf("%s/fapi/v1/premiumIndex?symbol=%s", b.futuresURL, sym)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("premiumIndex returned %d", resp.StatusCode)
	}

	var fr binanceFundingRateResponse
	if err := json.NewDecoder(resp.Body).Decode(&fr); err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	rate := parseFloat(fr.FundingRate)
	nextFunding := time.UnixMilli(fr.FundingTime)

	data := &FundingRateData{
		Symbol:      symbol,
		Rates:       map[string]float64{"binance": rate},
		MaxRate:     rate,
		MinRate:     rate,
		Spread:      0, // single exchange — no spread
		Annualized:  math.Abs(rate) * 3 * 365 * 100, // as percentage
		NextFunding: nextFunding,
		FetchedAt:   time.Now(),
	}

	return data, nil
}
