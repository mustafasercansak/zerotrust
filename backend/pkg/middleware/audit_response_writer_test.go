package middleware

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockResponseWriterFull struct {
	*httptest.ResponseRecorder
}

func (m *mockResponseWriterFull) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(m.ResponseRecorder, r)
}

func (m *mockResponseWriterFull) Flush() {
	m.ResponseRecorder.Flush()
}

func (m *mockResponseWriterFull) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

func (m *mockResponseWriterFull) Push(target string, opts *http.PushOptions) error {
	return nil
}

func TestAuditResponseWriter_Unwrap(t *testing.T) {
	rr := httptest.NewRecorder()
	w := newAuditResponseWriter(rr)
	
	if w.Unwrap() != rr {
		t.Error("Unwrap did not return the original ResponseWriter")
	}
}

func TestAuditResponseWriter_ReadFrom(t *testing.T) {
	rr := &mockResponseWriterFull{ResponseRecorder: httptest.NewRecorder()}
	w := newAuditResponseWriter(rr)
	
	reader := bytes.NewReader([]byte("test"))
	
	n, err := w.ReadFrom(reader)
	if err != nil {
		t.Errorf("ReadFrom error: %v", err)
	}
	if n != 4 {
		t.Errorf("expected to read 4 bytes, got %d", n)
	}

	if !w.wroteHeader {
		t.Error("expected wroteHeader to be true after ReadFrom")
	}
}

func TestAuditResponseWriter_ReadFromFallback(t *testing.T) {
	rr := httptest.NewRecorder()
	w := &auditResponseWriter{ResponseWriter: rr, status: http.StatusOK}
	
	reader := bytes.NewReader([]byte("fallback"))
	n, err := w.ReadFrom(reader)
	if err != nil {
		t.Errorf("ReadFrom fallback error: %v", err)
	}
	if n != 8 {
		t.Errorf("expected to read 8 bytes, got %d", n)
	}
}

func TestAuditResponseWriter_Flush(t *testing.T) {
	rr := &mockResponseWriterFull{ResponseRecorder: httptest.NewRecorder()}
	arw := newAuditResponseWriter(rr)
	w := wrapAuditResponseWriter(arw)
	
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	} else {
		t.Error("expected wrapper to implement http.Flusher")
	}
}

func TestAuditResponseWriter_Hijack(t *testing.T) {
	rr := &mockResponseWriterFull{ResponseRecorder: httptest.NewRecorder()}
	arw := newAuditResponseWriter(rr)
	w := wrapAuditResponseWriter(arw)
	
	if h, ok := w.(http.Hijacker); ok {
		_, _, err := h.Hijack()
		if err != nil {
			t.Errorf("Hijack error: %v", err)
		}
	} else {
		t.Error("expected wrapper to implement http.Hijacker")
	}
}

func TestAuditResponseWriter_Push(t *testing.T) {
	rr := &mockResponseWriterFull{ResponseRecorder: httptest.NewRecorder()}
	arw := newAuditResponseWriter(rr)
	w := wrapAuditResponseWriter(arw)
	
	if p, ok := w.(http.Pusher); ok {
		err := p.Push("/target", nil)
		if err != nil {
			t.Errorf("Push error: %v", err)
		}
	} else {
		t.Error("expected wrapper to implement http.Pusher")
	}
}

func TestAuditResponseWriter_Hijack_Error(t *testing.T) {
	rr := httptest.NewRecorder()
	w := auditHijacker{auditResponseWriter: &auditResponseWriter{ResponseWriter: rr}}
	
	_, _, err := w.Hijack()
	if err == nil || err.Error() != "response writer does not support hijacking" {
		t.Errorf("expected hijack error, got %v", err)
	}
}

func TestAuditResponseWriter_Push_Error(t *testing.T) {
	rr := httptest.NewRecorder()
	w := auditPusher{auditResponseWriter: &auditResponseWriter{ResponseWriter: rr}}
	
	err := w.Push("/target", nil)
	if err == nil || err.Error() != "feature not supported" {
		t.Errorf("expected push error, got %v", err)
	}
}
