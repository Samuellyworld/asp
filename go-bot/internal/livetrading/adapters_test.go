// tests for key decryptor and balance provider adapters
package livetrading

import (
	"context"
	"fmt"
	"testing"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// --- mock credential repository ---

type mockCredRepo struct {
	creds map[int]*Credentials
	err   error
}

func (m *mockCredRepo) GetCredentials(_ context.Context, userID int, _ string) (*Credentials, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.creds[userID], nil
}

// --- mock decryptor ---

type mockDecryptor struct {
	err error
}

func (m *mockDecryptor) Decrypt(ciphertext []byte, _ []byte) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	// simple "decryption": strip prefix "enc:"
	if len(ciphertext) > 4 && string(ciphertext[:4]) == "enc:" {
		return ciphertext[4:], nil
	}
	return ciphertext, nil
}

// --- mock audit logger ---

type mockAudit struct {
	logs     []auditLog
	logErr   error
}

type auditLog struct {
	userID int
	credID int
	success bool
	errMsg  string
}

func (m *mockAudit) LogDecrypt(_ context.Context, userID, credentialID int, success bool, errMsg string) error {
	m.logs = append(m.logs, auditLog{userID, credentialID, success, errMsg})
	return m.logErr
}

// --- mock balance client ---

type mockBalanceClient struct {
	balances []exchange.Balance
	err      error
}

func (m *mockBalanceClient) GetBalance(_ context.Context, _, _ string) ([]exchange.Balance, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.balances, nil
}

// --- KeyDecryptorAdapter tests ---

func TestKeyDecryptor_Success(t *testing.T) {
	repo := &mockCredRepo{creds: map[int]*Credentials{
		1: {
			ID:                 10,
			UserID:             1,
			APIKeyEncrypted:    []byte("enc:my_api_key"),
			APISecretEncrypted: []byte("enc:my_secret"),
			Salt:               []byte("salt"),
		},
	}}
	audit := &mockAudit{}
	adapter := NewKeyDecryptorAdapter(repo, &mockDecryptor{}, audit)

	key, secret, err := adapter.DecryptKeys(1)
	if err != nil {
		t.Fatalf("DecryptKeys() error: %v", err)
	}
	if key != "my_api_key" {
		t.Errorf("key = %q, want my_api_key", key)
	}
	if secret != "my_secret" {
		t.Errorf("secret = %q, want my_secret", secret)
	}

	// verify audit log was written
	if len(audit.logs) != 1 {
		t.Fatalf("audit logs count = %d, want 1", len(audit.logs))
	}
	if !audit.logs[0].success {
		t.Error("audit should log success")
	}
	if audit.logs[0].userID != 1 {
		t.Errorf("audit userID = %d, want 1", audit.logs[0].userID)
	}
	if audit.logs[0].credID != 10 {
		t.Errorf("audit credID = %d, want 10", audit.logs[0].credID)
	}
}

func TestKeyDecryptor_NoCredentials(t *testing.T) {
	repo := &mockCredRepo{creds: map[int]*Credentials{}}
	adapter := NewKeyDecryptorAdapter(repo, &mockDecryptor{}, nil)

	_, _, err := adapter.DecryptKeys(99)
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
}

func TestKeyDecryptor_RepoError(t *testing.T) {
	repo := &mockCredRepo{err: fmt.Errorf("db down")}
	adapter := NewKeyDecryptorAdapter(repo, &mockDecryptor{}, nil)

	_, _, err := adapter.DecryptKeys(1)
	if err == nil {
		t.Fatal("expected error for repo failure")
	}
}

