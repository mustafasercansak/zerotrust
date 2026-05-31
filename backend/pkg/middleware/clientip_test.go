package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseCIDRs(t *testing.T) {
	cidrs := ParseCIDRs("127.0.0.1/32, invalid, 10.0.0.0/8,,")
	if len(cidrs) != 2 {
		t.Errorf("expected 2 parsed cidrs, got %d", len(cidrs))
	}
}

func TestTrustedClientIP(t *testing.T) {
	cidrs := ParseCIDRs("10.0.0.0/8")

	handler := TrustedClientIP(cidrs)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r)
		w.Write([]byte(ip))
	}))

	// Not trusted proxy
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	req.Header.Set("X-Real-IP", "1.2.3.4")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Body.String() != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", rr.Body.String())
	}

	// Trusted proxy
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "10.0.0.1:1234"
	req2.Header.Set("X-Real-IP", "1.2.3.4")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Body.String() != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4, got %s", rr2.Body.String())
	}

	// Invalid remote addr without port
	req3 := httptest.NewRequest("GET", "/", nil)
	req3.RemoteAddr = "invalid_addr"
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Body.String() != "invalid_addr" {
		t.Errorf("expected invalid_addr, got %s", rr3.Body.String())
	}

	// Empty trusted CIDRs
	handlerEmpty := TrustedClientIP(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(ClientIP(r)))
	}))
	req4 := httptest.NewRequest("GET", "/", nil)
	req4.RemoteAddr = "10.0.0.1:1234"
	req4.Header.Set("X-Real-IP", "1.2.3.4")
	rr4 := httptest.NewRecorder()
	handlerEmpty.ServeHTTP(rr4, req4)
	if rr4.Body.String() != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", rr4.Body.String())
	}
}
