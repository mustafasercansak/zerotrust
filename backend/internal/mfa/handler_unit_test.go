package mfa

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zerotrust/backend/internal/auth"
	authmw "github.com/zerotrust/backend/pkg/middleware"
)

type mockMFAService struct {
	setupOTP    string
	setupSecret string
	setupCodes  []string
	setupErr    error
	verifyErr   error
	disableErr  error
	enabled     bool
	validateOK  bool
}

func (m *mockMFAService) Setup(context.Context, string, string, string) (string, string, []string, error) {
	if m.setupErr != nil {
		return "", "", nil, m.setupErr
	}
	return m.setupOTP, m.setupSecret, m.setupCodes, nil
}

func (m *mockMFAService) VerifyAndEnable(context.Context, string, string) error {
	return m.verifyErr
}

func (m *mockMFAService) Disable(context.Context, string, string) error {
	return m.disableErr
}

func (m *mockMFAService) IsEnabled(context.Context, string) bool {
	return m.enabled
}

func (m *mockMFAService) Validate(context.Context, string, string) bool {
	return m.validateOK
}

func withMFAClaims(req *http.Request) *http.Request {
	claims := &auth.Claims{UserID: "u1", Email: "u1@example.com"}
	ctx := context.WithValue(req.Context(), authmw.ClaimsKey, claims)
	return req.WithContext(ctx)
}

func TestMFAHandlerSetup(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		h := NewHandler(&mockMFAService{}, nil, 0)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mfa/setup", nil)
		rr := httptest.NewRecorder()
		h.Setup(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		h := NewHandler(&mockMFAService{}, nil, 0)
		req := withMFAClaims(httptest.NewRequest(http.MethodPost, "/api/v1/mfa/setup", bytes.NewBufferString("{bad")))
		rr := httptest.NewRecorder()
		h.Setup(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("invalid code", func(t *testing.T) {
		h := NewHandler(&mockMFAService{setupErr: errors.New("invalid_code")}, nil, 0)
		req := withMFAClaims(httptest.NewRequest(http.MethodPost, "/api/v1/mfa/setup", bytes.NewBufferString(`{"current_code":"000000"}`)))
		rr := httptest.NewRecorder()
		h.Setup(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("success", func(t *testing.T) {
		h := NewHandler(&mockMFAService{setupOTP: "otpauth://", setupSecret: "ABC", setupCodes: []string{"c1"}}, nil, 0)
		req := withMFAClaims(httptest.NewRequest(http.MethodPost, "/api/v1/mfa/setup", bytes.NewBufferString(`{}`)))
		rr := httptest.NewRecorder()
		h.Setup(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusOK)
		}
	})
}

func TestMFAHandlerVerifyDisableStatus(t *testing.T) {
	t.Run("verify invalid request", func(t *testing.T) {
		h := NewHandler(&mockMFAService{}, nil, 0)
		req := withMFAClaims(httptest.NewRequest(http.MethodPost, "/api/v1/mfa/verify", bytes.NewBufferString(`{}`)))
		rr := httptest.NewRecorder()
		h.Verify(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("verify invalid code", func(t *testing.T) {
		h := NewHandler(&mockMFAService{verifyErr: errors.New("bad")}, nil, 0)
		req := withMFAClaims(httptest.NewRequest(http.MethodPost, "/api/v1/mfa/verify", bytes.NewBufferString(`{"code":"111111"}`)))
		rr := httptest.NewRecorder()
		h.Verify(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("disable invalid code", func(t *testing.T) {
		h := NewHandler(&mockMFAService{disableErr: errors.New("bad")}, nil, 0)
		req := withMFAClaims(httptest.NewRequest(http.MethodPost, "/api/v1/mfa/disable", bytes.NewBufferString(`{"code":"111111"}`)))
		rr := httptest.NewRecorder()
		h.Disable(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("status success", func(t *testing.T) {
		h := NewHandler(&mockMFAService{enabled: true}, nil, 0)
		req := withMFAClaims(httptest.NewRequest(http.MethodGet, "/api/v1/mfa/status", nil))
		rr := httptest.NewRecorder()
		h.Status(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusOK)
		}
	})
}

func TestMFAHandlerStepUpGuards(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		h := NewHandler(&mockMFAService{}, nil, 0)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mfa/step-up", nil)
		rr := httptest.NewRecorder()
		h.StepUp(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
		}
	})

	t.Run("service unavailable without redis", func(t *testing.T) {
		h := NewHandler(&mockMFAService{}, nil, 0)
		req := withMFAClaims(httptest.NewRequest(http.MethodPost, "/api/v1/mfa/step-up", bytes.NewBufferString(`{"code":"111111"}`)))
		rr := httptest.NewRecorder()
		h.StepUp(rr, req)
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusServiceUnavailable)
		}
	})
}
