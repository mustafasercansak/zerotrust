package auth

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func tokenRequestWithDPoP(t *testing.T) *http.Request {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	proof, err := GenerateDPoPProofForTest(priv, "POST", "/api/v1/auth/token")
	if err != nil {
		t.Fatalf("generate proof: %v", err)
	}
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     "c1",
		"client_secret": "s1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", bytes.NewReader(body))
	req.Header.Set("DPoP", proof)
	return req
}

func TestToken_RejectsReplayedDPoPProof(t *testing.T) {
	svc := &fakeAuthService{
		clientCredentials: &ServiceTokenResponse{AccessToken: "tok", ExpiresIn: 300},
		dpopErr:           ErrDPoPReplay,
	}
	// nil auditRepo so the handler does not block waiting on an audit write.
	h := NewHandler(svc, nil, nil, false, false, nil, "", nil)

	rr := httptest.NewRecorder()
	h.Token(rr, tokenRequestWithDPoP(t))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on replayed DPoP proof, got %d (%s)", rr.Code, rr.Body.String())
	}
	if len(svc.dpopJTIs) != 1 || svc.dpopJTIs[0] == "" {
		t.Fatalf("expected ConsumeDPoPProof called once with a non-empty jti, got %v", svc.dpopJTIs)
	}
}

func TestToken_AcceptsFreshDPoPProof(t *testing.T) {
	svc := &fakeAuthService{
		clientCredentials: &ServiceTokenResponse{AccessToken: "tok", ExpiresIn: 300},
	}
	h := NewHandler(svc, nil, nil, false, false, nil, "", nil)

	rr := httptest.NewRecorder()
	h.Token(rr, tokenRequestWithDPoP(t))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for fresh DPoP proof, got %d (%s)", rr.Code, rr.Body.String())
	}
	if len(svc.dpopJTIs) != 1 {
		t.Fatalf("expected ConsumeDPoPProof to be called once, got %d", len(svc.dpopJTIs))
	}
}
