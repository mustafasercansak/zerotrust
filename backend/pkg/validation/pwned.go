package validation

import (
	"bufio"
	"context"
	"crypto/sha1"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PwnedAPIURL is the endpoint for the HaveIBeenPwned range API.
var PwnedAPIURL = "https://api.pwnedpasswords.com/range/"

// HTTPClient allows mocking or custom configurations in tests.
var HTTPClient = &http.Client{
	Timeout: 1500 * time.Millisecond,
}

// IsPasswordPwned checks if the password has been leaked in a known data breach.
// It uses the k-Anonymity model, sending only the first 5 characters of the SHA-1 hash.
// If the API call fails or times out, it fails open (returns false, nil) to avoid
// locking out users.
func IsPasswordPwned(password string) (bool, error) {
	if len(password) == 0 {
		return false, nil
	}

	// Skip external network calls to HaveIBeenPwned when running tests to avoid flakiness
	// and API rate limits. Mocked tests that override PwnedAPIURL to a local server are not skipped.
	if flag.Lookup("test.v") != nil && strings.HasPrefix(PwnedAPIURL, "https://api.pwnedpasswords.com") {
		return false, nil
	}

	// 1. Calculate SHA-1 hash of the password
	h := sha1.New()
	h.Write([]byte(password))
	hash := fmt.Sprintf("%X", h.Sum(nil)) // Uppercase hex representation

	prefix := hash[:5]
	suffix := hash[5:]

	// 2. Query HaveIBeenPwned API range endpoint
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", PwnedAPIURL+prefix, nil)
	if err != nil {
		// Fail open on request creation errors
		return false, nil
	}
	// HaveIBeenPwned API guidelines suggest setting a User-Agent
	req.Header.Set("User-Agent", "ZeroTrust-Auth-Portal/1.0")

	resp, err := HTTPClient.Do(req)
	if err != nil {
		// Fail open on network errors/timeouts
		return false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fail open on non-200 responses
		return false, nil
	}

	// 3. Parse the response line-by-line to search for the suffix hash matching ours
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) >= 1 {
			// HaveIBeenPwned returns suffix in uppercase
			if strings.EqualFold(parts[0], suffix) {
				return true, nil
			}
		}
	}

	return false, scanner.Err()
}
