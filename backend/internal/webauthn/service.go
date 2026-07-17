// Package webauthn implements FIDO2/WebAuthn passkeys as a phishing-resistant
// second factor alongside TOTP. It wraps github.com/go-webauthn/webauthn,
// persists credentials via a Repository, and keeps in-flight ceremony state in
// Redis (single-use, short-lived).
package webauthn

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	gowebauthn "github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// SettingReader provides cached access to system settings. nil disables the
// feature and the caller falls back to false.
type SettingReader interface {
	GetBool(ctx context.Context, key string, defaultVal bool) bool
}

var (
	ErrNoCredentials               = errors.New("no_webauthn_credentials")
	ErrSessionNotFound             = errors.New("webauthn_session_not_found")
	ErrCredentialInUse             = errors.New("webauthn_credential_already_registered")
	ErrHardwareAttestationRequired = errors.New("hardware_attestation_required")
)

// ceremonySessionTTL bounds how long a begun registration/login ceremony can be
// finished. Ceremonies are single-use (consumed on finish).
const ceremonySessionTTL = 5 * time.Minute

// store is the persistence surface consumed by Service (*Repository satisfies it).
type store interface {
	Insert(ctx context.Context, userID, credentialID string, data []byte, signCount int64, name string) error
	ListData(ctx context.Context, userID string) ([][]byte, error)
	ListMeta(ctx context.Context, userID string) ([]CredentialMeta, error)
	Count(ctx context.Context, userID string) (int, error)
	UpdateOnLogin(ctx context.Context, credentialID string, data []byte, signCount int64) error
	Delete(ctx context.Context, id, userID string) error
	Rename(ctx context.Context, id, userID, name string) error
	CredentialExists(ctx context.Context, credentialID string) (bool, error)
}

// Config holds the WebAuthn Relying Party settings.
type Config struct {
	RPID          string   // effective domain, e.g. "localhost" or "auth.example.com"
	RPDisplayName string   // human-facing RP name
	RPOrigins     []string // allowed origins, e.g. "http://localhost:3000"
}

type Service struct {
	repo     store
	rdb      *redis.Client
	wa       *gowebauthn.WebAuthn
	settings SettingReader
}

func NewService(repo store, rdb *redis.Client, cfg Config, settings SettingReader) (*Service, error) {
	wa, err := gowebauthn.New(&gowebauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: cfg.RPDisplayName,
		RPOrigins:     cfg.RPOrigins,
	})
	if err != nil {
		return nil, err
	}
	return &Service{repo: repo, rdb: rdb, wa: wa, settings: settings}, nil
}

