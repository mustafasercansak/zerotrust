package settings

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeCacheStore struct {
	values map[string]string
	err    error
	calls  []string
}

func (f *fakeCacheStore) Get(ctx context.Context, key string) (string, error) {
	f.calls = append(f.calls, key)
	if f.err != nil {
		return "", f.err
	}
	return f.values[key], nil
}

func TestCacheGetIntCachesParsedValue(t *testing.T) {
	store := &fakeCacheStore{values: map[string]string{"max_sessions_per_user": "5"}}
	cache := NewCache(store)

	if got := cache.GetInt(context.Background(), "max_sessions_per_user", 1); got != 5 {
		t.Fatalf("GetInt=%d want=5", got)
	}
	if got := cache.GetInt(context.Background(), "max_sessions_per_user", 1); got != 5 {
		t.Fatalf("GetInt cached=%d want=5", got)
	}
	if len(store.calls) != 1 {
		t.Fatalf("store calls=%d want=1", len(store.calls))
	}
}

func TestCacheGetIntReturnsDefaultOnParseOrRepoError(t *testing.T) {
	cache := NewCache(&fakeCacheStore{values: map[string]string{"bad": "nope"}})
	if got := cache.GetInt(context.Background(), "bad", 9); got != 9 {
		t.Fatalf("GetInt=%d want=9", got)
	}

	cache = NewCache(&fakeCacheStore{err: errors.New("boom")})
	if got := cache.GetInt(context.Background(), "missing", 7); got != 7 {
		t.Fatalf("GetInt=%d want=7", got)
	}
}

func TestCacheGetStringCachesAndRefreshesExpiredValue(t *testing.T) {
	store := &fakeCacheStore{values: map[string]string{"password_complexity": "strong"}}
	cache := NewCache(store)

	if got := cache.GetString(context.Background(), "password_complexity", "low"); got != "strong" {
		t.Fatalf("GetString=%q want=strong", got)
	}

	cache.mu.Lock()
	cache.vals["password_complexity"] = cachedEntry{value: "stale", expires: time.Now().Add(-time.Second)}
	cache.mu.Unlock()
	store.values["password_complexity"] = "medium"

	if got := cache.GetString(context.Background(), "password_complexity", "low"); got != "medium" {
		t.Fatalf("GetString after expiry=%q want=medium", got)
	}
	if len(store.calls) != 2 {
		t.Fatalf("store calls=%d want=2", len(store.calls))
	}
}

func TestCacheGetStringReturnsDefaultOnRepoError(t *testing.T) {
	cache := NewCache(&fakeCacheStore{err: errors.New("boom")})
	if got := cache.GetString(context.Background(), "password_complexity", "strong"); got != "strong" {
		t.Fatalf("GetString=%q want=strong", got)
	}
}

func TestCacheGetBoolCachesAndFallsBackToDefault(t *testing.T) {
	store := &fakeCacheStore{values: map[string]string{"global_mfa_required": "true", "bad": "not-bool"}}
	cache := NewCache(store)

	if got := cache.GetBool(context.Background(), "global_mfa_required", false); !got {
		t.Fatal("GetBool returned false want true")
	}
	if got := cache.GetBool(context.Background(), "global_mfa_required", false); !got {
		t.Fatal("GetBool cached returned false want true")
	}
	if len(store.calls) != 1 {
		t.Fatalf("store calls=%d want=1", len(store.calls))
	}

	if got := cache.GetBool(context.Background(), "bad", true); !got {
		t.Fatal("GetBool malformed value should return default true")
	}

	cache = NewCache(&fakeCacheStore{err: errors.New("boom")})
	if got := cache.GetBool(context.Background(), "global_mfa_required", true); !got {
		t.Fatal("GetBool repo error should return default true")
	}
}
