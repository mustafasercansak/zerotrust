package settings

import (
	"context"
	"strconv"
	"sync"
	"time"
)

const cacheTTL = time.Minute

type cacheStore interface {
	Get(ctx context.Context, key string) (string, error)
}

// Cache wraps Repository with a per-key TTL so high-frequency callers (e.g.
// every login) don't hammer the DB. Stale after cacheTTL — acceptable for
// admin-set configuration values.
type Cache struct {
	repo cacheStore
	mu   sync.RWMutex
	vals map[string]cachedEntry
}

type cachedEntry struct {
	value   string
	expires time.Time
}

func NewCache(repo cacheStore) *Cache {
	return &Cache{repo: repo, vals: make(map[string]cachedEntry)}
}

func (c *Cache) GetInt(ctx context.Context, key string, defaultVal int) int {
	c.mu.RLock()
	entry, ok := c.vals[key]
	c.mu.RUnlock()

	if ok && time.Now().Before(entry.expires) {
		if n, err := strconv.Atoi(entry.value); err == nil {
			return n
		}
		return defaultVal
	}

	val, err := c.repo.Get(ctx, key)
	if err != nil {
		return defaultVal
	}

	c.mu.Lock()
	c.vals[key] = cachedEntry{value: val, expires: time.Now().Add(cacheTTL)}
	c.mu.Unlock()

	if n, err := strconv.Atoi(val); err == nil {
		return n
	}
	return defaultVal
}

func (c *Cache) GetString(ctx context.Context, key string, defaultVal string) string {
	c.mu.RLock()
	entry, ok := c.vals[key]
	c.mu.RUnlock()

	if ok && time.Now().Before(entry.expires) {
		return entry.value
	}

	val, err := c.repo.Get(ctx, key)
	if err != nil {
		return defaultVal
	}

	c.mu.Lock()
	c.vals[key] = cachedEntry{value: val, expires: time.Now().Add(cacheTTL)}
	c.mu.Unlock()

	return val
}

func (c *Cache) GetBool(ctx context.Context, key string, defaultVal bool) bool {
	c.mu.RLock()
	entry, ok := c.vals[key]
	c.mu.RUnlock()

	if ok && time.Now().Before(entry.expires) {
		if b, err := strconv.ParseBool(entry.value); err == nil {
			return b
		}
		return defaultVal
	}

	val, err := c.repo.Get(ctx, key)
	if err != nil {
		return defaultVal
	}

	c.mu.Lock()
	c.vals[key] = cachedEntry{value: val, expires: time.Now().Add(cacheTTL)}
	c.mu.Unlock()

	if b, err := strconv.ParseBool(val); err == nil {
		return b
	}
	return defaultVal
}