// waUser adapts our user data to the go-webauthn User interface.
type waUser struct {
	id          []byte
	name        string
	displayName string
	creds       []gowebauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                           { return u.id }
func (u *waUser) WebAuthnName() string                         { return u.name }
func (u *waUser) WebAuthnDisplayName() string                  { return u.displayName }
func (u *waUser) WebAuthnCredentials() []gowebauthn.Credential { return u.creds }

func (s *Service) buildUser(ctx context.Context, userID, name, displayName string) (*waUser, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	blobs, err := s.repo.ListData(ctx, userID)
	if err != nil {
		return nil, err
	}
	creds := make([]gowebauthn.Credential, 0, len(blobs))
	for _, b := range blobs {
		var c gowebauthn.Credential
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	handle := make([]byte, 16)
	copy(handle, uid[:])
	if displayName == "" {
		displayName = name
	}
	return &waUser{id: handle, name: name, displayName: displayName, creds: creds}, nil
}

// HasCredentials reports whether the user has at least one passkey registered.
func (s *Service) HasCredentials(ctx context.Context, userID string) bool {
	n, err := s.repo.Count(ctx, userID)
	return err == nil && n > 0
}

// ListCredentials returns the user's passkeys for display/management.
func (s *Service) ListCredentials(ctx context.Context, userID string) ([]CredentialMeta, error) {
	return s.repo.ListMeta(ctx, userID)
}

// DeleteCredential removes one of the user's passkeys by row id.
func (s *Service) DeleteCredential(ctx context.Context, id, userID string) error {
	return s.repo.Delete(ctx, id, userID)
}

// RenameCredential updates the user-friendly name of one of the user's passkeys.
func (s *Service) RenameCredential(ctx context.Context, id, userID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name_cannot_be_empty")
	}
	if len([]rune(name)) > 100 {
		name = string([]rune(name)[:100])
	}
	return s.repo.Rename(ctx, id, userID, name)
}

// BeginRegistration starts a passkey registration ceremony and returns the
// CredentialCreation options (JSON) for navigator.credentials.create().
func (s *Service) BeginRegistration(ctx context.Context, userID, name, displayName string) (json.RawMessage, error) {
	user, err := s.buildUser(ctx, userID, name, displayName)
	if err != nil {
		return nil, err
	}

	// Exclude already-registered authenticators so the same device cannot be
	// enrolled twice.
	exclusions := make([]protocol.CredentialDescriptor, 0, len(user.creds))
	for _, c := range user.creds {
		exclusions = append(exclusions, protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: c.ID,
			Transport:    c.Transport,
		})
	}

	// Require a client-side discoverable (resident) credential so the passkey can
	// later be used for passwordless/usernameless login, where the authenticator
	// must surface the credential by userHandle with no allowCredentials list.
	// Require user verification at enrollment too, so adding a passkey confirms
	// the user (biometric/PIN) and matches the verification demanded at login.
	opts := []gowebauthn.RegistrationOption{
		gowebauthn.WithExclusions(exclusions),
		gowebauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationRequired,
		}),
	}

	requireHardware := false
	if s.settings != nil {
		requireHardware = s.settings.GetBool(ctx, "require_hardware_attestation", false)
	}

	if requireHardware {
		opts = append(opts, gowebauthn.WithConveyancePreference(protocol.PreferDirectAttestation))
	}

	creation, session, err := s.wa.BeginRegistration(user, opts...)
	if err != nil {
		return nil, err
	}
	if err := s.saveSession(ctx, regSessionKey(userID), session); err != nil {
		return nil, err
	}
	return json.Marshal(creation)
}

// FinishRegistration verifies the attestation response and stores the new
// credential under the given friendly name.
func (s *Service) FinishRegistration(ctx context.Context, userID, name, displayName, credName string, responseBody []byte) error {
	session, err := s.loadSession(ctx, regSessionKey(userID))
	if err != nil {
		return err
	}
	user, err := s.buildUser(ctx, userID, name, displayName)
	if err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(responseBody))
	if err != nil {
		return err
	}
	cred, err := s.wa.CreateCredential(user, session, parsed)
	if err != nil {
		return err
	}

	// Verify hardware attestation if required
	requireHardware := false
	if s.settings != nil {
		requireHardware = s.settings.GetBool(ctx, "require_hardware_attestation", false)
	}
	if requireHardware {
		isNone := cred.AttestationType == "none" || cred.AttestationType == ""
		allZeroes := true
		for _, b := range cred.Authenticator.AAGUID {
			if b != 0 {
				allZeroes = false
				break
			}
		}
		if isNone || allZeroes {
			return ErrHardwareAttestationRequired
		}
	}

	credID := base64.RawURLEncoding.EncodeToString(cred.ID)
	exists, err := s.repo.CredentialExists(ctx, credID)
	if err != nil {
		return err
	}
	if exists {
		return ErrCredentialInUse
	}

	data, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	return s.repo.Insert(ctx, userID, credID, data, int64(cred.Authenticator.SignCount), credName)
}

// BeginLogin starts an assertion ceremony for the second factor and returns the
// CredentialAssertion options (JSON) for navigator.credentials.get().
func (s *Service) BeginLogin(ctx context.Context, userID, name, displayName string) (json.RawMessage, error) {
	user, err := s.buildUser(ctx, userID, name, displayName)
	if err != nil {
		return nil, err
	}
	if len(user.creds) == 0 {
		return nil, ErrNoCredentials
	}
	assertion, session, err := s.wa.BeginLogin(user)
	if err != nil {
		return nil, err
	}
	if err := s.saveSession(ctx, loginSessionKey(userID), session); err != nil {
		return nil, err
	}
	return json.Marshal(assertion)
}

