// background market scanner that periodically analyzes watchlist symbols
// for all active users and sends notifications on high-confidence opportunities
package scanner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/trading-bot/go-bot/internal/claude"
	"github.com/trading-bot/go-bot/internal/pipeline"
	"github.com/trading-bot/go-bot/internal/preferences"
	"github.com/trading-bot/go-bot/internal/user"
	"github.com/trading-bot/go-bot/internal/watchlist"
)

// provides user listing for the scanner
type UserProvider interface {
	ListActive(ctx context.Context) ([]*user.User, error)
}

// provides watchlist access
type WatchlistProvider interface {
	List(ctx context.Context, userID int) ([]watchlist.Item, error)
}

// provides user scanning and notification preferences
type PreferencesProvider interface {
	GetScanning(ctx context.Context, userID int) (*preferences.Scanning, error)
	GetNotification(ctx context.Context, userID int) (*preferences.Notification, error)
}

// runs the analysis pipeline for a symbol
type Analyzer interface {
	Analyze(ctx context.Context, symbol string) (*pipeline.Result, error)
}

// sends notifications to users
type Notifier interface {
	NotifyTelegram(chatID int64, message string) error
	NotifyDiscord(channelID string, title, description string, fields []pipeline.DiscordField, color int) error
}

// tracks a recent notification to prevent duplicates
type recentNotification struct {
	symbol    string
	direction claude.Action
	sentAt    time.Time
}

// per-user daily notification count and recent history
type userState struct {
	dailyCount    int
	recentNotifs  []recentNotification
}

// configuration for the scanner
type Config struct {
	Interval              time.Duration
	DefaultMaxDaily       int
	DefaultMinConfidence  int
	DuplicateWindowMins   int
}

// returns sensible defaults
func DefaultConfig() Config {
	return Config{
		Interval:             5 * time.Minute,
		DefaultMaxDaily:      10,
		DefaultMinConfidence: 80,
		DuplicateWindowMins:  60,
	}
}

// background scanner that monitors markets for all users
type Scanner struct {
	users       UserProvider
	watchlists  WatchlistProvider
	prefs       PreferencesProvider
	analyzer    Analyzer
	notifier    Notifier
	config      Config

	mu          sync.RWMutex
	states      map[int]*userState // keyed by user id
	running     bool
	stopOnce    sync.Once
	stopCh      chan struct{}
	cycleCount  int
	lastCycleAt time.Time

	// for testing: override time.Now
	nowFunc func() time.Time
}

// creates a new scanner
func New(
	users UserProvider,
	watchlists WatchlistProvider,
	prefs PreferencesProvider,
	analyzer Analyzer,
	notifier Notifier,
	cfg Config,
) *Scanner {
	return &Scanner{
		users:      users,
		watchlists: watchlists,
		prefs:      prefs,
		analyzer:   analyzer,
		notifier:   notifier,
		config:     cfg,
		states:     make(map[int]*userState),
		stopCh:     make(chan struct{}),
		nowFunc:    time.Now,
	}
}

// starts the scanner loop in a goroutine. returns immediately.
func (s *Scanner) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go s.loop(ctx)
	go s.midnightResetLoop(ctx)
}

// signals the scanner to stop (safe to call multiple times)
func (s *Scanner) Stop() {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		close(s.stopCh)
	})
}

// returns true if the scanner is running
func (s *Scanner) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// returns the number of completed scan cycles
func (s *Scanner) CycleCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cycleCount
}

// returns daily notification count for a user
func (s *Scanner) DailyCount(userID int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if st, ok := s.states[userID]; ok {
		return st.dailyCount
	}
	return 0
}

// main scanning loop
func (s *Scanner) loop(ctx context.Context) {
	// run immediately on start
	s.runCycle(ctx)

	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.runCycle(ctx)
		}
	}
}

// runs one full scan cycle across all users
func (s *Scanner) runCycle(ctx context.Context) {
	start := s.now()

	users, err := s.users.ListActive(ctx)
	if err != nil {
		slog.Error("scanner: failed to list users", "error", err)
		return
	}

	totalSymbols := 0
	opportunities := 0

	for _, u := range users {
		if u.IsBanned || !u.IsActivated {
			continue
		}

		scanPrefs, err := s.prefs.GetScanning(ctx, u.ID)
		if err != nil || scanPrefs == nil {
			continue
		}
		if !scanPrefs.IsScanningEnabled {
			continue
		}

		notifPrefs, err := s.prefs.GetNotification(ctx, u.ID)
		if err != nil {
			continue
		}

		items, err := s.watchlists.List(ctx, u.ID)
		if err != nil {
			slog.Error("scanner: failed to list watchlist", "user_id", u.ID, "error", err)
			continue
		}

		for _, item := range items {
			if !item.IsActive {
				continue
			}
			totalSymbols++

			// check daily limit before doing work
			maxDaily := s.config.DefaultMaxDaily
			if notifPrefs != nil && notifPrefs.MaxDailyNotifications > 0 {
				maxDaily = notifPrefs.MaxDailyNotifications
			}
			if s.dailyLimitReached(u.ID, maxDaily) {
				break // stop scanning for this user
			}

			opp := s.analyzeAndNotify(ctx, u, item.Symbol, scanPrefs, notifPrefs)
			if opp {
				opportunities++
			}
		}
	}

	s.mu.Lock()
	s.cycleCount++
	s.lastCycleAt = s.now()
	s.mu.Unlock()

	elapsed := s.now().Sub(start)
	slog.Info("scanner: cycle complete",
		"cycle", s.cycleCount,
		"users", len(users),
		"symbols", totalSymbols,
		"opportunities", opportunities,
		"elapsed", elapsed.Round(time.Millisecond),
	)
}

