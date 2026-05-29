package geoip

import (
	"testing"
)

func TestGeoIP_MockLookups(t *testing.T) {
	s := NewService("")
	defer s.Close()

	cases := []struct {
		ip          string
		wantCountry string
		wantCity    string
	}{
		{"8.8.8.8", "United States", "Mountain View"},
		{"81.2.69.142", "United Kingdom", "London"},
		{"1.1.1.1", "Australia", "Sydney"},
		{"127.0.0.1", "Localhost", "Local"},
	}

	for _, c := range cases {
		loc, err := s.Lookup(c.ip)
		if err != nil {
			t.Fatalf("Lookup(%q) failed: %v", c.ip, err)
		}
		if loc.Country != c.wantCountry || loc.City != c.wantCity {
			t.Errorf("Lookup(%q) = (%q, %q), want (%q, %q)", c.ip, loc.Country, loc.City, c.wantCountry, c.wantCity)
		}
	}
}

func TestGeoIP_InvalidIP(t *testing.T) {
	s := NewService("")
	defer s.Close()

	_, err := s.Lookup("not-an-ip")
	if err == nil {
		t.Error("expected error for invalid IP, got nil")
	}
}
