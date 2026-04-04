package dca

import (
	"context"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/opportunity"
	"github.com/trading-bot/go-bot/internal/pipeline"
)

func TestCreatePlan(t *testing.T) {
	exec := NewExecutor(DefaultConfig(), &mockPriceProvider{})
	opp := makeTestOpp()

	plan, err := exec.CreatePlan(opp)
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}
	if plan.Symbol != "BTCUSDT" {
		t.Errorf("expected BTCUSDT, got %s", plan.Symbol)
	}
	if len(plan.Rounds) != 3 {
		t.Errorf("expected 3 rounds, got %d", len(plan.Rounds))
	}
	if plan.Status != "active" {
		t.Errorf("expected active, got %s", plan.Status)
	}

	// check total size equals plan
	var total float64
	for _, r := range plan.Rounds {
		total += r.Size
	}
	if total < 299 || total > 301 {
		t.Errorf("expected total ~300, got %.2f", total)
	}
}

func TestCreatePlanMaxSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxTotalSize = 100
	exec := NewExecutor(cfg, &mockPriceProvider{})
	opp := makeTestOpp() // has PositionSize 300

	plan, err := exec.CreatePlan(opp)
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}
	var total float64
	for _, r := range plan.Rounds {
		total += r.Size
	}
	if total < 99 || total > 101 {
		t.Errorf("expected total capped at ~100, got %.2f", total)
	}
}

func TestComputeRoundSizesEqual(t *testing.T) {
	exec := NewExecutor(DefaultConfig(), nil)
	rounds := exec.computeRoundSizes(300)
	if len(rounds) != 3 {
		t.Fatalf("expected 3 rounds, got %d", len(rounds))
	}
	for _, r := range rounds {
		if r.Size < 99 || r.Size > 101 {
			t.Errorf("expected ~100 per round, got %.2f", r.Size)
		}
	}
}

func TestComputeRoundSizesScaled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ScaleFactor = 2.0 // 1:2:4 weighting
	exec := NewExecutor(cfg, nil)
	rounds := exec.computeRoundSizes(700)

	// weights: 1, 2, 4 = total 7
	// sizes: 100, 200, 400
	if rounds[0].Size < 99 || rounds[0].Size > 101 {
		t.Errorf("round 1: expected ~100, got %.2f", rounds[0].Size)
	}
	if rounds[1].Size < 199 || rounds[1].Size > 201 {
		t.Errorf("round 2: expected ~200, got %.2f", rounds[1].Size)
	}
	if rounds[2].Size < 399 || rounds[2].Size > 401 {
		t.Errorf("round 3: expected ~400, got %.2f", rounds[2].Size)
	}
}

func TestExecuteRound(t *testing.T) {
	exec := NewExecutor(DefaultConfig(), &mockPriceProvider{})
	opp := makeTestOpp()
	plan, _ := exec.CreatePlan(opp)

	placer := &mockOrderPlacer{}
	err := exec.ExecuteRound(context.Background(), plan, placer)
	if err != nil {
		t.Fatalf("ExecuteRound failed: %v", err)
	}
	if !plan.Rounds[0].Executed {
		t.Error("expected round 1 to be executed")
	}
	if plan.Rounds[0].Price != 50000 {
		t.Errorf("expected price 50000, got %.2f", plan.Rounds[0].Price)
	}
	if plan.AvgEntryPrice == 0 {
		t.Error("expected avg entry price to be updated")
	}
}

func TestExecuteAllRounds(t *testing.T) {
	exec := NewExecutor(DefaultConfig(), &mockPriceProvider{})
	opp := makeTestOpp()
	plan, _ := exec.CreatePlan(opp)
	placer := &mockOrderPlacer{}

	for i := 0; i < 3; i++ {
		err := exec.ExecuteRound(context.Background(), plan, placer)
		if err != nil {
			t.Fatalf("round %d failed: %v", i+1, err)
		}
	}

	if plan.Status != "completed" {
		t.Errorf("expected completed, got %s", plan.Status)
	}
	for i, r := range plan.Rounds {
		if !r.Executed {
			t.Errorf("round %d not executed", i+1)
		}
	}
}

func TestCancelPlan(t *testing.T) {
	exec := NewExecutor(DefaultConfig(), &mockPriceProvider{})
	opp := makeTestOpp()
	plan, _ := exec.CreatePlan(opp)

	if !exec.CancelPlan(plan.ID) {
		t.Error("expected cancel to succeed")
	}
	if plan.Status != "cancelled" {
		t.Errorf("expected cancelled, got %s", plan.Status)
	}
	if exec.CancelPlan(plan.ID) {
		t.Error("expected second cancel to fail")
	}
}

func TestGetPlanForOpportunity(t *testing.T) {
	exec := NewExecutor(DefaultConfig(), &mockPriceProvider{})
	opp := makeTestOpp()
	exec.CreatePlan(opp)

	plan := exec.GetPlanForOpportunity(opp.ID)
	if plan == nil {
		t.Fatal("expected plan")
	}
}

func TestActivePlans(t *testing.T) {
	exec := NewExecutor(DefaultConfig(), &mockPriceProvider{})
	exec.CreatePlan(makeTestOpp())

	active := exec.ActivePlans()
	if len(active) != 1 {
		t.Errorf("expected 1 active plan, got %d", len(active))
	}
}

func TestStatsEmpty(t *testing.T) {
	exec := NewExecutor(DefaultConfig(), nil)
	stats := exec.Stats()
	if stats["active"] != 0 {
		t.Error("expected 0 active")
	}
}

func TestRoundCallback(t *testing.T) {
	exec := NewExecutor(DefaultConfig(), &mockPriceProvider{})
	called := false
	exec.OnRound(func(plan *Plan, round *Round) {
		called = true
	})

	opp := makeTestOpp()
	plan, _ := exec.CreatePlan(opp)
	exec.ExecuteRound(context.Background(), plan, &mockOrderPlacer{})

	if !called {
		t.Error("expected callback to be called")
	}
}

// helpers

func makeTestOpp() *opportunity.Opportunity {
	return &opportunity.Opportunity{
		ID:     "opp_1_1",
		UserID: 1,
		Symbol: "BTCUSDT",
		Action: claude.ActionBuy,
		Status: opportunity.StatusApproved,
		Result: &pipeline.Result{
			Decision: &claude.Decision{
				Action:     claude.ActionBuy,
				Confidence: 80,
				Plan: claude.TradePlan{
					Entry:        50000,
					StopLoss:     49000,
					TakeProfit:   52000,
					PositionSize: 300,
					RiskReward:   2.0,
				},
			},
		},
		CreatedAt: time.Now(),
	}
}

type mockPriceProvider struct{}

func (m *mockPriceProvider) GetPrice(_ context.Context, symbol string) (*exchange.Ticker, error) {
	return &exchange.Ticker{Price: 50000, QuoteVolume: 1000000, ChangePct: 2.5}, nil
}

type mockOrderPlacer struct{}

func (m *mockOrderPlacer) PlaceMarketOrder(_ context.Context, symbol string, side exchange.OrderSide, quantity float64) (*exchange.Order, error) {
	return &exchange.Order{
		OrderID:     1,
		Symbol:      symbol,
		Side:        side,
		AvgPrice:    50000,
		ExecutedQty: quantity,
		Status:      exchange.OrderStatusFilled,
	}, nil
}
