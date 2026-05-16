package ai

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/trading-bot/go-bot/internal/claude"
)

type fakeProvider struct {
	decision *claude.Decision
	err      error
}

func (f fakeProvider) Analyze(context.Context, *claude.AnalysisInput) (*claude.Decision, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.decision, nil
}

func TestConsensusAgreementAveragesConfidence(t *testing.T) {
	p := NewConsensusProvider(
		fakeProvider{decision: &claude.Decision{Action: claude.ActionBuy, Confidence: 80, Reasoning: "primary"}},
		fakeProvider{decision: &claude.Decision{Action: claude.ActionBuy, Confidence: 70, Reasoning: "secondary"}},
	)
	d, err := p.Analyze(context.Background(), &claude.AnalysisInput{})
	if err != nil {
		t.Fatalf("Analyze() error: %v", err)
	}
	if d.Action != claude.ActionBuy || d.Confidence != 75 {
		t.Fatalf("decision = %+v", d)
	}
	if !strings.Contains(d.Reasoning, "agreed") {
		t.Fatalf("expected agreement note, got %q", d.Reasoning)
	}
}

func TestConsensusDisagreementBlocksTrade(t *testing.T) {
	p := NewConsensusProvider(
		fakeProvider{decision: &claude.Decision{Action: claude.ActionBuy, Confidence: 82}},
		fakeProvider{decision: &claude.Decision{Action: claude.ActionSell, Confidence: 79}},
	)
	d, err := p.Analyze(context.Background(), &claude.AnalysisInput{})
	if err != nil {
		t.Fatalf("Analyze() error: %v", err)
	}
	if d.Action != claude.ActionHold || d.Confidence != 0 {
		t.Fatalf("disagreement should HOLD, got %+v", d)
	}
}

func TestConsensusFallsBackWhenOneProviderFails(t *testing.T) {
	p := NewConsensusProvider(
		fakeProvider{err: fmt.Errorf("provider down")},
		fakeProvider{decision: &claude.Decision{Action: claude.ActionHold, Reasoning: "secondary"}},
	)
	d, err := p.Analyze(context.Background(), &claude.AnalysisInput{})
	if err != nil {
		t.Fatalf("Analyze() error: %v", err)
	}
	if d.Action != claude.ActionHold || !strings.Contains(d.Reasoning, "failed") {
		t.Fatalf("expected secondary fallback with note, got %+v", d)
	}
}
