package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/auth"
)

func newRateLimitTestRedis(t *testing.T) (*redis.Client, func()) {
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

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRateLimiter_IPBased(t *testing.T) {
	rdb, cleanup := newRateLimitTestRedis(t)
	defer cleanup()

	rl := NewRateLimiter(rdb, "testip", 2, 10*time.Second)
	mw := rl.Middleware()

	handler := mw(okHandler())

	// First request - ok
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "192.168.1.1:1234"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr1.Code)
	}
	if rr1.Header().Get("X-RateLimit-Limit") != "2" {
		t.Errorf("expected Limit header to be 2, got %q", rr1.Header().Get("X-RateLimit-Limit"))
	}
	if rr1.Header().Get("X-RateLimit-Remaining") != "1" {
		t.Errorf("expected Remaining header to be 1, got %q", rr1.Header().Get("X-RateLimit-Remaining"))
	}

	// Second request - ok
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "192.168.1.1:1234"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr2.Code)
	}
	if rr2.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected Remaining header to be 0, got %q", rr2.Header().Get("X-RateLimit-Remaining"))
	}

	// Third request - 429
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "192.168.1.1:1234"
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr3.Code)
	}

	retryAfterHeader := rr3.Header().Get("Retry-After")
	if retryAfterHeader == "" {
		t.Errorf("expected Retry-After header, got empty")
	}

	retryAfter, err := strconv.Atoi(retryAfterHeader)
	if err != nil || retryAfter <= 0 {
		t.Errorf("invalid Retry-After value: %q", retryAfterHeader)
	}

	var respBody map[string]any
	if err := json.Unmarshal(rr3.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if respBody["error"] != "rate_limit_exceeded" {
		t.Errorf("expected error code rate_limit_exceeded, got %v", respBody["error"])
	}
}

func TestRateLimiter_AuthenticatedUserID(t *testing.T) {
	rdb, cleanup := newRateLimitTestRedis(t)
	defer cleanup()

	rl := NewRateLimiter(rdb, "testauth", 1, 10*time.Second)
	mw := rl.Middleware()

	handler := mw(okHandler())

	// Request 1 by User A - OK
	reqA1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqA1.RemoteAddr = "192.168.1.1:1234"
	claimsA := &auth.Claims{UserID: "user_a"}
	reqA1 = reqA1.WithContext(context.WithValue(reqA1.Context(), ClaimsKey, claimsA))
	rrA1 := httptest.NewRecorder()
	handler.ServeHTTP(rrA1, reqA1)
	if rrA1.Code != http.StatusOK {
		t.Errorf("expected 200 for user_a, got %d", rrA1.Code)
	}

	// Request 2 by User A - 429 (User limit = 1)
	reqA2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqA2.RemoteAddr = "192.168.1.1:1234"
	reqA2 = reqA2.WithContext(context.WithValue(reqA2.Context(), ClaimsKey, claimsA))
	rrA2 := httptest.NewRecorder()
	handler.ServeHTTP(rrA2, reqA2)
	if rrA2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for user_a, got %d", rrA2.Code)
	}

	// Request 1 by User B - OK (independent rate limit by User ID)
	reqB1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	reqB1.RemoteAddr = "192.168.1.1:1234" // Same IP address!
	claimsB := &auth.Claims{UserID: "user_b"}
	reqB1 = reqB1.WithContext(context.WithValue(reqB1.Context(), ClaimsKey, claimsB))
	rrB1 := httptest.NewRecorder()
	handler.ServeHTTP(rrB1, reqB1)
	if rrB1.Code != http.StatusOK {
		t.Errorf("expected 200 for user_b, got %d", rrB1.Code)
	}
}

// TestRateLimiter_FailsOpenAndRecordsMetricOnRedisError proves a Redis
// outage still allows the request through (fail open — a limiter outage
// must not become an availability outage) but is now visible via a counter
// instead of failing silently. (ISSUE_LIST #111)
func TestRateLimiter_FailsOpenAndRecordsMetricOnRedisError(t *testing.T) {
	rdb, cleanup := newRateLimitTestRedis(t)
	cleanup() // close the connection immediately so every Redis call errors

	rl := NewRateLimiter(rdb, "testfailopen", 1, 10*time.Second)
	handler := rl.Middleware()(okHandler())

	before := FailOpens()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (fail open) when Redis is unavailable, got %d", rr.Code)
	}
	if got := FailOpens(); got != before+1 {
		t.Errorf("FailOpens()=%d want=%d", got, before+1)
	}
}
