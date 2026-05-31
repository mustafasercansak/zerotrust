package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/geoip"
	"github.com/zerotrust/backend/pkg/mailer"
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
	Setup(ctx context.Context, userID, email, currentCode string) (string, string, []string, error)
	VerifyAndEnable(ctx context.Context, userID, code string) error
}

// SessionStore persists refresh-token sessions.
// Defined here to keep auth self-contained; implemented by session.Repository.
type SessionStore interface {
	Create(ctx context.Context, userID, tokenHash, ip, userAgent string, deviceInfo map[string]string, expiresAt time.Time) error
	RevokeForDevice(ctx context.Context, userID, ip, userAgent string, deviceInfo map[string]string) error
	// RotateSession revokes the old session and atomically creates a new one.
	// generate is called under a row lock with the owning userID, the old
	// session's last-active timestamp and its fixed absolute expiry.
	RotateSession(ctx context.Context, oldHash string, generate func(userID string, lastActiveAt, currentExpiresAt time.Time) (newHash, ip, ua string, deviceInfo map[string]string, expiresAt time.Time, err error)) error
	Revoke(ctx context.Context, hash string) error
	// EvictExcessSessions keeps the `keep` most-recently-active sessions and
	// revokes the rest. Call with keep = maxAllowed-1 before creating a new
	// session to enforce a per-user session cap.
	EvictExcessSessions(ctx context.Context, userID string, keep int) error
	// CheckReuse returns the owning userID if the hash belongs to an already-
	// used refresh token, indicating a potential token reuse attack.
	CheckReuse(ctx context.Context, hash string) (string, error)
	// RevokeAllForUser terminates every active session for the user. Used as
	// the security response when token reuse is detected.
	RevokeAllForUser(ctx context.Context, userID string) error
}

// ServiceAccountRecord maps data returned from saStoreAdapter.
type ServiceAccountRecord struct {
	ID                  string
	ClientID            string
	Name                string
	ClientSecretHash    string
	OldClientSecretHash *string
	OldSecretExpiresAt  *time.Time
	IsActive            bool
	Scopes              []string
	ExpiresAt           *time.Time
}

// ServiceAccountStore abstracts service account lookups for the auth service.
type ServiceAccountStore interface {
	FindByClientID(ctx context.Context, clientID string) (*ServiceAccountRecord, error)
	CheckSecret(hash, secret string) bool
}

// SettingReader provides cached access to system settings. nil disables the
// feature and the caller falls back to a hardcoded default.
type SettingReader interface {
	GetInt(ctx context.Context, key string, defaultVal int) int
	GetString(ctx context.Context, key string, defaultVal string) string
	GetBool(ctx context.Context, key string, defaultVal bool) bool
}

const (
	AccessTTL       = 1 * time.Minute
	RefreshTTL      = 8 * time.Hour
	serviceTokenTTL = 5 * time.Minute
	mfaPendingTTL   = 5 * time.Minute

	defaultSessionIdleTimeout      = 5 * time.Minute
	defaultAdminSessionIdleTimeout = 3 * time.Minute
	defaultSessionAbsoluteTimeout  = 8 * time.Hour

	settingSessionIdleTimeoutSec      = "session_idle_timeout_seconds"
	settingSessionIdleTimeoutAdminSec = "session_idle_timeout_seconds_admin"
	settingSessionAbsoluteTimeoutSec  = "session_absolute_timeout_seconds"
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
	Pair             *TokenPair
	MFARequired      bool
	MFAPendingToken  string
	MFASetupSecret   string
	MFASetupURL      string
	MFARecoveryCodes []string
	AnomalyType      string
	AnomalyDetails   string
}

type Service struct {
	users    UserReader
	sessions SessionStore
	saSvc    ServiceAccountStore
	mfa      MFAChecker    // nil when MFA is globally disabled
	settings SettingReader // nil falls back to hardcoded defaults
	rdb      *redis.Client
	ks       *KeyStore
	geoip    *geoip.Service
	mailer   mailer.Mailer
}

func NewService(users UserReader, sessions SessionStore, saSvc ServiceAccountStore, rdb *redis.Client, ks *KeyStore, mfa MFAChecker, settings SettingReader) *Service {
	return &Service{users: users, sessions: sessions, saSvc: saSvc, mfa: mfa, settings: settings, rdb: rdb, ks: ks}
}

func (s *Service) ConfigureSecurityAnomalies(g *geoip.Service, m mailer.Mailer) {
	s.geoip = g
	s.mailer = m
}

