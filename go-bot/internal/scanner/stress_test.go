package scanner

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/pipeline"
	"github.com/trading-bot/go-bot/internal/preferences"
	"github.com/trading-bot/go-bot/internal/user"
	"github.com/trading-bot/go-bot/internal/watchlist"
)

// simulate 100 concurrent users with watchlists to verify
// the scanner handles load without data races or panics.
// Run with: go test -race -run TestStress100Users -count=1 -timeout 60s

const stressUserCount = 100
const symbolsPerUser = 5

type stressUserProvider struct{ users []*user.User }
type stressWatchlistProvider struct{}
type stressPrefsProvider struct{}
type stressAnalyzer struct{ callCount int64 }
type stressNotifier struct {
	mu    sync.Mutex
	count int
}

func (p *stressUserProvider) ListActive(_ context.Context) ([]*user.User, error) {
	return p.users, nil
}

func (p *stressWatchlistProvider) List(_ context.Context, userID int) ([]watchlist.Item, error) {
	items := make([]watchlist.Item, symbolsPerUser)
	for i := 0; i < symbolsPerUser; i++ {
		items[i] = watchlist.Item{
			Symbol:   fmt.Sprintf("STRESS%dUSDT", i),
			IsActive: true,
		}
	}
	return items, nil
}

func (p *stressPrefsProvider) GetScanning(_ context.Context, _ int) (*preferences.Scanning, error) {
	return &preferences.Scanning{
		IsScanningEnabled: true,
		MinConfidence:     50,
	}, nil
}

func (p *stressPrefsProvider) GetNotification(_ context.Context, _ int) (*preferences.Notification, error) {
	return &preferences.Notification{
		OpportunityNotifications: true,
		MaxDailyNotifications:    100,
	}, nil
}

func (a *stressAnalyzer) Analyze(_ context.Context, symbol string) (*pipeline.Result, error) {
	atomic.AddInt64(&a.callCount, 1)
	return &pipeline.Result{
		Symbol: symbol,
		Decision: &claude.Decision{
			Action:     claude.ActionBuy,
			Confidence: 85,
			Reasoning:  "stress test signal",
		},
	}, nil
}

func (n *stressNotifier) NotifyTelegram(_ int64, _ string) error {
	n.mu.Lock()
	n.count++
	n.mu.Unlock()
	return nil
}

func (n *stressNotifier) NotifyDiscord(_ string, _, _ string, _ []pipeline.DiscordField, _ int) error {
	n.mu.Lock()
	n.count++
	n.mu.Unlock()
	return nil
}

func (n *stressNotifier) NotifyWhatsApp(_ string, _ string) error {
	n.mu.Lock()
	n.count++
	n.mu.Unlock()
	return nil
}

func TestStress100Users(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	users := make([]*user.User, stressUserCount)
	for i := 0; i < stressUserCount; i++ {
		tgID := int64(100000 + i)
		users[i] = &user.User{
			ID:          i + 1,
			TelegramID:  &tgID,
			IsActivated: true,
		}
	}

	analyzer := &stressAnalyzer{}
	notifier := &stressNotifier{}

	s := New(
		&stressUserProvider{users: users},
		&stressWatchlistProvider{},
		&stressPrefsProvider{},
		analyzer,
		notifier,
		Config{
			Interval:             100 * time.Millisecond, // fast for testing
			DefaultMaxDaily:      100,
			DefaultMinConfidence: 50,
			DuplicateWindowMins:  0, // disable dedup for stress test
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// run 3 cycles manually
	start := time.Now()
	for i := 0; i < 3; i++ {
		s.runCycle(ctx)
	}
	elapsed := time.Since(start)

	calls := atomic.LoadInt64(&analyzer.callCount)
	expectedCalls := int64(stressUserCount * symbolsPerUser * 3)

	t.Logf("stress test: %d users × %d symbols × 3 cycles = %d analyses in %s",
		stressUserCount, symbolsPerUser, calls, elapsed)

	if calls != expectedCalls {
		t.Fatalf("expected %d analysis calls, got %d", expectedCalls, calls)
	}

	notifier.mu.Lock()
	notifCount := notifier.count
	notifier.mu.Unlock()

	t.Logf("notifications sent: %d", notifCount)

	if s.CycleCount() != 3 {
		t.Fatalf("expected 3 cycles, got %d", s.CycleCount())
	}
}