// analyzes one symbol for one user and sends notification if warranted
func (s *Scanner) analyzeAndNotify(
	ctx context.Context,
	u *user.User,
	symbol string,
	scanPrefs *preferences.Scanning,
	notifPrefs *preferences.Notification,
) bool {
	// skip if opportunity notifications disabled
	if notifPrefs != nil && !notifPrefs.OpportunityNotifications {
		return false
	}

	result, err := s.analyzer.Analyze(ctx, symbol)
	if err != nil {
		slog.Error("scanner: analysis failed", "symbol", symbol, "user_id", u.ID, "error", err)
		return false
	}

	if result.Decision == nil {
		return false
	}

	// only notify on actionable decisions (buy/sell)
	if result.Decision.Action == claude.ActionHold {
		return false
	}

	// check confidence threshold
	minConf := float64(s.config.DefaultMinConfidence)
	if scanPrefs != nil && scanPrefs.MinConfidence > 0 {
		minConf = float64(scanPrefs.MinConfidence)
	}
	if result.Decision.Confidence < minConf {
		return false
	}

	// check duplicate suppression
	if s.isDuplicate(u.ID, symbol, result.Decision.Action) {
		return false
	}

	// check daily limit
	maxDaily := s.config.DefaultMaxDaily
	if notifPrefs != nil && notifPrefs.MaxDailyNotifications > 0 {
		maxDaily = notifPrefs.MaxDailyNotifications
	}
	if s.dailyLimitReached(u.ID, maxDaily) {
		return false
	}

	// send notifications
	s.sendNotifications(u, result)

	// record the notification
	s.recordNotification(u.ID, symbol, result.Decision.Action)

	return true
}

// sends formatted notifications to the user's connected platforms
func (s *Scanner) sendNotifications(u *user.User, result *pipeline.Result) {
	if u.TelegramID != nil {
		msg := pipeline.FormatTelegramMessage(result)
		header := fmt.Sprintf("🎯 *Opportunity Detected*\n\n%s", msg)
		if err := s.notifier.NotifyTelegram(*u.TelegramID, header); err != nil {
			slog.Error("scanner: telegram notify failed", "user_id", u.ID, "error", err)
		}
	}

	if u.DiscordID != nil {
		title, desc, fields, color := pipeline.FormatDiscordFields(result)
		channelID := fmt.Sprintf("%d", *u.DiscordID)
		if err := s.notifier.NotifyDiscord(channelID, title, desc, fields, color); err != nil {
			slog.Error("scanner: discord notify failed", "user_id", u.ID, "error", err)
		}
	}
}

// records a sent notification for daily counting and duplicate tracking
func (s *Scanner) recordNotification(userID int, symbol string, action claude.Action) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.states[userID]
	if !ok {
		st = &userState{}
		s.states[userID] = st
	}
	st.dailyCount++
	st.recentNotifs = append(st.recentNotifs, recentNotification{
		symbol:    symbol,
		direction: action,
		sentAt:    s.now(),
	})
}

// checks if the user has reached their daily notification limit
func (s *Scanner) dailyLimitReached(userID int, maxDaily int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st, ok := s.states[userID]
	if !ok {
		return false
	}
	return st.dailyCount >= maxDaily
}

// checks whether we already notified the same symbol+direction within the window
func (s *Scanner) isDuplicate(userID int, symbol string, action claude.Action) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st, ok := s.states[userID]
	if !ok {
		return false
	}

	window := time.Duration(s.config.DuplicateWindowMins) * time.Minute
	cutoff := s.now().Add(-window)

	for _, n := range st.recentNotifs {
		if n.symbol == symbol && n.direction == action && n.sentAt.After(cutoff) {
			return true
		}
	}
	return false
}

// resets all daily counters and prunes old recent notifications
func (s *Scanner) ResetDaily() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, st := range s.states {
		st.dailyCount = 0
	}

	// prune notifications older than the duplicate window
	window := time.Duration(s.config.DuplicateWindowMins) * time.Minute
	cutoff := s.now().Add(-window)
	for _, st := range s.states {
		pruned := st.recentNotifs[:0]
		for _, n := range st.recentNotifs {
			if n.sentAt.After(cutoff) {
				pruned = append(pruned, n)
			}
		}
		st.recentNotifs = pruned
	}
}

// runs a goroutine that resets daily counters at midnight (user timezone)
func (s *Scanner) midnightResetLoop(ctx context.Context) {
	for {
		now := s.now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		sleepDur := next.Sub(now)

		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-time.After(sleepDur):
			s.ResetDaily()
			slog.Info("scanner: daily counters reset at midnight")
		}
	}
}

// returns current time (overridable for tests)
func (s *Scanner) now() time.Time {
	return s.nowFunc()
}
