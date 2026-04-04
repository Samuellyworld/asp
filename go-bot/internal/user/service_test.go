package user

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/trading-bot/go-bot/internal/binance"
)

// mock user repository
type mockUserRepo struct {
	users          map[int64]*User
	discordUsers   map[int64]*User
	credentials    map[int]*Credentials
	nextID         int

	findErr        error
	findDiscordErr error
	createErr      error
	createDiscErr  error
	prefsErr       error
	activeErr      error
	activateErr    error
	saveCredErr    error
	hasCredsErr    error
	hasCredsVal    bool
	getCredErr     error
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		users:        make(map[int64]*User),
		discordUsers: make(map[int64]*User),
		credentials:  make(map[int]*Credentials),
		nextID:       1,
	}
}

func (m *mockUserRepo) FindByTelegramID(_ context.Context, telegramID int64) (*User, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	return m.users[telegramID], nil
}

func (m *mockUserRepo) FindByDiscordID(_ context.Context, discordID int64) (*User, error) {
	if m.findDiscordErr != nil {
		return nil, m.findDiscordErr
	}
	return m.discordUsers[discordID], nil
}

func (m *mockUserRepo) Create(_ context.Context, telegramID int64, username string) (*User, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	u := &User{
		ID:          m.nextID,
		UUID:        fmt.Sprintf("uuid-%d", m.nextID),
		TelegramID:  &telegramID,
		Username:    &username,
		TradingMode: "paper",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.nextID++
	m.users[telegramID] = u
	return u, nil
}

func (m *mockUserRepo) CreateFromDiscord(_ context.Context, discordID int64, username string) (*User, error) {
	if m.createDiscErr != nil {
		return nil, m.createDiscErr
	}
	u := &User{
		ID:          m.nextID,
		UUID:        fmt.Sprintf("uuid-%d", m.nextID),
		DiscordID:   &discordID,
		Username:    &username,
		TradingMode: "paper",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.nextID++
	m.discordUsers[discordID] = u
	return u, nil
}

func (m *mockUserRepo) CreateDefaultPreferences(_ context.Context, _ int) error {
	return m.prefsErr
}

func (m *mockUserRepo) UpdateLastActive(_ context.Context, _ int, _ string) error {
	return m.activeErr
}

func (m *mockUserRepo) LinkDiscordToTelegram(_ context.Context, telegramID, discordID int64) (*User, error) {
	u, ok := m.users[telegramID]
	if !ok {
		return nil, nil
	}
	u.DiscordID = &discordID
	m.discordUsers[discordID] = u
	return u, nil
}

func (m *mockUserRepo) Activate(_ context.Context, userID int) error {
	if m.activateErr != nil {
		return m.activateErr
	}
	for _, u := range m.users {
		if u.ID == userID {
			u.IsActivated = true
		}
	}
	return nil
}

func (m *mockUserRepo) SaveCredentials(_ context.Context, cred *Credentials) (*Credentials, error) {
	if m.saveCredErr != nil {
		return nil, m.saveCredErr
	}
	cred.ID = m.nextID
	m.nextID++
	cred.CreatedAt = time.Now()
	m.credentials[cred.UserID] = cred
	return cred, nil
}

func (m *mockUserRepo) HasValidCredentials(_ context.Context, userID int) (bool, error) {
	if m.hasCredsErr != nil {
		return false, m.hasCredsErr
	}
	if m.hasCredsVal {
		return true, nil
	}
	_, ok := m.credentials[userID]
	return ok, nil
}

func (m *mockUserRepo) GetCredentials(_ context.Context, userID int, _ string) (*Credentials, error) {
	if m.getCredErr != nil {
		return nil, m.getCredErr
	}
	return m.credentials[userID], nil
}

func (m *mockUserRepo) ListActive(_ context.Context) ([]*User, error) {
	return nil, nil
}

func (m *mockUserRepo) SetLeverageEnabled(_ context.Context, _ int, _ bool) error {
	return nil
}

func (m *mockUserRepo) IsLeverageEnabled(_ context.Context, _ int) (bool, error) {
	return false, nil
}

// seed an existing user
func (m *mockUserRepo) seed(telegramID int64, username string) *User {
	u := &User{
		ID:          m.nextID,
		UUID:        fmt.Sprintf("uuid-%d", m.nextID),
		TelegramID:  &telegramID,
		Username:    &username,
		TradingMode: "paper",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.nextID++
	m.users[telegramID] = u
	return u
}

// seed an existing discord user
func (m *mockUserRepo) seedDiscord(discordID int64, username string) *User {
	u := &User{
		ID:          m.nextID,
		UUID:        fmt.Sprintf("uuid-%d", m.nextID),
		DiscordID:   &discordID,
		Username:    &username,
		TradingMode: "paper",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	m.nextID++
	m.discordUsers[discordID] = u
	return u
}

// mock key validator
type mockValidator struct {
	perms *binance.APIPermissions
	err   error
}

func (m *mockValidator) ValidateKeys(_ context.Context, _, _ string) (*binance.APIPermissions, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.perms, nil
}

// mock encryptor
type mockEncryptor struct {
	err    error
	decErr error
}

func (m *mockEncryptor) Encrypt(plaintext []byte, _ []byte) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	// return plaintext reversed as "encrypted" data for testing
	enc := make([]byte, len(plaintext))
	for i, b := range plaintext {
		enc[len(plaintext)-1-i] = b
	}
	return enc, nil
}

func (m *mockEncryptor) Decrypt(ciphertext []byte, _ []byte) ([]byte, error) {
	if m.decErr != nil {
		return nil, m.decErr
	}
	// reverse to get back the original plaintext
	dec := make([]byte, len(ciphertext))
	for i, b := range ciphertext {
		dec[len(ciphertext)-1-i] = b
	}
	return dec, nil
}

// helper to build a service with defaults
func newTestService(repo *mockUserRepo, validator *mockValidator, enc *mockEncryptor) *Service {
	return NewService(repo, enc, nil, validator, false)
}

// --- Register tests ---

func TestRegister_NewUser(t *testing.T) {
	repo := newMockUserRepo()
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	result, err := svc.Register(context.Background(), 12345, "testuser")
	if err != nil {
		t.Fatalf("Register() error: %v", err)
	}
	if !result.IsNewUser {
		t.Error("expected IsNewUser to be true")
	}
	if result.User.ID == 0 {
		t.Error("expected user to have an ID")
	}
}

func TestRegister_ExistingUser(t *testing.T) {
	repo := newMockUserRepo()
	repo.seed(12345, "testuser")
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	result, err := svc.Register(context.Background(), 12345, "testuser")
	if err != nil {
		t.Fatalf("Register() error: %v", err)
	}
	if result.IsNewUser {
		t.Error("expected IsNewUser to be false for existing user")
	}
}

func TestRegister_FindError(t *testing.T) {
	repo := newMockUserRepo()
	repo.findErr = fmt.Errorf("db connection failed")
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	_, err := svc.Register(context.Background(), 12345, "testuser")
	if err == nil {
		t.Fatal("Register() expected error when find fails")
	}
}

func TestRegister_CreateError(t *testing.T) {
	repo := newMockUserRepo()
	repo.createErr = fmt.Errorf("unique constraint violation")
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	_, err := svc.Register(context.Background(), 12345, "testuser")
	if err == nil {
		t.Fatal("Register() expected error when create fails")
	}
}

func TestRegister_UpdateLastActiveError_IsWarning(t *testing.T) {
	repo := newMockUserRepo()
	repo.seed(12345, "testuser")
	repo.activeErr = fmt.Errorf("update failed")
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	// should succeed despite UpdateLastActive error
	result, err := svc.Register(context.Background(), 12345, "testuser")
	if err != nil {
		t.Fatalf("Register() should not fail for UpdateLastActive error: %v", err)
	}
	if result.IsNewUser {
		t.Error("expected existing user")
	}
}

func TestRegister_CreateDefaultPrefsError_IsWarning(t *testing.T) {
	repo := newMockUserRepo()
	repo.prefsErr = fmt.Errorf("insert default prefs failed")
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	// should succeed despite CreateDefaultPreferences error
	result, err := svc.Register(context.Background(), 12345, "testuser")
	if err != nil {
		t.Fatalf("Register() should not fail for prefs error: %v", err)
	}
	if !result.IsNewUser {
		t.Error("expected new user")
	}
}

// --- SetupAPIKeys tests ---

func TestSetupAPIKeys_SpotOnly(t *testing.T) {
	repo := newMockUserRepo()
	repo.seed(12345, "testuser")
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	svc := newTestService(repo, validator, &mockEncryptor{})

	result, err := svc.SetupAPIKeys(context.Background(), 1, "api-key", "api-secret")
	if err != nil {
		t.Fatalf("SetupAPIKeys() error: %v", err)
	}
	if !result.Permissions.Spot {
		t.Error("expected spot permission")
	}
	if result.Permissions.Futures {
		t.Error("expected no futures permission")
	}
	if !result.Activated {
		t.Error("expected activated to be true")
	}
}

func TestSetupAPIKeys_SpotAndFutures(t *testing.T) {
	repo := newMockUserRepo()
	repo.seed(12345, "testuser")
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: true, Withdraw: false},
	}
	svc := newTestService(repo, validator, &mockEncryptor{})

	result, err := svc.SetupAPIKeys(context.Background(), 1, "api-key", "api-secret")
	if err != nil {
		t.Fatalf("SetupAPIKeys() error: %v", err)
	}
	if !result.Permissions.Spot || !result.Permissions.Futures {
		t.Error("expected both spot and futures permissions")
	}
}

func TestSetupAPIKeys_WithdrawRejected(t *testing.T) {
	repo := newMockUserRepo()
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: true},
	}
	svc := newTestService(repo, validator, &mockEncryptor{})

	_, err := svc.SetupAPIKeys(context.Background(), 1, "api-key", "api-secret")
	if err == nil {
		t.Fatal("SetupAPIKeys() expected error for withdraw permission")
	}
}

