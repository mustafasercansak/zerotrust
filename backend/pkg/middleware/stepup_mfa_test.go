package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/auth"
)

type fakeMFAChecker struct {
	enabled  bool
	validFor map[string]bool
}

func (f *fakeMFAChecker) IsEnabled(ctx context.Context, userID string) bool {
	return f.enabled
}

func (f *fakeMFAChecker) Validate(ctx context.Context, userID, code string) bool {
	return f.validFor[code]
}

func newTestRedis(t *testing.T) (*redis.Client, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return rdb, func() {
		_ = rdb.Close()
		mr.Close()
	}
}

func withTestClaims(r *http.Request, userID string) *http.Request {
	claims := &auth.Claims{UserID: userID}
	ctx := context.WithValue(r.Context(), ClaimsKey, claims)
	return r.WithContext(ctx)
}

func decodeErrCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return body["error"]
}

func TestRequireRecentMFA_DeniesWithoutCode(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()

	mw := RequireRecentMFA(&fakeMFAChecker{enabled: true, validFor: map[string]bool{}}, rdb, time.Minute)
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { nextCalled = true })

	req := httptest.NewRequest(http.MethodPatch, "/sensitive", nil)
	req = withTestClaims(req, "u1")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "rt-1"})
	rr := httptest.NewRecorder()

	mw(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusForbidden)
	}
	if decodeErrCode(t, rr) != "mfa_required" {
		t.Fatalf("expected mfa_required error, got body=%s", rr.Body.String())
	}
	if nextCalled {
		t.Fatal("next handler must not be called")
	}
}

func TestRequireRecentMFA_AllowsWithValidCodeAndMarksSession(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()

	mw := RequireRecentMFA(&fakeMFAChecker{enabled: true, validFor: map[string]bool{"123456": true}}, rdb, 2*time.Minute)
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { nextCalled = true })

	req := httptest.NewRequest(http.MethodPatch, "/sensitive", nil)
	req = withTestClaims(req, "u1")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "rt-1"})
	req.Header.Set("X-MFA-Code", "123456")
	rr := httptest.NewRecorder()

	mw(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !nextCalled {
		t.Fatal("expected next handler to be called")
	}

	key := recentMFAKey("u1", hashOpaqueToken("rt-1"))
	exists, err := rdb.Exists(context.Background(), key).Result()
	if err != nil || exists != 1 {
		t.Fatalf("expected recent MFA marker key to exist, err=%v exists=%d", err, exists)
	}
}

func TestRequireRecentMFA_AllowsWhenRecentMarkerExists(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()

	key := recentMFAKey("u1", hashOpaqueToken("rt-1"))
	if err := rdb.Set(context.Background(), key, "1", time.Minute).Err(); err != nil {
		t.Fatalf("failed to seed recent MFA marker: %v", err)
	}

	mw := RequireRecentMFA(&fakeMFAChecker{enabled: true, validFor: map[string]bool{}}, rdb, time.Minute)
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { nextCalled = true })

	req := httptest.NewRequest(http.MethodPatch, "/sensitive", nil)
	req = withTestClaims(req, "u1")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "rt-1"})
	rr := httptest.NewRecorder()

	mw(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !nextCalled {
		t.Fatal("expected next handler to be called when recent marker exists")
	}
}
