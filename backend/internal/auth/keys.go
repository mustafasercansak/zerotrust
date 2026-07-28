package auth

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"sync"

	"github.com/golang-jwt/jwt/v5"
)

// Supported JWT signing algorithms. The algorithm is selected via the
// JWT_SIGNING_ALG environment variable; new algorithms (including future
// post-quantum signature schemes) only require a new entry here.
const (
	AlgEdDSA = "EdDSA"
	AlgES256 = "ES256"
	AlgRS256 = "RS256"
)

// keyEntry couples a private key with the JWT algorithm it signs with.
// Storing the algorithm per key allows zero-downtime rotation between
// different algorithms (e.g. EdDSA primary with an ES256 secondary).
type keyEntry struct {
	signer crypto.Signer
	alg    string
}

// KeyStore holds one or more signing keys indexed by kid.
// The primary key signs new tokens; secondary keys are kept for validation during rotation.
type KeyStore struct {
	mu      sync.RWMutex
	keys    map[string]keyEntry
	primary string
}

// JWK represents a single JSON Web Key (RFC 7517).
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv,omitempty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
}

// JWKS is the JSON Web Key Set returned from /.well-known/jwks.json.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// IsSupportedAlg reports whether alg is a JWT signing algorithm this build supports.
func IsSupportedAlg(alg string) bool {
	switch alg {
	case AlgEdDSA, AlgES256, AlgRS256:
		return true
	}
	return false
}

func NewKeyStore() *KeyStore {
	return &KeyStore{keys: make(map[string]keyEntry)}
}

// LoadOrGenerateKeyStore loads the primary (and optional secondary) signing key
// from PKCS#8 PEM files. The algorithm of file-based keys is derived from the
// key material itself. When primaryPath is empty, an ephemeral development key
// of the given algorithm is generated instead.
func LoadOrGenerateKeyStore(primaryPath, secondaryPath, alg string) (*KeyStore, error) {
	ks := NewKeyStore()

	if primaryPath == "" {
		if !IsSupportedAlg(alg) {
			return nil, fmt.Errorf("unsupported JWT signing algorithm %q", alg)
		}
		slog.Warn("JWT_PRIVATE_KEY_FILE not set — generating ephemeral signing key (development only!)", "alg", alg)
		signer, err := generateKey(alg)
		if err != nil {
			return nil, err
		}
		kid := keyID(signer.Public())
		ks.addKey(kid, keyEntry{signer: signer, alg: alg})
		ks.primary = kid
		return ks, nil
	}

	primary, err := loadKeyFromFile(primaryPath)
	if err != nil {
		return nil, err
	}
	kid := keyID(primary.signer.Public())
	ks.addKey(kid, primary)
	ks.primary = kid

	if secondaryPath != "" {
		secondary, err := loadKeyFromFile(secondaryPath)
		if err != nil {
			slog.Warn("secondary JWT key could not be loaded", "error", err)
		} else {
			ks.addKey(keyID(secondary.signer.Public()), secondary)
		}
	}

	return ks, nil
}

func (ks *KeyStore) PrimaryKID() string {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.primary
}

// PrimaryKey returns the primary private key (as a crypto.Signer).
func (ks *KeyStore) PrimaryKey() crypto.Signer {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.keys[ks.primary].signer
}

// PrimaryAlg returns the JWT algorithm of the primary key.
func (ks *KeyStore) PrimaryAlg() string {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.keys[ks.primary].alg
}

// SigningMethod returns the jwt.SigningMethod matching the primary key's algorithm.
func (ks *KeyStore) SigningMethod() jwt.SigningMethod {
	return signingMethodFor(ks.PrimaryAlg())
}

// Sign signs the claims with the primary key, setting the kid header.
func (ks *KeyStore) Sign(claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(ks.SigningMethod(), claims)
	token.Header["kid"] = ks.PrimaryKID()
	return token.SignedString(ks.PrimaryKey())
}

