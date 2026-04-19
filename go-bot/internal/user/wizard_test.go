package user

import (
	"testing"
)

func TestSetupWizard_FullFlow(t *testing.T) {
	w := NewSetupWizard()
	telegramID := int64(12345)
	userID := 1

	// should not be in setup initially
	if w.IsInSetup(telegramID) {
		t.Fatal("user should not be in setup initially")
	}

	// start setup
	w.Start(telegramID, userID)

	if !w.IsInSetup(telegramID) {
		t.Fatal("user should be in setup after Start()")
	}

	// check initial step
	session := w.GetSession(telegramID)
	if session == nil {
		t.Fatal("GetSession() returned nil")
	}
	if session.Step != StepExchange {
		t.Errorf("initial step = %q, want %q", session.Step, StepExchange)
	}
	if session.UserID != userID {
		t.Errorf("userID = %d, want %d", session.UserID, userID)
	}

	// set exchange
	if err := w.SetExchange(telegramID, "binance"); err != nil {
		t.Fatalf("SetExchange() error: %v", err)
	}

	session = w.GetSession(telegramID)
	if session.Step != StepAPIKey {
		t.Errorf("step after SetExchange = %q, want %q", session.Step, StepAPIKey)
	}

	// set api key
	if err := w.SetAPIKey(telegramID, "test-api-key"); err != nil {
		t.Fatalf("SetAPIKey() error: %v", err)
	}

	session = w.GetSession(telegramID)
	if session.Step != StepAPISecret {
		t.Errorf("step after SetAPIKey = %q, want %q", session.Step, StepAPISecret)
	}

	// set api secret
	if err := w.SetAPISecret(telegramID, "test-api-secret"); err != nil {
		t.Fatalf("SetAPISecret() error: %v", err)
	}

	session = w.GetSession(telegramID)
	if session.Step != StepConfirm {
		t.Errorf("step after SetAPISecret = %q, want %q", session.Step, StepConfirm)
	}

	// complete
	gotUserID, gotExchange, gotKey, gotSecret, err := w.Complete(telegramID)
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}

	if gotUserID != userID {
		t.Errorf("Complete() userID = %d, want %d", gotUserID, userID)
	}
	if gotExchange != "binance" {
		t.Errorf("Complete() exchange = %q, want %q", gotExchange, "binance")
	}
	if gotKey != "test-api-key" {
		t.Errorf("Complete() apiKey = %q, want %q", gotKey, "test-api-key")
	}
	if gotSecret != "test-api-secret" {
		t.Errorf("Complete() apiSecret = %q, want %q", gotSecret, "test-api-secret")
	}

	// should no longer be in setup
	if w.IsInSetup(telegramID) {
		t.Fatal("user should not be in setup after Complete()")
	}
}

func TestSetupWizard_Cancel(t *testing.T) {
	w := NewSetupWizard()
	telegramID := int64(12345)

	w.Start(telegramID, 1)

	if err := w.SetExchange(telegramID, "binance"); err != nil {
		t.Fatalf("SetExchange() error: %v", err)
	}

	if err := w.SetAPIKey(telegramID, "key"); err != nil {
		t.Fatalf("SetAPIKey() error: %v", err)
	}

	w.Cancel(telegramID)

	if w.IsInSetup(telegramID) {
		t.Fatal("user should not be in setup after Cancel()")
	}

	if w.GetSession(telegramID) != nil {
		t.Fatal("GetSession() should return nil after Cancel()")
	}
}

func TestSetupWizard_CancelNonExistent(t *testing.T) {
	w := NewSetupWizard()
	// should not panic
	w.Cancel(99999)
}

func TestSetupWizard_SetAPIKey_WrongStep(t *testing.T) {
	w := NewSetupWizard()
	telegramID := int64(12345)

	// not in setup at all
	err := w.SetAPIKey(telegramID, "key")
	if err == nil {
		t.Fatal("SetAPIKey() should fail when not in setup")
	}

	// start and advance past api key step
	w.Start(telegramID, 1)
	_ = w.SetExchange(telegramID, "binance")
	_ = w.SetAPIKey(telegramID, "key")

	// now at api secret step, setting api key again should fail
	err = w.SetAPIKey(telegramID, "another-key")
	if err == nil {
		t.Fatal("SetAPIKey() should fail when at wrong step")
	}
}

func TestSetupWizard_SetAPISecret_WrongStep(t *testing.T) {
	w := NewSetupWizard()
	telegramID := int64(12345)

	// not in setup at all
	err := w.SetAPISecret(telegramID, "secret")
	if err == nil {
		t.Fatal("SetAPISecret() should fail when not in setup")
	}

	// start - at exchange step, not api secret
	w.Start(telegramID, 1)
	err = w.SetAPISecret(telegramID, "secret")
	if err == nil {
		t.Fatal("SetAPISecret() should fail when at wrong step")
	}
}

func TestSetupWizard_Complete_WrongStep(t *testing.T) {
	w := NewSetupWizard()
	telegramID := int64(12345)

	// not in setup
	_, _, _, _, err := w.Complete(telegramID)
	if err == nil {
		t.Fatal("Complete() should fail when not in setup")
	}

	// start - at exchange step, not confirm
	w.Start(telegramID, 1)
	_, _, _, _, err = w.Complete(telegramID)
	if err == nil {
		t.Fatal("Complete() should fail when at wrong step")
	}
}

func TestSetupWizard_MultipleSessions(t *testing.T) {
	w := NewSetupWizard()
	user1 := int64(111)
	user2 := int64(222)

	w.Start(user1, 1)
	w.Start(user2, 2)

	if !w.IsInSetup(user1) {
		t.Fatal("user1 should be in setup")
	}
	if !w.IsInSetup(user2) {
		t.Fatal("user2 should be in setup")
	}

	// advance user1 through exchange, user2 should remain at exchange step
	_ = w.SetExchange(user1, "binance")
	_ = w.SetAPIKey(user1, "key1")

	s1 := w.GetSession(user1)
	s2 := w.GetSession(user2)

	if s1.Step != StepAPISecret {
		t.Errorf("user1 step = %q, want %q", s1.Step, StepAPISecret)
	}
	if s2.Step != StepExchange {
		t.Errorf("user2 step = %q, want %q", s2.Step, StepExchange)
	}

	// cancel user1, user2 should still be in setup
	w.Cancel(user1)
	if w.IsInSetup(user1) {
		t.Fatal("user1 should not be in setup after cancel")
	}
	if !w.IsInSetup(user2) {
		t.Fatal("user2 should still be in setup")
	}
}

func TestSetupWizard_RestartClearsOldSession(t *testing.T) {
	w := NewSetupWizard()
	telegramID := int64(12345)

	w.Start(telegramID, 1)
	_ = w.SetExchange(telegramID, "binance")
	_ = w.SetAPIKey(telegramID, "old-key")

	// restart should create a fresh session
	w.Start(telegramID, 2)

	session := w.GetSession(telegramID)
	if session.UserID != 2 {
		t.Errorf("restarted session userID = %d, want %d", session.UserID, 2)
	}
	if session.Step != StepExchange {
		t.Errorf("restarted session step = %q, want %q", session.Step, StepExchange)
	}
	if session.APIKey != "" {
		t.Errorf("restarted session should have empty api key, got %q", session.APIKey)
	}
}
