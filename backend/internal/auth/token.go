package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("invalid_token")
	ErrExpiredToken = errors.New("token_expired")
)

const (
	SubTypeUser    = "user"
	SubTypeService = "service"
)

type Confirmation struct {
	JKT string `json:"jkt,omitempty"`
}

type Claims struct {
	// User token fields
	UserID      string   `json:"uid,omitempty"`
	Email       string   `json:"email,omitempty"`
	Locale      string   `json:"locale,omitempty"`
	Roles       []string `json:"roles,omitempty"`
	Permissions []string `json:"perms,omitempty"`

	// Service token fields
	ClientID string   `json:"cid,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`

	// Common
	SubType string `json:"sub_type"`

	Confirmation *Confirmation `json:"cnf,omitempty"`

	jwt.RegisteredClaims
}

// HasPermission checks whether this token carries the given resource:action.
func (c *Claims) HasPermission(resource, action string) bool {
	perm := resource + ":" + action
	list := c.Permissions
	if c.SubType == SubTypeService {
		list = c.Scopes
	}
	for _, p := range list {
		if p == perm {
			return true
		}
	}
	return false
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type ServiceTokenResponse struct {
	AccessToken string
	ExpiresIn   int64
}

func GenerateTokenPair(ks *KeyStore, userID, email, locale string, roles, permissions []string, accessTTL time.Duration) (*TokenPair, error) {
	now := time.Now()
	claims := Claims{
		UserID:      userID,
		Email:       email,
		Locale:      locale,
		Roles:       roles,
		Permissions: permissions,
		SubType:     SubTypeUser,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	signed, err := signClaims(ks, claims)
	if err != nil {
		return nil, err
	}

	refresh, err := generateOpaqueToken()
	if err != nil {
		return nil, err
	}

	return &TokenPair{AccessToken: signed, RefreshToken: refresh}, nil
}

func GenerateServiceToken(ks *KeyStore, clientID, name string, scopes []string, ttl time.Duration, dpopJKT string) (*ServiceTokenResponse, error) {
	now := time.Now()
	claims := Claims{
		ClientID: clientID,
		Scopes:   scopes,
		SubType:  SubTypeService,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   name,
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	if dpopJKT != "" {
		claims.Confirmation = &Confirmation{
			JKT: dpopJKT,
		}
	}

	signed, err := signClaims(ks, claims)
	if err != nil {
		return nil, err
	}

	return &ServiceTokenResponse{
		AccessToken: signed,
		ExpiresIn:   int64(ttl.Seconds()),
	}, nil
}

func ValidateAccessToken(ks *KeyStore, tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, ErrInvalidToken
		}
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, ErrInvalidToken
		}
		pub, exists := ks.PublicKey(kid)
		if !exists {
			return nil, ErrInvalidToken
		}
		return pub, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func signClaims(ks *KeyStore, claims Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = ks.PrimaryKID()
	return token.SignedString(ks.PrimaryKey())
}

func generateOpaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
