package oidc

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/user"
)

var (
	ErrInvalidRedirectURI = errors.New("invalid_redirect_uri")
	ErrInvalidScope       = errors.New("invalid_scope")
	ErrInvalidGrant       = errors.New("invalid_grant")
	ErrCodeVerifierFailed = errors.New("code_verifier_verification_failed")
)

type Service struct {
	clientRepo   *ClientRepository
	codeStore    *AuthCodeStore
	refreshStore *RefreshTokenStore
	userSvc      *user.Service
	ks           *auth.KeyStore
	issuer       string
}

func NewService(clientRepo *ClientRepository, codeStore *AuthCodeStore, userSvc *user.Service, ks *auth.KeyStore, issuer string, refreshStore *RefreshTokenStore) *Service {
	return &Service{
		clientRepo:   clientRepo,
		codeStore:    codeStore,
		refreshStore: refreshStore,
		userSvc:      userSvc,
		ks:           ks,
		issuer:       issuer,
	}
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

// CreateAuthCodeSession initializes and saves an authorization code session
func (s *Service) CreateAuthCodeSession(ctx context.Context, userID, clientID, redirectURI string, scopes []string, codeChallenge, codeChallengeMethod, nonce string) (string, error) {
	client, err := s.clientRepo.FindByClientID(ctx, clientID)
	if err != nil {
		return "", err
	}

	if !client.ValidateRedirectURI(redirectURI) {
		return "", ErrInvalidRedirectURI
	}

	if !client.ValidateScope(scopes) {
		return "", ErrInvalidScope
	}

	code := uuid.NewString()
	session := &AuthCodeSession{
		Code:                code,
		UserID:              userID,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		Scopes:              scopes,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Nonce:               nonce,
		AuthTime:            time.Now(),
	}

	if err := s.codeStore.Save(ctx, session); err != nil {
		return "", err
	}

	return code, nil
}

// ExchangeCode exchanges an authorization code for access and ID tokens, validating client/PKCE credentials.
func (s *Service) ExchangeCode(ctx context.Context, code, clientID, clientSecret, redirectURI, codeVerifier string) (*TokenResponse, error) {
	session, err := s.codeStore.GetAndConsume(ctx, code)
	if err != nil {
		return nil, ErrInvalidGrant
	}

	if session.ClientID != clientID {
		return nil, ErrInvalidGrant
	}

	if session.RedirectURI != redirectURI {
		return nil, ErrInvalidRedirectURI
	}

	// Validate Client Authentication
	// Public clients (which use PKCE) don't have client_secret
	if clientSecret != "" {
		_, err = s.clientRepo.AuthenticateClient(ctx, clientID, clientSecret)
		if err != nil {
			return nil, ErrInvalidGrant
		}
	} else if session.CodeChallenge == "" {
		// If no client secret is supplied, PKCE challenge must be present.
		return nil, ErrInvalidGrant
	}

	// Validate PKCE if challenge was set
	if session.CodeChallenge != "" {
		if err := verifyPKCE(session.CodeChallenge, session.CodeChallengeMethod, codeVerifier); err != nil {
			return nil, ErrCodeVerifierFailed
		}
	}

	u, err := s.userSvc.FindByID(ctx, session.UserID)
	if err != nil {
		return nil, ErrInvalidGrant
	}

	// Generate standard OAuth2 Access Token
	now := time.Now()
	accessTTL := 1 * time.Hour

	accessClaims := auth.Claims{
		UserID:      u.ID,
		Email:       u.Email,
		Locale:      u.Locale,
		Roles:       u.Roles,
		Permissions: []string{}, // intentionally empty: OIDC access tokens are issued to external clients and must not carry internal RBAC permissions
		SubType:     auth.SubTypeUser,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   u.ID,
			Audience:  jwt.ClaimStrings{clientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}

	accessTokenStr, err := s.signClaims(accessClaims)
	if err != nil {
		return nil, err
	}

	// Generate OIDC ID Token if "openid" scope was requested
	var idTokenStr string
	hasOpenID := false
	for _, scope := range session.Scopes {
		if scope == "openid" {
			hasOpenID = true
			break
		}
	}

	if hasOpenID {
		idClaims := jwt.MapClaims{
			"iss":       s.issuer,
			"sub":       u.ID,
			"aud":       clientID,
			"exp":       now.Add(accessTTL).Unix(),
			"iat":       now.Unix(),
			"auth_time": session.AuthTime.Unix(),
		}

		if session.Nonce != "" {
			idClaims["nonce"] = session.Nonce
		}

		// Include optional scope info
		for _, scope := range session.Scopes {
			switch scope {
			case "profile":
				idClaims["name"] = strings.TrimSpace(u.FirstName + " " + u.LastName)
				idClaims["given_name"] = u.FirstName
				idClaims["family_name"] = u.LastName
				idClaims["locale"] = u.Locale
				idClaims["roles"] = u.Roles
				idClaims["groups"] = u.Roles
			case "email":
				idClaims["email"] = u.Email
				idClaims["email_verified"] = true
			}
		}

		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, idClaims)
		token.Header["kid"] = s.ks.PrimaryKID()
		idTokenStr, err = token.SignedString(s.ks.PrimaryKey())
		if err != nil {
			return nil, err
		}
	}

	resp := &TokenResponse{
		AccessToken: accessTokenStr,
		IDToken:     idTokenStr,
		TokenType:   "Bearer",
		ExpiresIn:   int64(accessTTL.Seconds()),
		Scope:       strings.Join(session.Scopes, " "),
	}

	if s.refreshStore != nil && hasScope(session.Scopes, "offline_access") {
		rt, rtErr := s.refreshStore.Save(ctx, &OIDCRefreshSession{
			UserID:   u.ID,
			ClientID: clientID,
			Scopes:   session.Scopes,
			AuthTime: session.AuthTime,
		})
		if rtErr != nil {
			return nil, rtErr
		}
		resp.RefreshToken = rt
	}

	return resp, nil
}

// ExchangeRefreshToken exchanges a refresh token for a new access token and
// rotated refresh token. Scope may be nil to reuse the original grant scopes,
// or a non-empty subset to downscope.
func (s *Service) ExchangeRefreshToken(ctx context.Context, refreshToken, clientID, clientSecret string, requestedScopes []string) (*TokenResponse, error) {
	if s.refreshStore == nil {
		return nil, errors.New("refresh_token_not_supported")
	}

	sess, err := s.refreshStore.GetAndConsume(ctx, refreshToken)
	if err != nil {
		return nil, ErrInvalidGrant
	}

	if sess.ClientID != clientID {
		return nil, ErrInvalidGrant
	}

	if clientSecret != "" {
		if _, err := s.clientRepo.AuthenticateClient(ctx, clientID, clientSecret); err != nil {
			return nil, ErrInvalidGrant
		}
	}

	// Use requested scopes if they are a subset of the original grant; otherwise
	// fall back to the original scopes.
	scopes := sess.Scopes
	if len(requestedScopes) > 0 {
		allowed := make(map[string]bool, len(sess.Scopes))
		for _, s := range sess.Scopes {
			allowed[s] = true
		}
		valid := make([]string, 0, len(requestedScopes))
		for _, rs := range requestedScopes {
			if allowed[rs] {
				valid = append(valid, rs)
			}
		}
		if len(valid) > 0 {
			scopes = valid
		}
	}

	u, err := s.userSvc.FindByID(ctx, sess.UserID)
	if err != nil {
		return nil, ErrInvalidGrant
	}

	now := time.Now()
	accessTTL := 1 * time.Hour
	accessClaims := auth.Claims{
		UserID:      u.ID,
		Email:       u.Email,
		Locale:      u.Locale,
		Roles:       u.Roles,
		Permissions: []string{},
		SubType:     auth.SubTypeUser,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   u.ID,
			Audience:  jwt.ClaimStrings{clientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}
	accessTokenStr, err := s.signClaims(accessClaims)
	if err != nil {
		return nil, err
	}

	newRT, err := s.refreshStore.Save(ctx, &OIDCRefreshSession{
		UserID:   u.ID,
		ClientID: clientID,
		Scopes:   scopes,
		AuthTime: sess.AuthTime,
	})
	if err != nil {
		return nil, err
	}

	return &TokenResponse{
		AccessToken:  accessTokenStr,
		RefreshToken: newRT,
		TokenType:    "Bearer",
		ExpiresIn:    int64(accessTTL.Seconds()),
		Scope:        strings.Join(scopes, " "),
	}, nil
}

func (s *Service) signClaims(claims auth.Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = s.ks.PrimaryKID()
	return token.SignedString(s.ks.PrimaryKey())
}

func verifyPKCE(challenge, method, verifier string) error {
	// RFC 7636 §4.1: verifier must be 43–128 unreserved ASCII chars.
	if l := len(verifier); l < 43 || l > 128 {
		return fmt.Errorf("code_verifier length %d not in 43–128 range", l)
	}
	for _, c := range verifier {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~') {
			return fmt.Errorf("code_verifier contains invalid character %q", c)
		}
	}

	switch method {
	case "S256", "":
		h := sha256.New()
		h.Write([]byte(verifier))
		hash := h.Sum(nil)
		computedChallenge := base64.RawURLEncoding.EncodeToString(hash)
		if subtle.ConstantTimeCompare([]byte(computedChallenge), []byte(challenge)) == 1 {
			return nil
		}
	default:
		return fmt.Errorf("unsupported code challenge method: %s", method)
	}

	return ErrCodeVerifierFailed
}

func hasScope(scopes []string, target string) bool {
	for _, s := range scopes {
		if s == target {
			return true
		}
	}
	return false
}

// RevokeRefreshToken deletes an OIDC refresh token from the store so it cannot
// be exchanged again. Per RFC 7009 the caller always receives 200; errors are
// swallowed here.
func (s *Service) RevokeRefreshToken(ctx context.Context, token string) {
	if s.refreshStore == nil {
		return
	}
	// GetAndConsume atomically deletes; we discard the returned session.
	s.refreshStore.GetAndConsume(ctx, token) //nolint:errcheck
}
