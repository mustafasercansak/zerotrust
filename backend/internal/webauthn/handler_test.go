package webauthn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/zerotrust/backend/internal/auth"
	authmw "github.com/zerotrust/backend/pkg/middleware"
)

type fakeService struct {
	beginOpts  json.RawMessage
	beginErr   error
	finishErr  error
	finishName string
	finishBody []byte
	listMeta   []CredentialMeta
	listErr    error
	deleteErr  error
	deletedID  string
}

func (f *fakeService) BeginRegistration(_ context.Context, _, _, _ string) (json.RawMessage, error) {
	return f.beginOpts, f.beginErr
}
func (f *fakeService) FinishRegistration(_ context.Context, _, _, _, credName string, body []byte) error {
	f.finishName = credName
	f.finishBody = body
	return f.finishErr
}
func (f *fakeService) ListCredentials(_ context.Context, _ string) ([]CredentialMeta, error) {
	return f.listMeta, f.listErr
}
func (f *fakeService) DeleteCredential(_ context.Context, id, _ string) error {
	f.deletedID = id
	return f.deleteErr
}

func authedReq(method, body string) *http.Request {
	r := httptest.NewRequest(method, "/", bytes.NewBufferString(body))
	ctx := context.WithValue(r.Context(), authmw.ClaimsKey, &auth.Claims{UserID: "u1", Email: "u@example.com"})
	return r.WithContext(ctx)
}

func TestRegisterBegin_RequiresAuth(t *testing.T) {
	h := NewHandler(&fakeService{})
	w := httptest.NewRecorder()
	// No claims in context.
	h.RegisterBegin(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRegisterBegin_ReturnsOptions(t *testing.T) {
	h := NewHandler(&fakeService{beginOpts: json.RawMessage(`{"publicKey":{"x":1}}`)})
	w := httptest.NewRecorder()
	h.RegisterBegin(w, authedReq(http.MethodPost, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("publicKey")) {
		t.Fatalf("expected options body, got %s", w.Body.String())
	}
}

func TestRegisterFinish_DefaultsNameAndPassesCredential(t *testing.T) {
	f := &fakeService{}
	h := NewHandler(f)
	w := httptest.NewRecorder()
	h.RegisterFinish(w, authedReq(http.MethodPost, `{"credential":{"id":"abc"}}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if f.finishName != "Passkey" {
		t.Fatalf("expected default name 'Passkey', got %q", f.finishName)
	}
	if !bytes.Contains(f.finishBody, []byte(`"id":"abc"`)) {
		t.Fatalf("credential not forwarded: %s", f.finishBody)
	}
}

func TestRegisterFinish_MissingCredential(t *testing.T) {
	h := NewHandler(&fakeService{})
	w := httptest.NewRecorder()
	h.RegisterFinish(w, authedReq(http.MethodPost, `{"name":"x"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRegisterFinish_DuplicateCredential(t *testing.T) {
	h := NewHandler(&fakeService{finishErr: ErrCredentialInUse})
	w := httptest.NewRecorder()
	h.RegisterFinish(w, authedReq(http.MethodPost, `{"name":"x","credential":{"id":"abc"}}`))
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestRegisterBegin_ServiceError(t *testing.T) {
	h := NewHandler(&fakeService{beginErr: errors.New("boom")})
	w := httptest.NewRecorder()
	h.RegisterBegin(w, authedReq(http.MethodPost, ""))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestRegisterFinish_RequiresAuth(t *testing.T) {
	h := NewHandler(&fakeService{})
	w := httptest.NewRecorder()
	h.RegisterFinish(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"credential":{"id":"a"}}`)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRegisterFinish_CeremonyExpired(t *testing.T) {
	h := NewHandler(&fakeService{finishErr: ErrSessionNotFound})
	w := httptest.NewRecorder()
	h.RegisterFinish(w, authedReq(http.MethodPost, `{"credential":{"id":"abc"}}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("ceremony_expired")) {
		t.Fatalf("expected ceremony_expired, got %s", w.Body.String())
	}
}

func TestRegisterFinish_InvalidCredential(t *testing.T) {
	h := NewHandler(&fakeService{finishErr: errors.New("bad attestation")})
	w := httptest.NewRecorder()
	h.RegisterFinish(w, authedReq(http.MethodPost, `{"credential":{"id":"abc"}}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("invalid_credential")) {
		t.Fatalf("expected invalid_credential, got %s", w.Body.String())
	}
}

func TestRegisterFinish_LongNameTruncated(t *testing.T) {
	f := &fakeService{}
	h := NewHandler(f)
	longName := strings.Repeat("é", 150) // 150 runes
	body := `{"name":"` + longName + `","credential":{"id":"abc"}}`
	w := httptest.NewRecorder()
	h.RegisterFinish(w, authedReq(http.MethodPost, body))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if n := len([]rune(f.finishName)); n != 100 {
		t.Fatalf("expected name truncated to 100 runes, got %d", n)
	}
}

func TestList_RequiresAuth(t *testing.T) {
	h := NewHandler(&fakeService{})
	w := httptest.NewRecorder()
	h.List(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestList_ServiceError(t *testing.T) {
	h := NewHandler(&fakeService{listErr: errors.New("db down")})
	w := httptest.NewRecorder()
	h.List(w, authedReq(http.MethodGet, ""))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestDelete_RequiresAuth(t *testing.T) {
	h := NewHandler(&fakeService{})
	w := httptest.NewRecorder()
	h.Delete(w, httptest.NewRequest(http.MethodDelete, "/", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestDelete_InternalError(t *testing.T) {
	h := NewHandler(&fakeService{deleteErr: errors.New("db down")})
	r := authedReq(http.MethodDelete, "")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "c1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Delete(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestList_ReturnsCredentials(t *testing.T) {
	h := NewHandler(&fakeService{listMeta: []CredentialMeta{{ID: "c1", Name: "YubiKey"}}})
	w := httptest.NewRecorder()
	h.List(w, authedReq(http.MethodGet, ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("YubiKey")) {
		t.Fatalf("expected credential in body, got %s", w.Body.String())
	}
}

func TestDelete_NotFound(t *testing.T) {
	h := NewHandler(&fakeService{deleteErr: ErrNotFound})
	r := authedReq(http.MethodDelete, "")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "missing")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Delete(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDelete_Success(t *testing.T) {
	f := &fakeService{}
	h := NewHandler(f)
	r := authedReq(http.MethodDelete, "")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "c1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.Delete(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if f.deletedID != "c1" {
		t.Fatalf("expected delete of c1, got %q", f.deletedID)
	}
}
