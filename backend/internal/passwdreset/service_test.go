package passwdreset

import (
	"context"
	"errors"
	"testing"
)

// stubStore implements store for testing without a real database.
type stubStore struct {
	consumeErr error
}

func (s *stubStore) Create(_ context.Context, _ string) (string, error) {
	return "raw-token", nil
}

func (s *stubStore) ConsumeAndReset(_ context.Context, _, _ string) error {
	return s.consumeErr
}

// TestReset_TokenErrors verifies that all token-state errors are mapped to the
// single "invalid_token" sentinel that the handler returns to the caller.
// Exposing ErrExpired vs ErrUsed to the API would leak information.
func TestReset_TokenErrors(t *testing.T) {
	cases := []struct {
		name    string
		repoErr error
	}{
		{"not found", ErrNotFound},
		{"expired", ErrExpired},
		{"used", ErrUsed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := &Service{repo: &stubStore{consumeErr: c.repoErr}}
			err := svc.Reset(context.Background(), "any-token", "Password1!")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != "invalid_token" {
				t.Errorf("got %q, want %q", err.Error(), "invalid_token")
			}
		})
	}
}

// TestReset_DBError verifies that unexpected repository errors propagate
// unchanged so the caller can log and return a 500.
func TestReset_DBError(t *testing.T) {
	dbErr := errors.New("connection refused")
	svc := &Service{repo: &stubStore{consumeErr: dbErr}}
	err := svc.Reset(context.Background(), "any-token", "Password1!")
	if !errors.Is(err, dbErr) {
		t.Errorf("expected DB error to propagate, got %v", err)
	}
}

// TestReset_Success verifies the happy path returns nil.
func TestReset_Success(t *testing.T) {
	svc := &Service{repo: &stubStore{}}
	if err := svc.Reset(context.Background(), "any-token", "Password1!"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