// Login authenticates a user. If MFA is enabled for the account it returns a
// LoginResult with MFARequired=true and a short-lived pending token; otherwise
// it returns a full TokenPair.
func (s *Service) Login(ctx context.Context, email, password, ip, ua string, deviceInfo map[string]string) (*LoginResult, error) {
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

	// Anomaly checking
	hasAnomaly, anomalyType, anomalyDetails := s.detectLoginAnomaly(ctx, u.ID, u.Email, ip, ua, deviceInfo)
	if hasAnomaly {
		slog.Warn("Login anomaly detected", "user_id", u.ID, "type", anomalyType, "details", anomalyDetails)

		// Send security alert email
		if s.mailer != nil {
			var locStr string
			if s.geoip != nil {
				if loc, err := s.geoip.Lookup(ip); err == nil {
					locStr = formatLocation(loc)
				}
			}
			if locStr == "" {
				locStr = "Unknown"
			}
			err := s.mailer.SendSecurityAlert(ctx, u.Email, anomalyType, ip, locStr, anomalyDetails)
			if err != nil {
				slog.Error("failed to queue security alert email", "user_id", u.ID, "error", err)
			}
		}
	}

	globalMFARequired := false
	if s.settings != nil {
		globalMFARequired = s.settings.GetBool(ctx, "global_mfa_required", false)
	}

	if s.mfa != nil && (s.mfa.IsEnabled(ctx, u.ID) || globalMFARequired) {
		token, err := generateOpaqueToken()
		if err != nil {
			return nil, err
		}

		var setupSecret, setupURL string
		var recoveryCodes []string
		if globalMFARequired && !s.mfa.IsEnabled(ctx, u.ID) {
			url, secret, recCodes, err := s.mfa.Setup(ctx, u.ID, u.Email, "")
			if err != nil {
				return nil, err
			}
			setupSecret = secret
			setupURL = url
			recoveryCodes = recCodes
		}

		data, _ := json.Marshal(map[string]any{
			"uid":            u.ID,
			"ip":             ip,
			"ua":             ua,
			"device_info":    deviceInfo,
			"setup_secret":   setupSecret,
			"setup_url":      setupURL,
			"recovery_codes": recoveryCodes,
		})
		if err := s.rdb.Set(ctx, mfaPendingKey(hashToken(token)), string(data), mfaPendingTTL).Err(); err != nil {
			return nil, err
		}
		return &LoginResult{
			MFARequired:      true,
			MFAPendingToken:  token,
			MFASetupSecret:   setupSecret,
			MFASetupURL:      setupURL,
			MFARecoveryCodes: recoveryCodes,
			AnomalyType:      anomalyType,
			AnomalyDetails:   anomalyDetails,
		}, nil
	}

	pair, err := s.completeLogin(ctx, u, ip, ua, deviceInfo)
	if err != nil {
		return nil, err
	}
	return &LoginResult{
		Pair:           pair,
		AnomalyType:    anomalyType,
		AnomalyDetails: anomalyDetails,
	}, nil
}

func (s *Service) detectLoginAnomaly(ctx context.Context, userID string, _ string, currentIP, currentUA string, _ map[string]string) (bool, string, string) {
	if s.geoip == nil {
		return false, "", ""
	}

	lister, ok := s.sessions.(interface {
		GetActiveSessions(ctx context.Context, userID string) ([]map[string]any, error)
	})
	if !ok {
		return false, "", ""
	}

	activeSessions, err := lister.GetActiveSessions(ctx, userID)
	if err != nil {
		slog.Error("failed to list active sessions for anomaly detection", "user_id", userID, "error", err)
		return false, "", ""
	}

	// Geolocate current IP
	currLoc, err := s.geoip.Lookup(currentIP)
	if err != nil {
		slog.Warn("failed to geolocate current IP", "ip", currentIP, "error", err)
		return false, "", ""
	}

	// Check if device/browser has been seen before for this user
	isFirstTimeDevice := true
	for _, sess := range activeSessions {
		ua, _ := sess["user_agent"].(string)
		if ua == currentUA {
			isFirstTimeDevice = false
			break
		}
	}

	var travelAnomalyDetails string

	// Detect impossible travel
	for _, sess := range activeSessions {
		ip, _ := sess["ip_address"].(string)
		if ip == "" || ip == currentIP {
			continue
		}

		sessLoc, err := s.geoip.Lookup(ip)
		if err != nil {
			continue
		}

		distance := haversineDistance(currLoc.Latitude, currLoc.Longitude, sessLoc.Latitude, sessLoc.Longitude)
		if distance < 10.0 {
			continue
		}

		var lastActive time.Time
		if val, exists := sess["last_used_at"]; exists && val != nil {
			if t, ok := val.(*time.Time); ok && t != nil {
				lastActive = *t
			} else if t, ok := val.(time.Time); ok {
				lastActive = t
			}
		}
		if lastActive.IsZero() {
			if val, exists := sess["created_at"]; exists {
				if t, ok := val.(time.Time); ok {
					lastActive = t
				}
			}
		}
		if lastActive.IsZero() {
			lastActive = time.Now()
		}

		timeDiff := time.Since(lastActive)
		if timeDiff <= 0 {
			timeDiff = 1 * time.Second
		}

		// Calculate velocity in km/h
		velocity := distance / timeDiff.Hours()

		// If velocity > 800 km/h and time difference is under 24 hours, flag it
		if velocity > 800.0 && timeDiff < 24*time.Hour {
			travelAnomalyDetails = fmt.Sprintf("Impossible travel detected from %s (%s) to %s (%s). Distance: %.1f km, Time elapsed: %s, Velocity: %.1f km/h",
				ip, formatLocation(sessLoc),
				currentIP, formatLocation(currLoc),
				distance, timeDiff.Round(time.Second), velocity)
			break
		}
	}

	if travelAnomalyDetails != "" {
		return true, "impossible_travel", travelAnomalyDetails
	}

	if isFirstTimeDevice && len(activeSessions) > 0 {
		return true, "new_device", fmt.Sprintf("New/first-time device login from IP %s (%s)", currentIP, formatLocation(currLoc))
	}

	return false, "", ""
}

