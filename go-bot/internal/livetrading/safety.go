// pre-trade safety checks for live trading.
// validates balance, position limits, daily loss, order size, and exchange health
// before allowing any real order to be placed.
package livetrading

import (
	"fmt"
	"sync"
	"time"
)

// a single check that must pass before trading is allowed
type CheckResult struct {
	Name    string
	Passed  bool
	Message string
}

// aggregate result of all pre-trade checks
type SafetyResult struct {
	Passed  bool
	Checks  []CheckResult
	Blocked string // reason if blocked
}

// safety limits configuration
type SafetyConfig struct {
	MaxPositionSize    float64 // max single position in usd
	MaxOpenPositions   int     // max concurrent open positions
	DailyLossLimit     float64 // max cumulative daily loss in usd
	MinOrderSize       float64 // minimum order size in usd
	MaxOrderSize       float64 // maximum order size in usd
	RequireConfirmation bool   // user must confirm risk acknowledgment
}

func DefaultSafetyConfig() SafetyConfig {
	return SafetyConfig{
		MaxPositionSize:    100,
		MaxOpenPositions:   3,
		DailyLossLimit:     50,
		MinOrderSize:       10,
		MaxOrderSize:       100,
		RequireConfirmation: true,
	}
}

// provides balance information for safety checks
type BalanceProvider interface {
	GetAvailableBalance(userID int, asset string) (float64, error)
}

// provides current position state for limit checks
type PositionCounter interface {
	OpenPositionCount(userID int) int
}

// provides daily realized loss for loss limit checks
type LossTracker interface {
	DailyLoss(userID int, date time.Time) float64
	RecordLoss(userID int, amount float64)
}

// validates whether a trade can proceed safely
type SafetyChecker struct {
	config    SafetyConfig
	balance   BalanceProvider
	positions PositionCounter
	losses    LossTracker
	confirm   *ConfirmationManager
}

func NewSafetyChecker(config SafetyConfig, balance BalanceProvider, positions PositionCounter, losses LossTracker, confirm *ConfirmationManager) *SafetyChecker {
	return &SafetyChecker{
		config:    config,
		balance:   balance,
		positions: positions,
		losses:    losses,
		confirm:   confirm,
	}
}

