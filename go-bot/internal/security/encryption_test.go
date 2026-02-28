package security

import (
	"crypto/rand"
	"encoding/base64"
	"testing"
)

// helper to create a valid base64-encoded master key for testing
func testMasterKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestNewEncryptor_ValidKey(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() unexpected error: %v", err)
	}
	if enc == nil {
		t.Fatal("NewEncryptor() returned nil")
	}
}

func TestNewEncryptor_InvalidBase64(t *testing.T) {
	_, err := NewEncryptor("not-valid-base64!!!")
	if err == nil {
		t.Fatal("NewEncryptor() expected error for invalid base64")
	}
}

func TestNewEncryptor_KeyTooShort(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString([]byte("short"))
	_, err := NewEncryptor(shortKey)
	if err == nil {
		t.Fatal("NewEncryptor() expected error for short key")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error: %v", err)
	}

	plaintext := []byte("my-secret-api-key-12345")

	ciphertext, err := enc.Encrypt(plaintext, salt)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	// ciphertext must differ from plaintext
	if string(ciphertext) == string(plaintext) {
		t.Fatal("ciphertext equals plaintext")
	}

	decrypted, err := enc.Decrypt(ciphertext, salt)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("round trip failed: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecrypt_StringRoundTrip(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error: %v", err)
	}

	original := "binance-api-secret-xyz789"

	encrypted, err := enc.EncryptString(original, salt)
	if err != nil {
		t.Fatalf("EncryptString() error: %v", err)
	}

	if encrypted == original {
		t.Fatal("encrypted string equals original")
	}

	decrypted, err := enc.DecryptString(encrypted, salt)
	if err != nil {
		t.Fatalf("DecryptString() error: %v", err)
	}

	if decrypted != original {
		t.Errorf("string round trip failed: got %q, want %q", decrypted, original)
	}
}

func TestEncrypt_DifferentSaltsProduceDifferentCiphertexts(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	salt1, _ := GenerateSalt()
	salt2, _ := GenerateSalt()

	plaintext := "same-api-key"

	enc1, err := enc.EncryptString(plaintext, salt1)
	if err != nil {
		t.Fatalf("EncryptString() with salt1 error: %v", err)
	}

	enc2, err := enc.EncryptString(plaintext, salt2)
	if err != nil {
		t.Fatalf("EncryptString() with salt2 error: %v", err)
	}

	if enc1 == enc2 {
		t.Error("different salts produced the same ciphertext")
	}
}

func TestEncrypt_SameSaltProducesDifferentCiphertexts(t *testing.T) {
	// due to random nonce, even same salt + plaintext should differ
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	salt, _ := GenerateSalt()
	plaintext := "same-api-key"

	enc1, _ := enc.EncryptString(plaintext, salt)
	enc2, _ := enc.EncryptString(plaintext, salt)

	if enc1 == enc2 {
		t.Error("same salt + same plaintext produced identical ciphertexts (nonce should differ)")
	}
}

func TestDecrypt_WrongSaltFails(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	salt1, _ := GenerateSalt()
	salt2, _ := GenerateSalt()

	encrypted, err := enc.EncryptString("secret-data", salt1)
	if err != nil {
		t.Fatalf("EncryptString() error: %v", err)
	}

	_, err = enc.DecryptString(encrypted, salt2)
	if err == nil {
		t.Fatal("DecryptString() with wrong salt should fail")
	}
}

func TestDecrypt_CiphertextTooShort(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	salt, _ := GenerateSalt()

	_, err = enc.Decrypt([]byte("short"), salt)
	if err == nil {
		t.Fatal("Decrypt() with short ciphertext should fail")
	}
}

func TestDecrypt_CorruptedCiphertextFails(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	salt, _ := GenerateSalt()

	ciphertext, err := enc.Encrypt([]byte("secret"), salt)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	// corrupt the ciphertext
	ciphertext[len(ciphertext)-1] ^= 0xFF

	_, err = enc.Decrypt(ciphertext, salt)
	if err == nil {
		t.Fatal("Decrypt() with corrupted ciphertext should fail")
	}
}

func TestDecryptString_InvalidBase64(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	salt, _ := GenerateSalt()

	_, err = enc.DecryptString("not-valid-base64!!!", salt)
	if err == nil {
		t.Fatal("DecryptString() with invalid base64 should fail")
	}
}

func TestGenerateSalt(t *testing.T) {
	salt1, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error: %v", err)
	}

	if len(salt1) != saltSize {
		t.Errorf("salt length = %d, want %d", len(salt1), saltSize)
	}

	salt2, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error: %v", err)
	}

	// two salts should be different
	if string(salt1) == string(salt2) {
		t.Error("two generated salts are identical")
	}
}

func TestEncryptor_Test(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	if err := enc.Test(); err != nil {
		t.Errorf("Encryptor.Test() failed: %v", err)
	}
}

func TestEncryptDecrypt_EmptyPlaintext(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	salt, _ := GenerateSalt()

	ciphertext, err := enc.Encrypt([]byte(""), salt)
	if err != nil {
		t.Fatalf("Encrypt() with empty plaintext error: %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext, salt)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if string(decrypted) != "" {
		t.Errorf("expected empty string, got %q", decrypted)
	}
}

func TestEncryptDecrypt_LargePlaintext(t *testing.T) {
	enc, err := NewEncryptor(testMasterKey(t))
	if err != nil {
		t.Fatalf("NewEncryptor() error: %v", err)
	}

	salt, _ := GenerateSalt()

	// 10KB of data
	large := make([]byte, 10240)
	for i := range large {
		large[i] = byte(i % 256)
	}

	ciphertext, err := enc.Encrypt(large, salt)
	if err != nil {
		t.Fatalf("Encrypt() with large plaintext error: %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext, salt)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if string(decrypted) != string(large) {
		t.Error("large plaintext round trip failed")
	}
}
