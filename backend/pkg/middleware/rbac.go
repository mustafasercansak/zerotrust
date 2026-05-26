package middleware

import (
	"encoding/json"
	"net/http"
)

// RequireRole allows the request only if the authenticated user holds at least one of the given roles.
// Must be applied after Authenticate.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFrom(r.Context())
			if claims == nil {
				writeForbidden(w, "unauthorized")
				return
			}
			for _, required := range roles {
				for _, userRole := range claims.Roles {
					if userRole == required {
						next.ServeHTTP(w, r)
						return
					}
				}
			}
			writeForbidden(w, "forbidden")
		})
	}
}

// RequirePermission allows the request only if the token carries the given resource:action permission.
// Works for both user tokens (Permissions field) and service tokens (Scopes field).
// Must be applied after Authenticate.
func RequirePermission(resource, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFrom(r.Context())
			if claims == nil {
				writeForbidden(w, "unauthorized")
				return
			}
			if claims.HasPermission(resource, action) {
				next.ServeHTTP(w, r)
				return
			}
			writeForbidden(w, "forbidden")
		})
	}
}

func writeForbidden(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "application/json")
	if code == "unauthorized" {
		w.WriteHeader(http.StatusUnauthorized)
	} else {
		w.WriteHeader(http.StatusForbidden)
	}
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}
