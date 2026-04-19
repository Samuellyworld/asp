// setup wizard manages the step-by-step api key onboarding flow
package user

import (
	"fmt"
	"sync"
)

// wizard step identifiers
const (
	StepNone      = ""
	StepExchange  = "exchange"
	StepAPIKey    = "api_key"
	StepAPISecret = "api_secret"
	StepConfirm   = "confirm"
)

// wizardState tracks a user's progress through the setup flow
type WizardState struct {
	UserID    int
	Step      string
	Exchange  string // selected exchange name (e.g. "binance", "bybit")
	APIKey    string
	APISecret string
}

// setupWizard manages in-progress setup sessions
type SetupWizard struct {
	mu       sync.RWMutex
	sessions map[int64]*WizardState // keyed by telegram id
}

func NewSetupWizard() *SetupWizard {
	return &SetupWizard{
		sessions: make(map[int64]*WizardState),
	}
}

// start begins a new setup session for a user
func (w *SetupWizard) Start(telegramID int64, userID int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sessions[telegramID] = &WizardState{
		UserID: userID,
		Step:   StepExchange,
	}
}

// SetExchange stores the chosen exchange and advances to API key step
func (w *SetupWizard) SetExchange(telegramID int64, exchange string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	session, ok := w.sessions[telegramID]
	if !ok || session.Step != StepExchange {
		return fmt.Errorf("no active setup session expecting exchange selection")
	}

	session.Exchange = exchange
	session.Step = StepAPIKey
	return nil
}

// getSession returns the current wizard state for a user (nil if not in setup)
func (w *SetupWizard) GetSession(telegramID int64) *WizardState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.sessions[telegramID]
}

// cancel removes a user's setup session
func (w *SetupWizard) Cancel(telegramID int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if session, ok := w.sessions[telegramID]; ok {
		// clear sensitive data
		session.APIKey = ""
		session.APISecret = ""
		delete(w.sessions, telegramID)
	}
}

// setAPIKey stores the api key and advances to the next step
func (w *SetupWizard) SetAPIKey(telegramID int64, apiKey string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	session, ok := w.sessions[telegramID]
	if !ok || session.Step != StepAPIKey {
		return fmt.Errorf("no active setup session expecting api key")
	}

	session.APIKey = apiKey
	session.Step = StepAPISecret
	return nil
}

// setAPISecret stores the api secret and advances to confirm step
func (w *SetupWizard) SetAPISecret(telegramID int64, apiSecret string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	session, ok := w.sessions[telegramID]
	if !ok || session.Step != StepAPISecret {
		return fmt.Errorf("no active setup session expecting api secret")
	}

	session.APISecret = apiSecret
	session.Step = StepConfirm
	return nil
}

// complete finalizes and clears the session, returning the collected credentials
func (w *SetupWizard) Complete(telegramID int64) (userID int, exchange, apiKey, apiSecret string, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	session, ok := w.sessions[telegramID]
	if !ok || session.Step != StepConfirm {
		return 0, "", "", "", fmt.Errorf("no active setup session ready to complete")
	}

	userID = session.UserID
	exchange = session.Exchange
	apiKey = session.APIKey
	apiSecret = session.APISecret

	// clear sensitive data from memory
	session.APIKey = ""
	session.APISecret = ""
	delete(w.sessions, telegramID)

	return userID, exchange, apiKey, apiSecret, nil
}

// isInSetup checks if a user is currently in the setup flow
func (w *SetupWizard) IsInSetup(telegramID int64) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	_, ok := w.sessions[telegramID]
	return ok
}
