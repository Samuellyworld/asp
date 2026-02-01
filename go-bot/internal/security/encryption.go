// aes-256-gcm encryption with pbkdf2 key derivation
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

const (
	saltSize   = 32
	nonceSize  = 12
	keySize    = 32
	iterations = 100000
)

type Encryptor struct {
	masterKey []byte
}

func NewEncryptor(masterKeyBase64 string) (*Encryptor, error) {
	masterKey, err := base64.StdEncoding.DecodeString(masterKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode master key: %w", err)
	}

	if len(masterKey) < keySize {
		return nil, fmt.Errorf("master key must be at least %d bytes", keySize)
	}

	return &Encryptor{masterKey: masterKey[:keySize]}, nil
}

// derive a unique key for each user using pbkdf2
func (e *Encryptor) deriveKey(salt []byte) []byte {
	return pbkdf2.Key(e.masterKey, salt, iterations, keySize, sha256.New)
}

// generate a random salt
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}

// encrypt data with per-user salt
func (e *Encryptor) Encrypt(plaintext []byte, salt []byte) ([]byte, error) {
	key := e.deriveKey(salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcm: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decrypt data with per-user salt
func (e *Encryptor) Decrypt(ciphertext []byte, salt []byte) ([]byte, error) {
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	key := e.deriveKey(salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcm: %w", err)
	}

	nonce := ciphertext[:nonceSize]
	ciphertext = ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// encrypt string and return base64
func (e *Encryptor) EncryptString(plaintext string, salt []byte) (string, error) {
	ciphertext, err := e.Encrypt([]byte(plaintext), salt)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt base64 string
func (e *Encryptor) DecryptString(ciphertextBase64 string, salt []byte) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}
	plaintext, err := e.Decrypt(ciphertext, salt)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// test encryption round trip
func (e *Encryptor) Test() error {
	testData := "test-api-key-12345"

	salt1, err := GenerateSalt()
	if err != nil {
		return fmt.Errorf("salt generation failed: %w", err)
	}

	salt2, err := GenerateSalt()
	if err != nil {
		return fmt.Errorf("salt generation failed: %w", err)
	}

	// test round trip
	encrypted, err := e.EncryptString(testData, salt1)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	decrypted, err := e.DecryptString(encrypted, salt1)
	if err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}

	if decrypted != testData {
		return fmt.Errorf("round trip failed: got %q, want %q", decrypted, testData)
	}

	// verify different salts produce different ciphertexts
	encrypted1, _ := e.EncryptString(testData, salt1)
	encrypted2, _ := e.EncryptString(testData, salt2)

	if encrypted1 == encrypted2 {
		return fmt.Errorf("different salts produced same ciphertext")
	}

	return nil
}