func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0

	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0

	radLat1 := lat1 * math.Pi / 180.0
	radLat2 := lat2 * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(radLat1)*math.Cos(radLat2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

func formatLocation(l *geoip.Location) string {
	if l.City != "" && l.Country != "" {
		return fmt.Sprintf("%s, %s", l.City, l.Country)
	}
	if l.Country != "" {
		return l.Country
	}
	return "Unknown"
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

	var m struct {
		UID         string            `json:"uid"`
		IP          string            `json:"ip"`
		UA          string            `json:"ua"`
		DeviceInfo  map[string]string `json:"device_info"`
		SetupSecret string            `json:"setup_secret"`
		SetupURL    string            `json:"setup_url"`
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, ErrInvalidCredentials
	}

	userID := m.UID
	if s.mfa == nil {
		return nil, ErrInvalidCredentials
	}

	if m.SetupSecret != "" {
		if err := s.mfa.VerifyAndEnable(ctx, userID, totpCode); err != nil {
			return nil, ErrInvalidCredentials
		}
	} else {
		if !s.mfa.Validate(ctx, userID, totpCode) {
			return nil, ErrInvalidCredentials
		}
	}

	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.completeLogin(ctx, u, m.IP, m.UA, m.DeviceInfo)
}

// completeLogin generates tokens and creates a session row. Called after all
// authentication factors have been verified.
func (s *Service) completeLogin(ctx context.Context, u *user.User, ip, ua string, deviceInfo map[string]string) (*TokenPair, error) {
	perms, err := s.users.GetPermissions(ctx, u.ID)
	if err != nil {
		slog.Error("failed to load permissions", "user_id", u.ID, "error", err)
	}
	pair, err := GenerateTokenPair(s.ks, u.ID, u.Email, u.Locale, u.Roles, perms, AccessTTL)
	if err != nil {
		return nil, err
	}
	// Enforce per-user session cap: keep the N-1 most recently active sessions,
	// evict any older ones, then create the new session (total stays at ≤N).
	maxSessionsPerUser := 5
	if s.settings != nil {
		maxSessionsPerUser = s.settings.GetInt(ctx, "max_sessions_per_user", 5)
	}
	if err := s.sessions.RevokeForDevice(ctx, u.ID, ip, ua, deviceInfo); err != nil {
		slog.Error("failed to revoke existing same-device sessions on login", "user_id", u.ID, "error", err)
	}
	if err := s.sessions.EvictExcessSessions(ctx, u.ID, maxSessionsPerUser-1); err != nil {
		slog.Error("failed to evict excess sessions on login", "user_id", u.ID, "error", err)
	}
	if err := s.sessions.Create(ctx, u.ID, hashToken(pair.RefreshToken), ip, ua, deviceInfo, time.Now().Add(s.sessionAbsoluteTimeout(ctx))); err != nil {
		return nil, err
	}
	return pair, nil
}

// RefreshTokens atomically rotates the refresh token inside a single DB transaction.
// The FOR UPDATE lock inside RotateSession prevents two concurrent requests from
// both succeeding with the same refresh token.
func (s *Service) RefreshTokens(ctx context.Context, refreshToken, ip, ua string, deviceInfo map[string]string) (*TokenPair, error) {
	var pair *TokenPair

	tokenHash := hashToken(refreshToken)
	err := s.sessions.RotateSession(ctx, tokenHash,
		func(userID string, lastActiveAt, currentExpiresAt time.Time) (string, string, string, map[string]string, time.Time, error) {
			u, err := s.users.FindByID(ctx, userID)
			if err != nil {
				return "", "", "", nil, time.Time{}, ErrInvalidToken
			}

			idleLimit := s.sessionIdleTimeout(ctx)
			if hasAdminRole(u.Roles) {
				idleLimit = s.adminSessionIdleTimeout(ctx)
			}
			if idleLimit > 0 && time.Since(lastActiveAt) > idleLimit {
				return "", "", "", nil, time.Time{}, ErrInvalidToken
			}
			if !currentExpiresAt.After(time.Now()) {
				return "", "", "", nil, time.Time{}, ErrInvalidToken
			}

			perms, err := s.users.GetPermissions(ctx, u.ID)
			if err != nil {
				slog.Error("failed to load permissions", "user_id", u.ID, "error", err)
			}

			p, err := GenerateTokenPair(s.ks, u.ID, u.Email, u.Locale, u.Roles, perms, AccessTTL)
			if err != nil {
				return "", "", "", nil, time.Time{}, err
			}
			pair = p
			// Keep the original absolute expiry fixed for the session chain.
			return hashToken(p.RefreshToken), ip, ua, deviceInfo, currentExpiresAt, nil
		},
	)
	if err != nil {
		// Check whether this was a revoked token being replayed (token theft).
		// If so, revoke every remaining session to force the legitimate user to
		// re-authenticate — they will notice the unexpected logout.
		if userID, cerr := s.sessions.CheckReuse(ctx, tokenHash); cerr == nil && userID != "" {
			if rerr := s.sessions.RevokeAllForUser(ctx, userID); rerr != nil {
				slog.Error("failed to revoke sessions after token reuse", "user_id", userID, "error", rerr)
			}
			slog.Warn("refresh token reuse detected — possible session hijack, all sessions revoked",
				"user_id", userID)
		}
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
	secretValid := s.saSvc.CheckSecret(sa.ClientSecretHash, secret)
	if !secretValid && sa.OldClientSecretHash != nil && sa.OldSecretExpiresAt != nil && time.Now().Before(*sa.OldSecretExpiresAt) {
		secretValid = s.saSvc.CheckSecret(*sa.OldClientSecretHash, secret)
	}
	if !secretValid {
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
	if s.rdb == nil {
		return false
	}
	exists, err := s.rdb.Exists(ctx, jtiBlocklistKey(jti)).Result()
	return err == nil && exists > 0
}

func (s *Service) checkLockout(ctx context.Context, email string) error {
	if s.rdb == nil {
		return nil
	}
	ttl, err := s.rdb.TTL(ctx, lockoutKey(email)).Result()
	if err == nil && ttl > 0 {
		return &AccountLockedError{RetryAfter: ttl}
	}
	return nil
}
func (s *Service) recordFailedAttempt(ctx context.Context, email string) {
	if s.rdb == nil {
		return
	}
	fkey := failKey(email)
	count, _ := s.rdb.Incr(ctx, fkey).Result()

	maxAttempts := 5
	if s.settings != nil {
		maxAttempts = s.settings.GetInt(ctx, "max_login_attempts", 5)
	}

	if count >= int64(maxAttempts) {
		diff := count - int64(maxAttempts)
		var d time.Duration
		switch {
		case diff < 3:
			d = 1 * time.Minute
		case diff < 6:
			d = 5 * time.Minute
		default:
			d = 30 * time.Minute
		}
		s.rdb.Set(ctx, lockoutKey(email), "1", d)
	}
}

func (s *Service) clearFailedAttempts(ctx context.Context, email string) {
	if s.rdb == nil {
		return
	}
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

func (s *Service) sessionIdleTimeout(ctx context.Context) time.Duration {
	if s.settings == nil {
		return defaultSessionIdleTimeout
	}
	seconds := s.settings.GetInt(ctx, settingSessionIdleTimeoutSec, int(defaultSessionIdleTimeout.Seconds()))
	if seconds <= 0 {
		return defaultSessionIdleTimeout
	}
	return time.Duration(seconds) * time.Second
}

func (s *Service) adminSessionIdleTimeout(ctx context.Context) time.Duration {
	if s.settings == nil {
		return defaultAdminSessionIdleTimeout
	}
	seconds := s.settings.GetInt(ctx, settingSessionIdleTimeoutAdminSec, int(defaultAdminSessionIdleTimeout.Seconds()))
	if seconds <= 0 {
		return defaultAdminSessionIdleTimeout
	}
	return time.Duration(seconds) * time.Second
}

func (s *Service) sessionAbsoluteTimeout(ctx context.Context) time.Duration {
	if s.settings == nil {
		return defaultSessionAbsoluteTimeout
	}
	seconds := s.settings.GetInt(ctx, settingSessionAbsoluteTimeoutSec, int(defaultSessionAbsoluteTimeout.Seconds()))
	if seconds <= 0 {
		return defaultSessionAbsoluteTimeout
	}
	return time.Duration(seconds) * time.Second
}

func hasAdminRole(roles []string) bool {
	for _, role := range roles {
		if role == "admin" {
			return true
		}
	}
	return false
}
