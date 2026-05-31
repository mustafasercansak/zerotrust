package session

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAddr(t *testing.T) {
	t.Run("parses host-port and bare ip", func(t *testing.T) {
		a := parseAddr("127.0.0.1:5432")
		if a == nil || a.String() != "127.0.0.1" {
			t.Fatalf("parse host-port got=%v", a)
		}

		b := parseAddr("2001:db8::1")
		if b == nil || b.String() != "2001:db8::1" {
			t.Fatalf("parse ipv6 got=%v", b)
		}
	})

	t.Run("invalid address returns nil", func(t *testing.T) {
		if got := parseAddr("not-an-address"); got != nil {
			t.Fatalf("expected nil for invalid address, got=%v", got)
		}
	})
}

func TestNormalizeDeviceInfo(t *testing.T) {
	t.Run("empty map becomes empty json object", func(t *testing.T) {
		if got := string(normalizeDeviceInfo(nil)); got != "{}" {
			t.Fatalf("normalize nil=%q want={}", got)
		}
	})

	t.Run("filters invalid keys and values", func(t *testing.T) {
		tooLongKey := strings.Repeat("k", 41)
		tooLongValue := strings.Repeat("v", 81)

		in := map[string]string{
			"os":          "linux",
			"":            "bad",
			tooLongKey:    "bad",
			"browser":     tooLongValue,
			"device":      "desktop",
			"buildNumber": "123",
		}

		got := normalizeDeviceInfo(in)
		var decoded map[string]string
		if err := json.Unmarshal(got, &decoded); err != nil {
			t.Fatalf("unmarshal normalized device info: %v", err)
		}

		if len(decoded) != 3 {
			t.Fatalf("decoded len=%d want=3 (%v)", len(decoded), decoded)
		}
		if decoded["os"] != "linux" || decoded["device"] != "desktop" || decoded["buildNumber"] != "123" {
			t.Fatalf("unexpected normalized fields: %v", decoded)
		}
	})

	t.Run("all invalid inputs collapse to empty object", func(t *testing.T) {
		tooLongKey := strings.Repeat("k", 41)
		in := map[string]string{tooLongKey: "x"}
		if got := string(normalizeDeviceInfo(in)); got != "{}" {
			t.Fatalf("normalize all-invalid=%q want={}", got)
		}
	})
}
