package auth

import (
	"errors"
	"net"
	"strings"

	"github.com/zerotrust/backend/pkg/geoip"
)

var ErrIPNotAllowed      = errors.New("ip_not_allowed")
var ErrCountryNotAllowed = errors.New("country_not_allowed")

// isIPAllowed reports whether ipStr is permitted by the allowlist.
// allowlist is a comma- or newline-separated list of CIDRs (e.g. 10.0.0.0/8)
// and/or exact IP addresses. An empty allowlist allows all IPs.
func isIPAllowed(ipStr, allowlist string) bool {
	allowlist = strings.TrimSpace(allowlist)
	if allowlist == "" {
		return true
	}

	ip := parseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, entry := range splitEntries(allowlist) {
		if strings.Contains(entry, "/") {
			_, network, err := net.ParseCIDR(entry)
			if err == nil && network.Contains(ip) {
				return true
			}
		} else {
			if allowed := net.ParseIP(entry); allowed != nil && allowed.Equal(ip) {
				return true
			}
		}
	}
	return false
}

// parseIP strips a port suffix (host:port) before parsing.
func parseIP(s string) net.IP {
	s = strings.TrimSpace(s)
	if ip := net.ParseIP(s); ip != nil {
		return ip
	}
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

// splitEntries splits on commas and newlines, trims whitespace, and drops empty tokens.
func splitEntries(s string) []string {
	s = strings.ReplaceAll(s, "\n", ",")
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// isCountryAllowed reports whether the country resolved from ipStr is in the
// allowlist (comma- or newline-separated ISO 3166-1 alpha-2 codes, e.g. "TR,US").
// Rules:
//   - Empty allowlist → allow all.
//   - nil geoip service → allow (can't resolve, fail-open).
//   - Private / loopback IPs → always allow (Docker, local dev).
//   - GeoIP lookup failure → allow (unknown public IP, fail-open).
//   - Comparison is case-insensitive.
func isCountryAllowed(ipStr, allowlist string, g *geoip.Service) bool {
	if strings.TrimSpace(allowlist) == "" || g == nil {
		return true
	}

	ip := parseIP(ipStr)
	if ip == nil {
		return true // unparseable → fail-open
	}
	if ip.IsLoopback() || ip.IsPrivate() {
		return true // Docker / LAN always passes
	}

	loc, err := g.Lookup(ip.String())
	if err != nil || loc == nil || loc.CountryCode == "" {
		return true // unresolvable → fail-open
	}

	code := strings.ToUpper(loc.CountryCode)
	for _, entry := range splitEntries(allowlist) {
		if strings.ToUpper(entry) == code {
			return true
		}
	}
	return false
}
