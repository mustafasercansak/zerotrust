package middleware

import (
	"net/http"
	"testing"
)

func TestExtractToken(t *testing.T) {
	// Header missing prefix
	r1, _ := http.NewRequest("GET", "/", nil)
	r1.Header.Set("Authorization", "Token abc")
	if extractToken(r1) != "" {
		t.Error("expected empty string")
	}

	// Header correct
	r2, _ := http.NewRequest("GET", "/", nil)
	r2.Header.Set("Authorization", "Bearer token123")
	if extractToken(r2) != "token123" {
		t.Error("expected token123")
	}

	// Cookie
	r3, _ := http.NewRequest("GET", "/", nil)
	r3.AddCookie(&http.Cookie{Name: "access_token", Value: "cookie_token"})
	if extractToken(r3) != "cookie_token" {
		t.Error("expected cookie_token")
	}
}
