package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebAuthnLoginBeginHandler(t *testing.T) {
	svc := &fakeAuthService{waBeginOpts: json.RawMessage(`{"publicKey":{}}`)}
	h := NewHandler(svc, nil, nil, false, false, nil, "", nil)

	body := `{"mfa_token":"tok"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/webauthn/login/begin", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.WebAuthnLoginBegin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("publicKey")) {
		t.Fatalf("expected assertion options, got %s", rr.Body.String())
	}
}

func TestWebAuthnLoginBeginHandler_MissingToken(t *testing.T) {
	h := NewHandler(&fakeAuthService{}, nil, nil, false, false, nil, "", nil)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	h.WebAuthnLoginBegin(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestWebAuthnLoginFinishHandler_SetsCookies(t *testing.T) {
	svc := &fakeAuthService{waFinishPair: &TokenPair{AccessToken: "at", RefreshToken: "rt"}}
	h := NewHandler(svc, nil, nil, false, false, nil, "", nil)

	body := `{"mfa_token":"tok","credential":{"id":"abc"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/webauthn/login/finish", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.WebAuthnLoginFinish(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	var sawAccess, sawRefresh bool
	for _, c := range rr.Result().Cookies() {
		switch c.Name {
		case "access_token":
			sawAccess = true
		case "refresh_token":
			sawRefresh = true
		}
	}
	if !sawAccess || !sawRefresh {
		t.Fatalf("expected session cookies to be set (access=%v refresh=%v)", sawAccess, sawRefresh)
	}
}

func TestWebAuthnLoginFinishHandler_InvalidAssertion(t *testing.T) {
	svc := &fakeAuthService{waFinishErr: ErrInvalidCredentials}
	h := NewHandler(svc, nil, nil, false, false, nil, "", nil)

	body := `{"mfa_token":"tok","credential":{"id":"abc"}}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.WebAuthnLoginFinish(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestWebAuthnLoginFinishHandler_MissingFields(t *testing.T) {
	h := NewHandler(&fakeAuthService{}, nil, nil, false, false, nil, "", nil)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"mfa_token":"tok"}`))
	rr := httptest.NewRecorder()
	h.WebAuthnLoginFinish(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
