package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/user"
)

// SessionStore persists refresh-token sessions.
// Defined here to keep auth self-contained; implemented by session.Repository.
type SessionStore interface {
	Create(ctx context.Context, userID, tokenHash, ip, userAgent string, expiresAt time.Time) error
	// RotateSession revokes the old session and atomically creates a new one.
	// generate is called under a row lock with the owning userID.
	RotateSession(ctx context.Context, oldHash string, generate func(userID string) (newHash, ip, ua string, expiresAt time.Time, err error)) error
	Revoke(ctx context.Context, hash string) error
}

// ServiceAccountRecord is the data auth.Service needs when issuing service tokens.
// It is populated by whatever implements ServiceAccountStore (avoids import cycle).
type ServiceAccountRecord struct {
	Name             string
	ClientSecretHash string
	Scopes           []string
	IsActive         bool
	ExpiresAt        *time.Time
}

// ServiceAccountStore abstracts service account lookups for the auth service.
type ServiceAccountStore interface {
	FindByClientID(ctx context.Context, clientID string) (*ServiceAccountRecord, error)
	CheckSecret(hash, secret string) bool
}

const (
	AccessTTL       = 1 * time.Minute
	RefreshTTL      = 7 * 24 * time.Hour
	serviceTokenTTL = 5 * time.Minute
)

var (
	ErrInvalidCredentials = errors.New("invalid_credentials")
	ErrInactiveUser       = errors.New("user_inactive")
)

type AccountLockedError struct {
	RetryAfter time.Duration
}

func (e *AccountLockedError) Error() string { return "account_locked" }

func progressiveLockout(attempts int64) time.Duration {
	switch {
	case attempts < 5:
		return 0
	case attempts < 8:
		return 1 * time.Minute
	case attempts < 11:
		return 5 * time.Minute
	default:
		return 30 * time.Minute
	}
}

type Service struct {
	users    *user.Service
	sessions SessionStore
	saSvc    ServiceAccountStore
	rdb      *redis.Client
	ks       *KeyStore
}

func NewService(users *user.Service, sessions SessionStore, saSvc ServiceAccountStore, rdb *redis.Client, ks *KeyStore) *Service {
	return &Service{users: users, sessions: sessions, saSvc: saSvc, rdb: rdb, ks: ks}
}

// Login authenticates a user and creates a new session. ip and ua are stored for audit.
func (s *Service) Login(ctx context.Context, email, password, ip, ua string) (*TokenPair, error) {
	if err := s.checkLockout(ctx, email); err != nil {
		return nil, err
	}

	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if !u.IsActive {
		return nil, ErrInvalidCredentials
	}
	if !s.users.CheckPassword(u.PasswordHash, password) {
		s.recordFailedAttempt(ctx, email)
		return nil, ErrInvalidCredentials
	}

	s.clearFailedAttempts(ctx, email)

	perms, _ := s.users.GetPermissions(ctx, u.ID)

	pair, err := GenerateTokenPair(s.ks, u.ID, u.Email, u.Locale, u.Roles, perms, AccessTTL)
	if err != nil {
		return nil, err
	}
	if err := s.sessions.Create(ctx, u.ID, hashToken(pair.RefreshToken), ip, ua, time.Now().Add(RefreshTTL)); err != nil {
		return nil, err
	}
	return pair, nil
}

// RefreshTokens atomically rotates the refresh token inside a single DB transaction.
// The FOR UPDATE lock inside RotateSession prevents two concurrent requests from
// both succeeding with the same refresh token.
func (s *Service) RefreshTokens(ctx context.Context, refreshToken, ip, ua string) (*TokenPair, error) {
	var pair *TokenPair

	err := s.sessions.RotateSession(ctx, hashToken(refreshToken),
		func(userID string) (string, string, string, time.Time, error) {
			u, err := s.users.FindByID(ctx, userID)
			if err != nil {
				return "", "", "", time.Time{}, ErrInvalidToken
			}
			perms, _ := s.users.GetPermissions(ctx, u.ID)

			p, err := GenerateTokenPair(s.ks, u.ID, u.Email, u.Locale, u.Roles, perms, AccessTTL)
			if err != nil {
				return "", "", "", time.Time{}, err
			}
			pair = p
			return hashToken(p.RefreshToken), ip, ua, time.Now().Add(RefreshTTL), nil
		},
	)
	if err != nil {
		return nil, ErrInvalidToken
	}
	return pair, nil
}

func (s *Service) ClientCredentials(ctx context.Context, clientID, secret string) (*ServiceTokenResponse, error) {
	sa, err := s.saSvc.FindByClientID(ctx, clientID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if !sa.IsActive {
		return nil, ErrInactiveUser
	}
	if sa.ExpiresAt != nil && time.Now().After(*sa.ExpiresAt) {
		return nil, ErrInactiveUser
	}
	if !s.saSvc.CheckSecret(sa.ClientSecretHash, secret) {
		return nil, ErrInvalidCredentials
	}
	return GenerateServiceToken(s.ks, clientID, sa.Name, sa.Scopes, serviceTokenTTL)
}

// Logout revokes the session and blocklists the access token JTI.
func (s *Service) Logout(ctx context.Context, refreshToken, accessToken string) error {
	if refreshToken != "" {
		s.sessions.Revoke(ctx, hashToken(refreshToken))
	}
	if accessToken != "" {
		if claims, err := ValidateAccessToken(s.ks, accessToken); err == nil {
			s.revokeJTI(ctx, claims.ID, time.Until(claims.ExpiresAt.Time))
		}
	}
	return nil
}

func (s *Service) IsRevoked(ctx context.Context, jti string) bool {
	exists, err := s.rdb.Exists(ctx, jtiBlocklistKey(jti)).Result()
	return err == nil && exists > 0
}

func (s *Service) checkLockout(ctx context.Context, email string) error {
	ttl, err := s.rdb.TTL(ctx, lockoutKey(email)).Result()
	if err == nil && ttl > 0 {
		return &AccountLockedError{RetryAfter: ttl}
	}
	return nil
}

func (s *Service) recordFailedAttempt(ctx context.Context, email string) {
	fkey := failKey(email)
	count, _ := s.rdb.Incr(ctx, fkey).Result()
	if d := progressiveLockout(count); d > 0 {
		s.rdb.Set(ctx, lockoutKey(email), "1", d)
	}
}

func (s *Service) clearFailedAttempts(ctx context.Context, email string) {
	s.rdb.Del(ctx, failKey(email), lockoutKey(email))
}

func (s *Service) revokeJTI(ctx context.Context, jti string, ttl time.Duration) {
	if ttl > 0 {
		s.rdb.Set(ctx, jtiBlocklistKey(jti), "1", ttl)
	}
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func jtiBlocklistKey(jti string) string {
	return "jti:blocked:" + jti
}

func lockoutKey(email string) string {
	return fmt.Sprintf("login:locked:%x", sha256.Sum256([]byte(email)))
}

func failKey(email string) string {
	return fmt.Sprintf("login:fails:%x", sha256.Sum256([]byte(email)))
}
