package validation_test

import (
	"testing"

	"github.com/zerotrust/backend/pkg/validation"
)

func TestEmail(t *testing.T) {
	valid := []string{
		"user@example.com",
		"user+tag@example.co.uk",
		"first.last@sub.domain.org",
		"x@y.io",
	}
	for _, email := range valid {
		if err := validation.Email(email); err != nil {
			t.Errorf("Email(%q) unexpected error: %v", email, err)
		}
	}

	invalid := []string{
		"",
		"notanemail",
		"@nodomain.com",
		"noatsign",
		"missing@",
		"spaces in@email.com",
		"double@@at.com",
	}
	for _, email := range invalid {
		if err := validation.Email(email); err == nil {
			t.Errorf("Email(%q) expected error, got nil", email)
		}
	}
}

func TestPassword(t *testing.T) {
	if err := validation.Password("Abcdef1!"); err != nil {
		t.Errorf("valid password rejected: %v", err)
	}

	cases := []struct {
		pw      string
		wantErr error
	}{
		{"Ab1!", validation.ErrPasswordTooShort},
		{"abcdef1!", validation.ErrPasswordNoUpper},
		{"ABCDEF1!", validation.ErrPasswordNoLower},
		{"Abcdefgh!", validation.ErrPasswordNoDigit},
		{"Abcdefg1", validation.ErrPasswordNoSpecial},
	}
	for _, c := range cases {
		err := validation.Password(c.pw)
		if err != c.wantErr {
			t.Errorf("Password(%q) = %v, want %v", c.pw, err, c.wantErr)
		}
	}
}