// FinishLogin verifies the assertion response, updates the signature counter,
// and returns nil on success. The caller is responsible for issuing tokens.
func (s *Service) FinishLogin(ctx context.Context, userID, name, displayName string, responseBody []byte) error {
	session, err := s.loadSession(ctx, loginSessionKey(userID))
	if err != nil {
		return err
	}
	user, err := s.buildUser(ctx, userID, name, displayName)
	if err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(responseBody))
	if err != nil {
		return err
	}
	cred, err := s.wa.ValidateLogin(user, session, parsed)
	if err != nil {
		return err
	}

	credID := base64.RawURLEncoding.EncodeToString(cred.ID)
	data, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	return s.repo.UpdateOnLogin(ctx, credID, data, int64(cred.Authenticator.SignCount))
}

// BeginDiscoverableLogin starts a passwordless (usernameless) assertion ceremony
// using discoverable credentials (resident keys). The authenticator itself
// reveals which user is signing in via the userHandle, so no email/password is
// required up front. It returns the CredentialAssertion options for
// navigator.credentials.get() with an opaque ceremony_id the caller echoes back
// to FinishDiscoverableLogin (the ceremony is not bound to a known user yet).
func (s *Service) BeginDiscoverableLogin(ctx context.Context) (json.RawMessage, error) {
	// Require user verification: with no password step, the authenticator's UV
	// (biometric/PIN) is the second factor that makes a passkey stand in for both
	// possession and inherence.
	assertion, session, err := s.wa.BeginDiscoverableLogin(
		gowebauthn.WithUserVerification(protocol.VerificationRequired),
	)
	if err != nil {
		return nil, err
	}
	ceremonyID := uuid.NewString()
	if err := s.saveSession(ctx, discoSessionKey(ceremonyID), session); err != nil {
		return nil, err
	}
	// Inject the ceremony id alongside the standard {"publicKey": {...}} payload;
	// performAssertion on the client ignores unknown top-level fields.
	raw, err := json.Marshal(assertion)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	m["ceremony_id"], _ = json.Marshal(ceremonyID)
	return json.Marshal(m)
}

// FinishDiscoverableLogin verifies a passwordless assertion. The userHandle in
// the response identifies the signing user; the returned userID lets the caller
// issue tokens. The single-use ceremony session makes concurrent finishes safe.
func (s *Service) FinishDiscoverableLogin(ctx context.Context, ceremonyID string, responseBody []byte) (string, error) {
	session, err := s.loadSession(ctx, discoSessionKey(ceremonyID))
	if err != nil {
		return "", err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(responseBody))
	if err != nil {
		return "", err
	}

	var resolvedUserID string
	handler := func(_, userHandle []byte) (gowebauthn.User, error) {
		uid, err := uuid.FromBytes(userHandle)
		if err != nil {
			return nil, err
		}
		resolvedUserID = uid.String()
		return s.buildUser(ctx, resolvedUserID, "", "")
	}

	cred, err := s.wa.ValidateDiscoverableLogin(handler, session, parsed)
	if err != nil {
		return "", err
	}

	credID := base64.RawURLEncoding.EncodeToString(cred.ID)
	data, err := json.Marshal(cred)
	if err != nil {
		return "", err
	}
	if err := s.repo.UpdateOnLogin(ctx, credID, data, int64(cred.Authenticator.SignCount)); err != nil {
		return "", err
	}
	return resolvedUserID, nil
}

func (s *Service) saveSession(ctx context.Context, key string, sd *gowebauthn.SessionData) error {
	b, err := json.Marshal(sd)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, key, b, ceremonySessionTTL).Err()
}

// loadSession reads and deletes (single-use) the stored ceremony session.
func (s *Service) loadSession(ctx context.Context, key string) (gowebauthn.SessionData, error) {
	var sd gowebauthn.SessionData
	b, err := s.rdb.GetDel(ctx, key).Bytes()
	if err != nil {
		return sd, ErrSessionNotFound
	}
	if err := json.Unmarshal(b, &sd); err != nil {
		return sd, err
	}
	return sd, nil
}

func regSessionKey(userID string) string   { return "webauthn:reg:" + userID }
func loginSessionKey(userID string) string { return "webauthn:login:" + userID }
func discoSessionKey(ceremonyID string) string { return "webauthn:disco:" + ceremonyID }