func TestSetupAPIKeys_NoTradingPerms(t *testing.T) {
	repo := newMockUserRepo()
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: false, Futures: false, Withdraw: false},
	}
	svc := newTestService(repo, validator, &mockEncryptor{})

	_, err := svc.SetupAPIKeys(context.Background(), 1, "api-key", "api-secret")
	if err == nil {
		t.Fatal("SetupAPIKeys() expected error for no trading permissions")
	}
}

func TestSetupAPIKeys_ValidationError(t *testing.T) {
	repo := newMockUserRepo()
	validator := &mockValidator{err: fmt.Errorf("invalid api key")}
	svc := newTestService(repo, validator, &mockEncryptor{})

	_, err := svc.SetupAPIKeys(context.Background(), 1, "bad-key", "bad-secret")
	if err == nil {
		t.Fatal("SetupAPIKeys() expected error when validation fails")
	}
}

func TestSetupAPIKeys_EncryptError(t *testing.T) {
	repo := newMockUserRepo()
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	enc := &mockEncryptor{err: fmt.Errorf("encryption failed")}
	svc := newTestService(repo, validator, enc)

	_, err := svc.SetupAPIKeys(context.Background(), 1, "api-key", "api-secret")
	if err == nil {
		t.Fatal("SetupAPIKeys() expected error when encryption fails")
	}
}

