package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := []byte("thisis32byteslongsecretkey123456") // 32 bytes
	plaintext := []byte("hello world, this is a secret message")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if bytes.Equal(ciphertext, plaintext) {
		t.Fatalf("Ciphertext equals plaintext")
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("Decrypted text %q != original %q", decrypted, plaintext)
	}
}

func TestEncryptInvalidKey(t *testing.T) {
	key := []byte("short")
	_, err := Encrypt(key, []byte("data"))
	if err == nil {
		t.Fatal("Expected error with short key in Encrypt")
	}
}

func TestDecryptInvalidKey(t *testing.T) {
	key := []byte("short")
	_, err := Decrypt(key, []byte("data"))
	if err == nil {
		t.Fatal("Expected error with short key in Decrypt")
	}
}

func TestDecryptTooShort(t *testing.T) {
	key := []byte("thisis32byteslongsecretkey123456")
	shortCipher := []byte("short")
	_, err := Decrypt(key, shortCipher)
	if err == nil {
		t.Fatal("Expected error with short ciphertext in Decrypt")
	}
	if err.Error() != "ciphertext too short" {
		t.Fatalf("Expected 'ciphertext too short', got %v", err)
	}
}

func TestDecryptTampered(t *testing.T) {
	key := []byte("thisis32byteslongsecretkey123456")
	plaintext := []byte("hello")
	ciphertext, _ := Encrypt(key, plaintext)
	
	// Tamper with the ciphertext
	ciphertext[len(ciphertext)-1] ^= 0xff

	_, err := Decrypt(key, ciphertext)
	if err == nil {
		t.Fatal("Expected error decrypting tampered ciphertext")
	}
}

type errReader struct{}

func (errReader) Read(p []byte) (n int, err error) {
	return 0, bytes.ErrTooLarge
}

func TestEncryptRandFailure(t *testing.T) {
	key := []byte("thisis32byteslongsecretkey123456")
	
	// Mock rand.Reader
	importRandReader := rand.Reader
	rand.Reader = errReader{}
	defer func() { rand.Reader = importRandReader }()

	_, err := Encrypt(key, []byte("data"))
	if err == nil {
		t.Fatal("Expected error from rand.Reader failure")
	}
}
