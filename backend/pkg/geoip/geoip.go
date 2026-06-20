package geoip

import (
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/oschwald/geoip2-golang"
)

// Location contains the geo-coordinates and names for an IP.
type Location struct {
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"` // ISO 3166-1 alpha-2, e.g. "TR", "US"
	City        string  `json:"city"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

// mmdbReader is the subset of *geoip2.Reader used by Service.
// Extracted as an interface so tests can inject a mock without a real .mmdb file.
type mmdbReader interface {
	City(ipAddress net.IP) (*geoip2.City, error)
	Close() error
}

// Service performs IP geolocation lookups.
type Service struct {
	dbPath string
	mu     sync.RWMutex
	db     mmdbReader
}

// mockIPs maps specific test IPs to distinct locations to allow testing and dev without a real database file.
var mockIPs = map[string]Location{
	"8.8.8.8":     {Country: "United States", CountryCode: "US", City: "Mountain View", Latitude: 37.386, Longitude: -122.083},
	"81.2.69.142": {Country: "United Kingdom", CountryCode: "GB", City: "London", Latitude: 51.507, Longitude: -0.127},
	"1.1.1.1":     {Country: "Australia", CountryCode: "AU", City: "Sydney", Latitude: -33.868, Longitude: 151.209},
	"100.0.0.1":   {Country: "Japan", CountryCode: "JP", City: "Tokyo", Latitude: 35.676, Longitude: 139.65},
	"100.0.0.2":   {Country: "United Kingdom", CountryCode: "GB", City: "London", Latitude: 51.507, Longitude: -0.127},
	"100.0.0.3":   {Country: "United States", CountryCode: "US", City: "New York", Latitude: 40.7128, Longitude: -74.006},
}

// NewService initializes a GeoIP service using the provided path to a MaxMind DB file.
// If the file cannot be opened (e.g. doesn't exist), it logs a warning and operates in fallback mode.
func NewService(dbPath string) *Service {
	s := &Service{dbPath: dbPath}
	if dbPath != "" {
		db, err := geoip2.Open(dbPath)
		if err != nil {
			slog.Warn("Failed to open MaxMind GeoIP database; running in fallback/mock mode", "path", dbPath, "error", err)
		} else {
			slog.Info("Successfully loaded MaxMind GeoIP database", "path", dbPath)
			s.db = db
		}
	}
	return s
}

// newServiceWithReader creates a Service with a pre-opened reader. Used in tests.
func newServiceWithReader(r mmdbReader) *Service {
	return &Service{db: r}
}

// Close releases database resources.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		err := s.db.Close()
		s.db = nil
		return err
	}
	return nil
}

// Lookup resolves the IP to coordinates and country/city.
func (s *Service) Lookup(ipStr string) (*Location, error) {
	parsedIP := net.ParseIP(ipStr)
	if parsedIP == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ipStr)
	}

	// 1. Check local mock overrides first (or use as fallback)
	if loc, ok := mockIPs[ipStr]; ok {
		return &loc, nil
	}

	s.mu.RLock()
	reader := s.db
	s.mu.RUnlock()

	// 2. If MMDB is loaded, perform lookup
	if reader != nil {
		record, err := reader.City(parsedIP)
		if err != nil {
			return nil, fmt.Errorf("geoip database lookup: %w", err)
		}
		loc := &Location{
			Country:     record.Country.Names["en"],
			CountryCode: record.Country.IsoCode,
			City:        record.City.Names["en"],
			Latitude:    record.Location.Latitude,
			Longitude:   record.Location.Longitude,
		}
		if loc.Country == "" {
			loc.Country = record.RegisteredCountry.Names["en"]
			loc.CountryCode = record.RegisteredCountry.IsoCode
		}
		return loc, nil
	}

	// 3. Fail/default fallback for other IPs when no db is loaded
	if parsedIP.IsLoopback() || parsedIP.IsPrivate() {
		return &Location{Country: "Localhost", City: "Local", Latitude: 0, Longitude: 0}, nil
	}

	return nil, fmt.Errorf("geoip database not loaded and no mock mapping for IP: %s", ipStr)
}
