package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/zerotrust/backend/internal/user"
)

// countingUserReader records how many times CheckPassword is invoked so tests
// can assert that the login path runs a bcrypt comparison even when the account
// is missing or inactive (ISSUE_LIST #33 — user enumeration timing defense).
type countingUserReader struct {
	u             *user.User
	notFound      bool
	checkCalls    int
	lastHashCheck string
}

func (r *countingUserReader) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	if r.notFound || r.u == nil {
		return nil, user.ErrNotFound
	}
	return r.u, nil
}

func (r *countingUserReader) FindByID(ctx context.Context, id string) (*user.User, error) {
	if r.u == nil {
		return nil, user.ErrNotFound
	}
	return r.u, nil
}

func (r *countingUserReader) CheckPassword(hash, password string) bool {
	r.checkCalls++
	r.lastHashCheck = hash
	// Never matches — these tests only exercise the failure paths.
	return false
}

func (r *countingUserReader) GetPermissions(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func TestLogin_RunsPasswordCompareToPreventEnumeration(t *testing.T) {
	cases := []struct {
		name   string
		reader *countingUserReader
	}{
		{
			name:   "unknown email still runs bcrypt compare",
			reader: &countingUserReader{notFound: true},
		},
		{
			name: "inactive user still runs bcrypt compare",
			reader: &countingUserReader{u: &user.User{
				ID:           "u1",
				Email:        "inactive@example.com",
				PasswordHash: dummyPasswordHash,
				IsActive:     false,
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// rdb/ks/mfa/settings all nil — the failure paths return before
			// touching them.
			svc := NewService(tc.reader, nil, nil, nil, nil, nil, nil)

			_, err := svc.Login(context.Background(), "someone@example.com", "whatever", "1.2.3.4", "ua", nil)
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("expected ErrInvalidCredentials, got %v", err)
			}
			if tc.reader.checkCalls != 1 {
				t.Fatalf("expected exactly one password comparison (timing-equalizing), got %d", tc.reader.checkCalls)
			}
		})
	}
}

func TestDummyPasswordHash_IsValidBcrypt(t *testing.T) {
	// A valid bcrypt hash is required, otherwise CompareHashAndPassword returns
	// instantly and the timing defense is defeated.
	r := &countingUserReader{}
	if got := r.CheckPassword(dummyPasswordHash, "x"); got {
		t.Fatal("dummy hash unexpectedly matched")
	}
	if len(dummyPasswordHash) < 59 || dummyPasswordHash[:4] != "$2a$" {
		t.Fatalf("dummy hash does not look like a bcrypt hash: %q", dummyPasswordHash)
	}
}
