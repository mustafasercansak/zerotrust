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
	clientRepo *ClientRepository
	codeStore  *AuthCodeStore
	userSvc    *user.Service
	ks         *auth.KeyStore
	issuer     string
}

func NewService(clientRepo *ClientRepository, codeStore *AuthCodeStore, userSvc *user.Service, ks *auth.KeyStore, issuer string) *Service {
	return &Service{
		clientRepo: clientRepo,
		codeStore:  codeStore,
		userSvc:    userSvc,
		ks:         ks,
		issuer:     issuer,
	}
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
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
		Permissions: []string{}, // Add permissions if needed
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

	return &TokenResponse{
		AccessToken: accessTokenStr,
		IDToken:     idTokenStr,
		TokenType:   "Bearer",
		ExpiresIn:   int64(accessTTL.Seconds()),
		Scope:       strings.Join(session.Scopes, " "),
	}, nil
}

func (s *Service) signClaims(claims auth.Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = s.ks.PrimaryKID()
	return token.SignedString(s.ks.PrimaryKey())
}

func verifyPKCE(challenge, method, verifier string) error {
	if verifier == "" {
		return errors.New("empty verifier")
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
	case "plain":
		if subtle.ConstantTimeCompare([]byte(verifier), []byte(challenge)) == 1 {
			return nil
		}
	default:
		return fmt.Errorf("unsupported code challenge method: %s", method)
	}

	return ErrCodeVerifierFailed
}
