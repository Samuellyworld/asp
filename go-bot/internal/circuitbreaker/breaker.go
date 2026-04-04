// portfolio-level circuit breaker. halts all trading for a user when cumulative
// losses exceed configurable thresholds. uses a standard closed/open/half-open
// state machine with cooldown periods.
package circuitbreaker

import (
	"fmt"
	"sync"
	"time"
)

// breaker states
type State int

const (
	StateClosed   State = iota // normal — trading allowed
	StateOpen                  // tripped — trading halted
	StateHalfOpen              // cooldown expired — allow one probe trade
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// Config holds thresholds and timing for the circuit breaker.
type Config struct {
	MaxDailyLoss        float64       // max cumulative loss in USD per day before tripping
	MaxConsecutiveLosses int          // consecutive losing trades before tripping
	CooldownDuration    time.Duration // how long to stay open before moving to half-open
}

func DefaultConfig() Config {
	return Config{
		MaxDailyLoss:         100,
		MaxConsecutiveLosses: 5,
		CooldownDuration:     1 * time.Hour,
	}
}

// per-user breaker state
type userState struct {
	state             State
	dailyLoss         float64
	consecutiveLosses int
	trippedAt         time.Time
	lastResetDate     string // "2006-01-02"
	tripReason        string
}

// Breaker is a portfolio-level circuit breaker that tracks cumulative losses
// per user and halts trading when thresholds are exceeded.
type Breaker struct {
	mu     sync.Mutex
	config Config
	users  map[int]*userState
	now    func() time.Time // injectable clock for testing
}

// New creates a circuit breaker with the given config.
func New(config Config) *Breaker {
	return &Breaker{
		config: config,
		users:  make(map[int]*userState),
		now:    time.Now,
	}
}

// AllowTrade checks whether a user is allowed to open a new trade.
// Returns true if trading is allowed, false with a reason if blocked.
func (b *Breaker) AllowTrade(userID int) (bool, string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	us := b.getOrCreate(userID)
	b.maybeResetDay(us)

	switch us.state {
	case StateClosed:
		return true, ""
	case StateOpen:
		// check if cooldown has expired
		if b.now().Sub(us.trippedAt) >= b.config.CooldownDuration {
			us.state = StateHalfOpen
			return true, ""
		}
		remaining := b.config.CooldownDuration - b.now().Sub(us.trippedAt)
		return false, fmt.Sprintf("circuit breaker open: %s (cooldown %s remaining)", us.tripReason, remaining.Truncate(time.Second))
	case StateHalfOpen:
		// allow exactly one probe trade — transition happens in RecordTrade
		return true, ""
	}
	return false, "unknown breaker state"
}

// RecordTrade records a completed trade's PnL and updates the breaker state.
// Negative pnl = loss, positive = profit.
func (b *Breaker) RecordTrade(userID int, pnl float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	us := b.getOrCreate(userID)
	b.maybeResetDay(us)

	if pnl < 0 {
		us.dailyLoss += -pnl // store as positive
		us.consecutiveLosses++
	} else {
		us.consecutiveLosses = 0
	}

	switch us.state {
	case StateClosed:
		// check if we should trip
		if b.config.MaxDailyLoss > 0 && us.dailyLoss >= b.config.MaxDailyLoss {
			b.trip(us, fmt.Sprintf("daily loss $%.2f >= limit $%.2f", us.dailyLoss, b.config.MaxDailyLoss))
		} else if b.config.MaxConsecutiveLosses > 0 && us.consecutiveLosses >= b.config.MaxConsecutiveLosses {
			b.trip(us, fmt.Sprintf("%d consecutive losses >= limit %d", us.consecutiveLosses, b.config.MaxConsecutiveLosses))
		}
	case StateHalfOpen:
		if pnl >= 0 {
			// probe trade succeeded — close the breaker
			us.state = StateClosed
			us.consecutiveLosses = 0
		} else {
			// probe trade failed — re-trip
			b.trip(us, fmt.Sprintf("probe trade lost $%.2f, re-tripping", -pnl))
		}
	case StateOpen:
		// shouldn't normally record trades while open, but handle gracefully
	}
}

// State returns the current breaker state for a user.
func (b *Breaker) State(userID int) State {
	b.mu.Lock()
	defer b.mu.Unlock()

	us := b.getOrCreate(userID)
	b.maybeResetDay(us)

	// also check for cooldown expiry
	if us.state == StateOpen && b.now().Sub(us.trippedAt) >= b.config.CooldownDuration {
		us.state = StateHalfOpen
	}
	return us.state
}

// Stats returns the current loss tracking stats for a user.
func (b *Breaker) Stats(userID int) (dailyLoss float64, consecutiveLosses int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	us := b.getOrCreate(userID)
	b.maybeResetDay(us)
	return us.dailyLoss, us.consecutiveLosses
}

// Reset manually resets the breaker back to closed for a user.
func (b *Breaker) Reset(userID int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	us := b.getOrCreate(userID)
	us.state = StateClosed
	us.dailyLoss = 0
	us.consecutiveLosses = 0
	us.tripReason = ""
	us.lastResetDate = b.now().Format("2006-01-02")
}

func (b *Breaker) trip(us *userState, reason string) {
	us.state = StateOpen
	us.trippedAt = b.now()
	us.tripReason = reason
}

func (b *Breaker) getOrCreate(userID int) *userState {
	if us, ok := b.users[userID]; ok {
		return us
	}
	us := &userState{
		state:         StateClosed,
		lastResetDate: b.now().Format("2006-01-02"),
	}
	b.users[userID] = us
	return us
}

// resets daily counters when the date changes
func (b *Breaker) maybeResetDay(us *userState) {
	today := b.now().Format("2006-01-02")
	if us.lastResetDate != today {
		us.dailyLoss = 0
		us.lastResetDate = today
		// keep consecutive losses across days — they represent a streak
		// keep state — if breaker is open, it stays open until cooldown
	}
}
