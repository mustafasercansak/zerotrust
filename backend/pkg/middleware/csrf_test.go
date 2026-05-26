package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zerotrust/backend/pkg/middleware"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCSRF_SafeMethodsAlwaysPass(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		req := httptest.NewRequest(method, "/", nil)
		rr := httptest.NewRecorder()
		middleware.CSRF()(okHandler()).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", method, rr.Code)
		}
	}
}

func TestCSRF_BearerTokenExempt(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	// Add session cookies to confirm Bearer wins over cookie presence check.
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "at"})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "cs"})
	rr := httptest.NewRecorder()
	middleware.CSRF()(okHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Bearer exempt: expected 200, got %d", rr.Code)
	}
}

func TestCSRF_NoSessionCookiesExempt(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	rr := httptest.NewRecorder()
	middleware.CSRF()(okHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("pre-auth exempt: expected 200, got %d", rr.Code)
	}
}

func TestCSRF_MissingTokenRejected(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "at"})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "secret"})
	// No X-CSRF-Token header.
	rr := httptest.NewRecorder()
	middleware.CSRF()(okHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("missing CSRF header: expected 403, got %d", rr.Code)
	}
}

func TestCSRF_WrongTokenRejected(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "at"})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "correct"})
	req.Header.Set("X-CSRF-Token", "wrong")
	rr := httptest.NewRecorder()
	middleware.CSRF()(okHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("wrong CSRF token: expected 403, got %d", rr.Code)
	}
}

func TestCSRF_ValidTokenAccepted(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "at"})
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "correct"})
	req.Header.Set("X-CSRF-Token", "correct")
	rr := httptest.NewRecorder()
	middleware.CSRF()(okHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("valid CSRF token: expected 200, got %d", rr.Code)
	}
}

func TestCSRF_RefreshTokenCookieTriggers(t *testing.T) {
	// Only refresh_token cookie present (no access_token) — CSRF still applies.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/1", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "rt"})
	// No CSRF header.
	rr := httptest.NewRecorder()
	middleware.CSRF()(okHandler()).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("refresh_token only: expected 403, got %d", rr.Code)
	}
}
