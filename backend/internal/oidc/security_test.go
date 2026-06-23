package oidc

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/user"
)

// TestUserInfo_ScopeFiltering verifies that the UserInfo endpoint only returns
// claims that correspond to the scopes carried in the OIDC access token
// (OIDC Core §5.3).
func TestUserInfo_ScopeFiltering(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, err := auth.LoadOrGenerateKeyStore("", "")
	if err != nil {
		t.Fatalf("keystore: %v", err)
	}

	u := &user.User{
		ID: "u-scope-1", Email: "scope@example.com",
		FirstName: "Alice", LastName: "Smith", Locale: "en",
		Roles: []string{"user"}, IsActive: true,
	}
	userSvc := user.NewService(&mockUserReader{user: u})
	codeStore := NewAuthCodeStore(rdb)
	refreshStore := NewRefreshTokenStore(rdb)
	svc := NewService(nil, codeStore, userSvc, ks, "https://issuer.example.com", refreshStore)

	issueToken := func(scopes []string) string {
		t.Helper()
		sess := &AuthCodeSession{
			Code: "code-" + strings.Join(scopes, "-"), UserID: u.ID, ClientID: "test-client",
			RedirectURI:         "http://localhost/cb",
			Scopes:              scopes,
			CodeChallenge:       "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
			CodeChallengeMethod: "S256",
			AuthTime:            time.Now(),
		}
		if err := codeStore.Save(context.Background(), sess); err != nil {
			t.Fatalf("save session: %v", err)
		}
		resp, err := svc.ExchangeCode(context.Background(), sess.Code, "test-client", "", "http://localhost/cb", "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
		if err != nil {
			t.Fatalf("exchange: %v", err)
		}
		return resp.AccessToken
	}

	authSvc := auth.NewService(userSvc, &mockSessionRepo{}, &testServiceAccountStore{}, rdb, ks, nil, nil)
	h := &Handler{svc: svc, userSvc: userSvc, authSvc: authSvc, ks: ks, issuer: "https://issuer.example.com", publicAppURL: "http://localhost:3000"}

	callUserInfo := func(token string) map[string]any {
		t.Helper()
		req, _ := http.NewRequest("GET", "/oauth2/userinfo", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		h.UserInfo(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("userinfo: expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		var out map[string]any
		json.NewDecoder(rr.Body).Decode(&out)
		return out
	}

	// openid only — no email, no profile claims
	t.Run("openid_only", func(t *testing.T) {
		token := issueToken([]string{"openid"})
		out := callUserInfo(token)
		if out["sub"] == nil {
			t.Error("sub must always be present")
		}
		if out["email"] != nil {
			t.Errorf("email must be absent for openid-only scope, got %v", out["email"])
		}
		if out["name"] != nil {
			t.Errorf("name must be absent for openid-only scope, got %v", out["name"])
		}
	})

	// openid + email — email present, no profile claims
	t.Run("openid_email", func(t *testing.T) {
		token := issueToken([]string{"openid", "email"})
		out := callUserInfo(token)
		if out["sub"] == nil {
			t.Error("sub must always be present")
		}
		if out["email"] == nil {
			t.Error("email must be present for email scope")
		}
		if out["name"] != nil {
			t.Errorf("name must be absent without profile scope, got %v", out["name"])
		}
	})

	// openid + profile — name/locale present, no email
	t.Run("openid_profile", func(t *testing.T) {
		token := issueToken([]string{"openid", "profile"})
		out := callUserInfo(token)
		if out["sub"] == nil {
			t.Error("sub must always be present")
		}
		if out["name"] == nil {
			t.Error("name must be present for profile scope")
		}
		if out["email"] != nil {
			t.Errorf("email must be absent without email scope, got %v", out["email"])
		}
	})

	// openid + email + profile — all claims present
	t.Run("openid_email_profile", func(t *testing.T) {
		token := issueToken([]string{"openid", "email", "profile"})
		out := callUserInfo(token)
		if out["sub"] == nil {
			t.Error("sub must always be present")
		}
		if out["email"] == nil {
			t.Error("email must be present")
		}
		if out["name"] == nil {
			t.Error("name must be present")
		}
	})
}

// TestToken_PragmaHeader verifies that the token endpoint includes both
// Cache-Control: no-store and Pragma: no-cache per RFC 6749 §5.1.
func TestToken_PragmaHeader(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, _ := auth.LoadOrGenerateKeyStore("", "")
	u := &user.User{ID: "u-pragma", Email: "pragma@example.com", Locale: "en", IsActive: true}
	userSvc := user.NewService(&mockUserReader{user: u})

	codeStore := NewAuthCodeStore(rdb)
	svc := NewService(nil, codeStore, userSvc, ks, "https://issuer.example.com", nil)
	h := &Handler{svc: svc, clientRepo: &stubClientRepo{redirectURIs: []string{"http://localhost/cb"}}, userSvc: userSvc, ks: ks, issuer: "https://issuer.example.com"}

	// Seed a code session so the exchange succeeds.
	sess := &AuthCodeSession{
		Code: "code-pragma", UserID: u.ID, ClientID: "stub-client",
		RedirectURI:         "http://localhost/cb",
		Scopes:              []string{"openid"},
		CodeChallenge:       "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		CodeChallengeMethod: "S256",
		AuthTime:            time.Now(),
	}
	if err := codeStore.Save(context.Background(), sess); err != nil {
		t.Fatalf("save: %v", err)
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", "code-pragma")
	form.Set("client_id", "stub-client")
	form.Set("redirect_uri", "http://localhost/cb")
	form.Set("code_verifier", "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")

	req, _ := http.NewRequest("POST", "/oauth2/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Token(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if p := rr.Header().Get("Pragma"); p != "no-cache" {
		t.Errorf("Pragma = %q, want no-cache", p)
	}
}

// TestConsent_ServerErrorJSON verifies that a server error in the Consent
// handler returns valid, properly escaped JSON even when err.Error() contains
// special characters such as quotes or backslashes.
func TestConsent_ServerErrorJSON(t *testing.T) {
	ks, _ := auth.LoadOrGenerateKeyStore("", "")

	// Use a clientStore stub that returns an error with special characters to
	// confirm the JSON is properly escaped.
	h := &Handler{
		ks: ks,
		// svc is nil — CreateAuthCodeSession will never be reached because
		// clientRepo.FindByClientID will succeed but ValidateScope will fail.
		clientRepo: &scopeFailClientRepo{},
	}

	tokenPair, _ := auth.GenerateTokenPair(ks, "user-1", "u@example.com", "en", nil, nil, time.Hour)
	cookie := &http.Cookie{Name: "access_token", Value: tokenPair.AccessToken}

	// Request a scope that the stub will reject.
	body, _ := json.Marshal(ConsentRequest{
		ClientID:    "client-1",
		RedirectURI: "http://localhost/cb",
		Scopes:      []string{"bad\"scope"},
		Approved:    true,
	})
	req, _ := http.NewRequest("POST", "/oauth2/consent", bytes.NewBuffer(body))
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	h.Consent(rr, req)

	// Expect a 4xx/5xx, but the body must be valid JSON in all cases.
	var out map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Errorf("response body is not valid JSON: %v — body: %s", err, rr.Body.String())
	}
}

// TestConsent_DenialOpenRedirect verifies that the consent denial path validates
// redirect_uri against the registered client URIs before using it in the response.
// Without this, a caller could supply an arbitrary redirect_uri and turn the
// denial response into an open redirect.
func TestConsent_DenialOpenRedirect(t *testing.T) {
	ks, _ := auth.LoadOrGenerateKeyStore("", "")
	tokenPair, _ := auth.GenerateTokenPair(ks, "user-1", "u@example.com", "en", nil, nil, time.Hour)
	cookie := &http.Cookie{Name: "access_token", Value: tokenPair.AccessToken}

	h := &Handler{
		ks:         ks,
		clientRepo: &stubClientRepo{redirectURIs: []string{"http://registered.example.com/cb"}},
	}

	// Denial with an unregistered redirect URI must be rejected.
	t.Run("unregistered_uri_rejected", func(t *testing.T) {
		body, _ := json.Marshal(ConsentRequest{
			ClientID:    "client-1",
			RedirectURI: "https://evil.com/steal",
			Approved:    false,
		})
		req, _ := http.NewRequest("POST", "/oauth2/consent", bytes.NewBuffer(body))
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()
		h.Consent(rr, req)
		if rr.Code == http.StatusOK {
			t.Errorf("expected non-200 for unregistered redirect_uri on denial, got 200 body=%s", rr.Body.String())
		}
	})

	// Denial with a registered redirect URI must succeed and include the error param.
	t.Run("registered_uri_returns_access_denied", func(t *testing.T) {
		body, _ := json.Marshal(ConsentRequest{
			ClientID:    "client-1",
			RedirectURI: "http://registered.example.com/cb",
			State:       "mystate",
			Approved:    false,
		})
		req, _ := http.NewRequest("POST", "/oauth2/consent", bytes.NewBuffer(body))
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()
		h.Consent(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
		}
		var out map[string]string
		json.NewDecoder(rr.Body).Decode(&out)
		redirectURL := out["redirect_url"]
		if !strings.Contains(redirectURL, "error=access_denied") {
			t.Errorf("expected error=access_denied in redirect_url, got %q", redirectURL)
		}
		if !strings.Contains(redirectURL, "registered.example.com") {
			t.Errorf("expected registered host in redirect_url, got %q", redirectURL)
		}
		if !strings.Contains(redirectURL, "state=mystate") {
			t.Errorf("expected state in redirect_url, got %q", redirectURL)
		}
	})
}

// scopeFailClientRepo is a clientStore stub whose ValidateScope always returns false.
type scopeFailClientRepo struct{ stubClientRepo }

func (s *scopeFailClientRepo) FindByClientID(_ context.Context, clientID string) (*Client, error) {
	return &Client{
		ClientID:      clientID,
		AllowedScopes: []string{"openid"},
		RedirectURIs:  []string{"http://localhost/cb"},
	}, nil
}

// TestUserInfo_ScopeInAccessToken verifies that the scope granted at code
// exchange is embedded in the access token and survives a round-trip.
func TestUserInfo_ScopeInAccessToken(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, _ := auth.LoadOrGenerateKeyStore("", "")
	u := &user.User{ID: "u-rt-scope", Email: "rt@example.com", Locale: "en", FirstName: "Bob", Roles: []string{"user"}, IsActive: true}
	userSvc := user.NewService(&mockUserReader{user: u})

	codeStore := NewAuthCodeStore(rdb)
	refreshStore := NewRefreshTokenStore(rdb)
	svc := NewService(nil, codeStore, userSvc, ks, "https://issuer.example.com", refreshStore)

	// Issue initial token with openid+offline_access only.
	sess := &AuthCodeSession{
		Code: "code-rt-scope", UserID: u.ID, ClientID: "client-1",
		RedirectURI:         "http://localhost/cb",
		Scopes:              []string{"openid", "offline_access"},
		CodeChallenge:       "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		CodeChallengeMethod: "S256",
		AuthTime:            time.Now(),
	}
	codeStore.Save(context.Background(), sess)

	firstResp, err := svc.ExchangeCode(context.Background(), "code-rt-scope", "client-1", "", "http://localhost/cb", "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}

	// After refresh, scope must be preserved.
	secondResp, err := svc.ExchangeRefreshToken(context.Background(), firstResp.RefreshToken, "client-1", "", nil)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	authSvc := auth.NewService(userSvc, &mockSessionRepo{}, &testServiceAccountStore{}, rdb, ks, nil, nil)
	h := &Handler{svc: svc, userSvc: userSvc, authSvc: authSvc, ks: ks, issuer: "https://issuer.example.com"}

	req, _ := http.NewRequest("GET", "/oauth2/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+secondResp.AccessToken)
	rr := httptest.NewRecorder()
	h.UserInfo(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("userinfo after refresh: expected 200, got %d", rr.Code)
	}
	var out map[string]any
	json.NewDecoder(rr.Body).Decode(&out)

	// openid+offline_access only — no email, no profile
	if out["email"] != nil {
		t.Errorf("email must be absent for openid-only token after refresh, got %v", out["email"])
	}
	if out["name"] != nil {
		t.Errorf("name must be absent for openid-only token after refresh, got %v", out["name"])
	}
	if out["sub"] == nil {
		t.Error("sub must always be present")
	}
}
