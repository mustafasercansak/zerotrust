package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	rdb    *redis.Client
	prefix string
	max    int
	window time.Duration
}

func NewRateLimiter(rdb *redis.Client, prefix string, max int, window time.Duration) *RateLimiter {
	return &RateLimiter{rdb: rdb, prefix: prefix, max: max, window: window}
}

func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyTarget := ClientIP(r)
			if claims := ClaimsFrom(r.Context()); claims != nil && claims.UserID != "" {
				keyTarget = claims.UserID
			}
			key := fmt.Sprintf("rl:%s:%s", rl.prefix, keyTarget)

			count, retryAfter, err := rl.increment(r.Context(), key)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			remaining := rl.max - count
			if remaining < 0 {
				remaining = 0
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.max))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			if count > rl.max {
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]any{
					"error":       "rate_limit_exceeded",
					"retry_after": int(retryAfter.Seconds()),
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) increment(ctx context.Context, key string) (int, time.Duration, error) {
	count, err := rl.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, 0, err
	}
	if count == 1 {
		if err := rl.rdb.Expire(ctx, key, rl.window).Err(); err != nil {
			return 0, 0, err
		}
		return int(count), rl.window, nil
	}
	ttl, err := rl.rdb.TTL(ctx, key).Result()
	if err != nil {
		return 0, 0, err
	}
	if ttl <= 0 {
		ttl = rl.window
	}
	return int(count), ttl, nil
}