func TestSetupAPIKeys_SaveCredentialsError(t *testing.T) {
	repo := newMockUserRepo()
	repo.saveCredErr = fmt.Errorf("db write failed")
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	svc := newTestService(repo, validator, &mockEncryptor{})

	_, err := svc.SetupAPIKeys(context.Background(), 1, "api-key", "api-secret")
	if err == nil {
		t.Fatal("SetupAPIKeys() expected error when save fails")
	}
}

func TestSetupAPIKeys_ActivateError_IsWarning(t *testing.T) {
	repo := newMockUserRepo()
	repo.seed(12345, "testuser")
	repo.activateErr = fmt.Errorf("activate failed")
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	svc := newTestService(repo, validator, &mockEncryptor{})

	// should succeed despite activate error (it's a warning)
	result, err := svc.SetupAPIKeys(context.Background(), 1, "api-key", "api-secret")
	if err != nil {
		t.Fatalf("SetupAPIKeys() should not fail for activate error: %v", err)
	}
	if !result.Activated {
		t.Error("expected Activated to be true (set before activate call)")
	}
}

func TestSetupAPIKeys_CredentialsStoredCorrectly(t *testing.T) {
	repo := newMockUserRepo()
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: true, Withdraw: false},
	}
	svc := newTestService(repo, validator, &mockEncryptor{})

	_, err := svc.SetupAPIKeys(context.Background(), 1, "api-key", "api-secret")
	if err != nil {
		t.Fatalf("SetupAPIKeys() error: %v", err)
	}

	cred, ok := repo.credentials[1]
	if !ok {
		t.Fatal("expected credentials to be stored for user 1")
	}
	if cred.Exchange != "binance" {
		t.Errorf("Exchange = %q, want %q", cred.Exchange, "binance")
	}
	if !cred.IsValid {
		t.Error("expected IsValid to be true")
	}
	if len(cred.APIKeyEncrypted) == 0 {
		t.Error("expected encrypted key to be non-empty")
	}
	if len(cred.APISecretEncrypted) == 0 {
		t.Error("expected encrypted secret to be non-empty")
	}
	if len(cred.Salt) == 0 {
		t.Error("expected salt to be non-empty")
	}
}

