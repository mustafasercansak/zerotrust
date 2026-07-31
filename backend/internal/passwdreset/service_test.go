package passwdreset

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/zerotrust/backend/internal/user"
)

// stubStore implements store for testing without a real database.
type stubStore struct {
	createToken  string
	createErr    error
	createUserID string
	consumeErr   error
}

func (s *stubStore) Create(_ context.Context, userID string) (string, error) {
	s.createUserID = userID
	if s.createErr != nil {
		return "", s.createErr
	}
	if s.createToken != "" {
		return s.createToken, nil
	}
	return "raw-token", nil
}

func (s *stubStore) ConsumeAndReset(_ context.Context, _, _ string) error {
	return s.consumeErr
}

type stubUsers struct {
	user  *user.User
	err   error
	email string
}

func (s *stubUsers) FindByEmail(_ context.Context, email string) (*user.User, error) {
	s.email = email
	if s.err != nil {
		return nil, s.err
	}
	return s.user, nil
}

type stubMailer struct {
	err      error
	to       string
	resetURL string
}

func (m *stubMailer) SendPasswordReset(_ context.Context, to, resetURL string) error {
	m.to = to
	m.resetURL = resetURL
	return m.err
}

func (m *stubMailer) SendSecurityAlert(context.Context, string, string, string, string, string) error {
	return nil
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

func TestSendReset(t *testing.T) {
	t.Run("unknown email stays silent", func(t *testing.T) {
		repo := &stubStore{}
		users := &stubUsers{err: user.ErrNotFound}
		mail := &stubMailer{}
		svc := NewService(repo, users, mail)

		if err := svc.SendReset(context.Background(), "missing@example.com", "https://app.example.com"); err != nil {
			t.Fatalf("SendReset error: %v", err)
		}
		if repo.createUserID != "" {
			t.Fatalf("repo should not create token for unknown user, got %q", repo.createUserID)
		}
		if mail.resetURL != "" {
			t.Fatalf("mailer should not be called, got %q", mail.resetURL)
		}
	})

	t.Run("repo create error returns", func(t *testing.T) {
		repo := &stubStore{createErr: errors.New("boom")}
		users := &stubUsers{user: &user.User{ID: "u1", Email: "user@example.com", Locale: "tr"}}
		mail := &stubMailer{}
		svc := NewService(repo, users, mail)

		err := svc.SendReset(context.Background(), "user@example.com", "https://app.example.com")
		if err == nil || err.Error() != "boom" {
			t.Fatalf("expected boom error, got %v", err)
		}
	})

	t.Run("uses locale fallback and sends email", func(t *testing.T) {
		repo := &stubStore{createToken: "token-123"}
		users := &stubUsers{user: &user.User{ID: "u1", Email: "user@example.com", Locale: ""}}
		mail := &stubMailer{}
		svc := NewService(repo, users, mail)

		if err := svc.SendReset(context.Background(), "user@example.com", "https://app.example.com"); err != nil {
			t.Fatalf("SendReset error: %v", err)
		}
		if repo.createUserID != "u1" {
			t.Fatalf("createUserID=%q want=u1", repo.createUserID)
		}
		if mail.to != "user@example.com" {
			t.Fatalf("mail to=%q want=user@example.com", mail.to)
		}
		wantURL := "https://app.example.com/en/auth/reset-password?token=token-123"
		if mail.resetURL != wantURL {
			t.Fatalf("resetURL=%q want=%q", mail.resetURL, wantURL)
		}
	})

	t.Run("preserves user locale", func(t *testing.T) {
		repo := &stubStore{createToken: "token-456"}
		users := &stubUsers{user: &user.User{ID: "u1", Email: "user@example.com", Locale: "tr"}}
		mail := &stubMailer{}
		svc := NewService(repo, users, mail)

		if err := svc.SendReset(context.Background(), "user@example.com", "https://app.example.com"); err != nil {
			t.Fatalf("SendReset error: %v", err)
		}
		wantURL := "https://app.example.com/tr/auth/reset-password?token=token-456"
		if mail.resetURL != wantURL {
			t.Fatalf("resetURL=%q want=%q", mail.resetURL, wantURL)
		}
	})

	t.Run("mailer error returns", func(t *testing.T) {
		repo := &stubStore{createToken: "token-789"}
		users := &stubUsers{user: &user.User{ID: "u1", Email: "user@example.com", Locale: "en"}}
		mail := &stubMailer{err: errors.New("smtp down")}
		svc := NewService(repo, users, mail)

		err := svc.SendReset(context.Background(), "user@example.com", "https://app.example.com")
		if err == nil || err.Error() != "smtp down" {
			t.Fatalf("expected smtp down error, got %v", err)
		}
	})
}

// TestSendReset_CooldownThrottlesRepeatSends proves a second SendReset within
// the cooldown window for the same email does not create another token or
// send another email — mitigating mailbox flooding and reset-token churn.
// (ISSUE_LIST #93)
func TestSendReset_CooldownThrottlesRepeatSends(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	repo := &stubStore{createToken: "token-1"}
	users := &stubUsers{user: &user.User{ID: "u1", Email: "user@example.com", Locale: "en"}}
	mail := &stubMailer{}
	svc := NewService(repo, users, mail)
	svc.SetRedis(rdb)

	if err := svc.SendReset(context.Background(), "user@example.com", "https://app.example.com"); err != nil {
		t.Fatalf("first SendReset error: %v", err)
	}
	if repo.createUserID != "u1" {
		t.Fatalf("expected first send to create a token, createUserID=%q", repo.createUserID)
	}

	repo.createUserID = ""
	mail.resetURL = ""
	if err := svc.SendReset(context.Background(), "user@example.com", "https://app.example.com"); err != nil {
		t.Fatalf("second SendReset error: %v", err)
	}
	if repo.createUserID != "" {
		t.Fatal("second send within cooldown must not create a new token")
	}
	if mail.resetURL != "" {
		t.Fatal("second send within cooldown must not send another email")
	}
}

func TestSendReset_CooldownIsPerEmail(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	repo := &stubStore{createToken: "token-1"}
	users := &stubUsers{user: &user.User{ID: "u1", Email: "user@example.com", Locale: "en"}}
	mail := &stubMailer{}
	svc := NewService(repo, users, mail)
	svc.SetRedis(rdb)

	if err := svc.SendReset(context.Background(), "user@example.com", "https://app.example.com"); err != nil {
		t.Fatalf("first SendReset error: %v", err)
	}

	users.user = &user.User{ID: "u2", Email: "other@example.com", Locale: "en"}
	repo.createUserID = ""
	if err := svc.SendReset(context.Background(), "other@example.com", "https://app.example.com"); err != nil {
		t.Fatalf("second SendReset error: %v", err)
	}
	if repo.createUserID != "u2" {
		t.Fatalf("a different email must not be throttled by another address's cooldown, createUserID=%q", repo.createUserID)
	}
}

func TestSendReset_NoRedisConfiguredSendsEveryTime(t *testing.T) {
	repo := &stubStore{createToken: "token-1"}
	users := &stubUsers{user: &user.User{ID: "u1", Email: "user@example.com", Locale: "en"}}
	mail := &stubMailer{}
	svc := NewService(repo, users, mail)

	for i := 0; i < 2; i++ {
		repo.createUserID = ""
		if err := svc.SendReset(context.Background(), "user@example.com", "https://app.example.com"); err != nil {
			t.Fatalf("SendReset error: %v", err)
		}
		if repo.createUserID != "u1" {
			t.Fatal("without Redis configured, every request must send a fresh reset link")
		}
	}
}

func TestGenerateToken(t *testing.T) {
	tok1, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken returned error: %v", err)
	}
	tok2, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken returned error: %v", err)
	}

	if len(tok1) != 64 || len(tok2) != 64 {
		t.Fatalf("unexpected token lengths: %d, %d", len(tok1), len(tok2))
	}
	if _, err := hex.DecodeString(tok1); err != nil {
		t.Fatalf("token 1 is not valid hex: %v", err)
	}
	if _, err := hex.DecodeString(tok2); err != nil {
		t.Fatalf("token 2 is not valid hex: %v", err)
	}
	if tok1 == tok2 {
		t.Fatal("expected two generated tokens to differ")
	}
}

func TestReset_PasswordReuseForbidden(t *testing.T) {
	svc := &Service{repo: &stubStore{consumeErr: ErrPasswordReuseForbidden}}
	err := svc.Reset(context.Background(), "any-token", "Secret123!")
	if !errors.Is(err, ErrPasswordReuseForbidden) {
		t.Fatalf("expected ErrPasswordReuseForbidden error, got %v", err)
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	h1 := hashToken("test-token-abc")
	h2 := hashToken("test-token-abc")
	if h1 != h2 {
		t.Fatal("hashToken should be deterministic")
	}
	if len(h1) != 64 {
		t.Fatalf("hash length=%d want=64 (SHA-256 hex)", len(h1))
	}
	if hashToken("a") == hashToken("b") {
		t.Fatal("different inputs must produce different hashes")
	}
}
