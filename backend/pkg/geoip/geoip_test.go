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
func TestGeoIP_UnmappedIP(t *testing.T) {
	s := NewService("")
	defer s.Close()

	_, err := s.Lookup("203.0.113.1")
	if err == nil {
		t.Error("expected error for unmapped external IP, got nil")
	}
}

func TestGeoIP_InvalidDB(t *testing.T) {
	s := NewService("nonexistent.mmdb")
	defer s.Close()
	if s.db != nil {
		t.Error("expected db to be nil for nonexistent file")
	}
}

func TestGeoIP_PrivateIP(t *testing.T) {
	s := NewService("")
	defer s.Close()

	loc, err := s.Lookup("192.168.1.1")
	if err != nil {
		t.Fatalf("Lookup(private) failed: %v", err)
	}
	if loc.Country != "Localhost" || loc.City != "Local" {
		t.Fatalf("country=%q city=%q want Localhost/Local", loc.Country, loc.City)
	}
}

func TestGeoIP_CloseNilDB(t *testing.T) {
	s := NewService("")
	if err := s.Close(); err != nil {
		t.Fatalf("Close with nil db returned error: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}