// --- GetStatus tests ---

func TestGetStatus_HasKeys(t *testing.T) {
	repo := newMockUserRepo()
	repo.hasCredsVal = true
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	activated, hasKeys, err := svc.GetStatus(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}
	if !activated || !hasKeys {
		t.Errorf("GetStatus() = (%v, %v), want (true, true)", activated, hasKeys)
	}
}

func TestGetStatus_NoKeys(t *testing.T) {
	repo := newMockUserRepo()
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	activated, hasKeys, err := svc.GetStatus(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}
	if activated || hasKeys {
		t.Errorf("GetStatus() = (%v, %v), want (false, false)", activated, hasKeys)
	}
}

func TestGetStatus_RepoError(t *testing.T) {
	repo := newMockUserRepo()
	repo.hasCredsErr = fmt.Errorf("db error")
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	_, _, err := svc.GetStatus(context.Background(), 1)
	if err == nil {
		t.Fatal("GetStatus() expected error")
	}
}

// --- edge cases ---

func TestRegisterThenSetup_FullFlow(t *testing.T) {
	repo := newMockUserRepo()
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	svc := newTestService(repo, validator, &mockEncryptor{})

	// register
	regResult, err := svc.Register(context.Background(), 12345, "testuser")
	if err != nil {
		t.Fatalf("Register() error: %v", err)
	}
	if !regResult.IsNewUser {
		t.Error("expected new user")
	}

	// check status before setup
	activated, hasKeys, err := svc.GetStatus(context.Background(), regResult.User.ID)
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}
	if activated || hasKeys {
		t.Error("expected not activated and no keys before setup")
	}

	// setup keys
	setupResult, err := svc.SetupAPIKeys(context.Background(), regResult.User.ID, "my-key", "my-secret")
	if err != nil {
		t.Fatalf("SetupAPIKeys() error: %v", err)
	}
	if !setupResult.Activated {
		t.Error("expected activated after setup")
	}

	// check status after setup
	activated, hasKeys, err = svc.GetStatus(context.Background(), regResult.User.ID)
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}
	if !activated || !hasKeys {
		t.Error("expected activated and has keys after setup")
	}
}

func TestRegister_SameUserTwice(t *testing.T) {
	repo := newMockUserRepo()
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	// first registration
	r1, err := svc.Register(context.Background(), 12345, "testuser")
	if err != nil {
		t.Fatalf("first Register() error: %v", err)
	}
	if !r1.IsNewUser {
		t.Error("first registration should be new")
	}

	// second registration
	r2, err := svc.Register(context.Background(), 12345, "testuser")
	if err != nil {
		t.Fatalf("second Register() error: %v", err)
	}
	if r2.IsNewUser {
		t.Error("second registration should not be new")
	}
	if r1.User.ID != r2.User.ID {
		t.Error("both registrations should return same user")
	}
}

func TestSetupAPIKeys_WithdrawAndSpot_StillRejected(t *testing.T) {
	repo := newMockUserRepo()
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: true, Withdraw: true},
	}
	svc := newTestService(repo, validator, &mockEncryptor{})

	_, err := svc.SetupAPIKeys(context.Background(), 1, "key", "secret")
	if err == nil {
		t.Fatal("keys with withdraw+trading should still be rejected")
	}
}

// --- GetDecryptedCredentials tests ---

func TestGetDecryptedCredentials_Success(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}

	// simulate stored credentials with "encrypted" key/secret
	encKey, _ := enc.Encrypt([]byte("my-api-key"), []byte("salt"))
	encSecret, _ := enc.Encrypt([]byte("my-api-secret"), []byte("salt"))
	repo.credentials[1] = &Credentials{
		ID:                 1,
		UserID:             1,
		Exchange:           "binance",
		APIKeyEncrypted:    encKey,
		APISecretEncrypted: encSecret,
		Salt:               []byte("salt"),
		IsValid:            true,
	}

	svc := newTestService(repo, &mockValidator{}, enc)

	key, secret, err := svc.GetDecryptedCredentials(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetDecryptedCredentials() error: %v", err)
	}
	if key != "my-api-key" {
		t.Errorf("key = %q, want %q", key, "my-api-key")
	}
	if secret != "my-api-secret" {
		t.Errorf("secret = %q, want %q", secret, "my-api-secret")
	}
}

