package datasources

import (
	"context"
	"testing"
)

func TestAggregatorFetchNilProviders(t *testing.T) {
	agg := NewAggregator() // no providers
	result := agg.Fetch(context.Background(), "BTCUSDT")
	if result == nil {
		t.Fatal("expected non-nil result even with no providers")
	}
	if result.OrderFlow != nil {
		t.Error("expected nil order flow")
	}
	if result.OnChain != nil {
		t.Error("expected nil on-chain")
	}
}

func TestAggregatorWithOrderFlow(t *testing.T) {
	agg := NewAggregator(
		WithOrderFlow(&mockOrderFlow{}),
	)
	result := agg.Fetch(context.Background(), "BTCUSDT")
	if result.OrderFlow == nil {
		t.Fatal("expected order flow data")
	}
	if result.OrderFlow.BuySellRatio != 1.2 {
		t.Errorf("expected 1.2, got %.2f", result.OrderFlow.BuySellRatio)
	}
}

func TestAggregatorWithSentiment(t *testing.T) {
	agg := NewAggregator(
		WithSentiment(&mockSentiment{}),
	)
	result := agg.Fetch(context.Background(), "BTCUSDT")
	if result.Sentiment == nil {
		t.Fatal("expected sentiment data")
	}
	if result.Sentiment.FearGreedIndex != 55 {
		t.Errorf("expected 55, got %d", result.Sentiment.FearGreedIndex)
	}
}

func TestAggregatorWithFundingRate(t *testing.T) {
	agg := NewAggregator(
		WithFundingRate(&mockFunding{}),
	)
	result := agg.Fetch(context.Background(), "BTCUSDT")
	if result.FundingRate == nil {
		t.Fatal("expected funding data")
	}
	if result.FundingRate.MaxRate != 0.0001 {
		t.Errorf("expected 0.0001, got %f", result.FundingRate.MaxRate)
	}
}

func TestAggregatorAllProviders(t *testing.T) {
	agg := NewAggregator(
		WithOrderFlow(&mockOrderFlow{}),
		WithSentiment(&mockSentiment{}),
		WithFundingRate(&mockFunding{}),
		WithOnChain(&mockOnChain{}),
	)
	result := agg.Fetch(context.Background(), "BTCUSDT")
	if result.OrderFlow == nil {
		t.Error("missing order flow")
	}
	if result.Sentiment == nil {
		t.Error("missing sentiment")
	}
	if result.FundingRate == nil {
		t.Error("missing funding rate")
	}
	if result.OnChain == nil {
		t.Error("missing on-chain")
	}
}

// mocks

type mockOrderFlow struct{}

func (m *mockOrderFlow) GetSnapshot(_ context.Context, _ string) (*OrderFlowSnapshot, error) {
	return &OrderFlowSnapshot{
		BuyVolume:      1000,
		SellVolume:     833,
		BuySellRatio:   1.2,
		DepthImbalance: 0.15,
		SpreadBps:      0.5,
	}, nil
}

type mockSentiment struct{}

func (m *mockSentiment) GetSentiment(_ context.Context, _ string) (*AggregatedSentiment, error) {
	return &AggregatedSentiment{
		OverallScore:   0.3,
		OverallLabel:   "BULLISH",
		FearGreedIndex: 55,
	}, nil
}

type mockFunding struct{}

func (m *mockFunding) GetFundingRates(_ context.Context, _ string) (*FundingRateData, error) {
	return &FundingRateData{
		Rates:      map[string]float64{"binance": 0.0001},
		MaxRate:    0.0001,
		MinRate:    0.0001,
		Annualized: 10.95,
	}, nil
}

type mockOnChain struct{}

func (m *mockOnChain) GetMetrics(_ context.Context, _ string) (*OnChainMetrics, error) {
	return &OnChainMetrics{
		ActiveAddresses24h: 500000,
		NetFlow:            -1000,
		WhaleTransactions:  42,
	}, nil
}

func (m *mockOnChain) SupportedSymbols() []string {
	return []string{"BTCUSDT", "ETHUSDT"}
}
