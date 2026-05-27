package middleware

import (
	"context"
	"encoding/json"
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
				Metadata:  auditMetadata(r),
			}
			if claims != nil {
				entry.UserID = &claims.UserID
			}

			go repo.Log(context.Background(), entry)
		})
	}
}

func auditMetadata(r *http.Request) map[string]any {
	raw := r.Header.Get("X-Client-Info")
	if raw == "" {
		return nil
	}

	var clientInfo map[string]string
	if err := json.Unmarshal([]byte(raw), &clientInfo); err != nil || len(clientInfo) == 0 {
		return nil
	}

	clean := make(map[string]string, len(clientInfo))
	for key, value := range clientInfo {
		if key == "" || value == "" || len(key) > 40 || len(value) > 100 {
			continue
		}
		clean[key] = value
	}
	if len(clean) == 0 {
		return nil
	}

	return map[string]any{"client_info": clean}
}
