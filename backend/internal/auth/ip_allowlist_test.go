package auth

import (
	"testing"

	"github.com/zerotrust/backend/pkg/geoip"
)

func TestIsIPAllowed_EmptyAllowlist(t *testing.T) {
	// Empty allowlist = permit everything
	for _, ip := range []string{"1.2.3.4", "172.18.0.5", "192.168.1.100", "::1"} {
		if !isIPAllowed(ip, "") {
			t.Errorf("expected %q to be allowed with empty allowlist", ip)
		}
	}
}

func TestIsIPAllowed_ExactIP(t *testing.T) {
	allowlist := "10.0.0.1"
	if !isIPAllowed("10.0.0.1", allowlist) {
		t.Error("exact match should be allowed")
	}
	if isIPAllowed("10.0.0.2", allowlist) {
		t.Error("different IP should be blocked")
	}
}

func TestIsIPAllowed_CIDR(t *testing.T) {
	allowlist := "172.18.0.0/16"
	if !isIPAllowed("172.18.0.5", allowlist) {
		t.Error("IP inside CIDR should be allowed")
	}
	if !isIPAllowed("172.18.255.255", allowlist) {
		t.Error("IP at CIDR boundary should be allowed")
	}
	if isIPAllowed("172.19.0.1", allowlist) {
		t.Error("IP outside CIDR should be blocked")
	}
}

func TestIsIPAllowed_MultipleEntries(t *testing.T) {
	// Comma-separated
	allowlist := "10.0.0.0/8,192.168.1.0/24"
	if !isIPAllowed("10.5.5.5", allowlist) {
		t.Error("should match first CIDR")
	}
	if !isIPAllowed("192.168.1.50", allowlist) {
		t.Error("should match second CIDR")
	}
	if isIPAllowed("8.8.8.8", allowlist) {
		t.Error("public IP should be blocked")
	}
}

func TestIsIPAllowed_NewlineSeparated(t *testing.T) {
	allowlist := "10.0.0.0/8\n172.18.0.0/16"
	if !isIPAllowed("10.1.2.3", allowlist) {
		t.Error("should match first entry")
	}
	if !isIPAllowed("172.18.0.99", allowlist) {
		t.Error("should match second entry")
	}
}

func TestIsIPAllowed_DockerSubnet(t *testing.T) {
	// Common Docker bridge subnets — critical for Docker deployments
	allowlist := "172.16.0.0/12"
	if !isIPAllowed("172.18.0.5", allowlist) {
		t.Error("Docker bridge IP should be allowed inside 172.16/12")
	}
	if !isIPAllowed("172.31.255.255", allowlist) {
		t.Error("last address in 172.16/12 should be allowed")
	}
	if isIPAllowed("172.32.0.1", allowlist) {
		t.Error("IP just outside 172.16/12 should be blocked")
	}
}

func TestIsIPAllowed_WithPort(t *testing.T) {
	// r.RemoteAddr includes port, e.g. "1.2.3.4:54321"
	allowlist := "1.2.3.4"
	if !isIPAllowed("1.2.3.4:54321", allowlist) {
		t.Error("host:port format should be stripped and matched")
	}
}

func TestIsIPAllowed_IPv6(t *testing.T) {
	allowlist := "::1"
	if !isIPAllowed("::1", allowlist) {
		t.Error("loopback IPv6 should be allowed")
	}
	if isIPAllowed("::2", allowlist) {
		t.Error("different IPv6 should be blocked")
	}
}

func TestIsIPAllowed_InvalidIP(t *testing.T) {
	// Unparseable IP should be blocked (fail-safe)
	if isIPAllowed("not-an-ip", "10.0.0.0/8") {
		t.Error("invalid IP should be blocked")
	}
}

