package middleware

import (
	"net"
	"net/http"
	"strings"
)

// ParseCIDRs parses a comma-separated list of CIDR strings.
// Invalid or empty entries are silently skipped.
func ParseCIDRs(s string) []*net.IPNet {
	var nets []*net.IPNet
	for _, raw := range strings.Split(s, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		_, cidr, err := net.ParseCIDR(raw)
		if err == nil {
			nets = append(nets, cidr)
		}
	}
	return nets
}

// TrustedClientIP resolves the real client IP from X-Real-IP only when the
// connecting socket address belongs to one of the trusted proxy CIDRs, then
// overwrites r.RemoteAddr on a cloned request so every downstream caller
// (handlers, audit log, session store) sees the correct IP without any import
// changes. When no trusted proxies are configured the raw socket address is
// used, which is not spoofable by the client.
func TrustedClientIP(trustedCIDRs []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := socketAddr(r.RemoteAddr)
			if len(trustedCIDRs) > 0 && inTrustedCIDR(ip, trustedCIDRs) {
				if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
					ip = xrip
				}
			}
			r2 := r.Clone(r.Context())
			r2.RemoteAddr = ip
			next.ServeHTTP(w, r2)
		})
	}
}

// ClientIP strips the port from r.RemoteAddr, which TrustedClientIP has
// already resolved to the real client IP when a trusted proxy is in use.
func ClientIP(r *http.Request) string {
	return socketAddr(r.RemoteAddr)
}

func socketAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func inTrustedCIDR(ip string, cidrs []*net.IPNet) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, cidr := range cidrs {
		if cidr.Contains(parsed) {
			return true
		}
	}
	return false
}
