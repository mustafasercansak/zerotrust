package serviceaccount

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zerotrust/backend/internal/auth"
)

func newEventTestKeyStore(t *testing.T) *auth.KeyStore {
	t.Helper()
	ks, err := auth.LoadOrGenerateKeyStore("", "")
	if err != nil {
		t.Fatalf("key store: %v", err)
	}
	return ks
}

func newServiceAccountReadToken(t *testing.T, ks *auth.KeyStore) string {
	t.Helper()
	pair, err := auth.GenerateTokenPair(
		ks,
		"user-1",
		"admin@example.com",
		"en",
		[]string{"admin"},
		[]string{"service_accounts:read"},
		time.Minute,
	)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return pair.AccessToken
}

func TestEventsRejectsQueryToken(t *testing.T) {
	ks := newEventTestKeyStore(t)
	token := newServiceAccountReadToken(t, ks)
	h := NewHandler(nil, NewEventHub(), ks, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-accounts/events?token="+token, nil)
	rr := httptest.NewRecorder()

	h.Events(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
}

func TestEventsAcceptsAuthorizationBearerToken(t *testing.T) {
	ks := newEventTestKeyStore(t)
	token := newServiceAccountReadToken(t, ks)
	h := NewHandler(nil, NewEventHub(), ks, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-accounts/events", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	h.Events(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type=%q want text/event-stream", got)
	}
	if body := rr.Body.String(); body != "data: connected\n\n" {
		t.Fatalf("body=%q want connected event", body)
	}
}

func TestEventsAcceptsAccessTokenCookie(t *testing.T) {
	ks := newEventTestKeyStore(t)
	token := newServiceAccountReadToken(t, ks)
	h := NewHandler(nil, NewEventHub(), ks, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-accounts/events", nil).WithContext(ctx)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	rr := httptest.NewRecorder()

	h.Events(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}