func TestIsIPAllowed_WhitespaceAndEmptyEntries(t *testing.T) {
	// Extra spaces and blank lines should be ignored
	allowlist := "  10.0.0.1  ,  ,\n  192.168.0.1\n"
	if !isIPAllowed("10.0.0.1", allowlist) {
		t.Error("should match despite surrounding whitespace")
	}
	if !isIPAllowed("192.168.0.1", allowlist) {
		t.Error("should match newline-separated entry")
	}
}

// --- Country allowlist tests ---

func TestIsCountryAllowed_EmptyAllowlist(t *testing.T) {
	g := newMockGeoIP()
	// Empty allowlist = allow everything regardless of country
	if !isCountryAllowed("100.0.0.1", "", g) { // Japan
		t.Error("empty allowlist should allow all countries")
	}
}

func TestIsCountryAllowed_NilGeoIP(t *testing.T) {
	// No GeoIP service = fail-open (can't resolve, allow)
	if !isCountryAllowed("8.8.8.8", "US", nil) {
		t.Error("nil geoip should fail-open")
	}
}

func TestIsCountryAllowed_PrivateIP(t *testing.T) {
	g := newMockGeoIP()
	// Private and loopback IPs always bypass the country check (Docker / local dev)
	for _, ip := range []string{"172.18.0.5", "192.168.1.1", "10.0.0.1", "127.0.0.1", "::1"} {
		if !isCountryAllowed(ip, "TR", g) {
			t.Errorf("private/loopback IP %q should bypass country check", ip)
		}
	}
}

func TestIsCountryAllowed_MatchingCountry(t *testing.T) {
	g := newMockGeoIP()
	// 100.0.0.1 → Japan (JP) in mock
	if !isCountryAllowed("100.0.0.1", "JP", g) {
		t.Error("Japanese IP should be allowed when JP is in allowlist")
	}
}

func TestIsCountryAllowed_NonMatchingCountry(t *testing.T) {
	g := newMockGeoIP()
	// 100.0.0.1 → Japan, allowlist = TR only
	if isCountryAllowed("100.0.0.1", "TR", g) {
		t.Error("Japanese IP should be blocked when only TR is allowed")
	}
}

func TestIsCountryAllowed_MultipleCountries(t *testing.T) {
	g := newMockGeoIP()
	// 100.0.0.2 → UK (GB), allowlist = TR,GB,US
	if !isCountryAllowed("100.0.0.2", "TR,GB,US", g) {
		t.Error("GB IP should be allowed when GB is in allowlist")
	}
	// 100.0.0.1 → Japan, not in TR,GB,US
	if isCountryAllowed("100.0.0.1", "TR,GB,US", g) {
		t.Error("JP IP should be blocked when JP is not in allowlist")
	}
}

func TestIsCountryAllowed_CaseInsensitive(t *testing.T) {
	g := newMockGeoIP()
	// Allowlist in lowercase should still work
	if !isCountryAllowed("100.0.0.3", "us,tr", g) { // 100.0.0.3 → US
		t.Error("lowercase country codes in allowlist should work")
	}
}

func TestIsCountryAllowed_UnresolvablePublicIP(t *testing.T) {
	g := newMockGeoIP()
	// A public IP not in mock maps and no DB loaded → lookup error → fail-open
	if !isCountryAllowed("5.5.5.5", "TR", g) {
		t.Error("unresolvable public IP should fail-open")
	}
}

func TestIsCountryAllowed_WithPort(t *testing.T) {
	g := newMockGeoIP()
	// r.RemoteAddr includes port — should still work
	if !isCountryAllowed("100.0.0.1:55432", "JP", g) {
		t.Error("host:port format should be handled")
	}
}

// newMockGeoIP returns a GeoIP service in fallback/mock mode (no real MMDB).
func newMockGeoIP() *geoip.Service {
	return geoip.NewService("") // empty path = fallback mode
}

func TestSplitEntries(t *testing.T) {
	got := splitEntries("a , b\nc,,  d  ")
	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: want %q, got %q", i, want[i], got[i])
		}
	}
}
