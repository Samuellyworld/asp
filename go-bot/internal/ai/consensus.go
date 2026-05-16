// package ai contains provider composition for trading analysis.
package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
)

type Provider interface {
	Analyze(ctx context.Context, input *claude.AnalysisInput) (*claude.Decision, error)
}

type ConsensusProvider struct {
	Primary   Provider
	Secondary Provider
}

func NewConsensusProvider(primary, secondary Provider) *ConsensusProvider {
	return &ConsensusProvider{Primary: primary, Secondary: secondary}
}

func (p *ConsensusProvider) Analyze(ctx context.Context, input *claude.AnalysisInput) (*claude.Decision, error) {
	if p.Primary == nil && p.Secondary == nil {
		return nil, fmt.Errorf("no ai providers configured")
	}
	if p.Primary == nil {
		return p.Secondary.Analyze(ctx, input)
	}
	if p.Secondary == nil {
		return p.Primary.Analyze(ctx, input)
	}

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)

	var primaryDecision, secondaryDecision *claude.Decision
	var primaryErr, secondaryErr error

	go func() {
		defer wg.Done()
		primaryDecision, primaryErr = p.Primary.Analyze(ctx, input)
	}()
	go func() {
		defer wg.Done()
		secondaryDecision, secondaryErr = p.Secondary.Analyze(ctx, input)
	}()
	wg.Wait()

	if primaryErr != nil && secondaryErr != nil {
		return nil, fmt.Errorf("all ai providers failed: primary=%v; secondary=%v", primaryErr, secondaryErr)
	}
	if primaryErr != nil {
		secondaryDecision.Reasoning = appendProviderNote(secondaryDecision.Reasoning, "primary AI failed; using secondary analysis only")
		return secondaryDecision, nil
	}
	if secondaryErr != nil {
		primaryDecision.Reasoning = appendProviderNote(primaryDecision.Reasoning, "secondary AI failed; using primary analysis only")
		return primaryDecision, nil
	}

	decision := mergeDecisions(primaryDecision, secondaryDecision)
	decision.Timestamp = time.Now()
	decision.Latency = time.Since(start)
	return decision, nil
}

func mergeDecisions(primary, secondary *claude.Decision) *claude.Decision {
	if primary == nil {
		return secondary
	}
	if secondary == nil {
		return primary
	}

	if primary.Action == secondary.Action {
		d := *primary
		d.Confidence = (primary.Confidence + secondary.Confidence) / 2
		d.Reasoning = appendProviderNote(primary.Reasoning, fmt.Sprintf("OpenAI cross-check agreed (%s, %.0f%% confidence).", secondary.Action, secondary.Confidence))
		return &d
	}

	if primary.Action == claude.ActionHold {
		d := *primary
		d.Confidence = 0
		d.Reasoning = fmt.Sprintf("Consensus blocked trade: primary said HOLD while secondary said %s. %s", secondary.Action, compactReason(primary.Reasoning))
		return &d
	}
	if secondary.Action == claude.ActionHold {
		d := *secondary
		d.Confidence = 0
		d.Reasoning = fmt.Sprintf("Consensus blocked trade: secondary said HOLD while primary said %s. %s", primary.Action, compactReason(secondary.Reasoning))
		return &d
	}

	return &claude.Decision{
		Action:     claude.ActionHold,
		Confidence: 0,
		Reasoning:  fmt.Sprintf("Consensus blocked trade: models disagreed (%s vs %s).", primary.Action, secondary.Action),
	}
}

func appendProviderNote(reasoning, note string) string {
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return note
	}
	return reasoning + " " + note
}

func compactReason(reasoning string) string {
	reasoning = strings.TrimSpace(reasoning)
	if len(reasoning) <= 140 {
		return reasoning
	}
	return reasoning[:137] + "..."
}
