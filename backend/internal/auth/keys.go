package auth

import (
	"crypto/ed25519"
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

// KeyStore holds one or more Ed25519 keys indexed by kid.
// The primary key signs new tokens; secondary keys are kept for validation during rotation.
type KeyStore struct {
	mu      sync.RWMutex
	keys    map[string]ed25519.PrivateKey
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
	Y   string `json:"y,omitempty"`
}

// JWKS is the JSON Web Key Set returned from /.well-known/jwks.json.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

func NewKeyStore() *KeyStore {
	return &KeyStore{keys: make(map[string]ed25519.PrivateKey)}
}

func LoadOrGenerateKeyStore(primaryPath, secondaryPath string) (*KeyStore, error) {
	ks := NewKeyStore()

	if primaryPath == "" {
		slog.Warn("JWT_PRIVATE_KEY_FILE not set — generating ephemeral Ed25519 key (development only!)")
		key, err := generateKey()
		if err != nil {
			return nil, err
		}
		kid := keyID(key.Public().(ed25519.PublicKey))
		ks.addKey(kid, key)
		ks.primary = kid
		return ks, nil
	}

	primary, err := loadKeyFromFile(primaryPath)
	if err != nil {
		return nil, err
	}
	kid := keyID(primary.Public().(ed25519.PublicKey))
	ks.addKey(kid, primary)
	ks.primary = kid

	if secondaryPath != "" {
		secondary, err := loadKeyFromFile(secondaryPath)
		if err != nil {
			slog.Warn("secondary JWT key could not be loaded", "error", err)
		} else {
			ks.addKey(keyID(secondary.Public().(ed25519.PublicKey)), secondary)
		}
	}

	return ks, nil
}

func (ks *KeyStore) PrimaryKID() string {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.primary
}

func (ks *KeyStore) PrimaryKey() ed25519.PrivateKey {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.keys[ks.primary]
}

func (ks *KeyStore) PublicKey(kid string) (ed25519.PublicKey, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	k, ok := ks.keys[kid]
	if !ok {
		return nil, false
	}
	return k.Public().(ed25519.PublicKey), true
}

// PublicJWKS exports all public keys as a JWKS document.
func (ks *KeyStore) PublicJWKS() JWKS {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	jwks := JWKS{Keys: make([]JWK, 0, len(ks.keys))}
	for kid, key := range ks.keys {
		jwks.Keys = append(jwks.Keys, ed25519KeyToJWK(kid, key.Public().(ed25519.PublicKey)))
	}
	return jwks
}

func (ks *KeyStore) addKey(kid string, key ed25519.PrivateKey) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.keys[kid] = key
}

// keyID returns the first 8 bytes of the SHA-256 fingerprint of the DER-encoded public key.
func keyID(pub ed25519.PublicKey) string {
	der, _ := x509.MarshalPKIXPublicKey(pub)
	h := sha256.Sum256(der)
	return hex.EncodeToString(h[:8])
}

func generateKey() (ed25519.PrivateKey, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	return priv, err
}

func loadKeyFromFile(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("PEM decode failed")
	}
	// Try PKCS#8.
	if keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		edKey, ok := keyAny.(ed25519.PrivateKey)
		if !ok {
			return nil, errors.New("key is not Ed25519")
		}
		return edKey, nil
	}
	return nil, errors.New("failed to parse Ed25519 private key")
}

// ed25519KeyToJWK converts an Ed25519 public key to JWK format.
func ed25519KeyToJWK(kid string, pub ed25519.PublicKey) JWK {
	return JWK{
		Kty: "OKP",
		Crv: "Ed25519",
		Kid: kid,
		Use: "sig",
		Alg: "EdDSA",
		X:   base64.RawURLEncoding.EncodeToString(pub),
	}
}
