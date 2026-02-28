// user registration and credential management service
package user

import (
	"context"
	"fmt"
	"log"

	"github.com/trading-bot/go-bot/internal/binance"
	"github.com/trading-bot/go-bot/internal/security"
)

// userRepository defines the data operations for user management
type userRepository interface {
	FindByTelegramID(ctx context.Context, telegramID int64) (*User, error)
	Create(ctx context.Context, telegramID int64, username string) (*User, error)
	CreateDefaultPreferences(ctx context.Context, userID int) error
	UpdateLastActive(ctx context.Context, userID int, channel string) error
	Activate(ctx context.Context, userID int) error
	SaveCredentials(ctx context.Context, cred *Credentials) (*Credentials, error)
	HasValidCredentials(ctx context.Context, userID int) (bool, error)
}

// keyValidator validates exchange api keys
type keyValidator interface {
	ValidateKeys(ctx context.Context, apiKey, apiSecret string) (*binance.APIPermissions, error)
}

// encryptor encrypts sensitive data with per-user salts
type encryptor interface {
	Encrypt(plaintext []byte, salt []byte) ([]byte, error)
}

// service handles user registration, api key onboarding, and activation
type Service struct {
	repo      userRepository
	encryptor encryptor
	audit     *security.AuditLogger
	binance   keyValidator
	isTestnet bool
}

func NewService(
	repo userRepository,
	encryptor encryptor,
	audit *security.AuditLogger,
	binanceClient keyValidator,
	isTestnet bool,
) *Service {
	return &Service{
		repo:      repo,
		encryptor: encryptor,
		audit:     audit,
		binance:   binanceClient,
		isTestnet: isTestnet,
	}
}

// registerResult contains the result of user registration
type RegisterResult struct {
	User      *User
	IsNewUser bool
}

// register finds or creates a user from a telegram /start command
func (s *Service) Register(ctx context.Context, telegramID int64, username string) (*RegisterResult, error) {
	existing, err := s.repo.FindByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, fmt.Errorf("failed to look up user: %w", err)
	}

	if existing != nil {
		if err := s.repo.UpdateLastActive(ctx, existing.ID, "telegram"); err != nil {
			log.Printf("warning: failed to update last active for user %d: %v", existing.ID, err)
		}
		return &RegisterResult{User: existing, IsNewUser: false}, nil
	}

	newUser, err := s.repo.Create(ctx, telegramID, username)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	if err := s.repo.CreateDefaultPreferences(ctx, newUser.ID); err != nil {
		log.Printf("warning: failed to create default preferences for user %d: %v", newUser.ID, err)
	}

	return &RegisterResult{User: newUser, IsNewUser: true}, nil
}

// setupResult contains the result of api key setup
type SetupResult struct {
	Permissions *binance.APIPermissions
	Activated   bool
}

// setupAPIKeys validates, encrypts, and stores binance api keys for a user
func (s *Service) SetupAPIKeys(ctx context.Context, userID int, apiKey, apiSecret string) (*SetupResult, error) {
	// validate keys against binance
	perms, err := s.binance.ValidateKeys(ctx, apiKey, apiSecret)
	if err != nil {
		return nil, fmt.Errorf("key validation failed: %w", err)
	}

	// reject keys with withdrawal permission
	if perms.Withdraw {
		return nil, fmt.Errorf("keys with withdrawal permission are not allowed, please create keys with only spot/futures trading enabled")
	}

	// check at least one trading permission
	if !perms.Spot && !perms.Futures {
		return nil, fmt.Errorf("keys must have at least spot or futures trading permission enabled")
	}

	// generate salt and encrypt
	salt, err := security.GenerateSalt()
	if err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	encryptedKey, err := s.encryptor.Encrypt([]byte(apiKey), salt)
	if err != nil {
		s.logAudit(ctx, userID, 0, "encrypt", false, err.Error())
		return nil, fmt.Errorf("failed to encrypt api key: %w", err)
	}

	encryptedSecret, err := s.encryptor.Encrypt([]byte(apiSecret), salt)
	if err != nil {
		s.logAudit(ctx, userID, 0, "encrypt", false, err.Error())
		return nil, fmt.Errorf("failed to encrypt api secret: %w", err)
	}

	// store credentials
	cred := &Credentials{
		UserID:             userID,
		Exchange:           "binance",
		APIKeyEncrypted:    encryptedKey,
		APISecretEncrypted: encryptedSecret,
		Salt:               salt,
		Permissions:        perms.ToJSON(),
		IsTestnet:          s.isTestnet,
		IsValid:            true,
	}

	saved, err := s.repo.SaveCredentials(ctx, cred)
	if err != nil {
		return nil, fmt.Errorf("failed to save credentials: %w", err)
	}

	s.logAudit(ctx, userID, saved.ID, "encrypt", true, "")

	// activate user
	if err := s.repo.Activate(ctx, userID); err != nil {
		log.Printf("warning: failed to activate user %d: %v", userID, err)
	}

	return &SetupResult{
		Permissions: perms,
		Activated:   true,
	}, nil
}

// getStatus returns the user's current setup status
func (s *Service) GetStatus(ctx context.Context, userID int) (activated bool, hasKeys bool, err error) {
	hasKeys, err = s.repo.HasValidCredentials(ctx, userID)
	if err != nil {
		return false, false, err
	}
	// if they have valid keys, they're activated
	return hasKeys, hasKeys, nil
}

func (s *Service) logAudit(ctx context.Context, userID, credentialID int, action string, success bool, errMsg string) {
	if s.audit == nil {
		return
	}
	if err := s.audit.Log(ctx, security.AuditEntry{
		UserID:       userID,
		CredentialID: credentialID,
		Action:       action,
		Success:      success,
		ErrorMessage: errMsg,
	}); err != nil {
		log.Printf("warning: failed to log audit entry: %v", err)
	}
}
