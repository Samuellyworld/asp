// user registration and credential management service
package user

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/trading-bot/go-bot/internal/exchange"
	"github.com/trading-bot/go-bot/internal/security"
)

// userRepository defines the data operations for user management
type userRepository interface {
	FindByTelegramID(ctx context.Context, telegramID int64) (*User, error)
	FindByDiscordID(ctx context.Context, discordID int64) (*User, error)
	Create(ctx context.Context, telegramID int64, username string) (*User, error)
	CreateFromDiscord(ctx context.Context, discordID int64, username string) (*User, error)
	CreateDefaultPreferences(ctx context.Context, userID int) error
	UpdateLastActive(ctx context.Context, userID int, channel string) error
	LinkDiscordToTelegram(ctx context.Context, telegramID, discordID int64) (*User, error)
	Activate(ctx context.Context, userID int) error
	SaveCredentials(ctx context.Context, cred *Credentials) (*Credentials, error)
	HasValidCredentials(ctx context.Context, userID int) (bool, error)
	GetCredentials(ctx context.Context, userID int, exchange string) (*Credentials, error)
	GetPrimaryCredentials(ctx context.Context, userID int) (*Credentials, error)
	ListActive(ctx context.Context) ([]*User, error)
	SetLeverageEnabled(ctx context.Context, userID int, enabled bool) error
	IsLeverageEnabled(ctx context.Context, userID int) (bool, error)
}

// keyValidator validates exchange api keys
type keyValidator interface {
	ValidateKeys(ctx context.Context, apiKey, apiSecret string) (*exchange.APIPermissions, error)
}

// encryptor handles encryption and decryption of sensitive data with per-user salts
type encryptor interface {
	Encrypt(plaintext []byte, salt []byte) ([]byte, error)
	Decrypt(ciphertext []byte, salt []byte) ([]byte, error)
}

// service handles user registration, api key onboarding, and activation
type Service struct {
	repo       userRepository
	encryptor  encryptor
	audit      *security.AuditLogger
	validators map[string]keyValidator
	isTestnet  bool
}

func NewService(
	repo userRepository,
	encryptor encryptor,
	audit *security.AuditLogger,
	binanceClient keyValidator,
	isTestnet bool,
) *Service {
	return &Service{
		repo:       repo,
		encryptor:  encryptor,
		audit:      audit,
		validators: map[string]keyValidator{"binance": binanceClient},
		isTestnet:  isTestnet,
	}
}

