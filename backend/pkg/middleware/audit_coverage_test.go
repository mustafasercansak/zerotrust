package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// bareRW is a minimal ResponseWriter that implements none of the optional
// streaming interfaces, so combos below add exactly the ones we want to test.
type bareRW struct{ hdr http.Header }

func (b *bareRW) Header() http.Header {
	if b.hdr == nil {
		b.hdr = http.Header{}
	}
	return b.hdr
}
func (b *bareRW) Write(p []byte) (int, error) { return len(p), nil }
func (b *bareRW) WriteHeader(int)             {}

type flusherHijackerRW struct{ *bareRW } // Flusher + Hijacker (no Pusher)
func (flusherHijackerRW) Flush()         {}
func (flusherHijackerRW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

type flusherPusherRW struct{ *bareRW }                       // Flusher + Pusher (no Hijacker)
func (flusherPusherRW) Flush()                               {}
func (flusherPusherRW) Push(string, *http.PushOptions) error { return nil }

type hijackerPusherRW struct{ *bareRW } // Hijacker + Pusher (no Flusher)
func (hijackerPusherRW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}
func (hijackerPusherRW) Push(string, *http.PushOptions) error { return nil }

// TestWrapAuditResponseWriter_TwoInterfaceCombos exercises the wrapper structs
// that expose exactly two of {Flusher, Hijacker, Pusher} and confirms the third
// interface is intentionally not surfaced.
func TestWrapAuditResponseWriter_TwoInterfaceCombos(t *testing.T) {
	t.Run("flusher+hijacker", func(t *testing.T) {
		w := wrapAuditResponseWriter(newAuditResponseWriter(flusherHijackerRW{&bareRW{}}))
		f, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected Flusher")
		}
		f.Flush()
		h, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("expected Hijacker")
		}
		if _, _, err := h.Hijack(); err != nil {
			t.Fatalf("Hijack: %v", err)
		}
		if _, ok := w.(http.Pusher); ok {
			t.Fatal("did not expect Pusher")
		}
	})

	t.Run("flusher+pusher", func(t *testing.T) {
		w := wrapAuditResponseWriter(newAuditResponseWriter(flusherPusherRW{&bareRW{}}))
		w.(http.Flusher).Flush()
		if err := w.(http.Pusher).Push("/x", nil); err != nil {
			t.Fatalf("Push: %v", err)
		}
		if _, ok := w.(http.Hijacker); ok {
			t.Fatal("did not expect Hijacker")
		}
	})

	t.Run("hijacker+pusher", func(t *testing.T) {
		w := wrapAuditResponseWriter(newAuditResponseWriter(hijackerPusherRW{&bareRW{}}))
		if _, _, err := w.(http.Hijacker).Hijack(); err != nil {
			t.Fatalf("Hijack: %v", err)
		}
		if err := w.(http.Pusher).Push("/x", nil); err != nil {
			t.Fatalf("Push: %v", err)
		}
		if _, ok := w.(http.Flusher); ok {
			t.Fatal("did not expect Flusher")
		}
	})
}

func TestAuditMetadata_ClientInfoBranches(t *testing.T) {
	newReq := func(clientInfo string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		if clientInfo != "" {
			r.Header.Set("X-Client-Info", clientInfo)
		}
		return r
	}

	t.Run("no header → base metadata only", func(t *testing.T) {
		m := auditMetadata(newReq(""), http.StatusOK)
		if m["outcome"] != "success" || m["status"] != http.StatusOK {
			t.Fatalf("unexpected base metadata: %v", m)
		}
		if _, ok := m["client_info"]; ok {
			t.Fatal("did not expect client_info without header")
		}
	})

	t.Run("invalid json → no client_info", func(t *testing.T) {
		m := auditMetadata(newReq("{not json"), http.StatusUnauthorized)
		if m["outcome"] != "failure" {
			t.Fatalf("expected failure outcome, got %v", m["outcome"])
		}
		if _, ok := m["client_info"]; ok {
			t.Fatal("did not expect client_info for invalid json")
		}
	})

	t.Run("empty object → no client_info", func(t *testing.T) {
		m := auditMetadata(newReq("{}"), http.StatusOK)
		if _, ok := m["client_info"]; ok {
			t.Fatal("did not expect client_info for empty object")
		}
	})

	t.Run("oversized/empty fields filtered, valid kept", func(t *testing.T) {
		longKey := make([]byte, 41)
		for i := range longKey {
			longKey[i] = 'k'
		}
		ci := `{"os":"linux","":"x","empty":"","` + string(longKey) + `":"v"}`
		m := auditMetadata(newReq(ci), http.StatusOK)
		clean, ok := m["client_info"].(map[string]string)
		if !ok {
			t.Fatalf("expected client_info map, got %T", m["client_info"])
		}
		if clean["os"] != "linux" {
			t.Fatalf("expected os=linux kept, got %v", clean)
		}
		if len(clean) != 1 {
			t.Fatalf("expected only the valid field, got %v", clean)
		}
	})
}

// TestAuditFailures_LogErrorBranches covers the logAuditFailure path when the
// audit write itself fails, on both the synchronous (critical route) and
// asynchronous (non-critical route) code paths.
func TestAuditFailures_LogErrorBranches(t *testing.T) {
	t.Run("critical route, synchronous write error", func(t *testing.T) {
		logger := newRecordingAuditLogger(errors.New("write failed"))
		h := AuditAuthFailures(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		// PATCH /admin/users/{id}/roles is a critical route → synchronous logging.
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/roles", nil))
		if logger.count() != 1 {
			t.Fatalf("expected 1 audit entry, got %d", logger.count())
		}
	})

	t.Run("non-critical route, async write error", func(t *testing.T) {
		logger := newRecordingAuditLogger(errors.New("write failed"))
		h := AuditAuthFailures(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		// GET /users is non-critical → the write runs in a goroutine.
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
		logger.wait(t) // block until the async write (and logAuditFailure) ran
	})
}

func TestAuditCSRFFailures_LogErrorBranch(t *testing.T) {
	logger := newRecordingAuditLogger(errors.New("write failed"))
	h := AuditCSRFFailures(logger)(CSRF()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/roles", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "x"}) // session present → CSRF enforced
	h.ServeHTTP(httptest.NewRecorder(), req)
	if logger.count() != 1 {
		t.Fatalf("expected 1 audit entry, got %d", logger.count())
	}
}

// TestAuditResponseWriter_WriteHeaderBranches covers Write both with and without
// a prior explicit WriteHeader.
func TestAuditResponseWriter_WriteHeaderBranches(t *testing.T) {
	// Write without a prior WriteHeader → implicit 200.
	w1 := newAuditResponseWriter(httptest.NewRecorder())
	if _, err := w1.Write([]byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if w1.status != http.StatusOK {
		t.Fatalf("expected implicit 200, got %d", w1.status)
	}

	// Write after an explicit WriteHeader → status preserved, no re-write.
	w2 := newAuditResponseWriter(httptest.NewRecorder())
	w2.WriteHeader(http.StatusCreated)
	if _, err := w2.Write([]byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if w2.status != http.StatusCreated {
		t.Fatalf("expected 201 preserved, got %d", w2.status)
	}
}
