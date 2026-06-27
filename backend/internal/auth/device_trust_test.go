package auth

import (
	"context"
	"strconv"
	"testing"
)

type mockSettingReader struct {
	settings map[string]string
}

func (m *mockSettingReader) GetInt(_ context.Context, key string, defaultVal int) int {
	val, ok := m.settings[key]
	if !ok {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

func (m *mockSettingReader) GetString(_ context.Context, key string, defaultVal string) string {
	val, ok := m.settings[key]
	if !ok {
		return defaultVal
	}
	return val
}

func (m *mockSettingReader) GetBool(_ context.Context, key string, defaultVal bool) bool {
	val, ok := m.settings[key]
	if !ok {
		return defaultVal
	}
	return val == "true"
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"120.0.1", "120.0.1", 0},
		{"120.0", "120.0.0", 0},
		{"120.0.2", "120.0.1", 1},
		{"120.0.1", "120.0.2", -1},
		{"121.0", "120.9", 1},
		{"13", "14.1", -1},
		{"13.0.0", "13", 0},
		{"invalid", "1.0", -1}, // non-numeric treated as 0
	}

	for _, tt := range tests {
		got := compareVersions(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d; want %d", tt.v1, tt.v2, got, tt.want)
		}
	}
}

func TestIsDeviceAllowed_Disabled(t *testing.T) {
	ctx := context.Background()
	settings := &mockSettingReader{
		settings: map[string]string{
			"device_trust_enabled": "false",
		},
	}

	// Any empty device info should be allowed if the feature is disabled
	if !isDeviceAllowed(ctx, nil, settings) {
		t.Error("Should allow missing info if device trust is disabled")
	}
}

func TestIsDeviceAllowed_EmptyInfo(t *testing.T) {
	ctx := context.Background()
	settings := &mockSettingReader{
		settings: map[string]string{
			"device_trust_enabled": "true",
		},
	}

	// Missing device info should fail-closed when enabled
	if isDeviceAllowed(ctx, nil, settings) {
		t.Error("Should fail-closed on nil device info when enabled")
	}
	if isDeviceAllowed(ctx, map[string]string{}, settings) {
		t.Error("Should fail-closed on empty device info map when enabled")
	}
}

func TestIsDeviceAllowed_OSAllowlist(t *testing.T) {
	ctx := context.Background()
	settings := &mockSettingReader{
		settings: map[string]string{
			"device_trust_enabled":    "true",
			"device_trust_allowed_os": "Windows, macOS",
		},
	}

	// Match (case insensitive)
	if !isDeviceAllowed(ctx, map[string]string{"os": "windows"}, settings) {
		t.Error("Should allow allowed OS (windows)")
	}
	if !isDeviceAllowed(ctx, map[string]string{"os": "macOS"}, settings) {
		t.Error("Should allow allowed OS (macOS)")
	}

	// Deny
	if isDeviceAllowed(ctx, map[string]string{"os": "Linux"}, settings) {
		t.Error("Should block non-allowlisted OS (Linux)")
	}
	if isDeviceAllowed(ctx, map[string]string{"os": ""}, settings) {
		t.Error("Should block missing OS")
	}
}

func TestIsDeviceAllowed_BlockMobile(t *testing.T) {
	ctx := context.Background()
	settings := &mockSettingReader{
		settings: map[string]string{
			"device_trust_enabled":      "true",
			"device_trust_block_mobile": "true",
		},
	}

	if isDeviceAllowed(ctx, map[string]string{"os": "iOS"}, settings) {
		t.Error("Should block iOS when block mobile is true")
	}
	if isDeviceAllowed(ctx, map[string]string{"os": "Android"}, settings) {
		t.Error("Should block Android when block mobile is true")
	}
	if isDeviceAllowed(ctx, map[string]string{"mobile": "true"}, settings) {
		t.Error("Should block mobile:true when block mobile is true")
	}
	if !isDeviceAllowed(ctx, map[string]string{"os": "Windows", "mobile": "false"}, settings) {
		t.Error("Should allow desktop OS when block mobile is true")
	}
}

func TestIsDeviceAllowed_OSVersionCheck(t *testing.T) {
	ctx := context.Background()
	settings := &mockSettingReader{
		settings: map[string]string{
			"device_trust_enabled":            "true",
			"device_trust_min_os_version_mac": "13.0.0",
			"device_trust_min_os_version_win": "10.0",
		},
	}

	// macOS tests
	if !isDeviceAllowed(ctx, map[string]string{"os": "macOS", "os_version": "13.2.1"}, settings) {
		t.Error("Should allow macOS matching or exceeding min version")
	}
	if isDeviceAllowed(ctx, map[string]string{"os": "macOS", "os_version": "12.5"}, settings) {
		t.Error("Should block macOS below min version")
	}
	if isDeviceAllowed(ctx, map[string]string{"os": "macOS", "os_version": ""}, settings) {
		t.Error("Should block macOS missing version")
	}

	// Windows tests
	if !isDeviceAllowed(ctx, map[string]string{"os": "Windows", "os_version": "10.0.1"}, settings) {
		t.Error("Should allow Windows matching or exceeding min version")
	}
	if isDeviceAllowed(ctx, map[string]string{"os": "Windows", "os_version": "6.3"}, settings) {
		t.Error("Should block Windows below min version")
	}

	// Linux is unaffected by macOS/Windows version rules
	if !isDeviceAllowed(ctx, map[string]string{"os": "Linux", "os_version": "1.0"}, settings) {
		t.Error("Should allow Linux regardless of Windows/macOS version rules")
	}
}

func TestIsDeviceAllowed_BrowserAllowlistAndVersion(t *testing.T) {
	ctx := context.Background()
	settings := &mockSettingReader{
		settings: map[string]string{
			"device_trust_enabled":                    "true",
			"device_trust_allowed_browsers":           "Chrome, Safari",
			"device_trust_min_browser_version_chrome": "120.0",
		},
	}

	// Browser check
	if !isDeviceAllowed(ctx, map[string]string{"browser": "Chrome", "browser_version": "121.0"}, settings) {
		t.Error("Should allow Chrome matching min version")
	}
	if !isDeviceAllowed(ctx, map[string]string{"browser": "Safari", "browser_version": "17.0"}, settings) {
		t.Error("Should allow Safari (which has no min version config)")
	}
	if isDeviceAllowed(ctx, map[string]string{"browser": "Firefox"}, settings) {
		t.Error("Should block non-allowlisted browser (Firefox)")
	}

	// Version check
	if isDeviceAllowed(ctx, map[string]string{"browser": "Chrome", "browser_version": "119.0.5"}, settings) {
		t.Error("Should block Chrome below min version")
	}
	if isDeviceAllowed(ctx, map[string]string{"browser": "Chrome", "browser_version": ""}, settings) {
		t.Error("Should block Chrome with missing version")
	}
}
