// adapters bridge user/security services to livetrading interfaces.
// composes existing services to satisfy KeyDecryptor and BalanceProvider.
package livetrading

import (
	"context"
	"fmt"

	"github.com/trading-bot/go-bot/internal/exchange"
)

// credential repository for fetching encrypted keys
type CredentialRepository interface {
	GetCredentials(ctx context.Context, userID int, exchange string) (*Credentials, error)
}

// Credentials holds encrypted api keys and salt (mirrors user.Credentials fields we need)
type Credentials struct {
	ID                 int
	UserID             int
	APIKeyEncrypted    []byte
	APISecretEncrypted []byte
	Salt               []byte
}

// decrypts ciphertext using a salt
type decryptor interface {
	Decrypt(ciphertext []byte, salt []byte) ([]byte, error)
}

// logs decrypt operations for audit trail
type auditLogger interface {
	LogDecrypt(ctx context.Context, userID, credentialID int, success bool, errMsg string) error
}

// decrypts stored user credentials for order placement.
// composes a credential repository, encryptor, and audit logger.
type KeyDecryptorAdapter struct {
	repo      CredentialRepository
	decryptor decryptor
	audit     auditLogger
	exchange  string // "binance" by default
}

func NewKeyDecryptorAdapter(repo CredentialRepository, dec decryptor, audit auditLogger) *KeyDecryptorAdapter {
	return &KeyDecryptorAdapter{
		repo:      repo,
		decryptor: dec,
		audit:     audit,
		exchange:  "binance",
	}
}

// decrypts the api key and secret for a user.
// logs every decrypt attempt to the audit trail.
func (a *KeyDecryptorAdapter) DecryptKeys(userID int) (apiKey, apiSecret string, err error) {
	ctx := context.Background()

	cred, err := a.repo.GetCredentials(ctx, userID, a.exchange)
	if err != nil {
		return "", "", fmt.Errorf("failed to get credentials: %w", err)
	}
	if cred == nil {
		return "", "", fmt.Errorf("no credentials found for user %d", userID)
	}

	keyPlain, err := a.decryptor.Decrypt(cred.APIKeyEncrypted, cred.Salt)
	if err != nil {
		if a.audit != nil {
			_ = a.audit.LogDecrypt(ctx, userID, cred.ID, false, err.Error())
		}
		return "", "", fmt.Errorf("failed to decrypt api key: %w", err)
	}

	secretPlain, err := a.decryptor.Decrypt(cred.APISecretEncrypted, cred.Salt)
	if err != nil {
		if a.audit != nil {
			_ = a.audit.LogDecrypt(ctx, userID, cred.ID, false, err.Error())
		}
		return "", "", fmt.Errorf("failed to decrypt api secret: %w", err)
	}

	if a.audit != nil {
		_ = a.audit.LogDecrypt(ctx, userID, cred.ID, true, "")
	}

	return string(keyPlain), string(secretPlain), nil
}

// exchange client that can fetch balances with credentials
type balanceClient interface {
	GetBalance(ctx context.Context, apiKey, apiSecret string) ([]exchange.Balance, error)
}

// provides available balance by decrypting user keys and querying the exchange.
type BalanceProviderAdapter struct {
	keys     KeyDecryptor
	exchange balanceClient
}

func NewBalanceProviderAdapter(keys KeyDecryptor, exch balanceClient) *BalanceProviderAdapter {
	return &BalanceProviderAdapter{
		keys:     keys,
		exchange: exch,
	}
}

// returns the free balance for the given asset
func (a *BalanceProviderAdapter) GetAvailableBalance(userID int, asset string) (float64, error) {
	apiKey, apiSecret, err := a.keys.DecryptKeys(userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get keys: %w", err)
	}

	balances, err := a.exchange.GetBalance(context.Background(), apiKey, apiSecret)
	if err != nil {
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}

	for _, b := range balances {
		if b.Asset == asset {
			return b.Free, nil
		}
	}

	return 0, nil
}
