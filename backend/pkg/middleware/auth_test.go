package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/session"
	"github.com/zerotrust/backend/internal/testdb"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/database"
	"github.com/zerotrust/backend/pkg/middleware"
)

func TestAuthenticate(t *testing.T) {
	dbURL := testdb.URL(t)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("test db unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("test db unreachable: %v", err)
	}
	if err := database.RunMigrations(dbURL, "../../migrations"); err != nil {
		pool.Close()
		t.Fatalf("migrations failed: %v", err)
	}
	defer pool.Close()

	userRepo := user.NewRepository(pool)
	userSvc := user.NewService(userRepo)
	sessionRepo := session.NewRepository(pool, nil)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	ks, _ := auth.LoadOrGenerateKeyStore("", "")
	authSvc := auth.NewService(userSvc, sessionRepo, nil, rdb, ks, nil, nil)

	handler := middleware.Authenticate(ks, authSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := middleware.ClaimsFrom(r.Context())
		if claims != nil {
			w.Write([]byte(claims.UserID))
		}
	}))

	// Missing token
	req1 := httptest.NewRequest("GET", "/", nil)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing token")
	}

	// Invalid token
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Authorization", "Bearer invalid.token.here")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token")
	}

	// Create real user and token
	pool.Exec(ctx, "TRUNCATE TABLE users CASCADE")
	u, err := userSvc.Register(ctx, "auth_mid@example.com", "passW0rd123!", "en")
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}
	tokenPair, _ := auth.GenerateTokenPair(ks, "session1", u.ID, u.Email, u.Roles, []string{}, time.Hour)

	req3 := httptest.NewRequest("GET", "/", nil)
	req3.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("expected 200 for valid token, got %d", rr3.Code)
	}

	sessionRepo.Create(ctx, u.ID, "session1", "127.0.0.1", "test", nil, time.Now().Add(time.Hour))

	req4 := httptest.NewRequest("GET", "/", nil)
	req4.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
	rr4 := httptest.NewRecorder()
	handler.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Errorf("expected 200 for valid session, got %d", rr4.Code)
	}

	if err := authSvc.Logout(ctx, "", tokenPair.AccessToken); err != nil {
		t.Fatalf("logout failed: %v", err)
	}
	rr5 := httptest.NewRecorder()
	handler.ServeHTTP(rr5, req4)
	if rr5.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for revoked session")
	}
}