func TestGetDecryptedCredentials_NoCredentials(t *testing.T) {
	repo := newMockUserRepo()
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	_, _, err := svc.GetDecryptedCredentials(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
}

func TestGetDecryptedCredentials_RepoError(t *testing.T) {
	repo := newMockUserRepo()
	repo.getCredErr = fmt.Errorf("db connection lost")
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	_, _, err := svc.GetDecryptedCredentials(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error when repo fails")
	}
}

func TestGetDecryptedCredentials_DecryptKeyError(t *testing.T) {
	repo := newMockUserRepo()
	enc := &mockEncryptor{}
	encKey, _ := enc.Encrypt([]byte("key"), []byte("salt"))
	encSecret, _ := enc.Encrypt([]byte("secret"), []byte("salt"))
	repo.credentials[1] = &Credentials{
		ID:                 1,
		UserID:             1,
		APIKeyEncrypted:    encKey,
		APISecretEncrypted: encSecret,
		Salt:               []byte("salt"),
	}

	// fail on decrypt
	failEnc := &mockEncryptor{decErr: fmt.Errorf("decrypt failed")}
	svc := newTestService(repo, &mockValidator{}, failEnc)

	_, _, err := svc.GetDecryptedCredentials(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error when decrypt fails")
	}
}

// --- RegisterDiscord tests ---

func TestRegisterDiscord_NewUser(t *testing.T) {
	repo := newMockUserRepo()
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	result, err := svc.RegisterDiscord(context.Background(), 99999, "discorduser")
	if err != nil {
		t.Fatalf("RegisterDiscord() error: %v", err)
	}
	if !result.IsNewUser {
		t.Error("expected IsNewUser to be true")
	}
	if result.User.ID == 0 {
		t.Error("expected user to have an ID")
	}
	if result.User.DiscordID == nil {
		t.Error("expected DiscordID to be set")
	}
}

func TestRegisterDiscord_ExistingUser(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedDiscord(99999, "discorduser")
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	result, err := svc.RegisterDiscord(context.Background(), 99999, "discorduser")
	if err != nil {
		t.Fatalf("RegisterDiscord() error: %v", err)
	}
	if result.IsNewUser {
		t.Error("expected IsNewUser to be false for existing user")
	}
}

func TestRegisterDiscord_FindError(t *testing.T) {
	repo := newMockUserRepo()
	repo.findDiscordErr = fmt.Errorf("db connection failed")
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	_, err := svc.RegisterDiscord(context.Background(), 99999, "discorduser")
	if err == nil {
		t.Fatal("RegisterDiscord() expected error when find fails")
	}
}

func TestRegisterDiscord_CreateError(t *testing.T) {
	repo := newMockUserRepo()
	repo.createDiscErr = fmt.Errorf("unique constraint violation")
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	_, err := svc.RegisterDiscord(context.Background(), 99999, "discorduser")
	if err == nil {
		t.Fatal("RegisterDiscord() expected error when create fails")
	}
}

func TestRegisterDiscord_UpdateLastActiveError_IsWarning(t *testing.T) {
	repo := newMockUserRepo()
	repo.seedDiscord(99999, "discorduser")
	repo.activeErr = fmt.Errorf("update failed")
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	result, err := svc.RegisterDiscord(context.Background(), 99999, "discorduser")
	if err != nil {
		t.Fatalf("RegisterDiscord() should not fail for UpdateLastActive error: %v", err)
	}
	if result.IsNewUser {
		t.Error("expected existing user")
	}
}

func TestRegisterDiscord_CreateDefaultPrefsError_IsWarning(t *testing.T) {
	repo := newMockUserRepo()
	repo.prefsErr = fmt.Errorf("insert default prefs failed")
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	result, err := svc.RegisterDiscord(context.Background(), 99999, "discorduser")
	if err != nil {
		t.Fatalf("RegisterDiscord() should not fail for prefs error: %v", err)
	}
	if !result.IsNewUser {
		t.Error("expected new user")
	}
}

func TestRegisterDiscord_ThenSetup_FullFlow(t *testing.T) {
	repo := newMockUserRepo()
	validator := &mockValidator{
		perms: &binance.APIPermissions{Spot: true, Futures: false, Withdraw: false},
	}
	svc := newTestService(repo, validator, &mockEncryptor{})

	// register via discord
	regResult, err := svc.RegisterDiscord(context.Background(), 99999, "discorduser")
	if err != nil {
		t.Fatalf("RegisterDiscord() error: %v", err)
	}
	if !regResult.IsNewUser {
		t.Error("expected new user")
	}

	// check status before setup
	activated, hasKeys, err := svc.GetStatus(context.Background(), regResult.User.ID)
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}
	if activated || hasKeys {
		t.Error("expected not activated and no keys before setup")
	}

	// setup keys
	setupResult, err := svc.SetupAPIKeys(context.Background(), regResult.User.ID, "my-key", "my-secret")
	if err != nil {
		t.Fatalf("SetupAPIKeys() error: %v", err)
	}
	if !setupResult.Activated {
		t.Error("expected activated after setup")
	}

	// check status after setup
	activated, hasKeys, err = svc.GetStatus(context.Background(), regResult.User.ID)
	if err != nil {
		t.Fatalf("GetStatus() error: %v", err)
	}
	if !activated || !hasKeys {
		t.Error("expected activated and has keys after setup")
	}
}