func TestKeyDecryptor_DecryptKeyError(t *testing.T) {
	repo := &mockCredRepo{creds: map[int]*Credentials{
		1: {ID: 10, APIKeyEncrypted: []byte("bad"), APISecretEncrypted: []byte("enc:ok"), Salt: []byte("s")},
	}}
	audit := &mockAudit{}
	adapter := NewKeyDecryptorAdapter(repo, &mockDecryptor{err: fmt.Errorf("decrypt failed")}, audit)

	_, _, err := adapter.DecryptKeys(1)
	if err == nil {
		t.Fatal("expected error for decrypt failure")
	}

	// verify failure was audited
	if len(audit.logs) != 1 {
		t.Fatalf("audit logs count = %d, want 1", len(audit.logs))
	}
	if audit.logs[0].success {
		t.Error("audit should log failure")
	}
}

func TestKeyDecryptor_NilAudit(t *testing.T) {
	repo := &mockCredRepo{creds: map[int]*Credentials{
		1: {ID: 10, APIKeyEncrypted: []byte("enc:key"), APISecretEncrypted: []byte("enc:secret"), Salt: []byte("s")},
	}}
	adapter := NewKeyDecryptorAdapter(repo, &mockDecryptor{}, nil)

	key, secret, err := adapter.DecryptKeys(1)
	if err != nil {
		t.Fatalf("DecryptKeys() error: %v", err)
	}
	if key != "key" || secret != "secret" {
		t.Errorf("got key=%q secret=%q, want key/secret", key, secret)
	}
}

// --- BalanceProviderAdapter tests ---

func TestBalanceProvider_Success(t *testing.T) {
	keys := &mockKeys{keys: map[int][2]string{1: {"k", "s"}}}
	exch := &mockBalanceClient{balances: []exchange.Balance{
		{Asset: "BTC", Free: 0.5, Locked: 0.1},
		{Asset: "USDT", Free: 1000, Locked: 0},
	}}
	adapter := NewBalanceProviderAdapter(keys, exch)

	balance, err := adapter.GetAvailableBalance(1, "USDT")
	if err != nil {
		t.Fatalf("GetAvailableBalance() error: %v", err)
	}
	if balance != 1000 {
		t.Errorf("balance = %v, want 1000", balance)
	}
}

func TestBalanceProvider_AssetNotFound(t *testing.T) {
	keys := &mockKeys{keys: map[int][2]string{1: {"k", "s"}}}
	exch := &mockBalanceClient{balances: []exchange.Balance{
		{Asset: "BTC", Free: 0.5},
	}}
	adapter := NewBalanceProviderAdapter(keys, exch)

	balance, err := adapter.GetAvailableBalance(1, "USDT")
	if err != nil {
		t.Fatalf("GetAvailableBalance() error: %v", err)
	}
	if balance != 0 {
		t.Errorf("balance = %v, want 0 for missing asset", balance)
	}
}

func TestBalanceProvider_KeyDecryptError(t *testing.T) {
	keys := &mockKeys{err: fmt.Errorf("no keys")}
	adapter := NewBalanceProviderAdapter(keys, nil)

	_, err := adapter.GetAvailableBalance(1, "USDT")
	if err == nil {
		t.Fatal("expected error when key decrypt fails")
	}
}

func TestBalanceProvider_ExchangeError(t *testing.T) {
	keys := &mockKeys{keys: map[int][2]string{1: {"k", "s"}}}
	exch := &mockBalanceClient{err: fmt.Errorf("api down")}
	adapter := NewBalanceProviderAdapter(keys, exch)

	_, err := adapter.GetAvailableBalance(1, "USDT")
	if err == nil {
		t.Fatal("expected error when exchange fails")
	}
}

func TestBalanceProvider_EmptyBalances(t *testing.T) {
	keys := &mockKeys{keys: map[int][2]string{1: {"k", "s"}}}
	exch := &mockBalanceClient{balances: []exchange.Balance{}}
	adapter := NewBalanceProviderAdapter(keys, exch)

	balance, err := adapter.GetAvailableBalance(1, "USDT")
	if err != nil {
		t.Fatalf("GetAvailableBalance() error: %v", err)
	}
	if balance != 0 {
		t.Errorf("balance = %v, want 0", balance)
	}
}
