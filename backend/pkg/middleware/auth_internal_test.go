package middleware

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/auth"
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

func TestAuthenticate(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, err := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	if err != nil {
		t.Fatalf("failed to create key store: %v", err)
	}
	authSvc := auth.NewService(nil, nil, nil, rdb, ks, nil, nil)

	handler := Authenticate(ks, authSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFrom(r.Context())
		if claims == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(claims.UserID))
	}))

	t.Run("missing token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer invalid.token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		pair, err := auth.GenerateTokenPair(ks, "user-expired", "expired@example.com", "en", nil, nil, -time.Minute)
		if err != nil {
			t.Fatalf("failed to generate expired token: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
		}
	})

	t.Run("valid token", func(t *testing.T) {
		pair, err := auth.GenerateTokenPair(ks, "user-ok", "ok@example.com", "en", []string{"admin"}, []string{"users:read"}, time.Hour)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK || rr.Body.String() != "user-ok" {
			t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
		}
	})

	t.Run("revoked token", func(t *testing.T) {
		pair, err := auth.GenerateTokenPair(ks, "user-revoked", "revoked@example.com", "en", nil, nil, time.Hour)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}
		if err := authSvc.Logout(context.Background(), "", pair.AccessToken); err != nil {
			t.Fatalf("failed to revoke token: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
		}
	})

	t.Run("dpop token - missing proof", func(t *testing.T) {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate client key: %v", err)
		}
		jwk := map[string]any{
			"kty": "OKP",
			"crv": "Ed25519",
			"x":   base64.RawURLEncoding.EncodeToString(pub),
		}
		jkt, err := auth.CalculateJKT(jwk)
		if err != nil {
			t.Fatalf("failed to calculate JKT: %v", err)
		}

		resp, err := auth.GenerateServiceToken(ks, "client1", "service1", []string{"read"}, time.Hour, jkt)
		if err != nil {
			t.Fatalf("failed to generate service token: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/resource", nil)
		req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rr.Code)
		}
	})

	t.Run("dpop token - invalid proof", func(t *testing.T) {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate client key: %v", err)
		}
		jwk := map[string]any{
			"kty": "OKP",
			"crv": "Ed25519",
			"x":   base64.RawURLEncoding.EncodeToString(pub),
		}
		jkt, err := auth.CalculateJKT(jwk)
		if err != nil {
			t.Fatalf("failed to calculate JKT: %v", err)
		}

		resp, err := auth.GenerateServiceToken(ks, "client1", "service1", []string{"read"}, time.Hour, jkt)
		if err != nil {
			t.Fatalf("failed to generate service token: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/resource", nil)
		req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
		req.Header.Set("DPoP", "invalid-proof-value")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rr.Code)
		}
	})

	t.Run("dpop token - JKT mismatch", func(t *testing.T) {
		pubA, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate key A: %v", err)
		}
		jwk := map[string]any{
			"kty": "OKP",
			"crv": "Ed25519",
			"x":   base64.RawURLEncoding.EncodeToString(pubA),
		}
		jktA, err := auth.CalculateJKT(jwk)
		if err != nil {
			t.Fatalf("failed to calculate JKT A: %v", err)
		}

		resp, err := auth.GenerateServiceToken(ks, "client1", "service1", []string{"read"}, time.Hour, jktA)
		if err != nil {
			t.Fatalf("failed to generate service token: %v", err)
		}

		// Sign proof with key B
		_, privB, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate key B: %v", err)
		}

		proof, err := auth.GenerateDPoPProofForTest(privB, "GET", "/api/resource")
		if err != nil {
			t.Fatalf("failed to generate proof: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/resource", nil)
		req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
		req.Header.Set("DPoP", proof)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rr.Code)
		}
	})

	t.Run("dpop token - success", func(t *testing.T) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("failed to generate client key: %v", err)
		}
		jwk := map[string]any{
			"kty": "OKP",
			"crv": "Ed25519",
			"x":   base64.RawURLEncoding.EncodeToString(pub),
		}
		jkt, err := auth.CalculateJKT(jwk)
		if err != nil {
			t.Fatalf("failed to calculate JKT: %v", err)
		}

		resp, err := auth.GenerateServiceToken(ks, "client1", "service1", []string{"read"}, time.Hour, jkt)
		if err != nil {
			t.Fatalf("failed to generate service token: %v", err)
		}

		proof, err := auth.GenerateDPoPProofForTest(priv, "GET", "/api/resource")
		if err != nil {
			t.Fatalf("failed to generate proof: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/resource", nil)
		req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
		req.Header.Set("DPoP", proof)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})
}