func TestRegisterDiscord_SameUserTwice(t *testing.T) {
	repo := newMockUserRepo()
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	r1, err := svc.RegisterDiscord(context.Background(), 99999, "discorduser")
	if err != nil {
		t.Fatalf("first RegisterDiscord() error: %v", err)
	}
	if !r1.IsNewUser {
		t.Error("first registration should be new")
	}

	r2, err := svc.RegisterDiscord(context.Background(), 99999, "discorduser")
	if err != nil {
		t.Fatalf("second RegisterDiscord() error: %v", err)
	}
	if r2.IsNewUser {
		t.Error("second registration should not be new")
	}
	if r1.User.ID != r2.User.ID {
		t.Error("both registrations should return same user")
	}
}

// --- link discord to telegram tests ---

func TestLinkDiscordToTelegram_Success(t *testing.T) {
	repo := newMockUserRepo()
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	_, err := svc.Register(context.Background(), 99999, "tguser")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	linked, err := svc.LinkDiscordToTelegram(context.Background(), 99999, 88888)
	if err != nil {
		t.Fatalf("link failed: %v", err)
	}
	if linked.DiscordID == nil || *linked.DiscordID != 88888 {
		t.Error("discord id not set after link")
	}
	if linked.TelegramID == nil || *linked.TelegramID != 99999 {
		t.Error("telegram id lost after link")
	}
}

func TestLinkDiscordToTelegram_NoTelegramUser(t *testing.T) {
	repo := newMockUserRepo()
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	_, err := svc.LinkDiscordToTelegram(context.Background(), 99999, 88888)
	if err == nil {
		t.Error("expected error for non-existent telegram user")
	}
}

func TestLinkDiscordToTelegram_DiscordAlreadyLinked(t *testing.T) {
	repo := newMockUserRepo()
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	svc.Register(context.Background(), 11111, "user_one")
	svc.Register(context.Background(), 22222, "user_two")

	_, err := svc.LinkDiscordToTelegram(context.Background(), 11111, 88888)
	if err != nil {
		t.Fatalf("first link failed: %v", err)
	}

	_, err = svc.LinkDiscordToTelegram(context.Background(), 22222, 88888)
	if err == nil {
		t.Error("expected error when discord already linked to another user")
	}
}

func TestLinkDiscordToTelegram_SameUserIdempotent(t *testing.T) {
	repo := newMockUserRepo()
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	svc.Register(context.Background(), 99999, "idem")

	u1, err := svc.LinkDiscordToTelegram(context.Background(), 99999, 88888)
	if err != nil {
		t.Fatalf("first link failed: %v", err)
	}

	u2, err := svc.LinkDiscordToTelegram(context.Background(), 99999, 88888)
	if err != nil {
		t.Fatalf("second link failed: %v", err)
	}

	if u1.ID != u2.ID {
		t.Errorf("idempotent link returned different ids: %d vs %d", u1.ID, u2.ID)
	}
}

func TestLinkDiscordToTelegram_LookupByBothPlatforms(t *testing.T) {
	repo := newMockUserRepo()
	svc := newTestService(repo, &mockValidator{}, &mockEncryptor{})

	svc.Register(context.Background(), 99999, "dual")
	svc.LinkDiscordToTelegram(context.Background(), 99999, 88888)

	tgResult, _ := svc.Register(context.Background(), 99999, "dual")
	dcResult, _ := svc.RegisterDiscord(context.Background(), 88888, "dual")

	if tgResult.User.ID != dcResult.User.ID {
		t.Errorf("telegram id %d != discord id %d", tgResult.User.ID, dcResult.User.ID)
	}
}
