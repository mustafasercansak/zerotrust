package validation_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/zerotrust/backend/pkg/validation"
)

func TestMain(m *testing.M) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Path[1:]
		if prefix == "5348D" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("06A13507EF5B0A0EBD080FF9305DB18445E:5\n"))
		} else {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(""))
		}
	}))
	defer server.Close()

	oldURL := validation.PwnedAPIURL
	validation.PwnedAPIURL = server.URL + "/"

	code := m.Run()

	validation.PwnedAPIURL = oldURL
	os.Exit(code)
}

func TestEmail(t *testing.T) {
	if err := validation.Email("test@example.com"); err != nil {
		t.Error("valid email failed")
	}
	if err := validation.Email("invalid"); err == nil {
		t.Error("invalid email passed")
	}
}

func TestPassword(t *testing.T) {
	if err := validation.Password("short"); err != validation.ErrPasswordTooShort {
		t.Error("short password passed")
	}
	if err := validation.Password("nouppercase1!"); err != validation.ErrPasswordNoUpper {
		t.Error("no upper password passed")
	}
	if err := validation.Password("NOLOWERCASE1!"); err != validation.ErrPasswordNoLower {
		t.Error("no lower password passed")
	}
	if err := validation.Password("NoDigitPassword!"); err != validation.ErrPasswordNoDigit {
		t.Error("no digit password passed")
	}
	if err := validation.Password("NoSpecial1234"); err != validation.ErrPasswordNoSpecial {
		t.Error("no special password passed")
	}
	if err := validation.Password("ValidPass1!"); err != nil {
		t.Error("valid password failed")
	}
	if err := validation.Password("PwnedPass1!"); err != validation.ErrPasswordCompromised {
		t.Error("compromised password passed")
	}
}

func TestPasswordWithComplexity(t *testing.T) {
	// low
	if err := validation.PasswordWithComplexity("12345", "low"); err != validation.ErrPasswordTooShort6 {
		t.Errorf("low complexity expected too short")
	}
	if err := validation.PasswordWithComplexity("123456", "low"); err != nil {
		t.Errorf("low complexity expected ok")
	}
	if err := validation.PasswordWithComplexity("PwnedPass1!", "low"); err != validation.ErrPasswordCompromised {
		t.Errorf("low complexity expected compromised check")
	}

	// medium
	if err := validation.PasswordWithComplexity("1234567", "medium"); err != validation.ErrPasswordTooShort {
		t.Errorf("medium complexity expected too short")
	}
	if err := validation.PasswordWithComplexity("12345678", "medium"); err != validation.ErrPasswordNoLetter {
		t.Errorf("medium complexity expected no letter")
	}
	if err := validation.PasswordWithComplexity("abcdefgh", "medium"); err != validation.ErrPasswordNoDigit {
		t.Errorf("medium complexity expected no digit")
	}
	if err := validation.PasswordWithComplexity("abcd1234", "medium"); err != nil {
		t.Errorf("medium complexity expected ok")
	}
	if err := validation.PasswordWithComplexity("abcd!@#$", "medium"); err != nil {
		t.Errorf("medium complexity expected ok with punctuation")
	}
	if err := validation.PasswordWithComplexity("PwnedPass1!", "medium"); err != validation.ErrPasswordCompromised {
		t.Errorf("medium complexity expected compromised check")
	}

	// strong / default
	if err := validation.PasswordWithComplexity("abcd1234", "strong"); err != validation.ErrPasswordNoUpper {
		t.Errorf("strong complexity expected no upper")
	}
	if err := validation.PasswordWithComplexity("abcd1234", ""); err != validation.ErrPasswordNoUpper {
		t.Errorf("default complexity expected no upper")
	}
	if err := validation.PasswordWithComplexity("PwnedPass1!", "strong"); err != validation.ErrPasswordCompromised {
		t.Errorf("strong complexity expected compromised check")
	}
}
