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
			key := fmt.Sprintf("rl:%s:%s", rl.prefix, ClientIP(r))

			count, err := rl.increment(r.Context(), key)
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
				w.Header().Set("Retry-After", strconv.Itoa(int(rl.window.Seconds())))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{"error": "rate_limit_exceeded"})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) increment(ctx context.Context, key string) (int, error) {
	pipe := rl.rdb.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, rl.window)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return int(incr.Val()), nil
}

