package user

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestCheckPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("Password1!"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("generate hash: %v", err)
	}

	svc := &Service{}
	if !svc.CheckPassword(string(hash), "Password1!") {
		t.Fatal("expected password verification to succeed")
	}
	if svc.CheckPassword(string(hash), "wrong") {
		t.Fatal("expected password verification to fail for wrong password")
	}
}

func TestUpdateProfileRejectsLongNames(t *testing.T) {
	svc := &Service{}
	long := strings.Repeat("a", 81)

	_, err := svc.UpdateProfile(nil, "user-id", long, "ok")
	if err != ErrInvalidProfile {
		t.Fatalf("first name error=%v want=%v", err, ErrInvalidProfile)
	}

	_, err = svc.UpdateProfile(nil, "user-id", "ok", long)
	if err != ErrInvalidProfile {
		t.Fatalf("last name error=%v want=%v", err, ErrInvalidProfile)
	}
}
