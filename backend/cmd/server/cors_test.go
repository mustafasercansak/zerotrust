package main

import "testing"

func TestParseCORSOrigins(t *testing.T) {
	t.Run("rejects wildcard", func(t *testing.T) {
		if _, err := parseCORSOrigins("*"); err == nil {
			t.Fatal("expected wildcard origin to be rejected when credentials are enabled")
		}
	})

	t.Run("rejects wildcard among others", func(t *testing.T) {
		if _, err := parseCORSOrigins("https://a.example.com, *"); err == nil {
			t.Fatal("expected wildcard to be rejected even alongside explicit origins")
		}
	})

	t.Run("rejects empty", func(t *testing.T) {
		if _, err := parseCORSOrigins("  ,  "); err == nil {
			t.Fatal("expected error when no origins are configured")
		}
	})

	t.Run("trims and keeps explicit origins", func(t *testing.T) {
		got, err := parseCORSOrigins(" https://a.example.com , https://b.example.com ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0] != "https://a.example.com" || got[1] != "https://b.example.com" {
			t.Fatalf("unexpected origins: %v", got)
		}
	})
}
