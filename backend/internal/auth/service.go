package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/user"
)

// UserReader abstracts the user-service methods that auth.Service needs.
// *user.Service satisfies this interface; tests supply a lightweight fake.
type UserReader interface {
	FindByEmail(ctx context.Context, email string) (*user.User, error)
	FindByID(ctx context.Context, id string) (*user.User, error)
	CheckPassword(hash, password string) bool
	GetPermissions(ctx context.Context, userID string) ([]string, error)
}

// MFAChecker is implemented by mfa.Service and injected into auth.Service.
// When nil, MFA is disabled globally.
type MFAChecker interface {
	IsEnabled(ctx context.Context, userID string) bool
	Validate(ctx context.Context, userID, code string) bool
}

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
	mfaPendingTTL   = 5 * time.Minute
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

// LoginResult is returned by Service.Login.
// When MFARequired is true, Pair is nil and MFAPendingToken holds a short-lived
// opaque token the client must exchange via MFAChallenge.
type LoginResult struct {
	Pair            *TokenPair
	MFARequired     bool
	MFAPendingToken string
}

type Service struct {
	users    UserReader
	sessions SessionStore
	saSvc    ServiceAccountStore
	mfa      MFAChecker // nil when MFA is globally disabled
	rdb      *redis.Client
	ks       *KeyStore
}

func NewService(users UserReader, sessions SessionStore, saSvc ServiceAccountStore, rdb *redis.Client, ks *KeyStore, mfa MFAChecker) *Service {
	return &Service{users: users, sessions: sessions, saSvc: saSvc, mfa: mfa, rdb: rdb, ks: ks}
}

// Login authenticates a user. If MFA is enabled for the account it returns a
// LoginResult with MFARequired=true and a short-lived pending token; otherwise
// it returns a full TokenPair.
func (s *Service) Login(ctx context.Context, email, password, ip, ua string) (*LoginResult, error) {
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

	// Password correct — clear lockout counters regardless of MFA outcome.
	s.clearFailedAttempts(ctx, email)

	if s.mfa != nil && s.mfa.IsEnabled(ctx, u.ID) {
		token, err := generateOpaqueToken()
		if err != nil {
			return nil, err
		}
		data, _ := json.Marshal(map[string]string{"uid": u.ID, "ip": ip, "ua": ua})
		if err := s.rdb.Set(ctx, mfaPendingKey(hashToken(token)), string(data), mfaPendingTTL).Err(); err != nil {
			return nil, err
		}
		return &LoginResult{MFARequired: true, MFAPendingToken: token}, nil
	}

	pair, err := s.completeLogin(ctx, u, ip, ua)
	if err != nil {
		return nil, err
	}
	return &LoginResult{Pair: pair}, nil
}

// MFAChallenge completes a login that required a second factor.
// pendingToken is the opaque token from LoginResult.MFAPendingToken;
// totpCode is the 6-digit TOTP code from the user's authenticator app.
func (s *Service) MFAChallenge(ctx context.Context, pendingToken, totpCode string) (*TokenPair, error) {
	key := mfaPendingKey(hashToken(pendingToken))
	raw, err := s.rdb.GetDel(ctx, key).Result()
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, ErrInvalidCredentials
	}

	userID := m["uid"]
	if s.mfa == nil || !s.mfa.Validate(ctx, userID, totpCode) {
		return nil, ErrInvalidCredentials
	}

	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.completeLogin(ctx, u, m["ip"], m["ua"])
}

// completeLogin generates tokens and creates a session row. Called after all
// authentication factors have been verified.
func (s *Service) completeLogin(ctx context.Context, u *user.User, ip, ua string) (*TokenPair, error) {
	perms, err := s.users.GetPermissions(ctx, u.ID)
	if err != nil {
		slog.Error("failed to load permissions", "user_id", u.ID, "error", err)
	}
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
			perms, err := s.users.GetPermissions(ctx, u.ID)
			if err != nil {
				slog.Error("failed to load permissions", "user_id", u.ID, "error", err)
			}

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

func mfaPendingKey(hash string) string {
	return "mfa:pending:" + hash
}

func lockoutKey(email string) string {
	return fmt.Sprintf("login:locked:%x", sha256.Sum256([]byte(email)))
}

func failKey(email string) string {
	return fmt.Sprintf("login:fails:%x", sha256.Sum256([]byte(email)))
}