// RegisterKeyValidator adds or replaces the validator for an exchange.
func (s *Service) RegisterKeyValidator(exchangeName string, validator keyValidator) {
	exchangeName = normalizeExchangeName(exchangeName)
	if exchangeName == "" || validator == nil {
		return
	}
	s.validators[exchangeName] = validator
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

// registers or retrieves a user from a discord interaction
func (s *Service) RegisterDiscord(ctx context.Context, discordID int64, username string) (*RegisterResult, error) {
	existing, err := s.repo.FindByDiscordID(ctx, discordID)
	if err != nil {
		return nil, fmt.Errorf("failed to look up user: %w", err)
	}

	if existing != nil {
		if err := s.repo.UpdateLastActive(ctx, existing.ID, "discord"); err != nil {
			log.Printf("warning: failed to update last active for user %d: %v", existing.ID, err)
		}
		return &RegisterResult{User: existing, IsNewUser: false}, nil
	}

	newUser, err := s.repo.CreateFromDiscord(ctx, discordID, username)
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
	Exchange    string
	Permissions *exchange.APIPermissions
	Activated   bool
}

// setupAPIKeys validates, encrypts, and stores binance api keys for a user
func (s *Service) SetupAPIKeys(ctx context.Context, userID int, apiKey, apiSecret string) (*SetupResult, error) {
	return s.SetupExchangeAPIKeys(ctx, userID, "binance", apiKey, apiSecret)
}

// SetupExchangeAPIKeys validates, encrypts, and stores api keys for a supported exchange.
func (s *Service) SetupExchangeAPIKeys(ctx context.Context, userID int, exchangeName, apiKey, apiSecret string) (*SetupResult, error) {
	exchangeName = normalizeExchangeName(exchangeName)
	validator, ok := s.validators[exchangeName]
	if !ok || validator == nil {
		return nil, fmt.Errorf("exchange %q is not supported", exchangeName)
	}

	// validate keys against the selected exchange
	perms, err := validator.ValidateKeys(ctx, apiKey, apiSecret)
	if err != nil {
		return nil, fmt.Errorf("key validation failed: %w", err)
	}
	if perms == nil {
		return nil, fmt.Errorf("key validation returned no permissions")
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
		Exchange:           exchangeName,
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
		Exchange:    exchangeName,
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

// decrypts and returns the user's api credentials for the given exchange
func (s *Service) GetDecryptedCredentials(ctx context.Context, userID int) (string, string, error) {
	return s.GetDecryptedCredentialsForExchange(ctx, userID, "binance")
}

// GetDecryptedCredentialsForExchange decrypts and returns a user's api credentials
// for a specific exchange.
func (s *Service) GetDecryptedCredentialsForExchange(ctx context.Context, userID int, exchangeName string) (string, string, error) {
	exchangeName = normalizeExchangeName(exchangeName)
	cred, err := s.repo.GetCredentials(ctx, userID, exchangeName)
	if err != nil {
		return "", "", fmt.Errorf("failed to get credentials: %w", err)
	}
	if cred == nil {
		return "", "", fmt.Errorf("no credentials found for user %d", userID)
	}

	apiKey, err := s.encryptor.Decrypt(cred.APIKeyEncrypted, cred.Salt)
	if err != nil {
		s.logAudit(ctx, userID, cred.ID, "decrypt", false, err.Error())
		return "", "", fmt.Errorf("failed to decrypt api key: %w", err)
	}

	apiSecret, err := s.encryptor.Decrypt(cred.APISecretEncrypted, cred.Salt)
	if err != nil {
		s.logAudit(ctx, userID, cred.ID, "decrypt", false, err.Error())
		return "", "", fmt.Errorf("failed to decrypt api secret: %w", err)
	}

	s.logAudit(ctx, userID, cred.ID, "decrypt", true, "")
	return string(apiKey), string(apiSecret), nil
}

// GetDecryptedPrimaryCredentials decrypts the user's most recently validated
// exchange credentials and returns the exchange name with the key pair.
func (s *Service) GetDecryptedPrimaryCredentials(ctx context.Context, userID int) (string, string, string, error) {
	cred, err := s.repo.GetPrimaryCredentials(ctx, userID)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get credentials: %w", err)
	}
	if cred == nil {
		return "", "", "", fmt.Errorf("no credentials found for user %d", userID)
	}

	apiKey, err := s.encryptor.Decrypt(cred.APIKeyEncrypted, cred.Salt)
	if err != nil {
		s.logAudit(ctx, userID, cred.ID, "decrypt", false, err.Error())
		return "", "", "", fmt.Errorf("failed to decrypt api key: %w", err)
	}

	apiSecret, err := s.encryptor.Decrypt(cred.APISecretEncrypted, cred.Salt)
	if err != nil {
		s.logAudit(ctx, userID, cred.ID, "decrypt", false, err.Error())
		return "", "", "", fmt.Errorf("failed to decrypt api secret: %w", err)
	}

	s.logAudit(ctx, userID, cred.ID, "decrypt", true, "")
	return cred.Exchange, string(apiKey), string(apiSecret), nil
}

// GetPrimaryCredentialExchange returns the user's most recently validated
// exchange without decrypting the credential payload.
func (s *Service) GetPrimaryCredentialExchange(ctx context.Context, userID int) (string, error) {
	cred, err := s.repo.GetPrimaryCredentials(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get credentials: %w", err)
	}
	if cred == nil {
		return "", fmt.Errorf("no credentials found for user %d", userID)
	}
	return normalizeExchangeName(cred.Exchange), nil
}

func normalizeExchangeName(exchangeName string) string {
	return strings.ToLower(strings.TrimSpace(exchangeName))
}

// links a discord identity to an existing telegram user
func (s *Service) LinkDiscordToTelegram(ctx context.Context, telegramID, discordID int64) (*User, error) {
	// check that the telegram user exists
	existing, err := s.repo.FindByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, fmt.Errorf("failed to look up telegram user: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("no user found with telegram id %d", telegramID)
	}

	// check if discord id is already linked to another user
	discordUser, err := s.repo.FindByDiscordID(ctx, discordID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing discord link: %w", err)
	}
	if discordUser != nil {
		if discordUser.ID == existing.ID {
			return existing, nil // already linked to this user
		}
		return nil, fmt.Errorf("this discord account is already linked to a different user")
	}

	// perform the link
	linked, err := s.repo.LinkDiscordToTelegram(ctx, telegramID, discordID)
	if err != nil {
		return nil, fmt.Errorf("failed to link accounts: %w", err)
	}
	if linked == nil {
		return nil, fmt.Errorf("no user found with telegram id %d", telegramID)
	}

	return linked, nil
}

// returns all activated, non-banned users for background scanning
func (s *Service) ListActive(ctx context.Context) ([]*User, error) {
	return s.repo.ListActive(ctx)
}

// enables leverage trading for a user
func (s *Service) EnableLeverage(ctx context.Context, userID int) error {
	return s.repo.SetLeverageEnabled(ctx, userID, true)
}

// disables leverage trading for a user
func (s *Service) DisableLeverage(ctx context.Context, userID int) error {
	return s.repo.SetLeverageEnabled(ctx, userID, false)
}

// checks if leverage is enabled for a user
func (s *Service) IsLeverageEnabled(ctx context.Context, userID int) (bool, error) {
	return s.repo.IsLeverageEnabled(ctx, userID)
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
