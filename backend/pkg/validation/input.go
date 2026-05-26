package validation

import (
	"errors"
	"regexp"
	"unicode"
)

var (
	ErrEmailInvalid      = errors.New("email_invalid")
	ErrPasswordTooShort  = errors.New("password_too_short")
	ErrPasswordNoUpper   = errors.New("password_no_uppercase")
	ErrPasswordNoLower   = errors.New("password_no_lowercase")
	ErrPasswordNoDigit   = errors.New("password_no_digit")
	ErrPasswordNoSpecial = errors.New("password_no_special")
)

var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func Email(email string) error {
	if !emailRe.MatchString(email) {
		return ErrEmailInvalid
	}
	return nil
}

func Password(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, c := range password {
		switch {
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsDigit(c):
			hasDigit = true
		case unicode.IsPunct(c) || unicode.IsSymbol(c):
			hasSpecial = true
		}
	}
	if !hasUpper {
		return ErrPasswordNoUpper
	}
	if !hasLower {
		return ErrPasswordNoLower
	}
	if !hasDigit {
		return ErrPasswordNoDigit
	}
	if !hasSpecial {
		return ErrPasswordNoSpecial
	}
	return nil
}
