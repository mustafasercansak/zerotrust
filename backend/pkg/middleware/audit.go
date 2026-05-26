package middleware

import (
	"context"
	"net/http"

	"github.com/zerotrust/backend/internal/audit"
)

func AuditLog(repo *audit.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)

			claims := ClaimsFrom(r.Context())

			entry := audit.Entry{
				Action:    r.Method + " " + r.URL.Path,
				Resource:  r.URL.Path,
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
			}
			if claims != nil {
				entry.UserID = &claims.UserID
			}

			go repo.Log(context.Background(), entry)
		})
	}
}
