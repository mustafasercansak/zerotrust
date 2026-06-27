package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/zerotrust/backend/internal/auth"
)

type contextKey string

const ClaimsKey contextKey = "claims"

func Authenticate(ks *auth.KeyStore, authSvc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				writeError(w, http.StatusUnauthorized, "missing_token")
				return
			}

			claims, err := auth.ValidateAccessToken(ks, token)
			if err != nil {
				if err == auth.ErrExpiredToken {
					writeError(w, http.StatusUnauthorized, "token_expired")
					return
				}
				writeError(w, http.StatusUnauthorized, "invalid_token")
				return
			}

			if err := authSvc.EvaluateAccess(r.Context(), claims, r.RemoteAddr, r.Header.Get("User-Agent"), parseXClientInfo(r)); err != nil {
				if errors.Is(err, auth.ErrDeviceNotAllowed) {
					writeError(w, http.StatusForbidden, "device_not_allowed")
					return
				}
				if errors.Is(err, auth.ErrIPNotAllowed) {
					writeError(w, http.StatusForbidden, "ip_not_allowed")
					return
				}
				if errors.Is(err, auth.ErrCountryNotAllowed) {
					writeError(w, http.StatusForbidden, "country_not_allowed")
					return
				}
				if errors.Is(err, auth.ErrInactiveUser) {
					writeError(w, http.StatusUnauthorized, "user_inactive")
					return
				}
				writeError(w, http.StatusUnauthorized, "invalid_token")
				return
			}

			if claims.Confirmation != nil && claims.Confirmation.JKT != "" {
				dpopHeader := r.Header.Get("DPoP")
				if dpopHeader == "" {
					writeError(w, http.StatusUnauthorized, "invalid_dpop_proof")
					return
				}
				jkt, jti, err := auth.ValidateDPoPProofWithJTI(dpopHeader, r.Method, r.URL.Path)
				if err != nil || jkt != claims.Confirmation.JKT {
					writeError(w, http.StatusUnauthorized, "invalid_dpop_proof")
					return
				}
				// Reject replayed proofs (same jti within the skew window). #35
				if err := authSvc.ConsumeDPoPProof(r.Context(), jti); err != nil {
					writeError(w, http.StatusUnauthorized, "invalid_dpop_proof")
					return
				}
			}

			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ClaimsFrom(ctx context.Context) *auth.Claims {
	c, _ := ctx.Value(ClaimsKey).(*auth.Claims)
	return c
}

// extractToken reads the access token from the httpOnly cookie first,
// then falls back to the Authorization: Bearer header for API clients.
func extractToken(r *http.Request) string {
	if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
		return c.Value
	}
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func parseXClientInfo(r *http.Request) map[string]string {
	raw := r.Header.Get("X-Client-Info")
	if raw == "" {
		return nil
	}
	var clientInfo map[string]string
	if err := json.Unmarshal([]byte(raw), &clientInfo); err != nil {
		return nil
	}
	return clientInfo
}
