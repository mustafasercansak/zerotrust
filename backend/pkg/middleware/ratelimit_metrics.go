package middleware

import "sync/atomic"

// failOpens counts requests that bypassed rate limiting because Redis was
// unavailable. A silent fail-open during a Redis outage disables every
// limiter (including the login limiter) with no visibility. (ISSUE_LIST #111)
var failOpens uint64

func recordFailOpen() {
	atomic.AddUint64(&failOpens, 1)
}

// FailOpens returns the number of requests that bypassed rate limiting due
// to a Redis error, for exposure via /metrics.
func FailOpens() uint64 {
	return atomic.LoadUint64(&failOpens)
}
