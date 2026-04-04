// emergency stop and risk confirmation flow for live trading.
// emergency stop closes all open positions immediately.
// confirmation manager tracks user risk acknowledgment state.
package livetrading

import (
	"fmt"
	"sync"
	"time"
)

// emergency stop handler - closes all positions for a user and disables trading
type EmergencyStop struct {
	executor *Executor
	onStop   func(userID int, closed []*LivePosition)
}

func NewEmergencyStop(executor *Executor) *EmergencyStop {
	return &EmergencyStop{executor: executor}
}

// closes all open positions for the user and returns the results.
// this is a best-effort operation - it will attempt to close every position
// even if some closures fail.
func (e *EmergencyStop) Execute(userID int) ([]*LivePosition, []error) {
	positions := e.executor.OpenPositions(userID)
	if len(positions) == 0 {
		return nil, nil
	}

	var closed []*LivePosition
	var errors []error

	for _, pos := range positions {
		p, err := e.executor.Close(pos.ID, "emergency_stop")
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to close %s (%s): %w", pos.ID, pos.Symbol, err))
		} else {
			closed = append(closed, p)
		}
	}

	if e.onStop != nil {
		e.onStop(userID, closed)
	}

	return closed, errors
}

// sets a callback for when emergency stop is triggered
func (e *EmergencyStop) OnStop(fn func(userID int, closed []*LivePosition)) {
	e.onStop = fn
}

// confirmation state for a user
type Confirmation struct {
	UserID      int
	Confirmed   bool
	ConfirmedAt time.Time
}

// requires users to explicitly acknowledge risks before live trading.
// users must type the exact confirmation phrase to enable live mode.
type ConfirmationManager struct {
	mu            sync.RWMutex
	confirmations map[int]*Confirmation
	phrase        string
}

// the phrase users must type to confirm risk acknowledgment
const DefaultConfirmPhrase = "I UNDERSTAND THE RISKS"

func NewConfirmationManager() *ConfirmationManager {
	return &ConfirmationManager{
		confirmations: make(map[int]*Confirmation),
		phrase:        DefaultConfirmPhrase,
	}
}

// attempts to confirm a user by checking their input against the required phrase
func (c *ConfirmationManager) Confirm(userID int, input string) bool {
	if input != c.phrase {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.confirmations[userID] = &Confirmation{
		UserID:      userID,
		Confirmed:   true,
		ConfirmedAt: time.Now(),
	}
	return true
}

// checks whether a user has confirmed risk acknowledgment
func (c *ConfirmationManager) IsConfirmed(userID int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	conf, ok := c.confirmations[userID]
	return ok && conf.Confirmed
}

// revokes a user's risk confirmation (e.g. after emergency stop)
func (c *ConfirmationManager) Revoke(userID int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.confirmations, userID)
}

// returns the required confirmation phrase
func (c *ConfirmationManager) Phrase() string {
	return c.phrase
}

// returns the confirmation state for a user
func (c *ConfirmationManager) GetConfirmation(userID int) *Confirmation {
	c.mu.RLock()
	defer c.mu.RUnlock()

	conf, ok := c.confirmations[userID]
	if !ok {
		return nil
	}
	return conf
}