// runs all pre-trade safety checks and returns the aggregate result
func (s *SafetyChecker) Check(userID int, symbol string, positionSize float64, asset string) SafetyResult {
	var checks []CheckResult
	allPassed := true

	// check 1: risk confirmation required
	if s.config.RequireConfirmation && s.confirm != nil {
		confirmed := s.confirm.IsConfirmed(userID)
		check := CheckResult{
			Name:   "risk_confirmation",
			Passed: confirmed,
		}
		if confirmed {
			check.Message = "risk acknowledged"
		} else {
			check.Message = "user has not confirmed trading risks"
			allPassed = false
		}
		checks = append(checks, check)
	}

	// check 2: order size within bounds
	sizeCheck := CheckResult{Name: "order_size"}
	if positionSize < s.config.MinOrderSize {
		sizeCheck.Passed = false
		sizeCheck.Message = fmt.Sprintf("order size $%.2f below minimum $%.2f", positionSize, s.config.MinOrderSize)
		allPassed = false
	} else if positionSize > s.config.MaxOrderSize {
		sizeCheck.Passed = false
		sizeCheck.Message = fmt.Sprintf("order size $%.2f exceeds maximum $%.2f", positionSize, s.config.MaxOrderSize)
		allPassed = false
	} else {
		sizeCheck.Passed = true
		sizeCheck.Message = fmt.Sprintf("order size $%.2f within limits", positionSize)
	}
	checks = append(checks, sizeCheck)

	// check 3: position size limit
	posCheck := CheckResult{Name: "position_size"}
	if positionSize > s.config.MaxPositionSize {
		posCheck.Passed = false
		posCheck.Message = fmt.Sprintf("position $%.2f exceeds max $%.2f", positionSize, s.config.MaxPositionSize)
		allPassed = false
	} else {
		posCheck.Passed = true
		posCheck.Message = fmt.Sprintf("position $%.2f within limit", positionSize)
	}
	checks = append(checks, posCheck)

	// check 4: open position count
	countCheck := CheckResult{Name: "position_count"}
	if s.positions != nil {
		count := s.positions.OpenPositionCount(userID)
		if count >= s.config.MaxOpenPositions {
			countCheck.Passed = false
			countCheck.Message = fmt.Sprintf("%d open positions (max %d)", count, s.config.MaxOpenPositions)
			allPassed = false
		} else {
			countCheck.Passed = true
			countCheck.Message = fmt.Sprintf("%d/%d positions used", count, s.config.MaxOpenPositions)
		}
	} else {
		countCheck.Passed = true
		countCheck.Message = "position tracking not available"
	}
	checks = append(checks, countCheck)

	// check 5: sufficient balance
	balCheck := CheckResult{Name: "balance"}
	if s.balance != nil {
		available, err := s.balance.GetAvailableBalance(userID, asset)
		if err != nil {
			balCheck.Passed = false
			balCheck.Message = fmt.Sprintf("failed to check balance: %v", err)
			allPassed = false
		} else if available < positionSize {
			balCheck.Passed = false
			balCheck.Message = fmt.Sprintf("insufficient balance: $%.2f available, need $%.2f", available, positionSize)
			allPassed = false
		} else {
			balCheck.Passed = true
			balCheck.Message = fmt.Sprintf("$%.2f available", available)
		}
	} else {
		balCheck.Passed = true
		balCheck.Message = "balance check not available"
	}
	checks = append(checks, balCheck)

	// check 6: daily loss limit
	lossCheck := CheckResult{Name: "daily_loss"}
	if s.losses != nil {
		dailyLoss := s.losses.DailyLoss(userID, time.Now())
		remaining := s.config.DailyLossLimit - dailyLoss
		if remaining <= 0 {
			lossCheck.Passed = false
			lossCheck.Message = fmt.Sprintf("daily loss limit reached: $%.2f lost (limit $%.2f)", dailyLoss, s.config.DailyLossLimit)
			allPassed = false
		} else {
			lossCheck.Passed = true
			lossCheck.Message = fmt.Sprintf("$%.2f loss remaining today", remaining)
		}
	} else {
		lossCheck.Passed = true
		lossCheck.Message = "loss tracking not available"
	}
	checks = append(checks, lossCheck)

	result := SafetyResult{
		Passed: allPassed,
		Checks: checks,
	}

	if !allPassed {
		for _, c := range checks {
			if !c.Passed {
				result.Blocked = c.Message
				break
			}
		}
	}

	return result
}

// in-memory daily loss tracker
type InMemoryLossTracker struct {
	mu     sync.Mutex
	losses map[int]map[string]float64 // userID -> date -> cumulative loss
}

func NewLossTracker() *InMemoryLossTracker {
	return &InMemoryLossTracker{
		losses: make(map[int]map[string]float64),
	}
}

func (t *InMemoryLossTracker) DailyLoss(userID int, date time.Time) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := date.Format("2006-01-02")
	if userLosses, ok := t.losses[userID]; ok {
		return userLosses[key]
	}
	return 0
}

func (t *InMemoryLossTracker) RecordLoss(userID int, amount float64) {
	if amount >= 0 {
		return // only track actual losses
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	key := time.Now().Format("2006-01-02")
	if _, ok := t.losses[userID]; !ok {
		t.losses[userID] = make(map[string]float64)
	}
	t.losses[userID][key] += -amount // store as positive value
}

// resets daily loss counters (called at midnight)
func (t *InMemoryLossTracker) ResetDaily() {
	t.mu.Lock()
	defer t.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	for uid := range t.losses {
		for date := range t.losses[uid] {
			if date != today {
				delete(t.losses[uid], date)
			}
		}
	}
}
