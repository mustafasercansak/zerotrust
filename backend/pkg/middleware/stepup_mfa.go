package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/auth"
)

const defaultStepUpMFAWindow = 10 * time.Minute

// RequireRecentMFA enforces a recent second-factor proof for sensitive actions.
// If no recent proof exists for the current session, callers must send
// X-MFA-Code with a valid live TOTP code.
func RequireRecentMFA(mfa auth.MFAChecker, rdb *redis.Client, window time.Duration) func(http.Handler) http.Handler {
	if window <= 0 {
		window = defaultStepUpMFAWindow
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFrom(r.Context())
			if claims == nil {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if mfa == nil || rdb == nil {
				writeError(w, http.StatusServiceUnavailable, "mfa_unavailable")
				return
			}
			if !mfa.IsEnabled(r.Context(), claims.UserID) {
				writeError(w, http.StatusForbidden, "mfa_required")
				return
			}

			rt, err := refreshTokenFromCookie(r)
			if err != nil {
				writeError(w, http.StatusForbidden, "mfa_required")
				return
			}

			sessionHash := hashOpaqueToken(rt)
			if hasRecentMFA(r.Context(), rdb, claims.UserID, sessionHash) {
				next.ServeHTTP(w, r)
				return
			}

			code := strings.TrimSpace(r.Header.Get("X-MFA-Code"))
			if code == "" || !mfa.Validate(r.Context(), claims.UserID, code) {
				writeError(w, http.StatusForbidden, "mfa_required")
				return
			}

			if err := markRecentMFA(r.Context(), rdb, claims.UserID, sessionHash, window); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// MarkRecentMFACookie marks the current refresh-token session as recently MFA-verified.
func MarkRecentMFACookie(ctx context.Context, rdb *redis.Client, userID, refreshToken string, window time.Duration) error {
	if rdb == nil {
		return errors.New("redis_not_configured")
	}
	if userID == "" || refreshToken == "" {
		return errors.New("missing_mfa_marker_fields")
	}
	if window <= 0 {
		window = defaultStepUpMFAWindow
	}
	return markRecentMFA(ctx, rdb, userID, hashOpaqueToken(refreshToken), window)
}

func markRecentMFA(ctx context.Context, rdb *redis.Client, userID, sessionHash string, window time.Duration) error {
	return rdb.Set(ctx, recentMFAKey(userID, sessionHash), "1", window).Err()
}

func hasRecentMFA(ctx context.Context, rdb *redis.Client, userID, sessionHash string) bool {
	exists, err := rdb.Exists(ctx, recentMFAKey(userID, sessionHash)).Result()
	return err == nil && exists > 0
}

func recentMFAKey(userID, sessionHash string) string {
	return fmt.Sprintf("mfa:recent:%s:%s", userID, sessionHash)
}

func hashOpaqueToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func refreshTokenFromCookie(r *http.Request) (string, error) {
	c, err := r.Cookie("refresh_token")
	if err != nil || c.Value == "" {
		return "", errors.New("missing_refresh_cookie")
	}
	return c.Value, nil
}
