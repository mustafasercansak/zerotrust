package validation

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsPasswordPwned(t *testing.T) {
	// SHA-1 of "pwnedpassword" -> d89efb7b58a2da8718db68d89aeffd5083174c0d
	// Prefix: D89EF
	// Suffix: B7B58A2DA8718DB68D89AEFFD5083174C0D

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Path[1:]
		if prefix == "D89EF" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("B7B58A2DA8718DB68D89AEFFD5083174C0D:12\n"))
			_, _ = w.Write([]byte("AAAAAE23BD5B727046A9E3B4B7DB57BD8D6:2\n"))
		} else {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("BBBBBE23BD5B727046A9E3B4B7DB57BD8D6:1\n"))
		}
	}))
	defer server.Close()

	oldURL := PwnedAPIURL
	PwnedAPIURL = server.URL + "/"
	defer func() { PwnedAPIURL = oldURL }()

	// Test empty password
	pwned, err := IsPasswordPwned("")
	if err != nil || pwned {
		t.Errorf("empty password: got pwned=%v, err=%v", pwned, err)
	}

	// Test pwned password
	pwned, err = IsPasswordPwned("pwnedpassword")
	if err != nil {
		t.Errorf("pwnedpassword returned error: %v", err)
	}
	if !pwned {
		t.Error("expected pwnedpassword to be pwned, but got false")
	}

	// Test unpwned password
	pwned, err = IsPasswordPwned("unpwnedpassword")
	if err != nil {
		t.Errorf("unpwnedpassword returned error: %v", err)
	}
	if pwned {
		t.Error("expected unpwnedpassword to NOT be pwned, but got true")
	}
}

func TestIsPasswordPwned_FailOpen(t *testing.T) {
	// Test server that returns 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	oldURL := PwnedAPIURL
	PwnedAPIURL = server.URL + "/"
	defer func() { PwnedAPIURL = oldURL }()

	pwned, err := IsPasswordPwned("pwnedpassword")
	if err != nil {
		t.Errorf("expected fail-open to not return error, got %v", err)
	}
	if pwned {
		t.Error("expected fail-open to return false (allow password), got true")
	}
}