// PublicKey returns the public key and its JWT algorithm for the given kid.
// Callers validating tokens MUST reject tokens whose alg header does not
// match the returned algorithm (algorithm-confusion protection).
func (ks *KeyStore) PublicKey(kid string) (crypto.PublicKey, string, bool) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	e, ok := ks.keys[kid]
	if !ok {
		return nil, "", false
	}
	return e.signer.Public(), e.alg, true
}

// PublicJWKS exports all public keys as a JWKS document.
func (ks *KeyStore) PublicJWKS() JWKS {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	jwks := JWKS{Keys: make([]JWK, 0, len(ks.keys))}
	for kid, entry := range ks.keys {
		jwks.Keys = append(jwks.Keys, keyToJWK(kid, entry))
	}
	return jwks
}

func (ks *KeyStore) addKey(kid string, entry keyEntry) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.keys[kid] = entry
}

func signingMethodFor(alg string) jwt.SigningMethod {
	switch alg {
	case AlgES256:
		return jwt.SigningMethodES256
	case AlgRS256:
		return jwt.SigningMethodRS256
	default:
		return jwt.SigningMethodEdDSA
	}
}

// keyID returns the first 8 bytes of the SHA-256 fingerprint of the DER-encoded public key.
func keyID(pub crypto.PublicKey) string {
	der, _ := x509.MarshalPKIXPublicKey(pub)
	h := sha256.Sum256(der)
	return hex.EncodeToString(h[:8])
}

// generateKey creates a fresh private key for the given JWT algorithm.
func generateKey(alg string) (crypto.Signer, error) {
	switch alg {
	case AlgEdDSA:
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		return priv, err
	case AlgES256:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case AlgRS256:
		return rsa.GenerateKey(rand.Reader, 2048)
	}
	return nil, fmt.Errorf("unsupported JWT signing algorithm %q", alg)
}

// loadKeyFromFile parses a PKCS#8 PEM private key and derives its JWT algorithm
// from the key material: Ed25519 → EdDSA, ECDSA P-256 → ES256, RSA ≥ 2048 → RS256.
func loadKeyFromFile(path string) (keyEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return keyEntry{}, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return keyEntry{}, errors.New("PEM decode failed")
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return keyEntry{}, errors.New("failed to parse PKCS#8 private key")
	}
	switch key := keyAny.(type) {
	case ed25519.PrivateKey:
		return keyEntry{signer: key, alg: AlgEdDSA}, nil
	case *ecdsa.PrivateKey:
		if key.Curve != elliptic.P256() {
			return keyEntry{}, errors.New("ECDSA key must use the P-256 curve (ES256)")
		}
		return keyEntry{signer: key, alg: AlgES256}, nil
	case *rsa.PrivateKey:
		if key.N.BitLen() < 2048 {
			return keyEntry{}, errors.New("RSA key must be at least 2048 bits (RS256)")
		}
		return keyEntry{signer: key, alg: AlgRS256}, nil
	}
	return keyEntry{}, fmt.Errorf("unsupported private key type %T", keyAny)
}

// keyToJWK converts a public key to JWK format according to its algorithm.
func keyToJWK(kid string, entry keyEntry) JWK {
	jwk := JWK{Kid: kid, Use: "sig", Alg: entry.alg}
	switch pub := entry.signer.Public().(type) {
	case ed25519.PublicKey:
		jwk.Kty = "OKP"
		jwk.Crv = "Ed25519"
		jwk.X = base64.RawURLEncoding.EncodeToString(pub)
	case *ecdsa.PublicKey:
		jwk.Kty = "EC"
		jwk.Crv = "P-256"
		jwk.X = base64.RawURLEncoding.EncodeToString(pad32(pub.X))
		jwk.Y = base64.RawURLEncoding.EncodeToString(pad32(pub.Y))
	case *rsa.PublicKey:
		jwk.Kty = "RSA"
		jwk.N = base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
		jwk.E = base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	}
	return jwk
}

// pad32 left-pads a P-256 coordinate to its fixed 32-byte length (RFC 7518 §6.2.1.2).
func pad32(v *big.Int) []byte {
	b := v.Bytes()
	if len(b) >= 32 {
		return b
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}
