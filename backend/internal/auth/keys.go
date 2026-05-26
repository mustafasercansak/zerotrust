package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"log/slog"
	"os"
	"sync"
)

// KeyStore holds one or more EC keys indexed by kid.
// The primary key signs new tokens; secondary keys are kept for validation during rotation.
type KeyStore struct {
	mu      sync.RWMutex
	keys    map[string]*ecdsa.PrivateKey
	primary string
}

// JWK represents a single JSON Web Key (RFC 7517).
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// JWKS is the JSON Web Key Set returned from /.well-known/jwks.json.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

func NewKeyStore() *KeyStore {
	return &KeyStore{keys: make(map[string]*ecdsa.PrivateKey)}
}

func LoadOrGenerateKeyStore(primaryPath, secondaryPath string) (*KeyStore, error) {
	ks := NewKeyStore()

	if primaryPath == "" {
		slog.Warn("JWT_PRIVATE_KEY_FILE not set — generating ephemeral EC key (development only!)")
		key, err := generateKey()
		if err != nil {
			return nil, err
		}
		kid := keyID(&key.PublicKey)
		ks.addKey(kid, key)
		ks.primary = kid
		return ks, nil
	}

	primary, err := loadKeyFromFile(primaryPath)
	if err != nil {
		return nil, err
	}
	kid := keyID(&primary.PublicKey)
	ks.addKey(kid, primary)
	ks.primary = kid

	if secondaryPath != "" {
		secondary, err := loadKeyFromFile(secondaryPath)
		if err != nil {
			slog.Warn("secondary JWT key could not be loaded", "error", err)
		} else {
			ks.addKey(keyID(&secondary.PublicKey), secondary)
		}
	}

	return ks, nil
}

func (ks *KeyStore) PrimaryKID() string {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.primary
}

func (ks *KeyStore) PrimaryKey() *ecdsa.PrivateKey {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.keys[ks.primary]
}

func (ks *KeyStore) PublicKey(kid string) (*ecdsa.PublicKey, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	k, ok := ks.keys[kid]
	if !ok {
		return nil, false
	}
	return &k.PublicKey, true
}

// PublicJWKS exports all public keys as a JWKS document.
func (ks *KeyStore) PublicJWKS() JWKS {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	jwks := JWKS{Keys: make([]JWK, 0, len(ks.keys))}
	for kid, key := range ks.keys {
		jwks.Keys = append(jwks.Keys, ecKeyToJWK(kid, &key.PublicKey))
	}
	return jwks
}

func (ks *KeyStore) addKey(kid string, key *ecdsa.PrivateKey) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.keys[kid] = key
}

// keyID returns the first 8 bytes of the SHA-256 fingerprint of the DER-encoded public key.
func keyID(pub *ecdsa.PublicKey) string {
	der, _ := x509.MarshalPKIXPublicKey(pub)
	h := sha256.Sum256(der)
	return hex.EncodeToString(h[:8])
}

func generateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

func loadKeyFromFile(path string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("PEM decode failed")
	}
	// Try PKCS#8 first — produced by `openssl pkcs8 -topk8` (gen-jwt-key.sh output).
	if keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		ecKey, ok := keyAny.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("key is not ECDSA")
		}
		return ecKey, nil
	}
	// Fall back to SEC1 / legacy EC PRIVATE KEY format.
	return x509.ParseECPrivateKey(block.Bytes)
}

// ecKeyToJWK converts a P-256 public key to JWK format.
// X and Y are zero-padded to 32 bytes before base64url encoding.
func ecKeyToJWK(kid string, pub *ecdsa.PublicKey) JWK {
	return JWK{
		Kty: "EC",
		Crv: "P-256",
		Kid: kid,
		Use: "sig",
		Alg: "ES256",
		X:   base64.RawURLEncoding.EncodeToString(padTo32(pub.X.Bytes())),
		Y:   base64.RawURLEncoding.EncodeToString(padTo32(pub.Y.Bytes())),
	}
}

func padTo32(b []byte) []byte {
	if len(b) == 32 {
		return b
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}
