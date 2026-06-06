package middleware

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestRateLimiter_IncrementBranches covers increment's TTL-fallback path (a key
// already present without an expiry) and the error path when Redis is down.
func TestRateLimiter_IncrementBranches(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rl := NewRateLimiter(rdb, "p", 5, 10*time.Second)
	ctx := context.Background()

	// Pre-existing key with no TTL → count>1 path with TTL() == -1 → window fallback.
	if err := rdb.Set(ctx, "nottl", "3", 0).Err(); err != nil {
		t.Fatalf("seed key: %v", err)
	}
	count, ttl, err := rl.increment(ctx, "nottl")
	if err != nil {
		t.Fatalf("increment: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected count 4, got %d", count)
	}
	if ttl != 10*time.Second {
		t.Fatalf("expected window fallback ttl, got %v", ttl)
	}

	// Redis unreachable → the Incr error is surfaced.
	mr.Close()
	if _, _, err := rl.increment(ctx, "x"); err == nil {
		t.Fatal("expected an error when Redis is down")
	}
}
