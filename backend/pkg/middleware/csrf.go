package middleware

import (
	"crypto/subtle"
	"net/http"
)

// CSRF enforces the double-submit cookie pattern for browser sessions.
// Requests carrying Authorization: Bearer are exempt (API / service-account clients).
// Requests with no session cookies at all are exempt (e.g. /auth/login before cookies exist).
func CSRF() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Safe methods never mutate state.
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			// Bearer token = API client; CSRF is not applicable.
			if r.Header.Get("Authorization") != "" {
				next.ServeHTTP(w, r)
				return
			}

			// No session cookies present means this is a pre-auth request (e.g. login).
			_, errAT := r.Cookie("access_token")
			_, errRT := r.Cookie("refresh_token")
			if errAT != nil && errRT != nil {
				next.ServeHTTP(w, r)
				return
			}

			// Browser session exists — enforce double-submit check.
			c, err := r.Cookie("csrf_token")
			if err != nil || c.Value == "" {
				writeError(w, http.StatusForbidden, "csrf_missing")
				return
			}
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(c.Value)) != 1 {
				writeError(w, http.StatusForbidden, "csrf_invalid")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
