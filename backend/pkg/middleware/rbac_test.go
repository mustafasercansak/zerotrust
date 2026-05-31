package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zerotrust/backend/internal/auth"
)

func TestRequireRole(t *testing.T) {
	handler := RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Missing claims
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	// Missing required role
	claims := &auth.Claims{Roles: []string{"user"}}
	ctx := context.WithValue(req.Context(), ClaimsKey, claims)
	req2 := req.WithContext(ctx)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr2.Code)
	}

	// Has required role
	claimsAdmin := &auth.Claims{Roles: []string{"admin", "user"}}
	ctxAdmin := context.WithValue(req.Context(), ClaimsKey, claimsAdmin)
	req3 := req.WithContext(ctxAdmin)
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr3.Code)
	}
}

func TestRequirePermission(t *testing.T) {
	handler := RequirePermission("users", "write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Missing claims
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	// Missing permission
	claims := &auth.Claims{Permissions: []string{"users:read"}}
	ctx := context.WithValue(req.Context(), ClaimsKey, claims)
	req2 := req.WithContext(ctx)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr2.Code)
	}

	// Has permission
	claimsPerm := &auth.Claims{Permissions: []string{"users:write", "users:read"}}
	ctxPerm := context.WithValue(req.Context(), ClaimsKey, claimsPerm)
	req3 := req.WithContext(ctxPerm)
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr3.Code)
	}
}
