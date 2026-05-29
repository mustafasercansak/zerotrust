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
	ErrPasswordTooShort6 = errors.New("password_too_short_6")
	ErrPasswordNoLetter  = errors.New("password_no_letter")
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

func PasswordWithComplexity(password string, complexity string) error {
	switch complexity {
	case "low":
		if len(password) < 6 {
			return ErrPasswordTooShort6
		}
		return nil
	case "medium":
		if len(password) < 8 {
			return ErrPasswordTooShort
		}
		var hasLetter, hasDigit bool
		for _, c := range password {
			if unicode.IsLetter(c) {
				hasLetter = true
			} else if unicode.IsDigit(c) || unicode.IsPunct(c) || unicode.IsSymbol(c) {
				hasDigit = true
			}
		}
		if !hasLetter {
			return ErrPasswordNoLetter
		}
		if !hasDigit {
			return ErrPasswordNoDigit
		}
		return nil
	case "strong":
		fallthrough
	default:
		return Password(password)
	}
}

