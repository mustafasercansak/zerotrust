package geoip

import (
	"errors"
	"net"
	"testing"

	"github.com/oschwald/geoip2-golang"
)

// mockMMDB implements mmdbReader for tests.
type mockMMDB struct {
	record *geoip2.City
	err    error
	closed bool
}

func (m *mockMMDB) City(_ net.IP) (*geoip2.City, error) {
	return m.record, m.err
}

func (m *mockMMDB) Close() error {
	m.closed = true
	return nil
}

func cityRecord(country, city string, lat, lon float64) *geoip2.City {
	r := &geoip2.City{}
	r.Country.Names = map[string]string{"en": country}
	r.City.Names = map[string]string{"en": city}
	r.Location.Latitude = lat
	r.Location.Longitude = lon
	return r
}

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

func TestGeoIP_WithMockReader_Success(t *testing.T) {
	mock := &mockMMDB{record: cityRecord("Germany", "Berlin", 52.52, 13.405)}
	s := newServiceWithReader(mock)

	loc, err := s.Lookup("2.3.4.5")
	if err != nil {
		t.Fatalf("Lookup with mock reader: %v", err)
	}
	if loc.Country != "Germany" || loc.City != "Berlin" {
		t.Fatalf("country=%q city=%q want Germany/Berlin", loc.Country, loc.City)
	}
	if loc.Latitude != 52.52 || loc.Longitude != 13.405 {
		t.Fatalf("lat=%v lon=%v want 52.52/13.405", loc.Latitude, loc.Longitude)
	}
}

func TestGeoIP_WithMockReader_FallbackToRegisteredCountry(t *testing.T) {
	r := &geoip2.City{}
	r.RegisteredCountry.Names = map[string]string{"en": "France"}
	mock := &mockMMDB{record: r}
	s := newServiceWithReader(mock)

	loc, err := s.Lookup("2.3.4.5")
	if err != nil {
		t.Fatalf("Lookup with empty country: %v", err)
	}
	if loc.Country != "France" {
		t.Fatalf("country=%q want France (from registered country fallback)", loc.Country)
	}
}

func TestGeoIP_WithMockReader_LookupError(t *testing.T) {
	mock := &mockMMDB{err: errors.New("mmdb lookup failed")}
	s := newServiceWithReader(mock)

	_, err := s.Lookup("2.3.4.5")
	if err == nil {
		t.Fatal("expected error from mock reader, got nil")
	}
}

func TestGeoIP_CloseWithReader(t *testing.T) {
	mock := &mockMMDB{record: cityRecord("US", "NY", 40.7, -74.0)}
	s := newServiceWithReader(mock)

	if err := s.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !mock.closed {
		t.Fatal("expected mock reader Close() to be called")
	}
	if s.db != nil {
		t.Fatal("expected db to be nil after Close()")
	}
}
