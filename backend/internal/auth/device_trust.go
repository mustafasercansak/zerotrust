package auth

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

var ErrDeviceNotAllowed = errors.New("device_not_allowed")

// isDeviceAllowed checks the client device details against active security policies.
func isDeviceAllowed(ctx context.Context, deviceInfo map[string]string, settings SettingReader) bool {
	if settings == nil {
		return true
	}
	if !settings.GetBool(ctx, "device_trust_enabled", false) {
		return true
	}

	// Fail closed if device info is missing entirely
	if len(deviceInfo) == 0 {
		return false
	}

	os := strings.TrimSpace(deviceInfo["os"])
	osVersion := strings.TrimSpace(deviceInfo["os_version"])
	browser := strings.TrimSpace(deviceInfo["browser"])
	browserVersion := strings.TrimSpace(deviceInfo["browser_version"])
	isMobile := strings.TrimSpace(deviceInfo["mobile"]) == "true"

	// 1. Mobile restriction check
	if settings.GetBool(ctx, "device_trust_block_mobile", false) {
		if isMobile || strings.EqualFold(os, "ios") || strings.EqualFold(os, "android") {
			return false
		}
	}

	// 2. Allowed OS check
	allowedOSRaw := settings.GetString(ctx, "device_trust_allowed_os", "")
	if allowedOSRaw != "" {
		if os == "" {
			return false
		}
		allowedOSList := splitAndTrim(allowedOSRaw)
		osMatch := false
		for _, allowed := range allowedOSList {
			if strings.EqualFold(allowed, os) {
				osMatch = true
				break
			}
		}
		if !osMatch {
			return false
		}
	}

	// 3. OS version checks
	if strings.EqualFold(os, "macos") {
		minMacVer := settings.GetString(ctx, "device_trust_min_os_version_mac", "")
		if minMacVer != "" {
			if osVersion == "" || compareVersions(osVersion, minMacVer) < 0 {
				return false
			}
		}
	} else if strings.EqualFold(os, "windows") {
		minWinVer := settings.GetString(ctx, "device_trust_min_os_version_win", "")
		if minWinVer != "" {
			if osVersion == "" || compareVersions(osVersion, minWinVer) < 0 {
				return false
			}
		}
	}

	// 4. Allowed Browsers check
	allowedBrowsersRaw := settings.GetString(ctx, "device_trust_allowed_browsers", "")
	if allowedBrowsersRaw != "" {
		if browser == "" {
			return false
		}
		allowedBrowsersList := splitAndTrim(allowedBrowsersRaw)
		browserMatch := false
		for _, allowed := range allowedBrowsersList {
			if strings.EqualFold(allowed, browser) {
				browserMatch = true
				break
			}
		}
		if !browserMatch {
			return false
		}
	}

	// 5. Browser version checks
	if strings.EqualFold(browser, "chrome") {
		minVer := settings.GetString(ctx, "device_trust_min_browser_version_chrome", "")
		if minVer != "" {
			if browserVersion == "" || compareVersions(browserVersion, minVer) < 0 {
				return false
			}
		}
	} else if strings.EqualFold(browser, "safari") {
		minVer := settings.GetString(ctx, "device_trust_min_browser_version_safari", "")
		if minVer != "" {
			if browserVersion == "" || compareVersions(browserVersion, minVer) < 0 {
				return false
			}
		}
	} else if strings.EqualFold(browser, "firefox") {
		minVer := settings.GetString(ctx, "device_trust_min_browser_version_firefox", "")
		if minVer != "" {
			if browserVersion == "" || compareVersions(browserVersion, minVer) < 0 {
				return false
			}
		}
	} else if strings.EqualFold(browser, "edge") {
		minVer := settings.GetString(ctx, "device_trust_min_browser_version_edge", "")
		if minVer != "" {
			if browserVersion == "" || compareVersions(browserVersion, minVer) < 0 {
				return false
			}
		}
	}

	return true
}

// compareVersions compares two dot-separated version strings.
// Returns -1 if v1 < v2, 1 if v1 > v2, and 0 if v1 == v2.
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	for i := 0; i < len(parts1) || i < len(parts2); i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	return 0
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
