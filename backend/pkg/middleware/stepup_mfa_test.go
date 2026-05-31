package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/auth"
)

type mockMFAChecker struct {
	enabled bool
	valid   bool
}

func (m *mockMFAChecker) IsEnabled(ctx context.Context, userID string) bool {
	return m.enabled
}

func (m *mockMFAChecker) Validate(ctx context.Context, userID, code string) bool {
	return m.valid && code != ""
}

func (m *mockMFAChecker) Setup(ctx context.Context, userID, email, currentCode string) (string, string, []string, error) {
	return "", "", nil, nil
}

func (m *mockMFAChecker) VerifyAndEnable(ctx context.Context, userID, code string) error {
	return nil
}

func TestMarkRecentMFACookie(t *testing.T) {
	mr, _ := miniredis.Run()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	err := MarkRecentMFACookie(context.Background(), nil, "user1", "token1", time.Minute)
	if err == nil {
		t.Error("expected err for nil rdb")
	}
	err = MarkRecentMFACookie(context.Background(), rdb, "", "token1", time.Minute)
	if err == nil {
		t.Error("expected err for empty user")
	}
	err = MarkRecentMFACookie(context.Background(), rdb, "user1", "token1", 0)
	if err != nil {
		t.Error("expected success for 0 window")
	}
}

func TestRequireRecentMFA(t *testing.T) {
	mr, _ := miniredis.Run()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	mfa := &mockMFAChecker{enabled: true, valid: true}
	handler := RequireRecentMFA(mfa, rdb, 0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Missing claims
	req1 := httptest.NewRequest("GET", "/", nil)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr1.Code)
	}

	claims := &auth.Claims{UserID: "user1"}
	ctx := context.WithValue(context.Background(), ClaimsKey, claims)

	// MFA disabled
	mfaDisabled := &mockMFAChecker{enabled: false}
	handlerDisabled := RequireRecentMFA(mfaDisabled, rdb, 0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req2 := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	rr2 := httptest.NewRecorder()
	handlerDisabled.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusForbidden {
		t.Errorf("expected 403 when mfa required, got %d", rr2.Code)
	}

	// Missing refresh cookie
	req3 := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusForbidden {
		t.Errorf("expected 403 for missing refresh cookie, got %d", rr3.Code)
	}

	// Has refresh cookie, missing code
	req4 := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	req4.AddCookie(&http.Cookie{Name: "refresh_token", Value: "token1"})
	rr4 := httptest.NewRecorder()
	handler.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusForbidden {
		t.Errorf("expected 403 for missing code, got %d", rr4.Code)
	}

	// Valid code
	req5 := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	req5.AddCookie(&http.Cookie{Name: "refresh_token", Value: "token1"})
	req5.Header.Set("X-MFA-Code", "123456")
	rr5 := httptest.NewRecorder()
	handler.ServeHTTP(rr5, req5)
	if rr5.Code != http.StatusOK {
		t.Errorf("expected 200 for valid code, got %d", rr5.Code)
	}

	// Try again without code (should pass via recent MFA marker)
	req6 := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	req6.AddCookie(&http.Cookie{Name: "refresh_token", Value: "token1"})
	rr6 := httptest.NewRecorder()
	handler.ServeHTTP(rr6, req6)
	if rr6.Code != http.StatusOK {
		t.Errorf("expected 200 for recent MFA, got %d", rr6.Code)
	}
}
