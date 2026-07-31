package webauthn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"strings"

	"github.com/alicebob/miniredis/v2"
	vwa "github.com/descope/virtualwebauthn"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// memStore is an in-memory implementation of the store interface for tests.
type memStore struct {
	data map[string][]byte         // credentialID -> JSON blob
	user map[string][]string       // userID -> []credentialID
	meta map[string]CredentialMeta // credentialID -> meta
}

func newMemStore() *memStore {
	return &memStore{
		data: map[string][]byte{},
		user: map[string][]string{},
		meta: map[string]CredentialMeta{},
	}
}

func (m *memStore) Insert(_ context.Context, userID, credentialID string, data []byte, signCount int64, name string) error {
	m.data[credentialID] = data
	m.user[userID] = append(m.user[userID], credentialID)
	m.meta[credentialID] = CredentialMeta{ID: credentialID, Name: name, SignCount: signCount}
	return nil
}

func (m *memStore) ListData(_ context.Context, userID string) ([][]byte, error) {
	var out [][]byte
	for _, id := range m.user[userID] {
		out = append(out, m.data[id])
	}
	return out, nil
}

func (m *memStore) ListMeta(_ context.Context, userID string) ([]CredentialMeta, error) {
	out := make([]CredentialMeta, 0)
	for _, id := range m.user[userID] {
		out = append(out, m.meta[id])
	}
	return out, nil
}

func (m *memStore) Count(_ context.Context, userID string) (int, error) {
	return len(m.user[userID]), nil
}

func (m *memStore) UpdateOnLogin(_ context.Context, credentialID string, data []byte, signCount int64) error {
	m.data[credentialID] = data
	meta := m.meta[credentialID]
	meta.SignCount = signCount
	m.meta[credentialID] = meta
	return nil
}

func (m *memStore) Delete(_ context.Context, id, userID string) error {
	ids := m.user[userID]
	for i, cid := range ids {
		if cid == id {
			m.user[userID] = append(ids[:i], ids[i+1:]...)
			delete(m.data, cid)
			delete(m.meta, cid)
			return nil
		}
	}
	return ErrNotFound
}

func (m *memStore) CredentialExists(_ context.Context, credentialID string) (bool, error) {
	_, ok := m.data[credentialID]
	return ok, nil
}

func (m *memStore) Rename(_ context.Context, id, userID, name string) error {
	ids := m.user[userID]
	found := false
	for _, cid := range ids {
		if cid == id {
			found = true
			break
		}
	}
	if !found {
		return ErrNotFound
	}
	meta, ok := m.meta[id]
	if !ok {
		return ErrNotFound
	}
	meta.Name = name
	m.meta[id] = meta
	return nil
}

func newTestService(t *testing.T) (*Service, *memStore) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := newMemStore()
	svc, err := NewService(store, rdb, Config{
		RPID:          "localhost",
		RPDisplayName: "ZeroTrust",
		RPOrigins:     []string{"http://localhost:3000"},
	}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, store
}

// TestRegisterThenLogin drives a full registration + login ceremony through the
// real go-webauthn crypto using a virtual authenticator.
func TestRegisterThenLogin(t *testing.T) {
	svc, store := newTestService(t)
	ctx := context.Background()
	userID := uuid.NewString()

	rp := vwa.RelyingParty{ID: "localhost", Name: "ZeroTrust", Origin: "http://localhost:3000"}
	authenticator := vwa.NewAuthenticator()
	cred := vwa.NewCredential(vwa.KeyTypeEC2)

	// --- Registration ---
	if svc.HasCredentials(ctx, userID) {
		t.Fatal("expected no credentials before registration")
	}
	optsJSON, err := svc.BeginRegistration(ctx, userID, "user@example.com", "User")
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	attOpts, err := vwa.ParseAttestationOptions(string(optsJSON))
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	attResponse := vwa.CreateAttestationResponse(rp, authenticator, cred, *attOpts)
	if err := svc.FinishRegistration(ctx, userID, "user@example.com", "User", "My Passkey", []byte(attResponse)); err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}
	authenticator.AddCredential(cred)

	if !svc.HasCredentials(ctx, userID) {
		t.Fatal("expected credentials after registration")
	}
	metas, err := svc.ListCredentials(ctx, userID)
	if err != nil || len(metas) != 1 || metas[0].Name != "My Passkey" {
		t.Fatalf("ListCredentials = %+v, err=%v", metas, err)
	}

	// --- Login ---
	loginOptsJSON, err := svc.BeginLogin(ctx, userID, "user@example.com", "User")
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	asrOpts, err := vwa.ParseAssertionOptions(string(loginOptsJSON))
	if err != nil {
		t.Fatalf("ParseAssertionOptions: %v", err)
	}
	asrResponse := vwa.CreateAssertionResponse(rp, authenticator, cred, *asrOpts)
	if err := svc.FinishLogin(ctx, userID, "user@example.com", "User", []byte(asrResponse)); err != nil {
		t.Fatalf("FinishLogin: %v", err)
	}

	// Deleting the credential clears it.
	credID := base64.RawURLEncoding.EncodeToString(cred.ID)
	if err := svc.DeleteCredential(ctx, credID, userID); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}
	if svc.HasCredentials(ctx, userID) {
		t.Fatal("expected no credentials after delete")
	}
	_ = store
}

func TestBeginLogin_NoCredentials(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.BeginLogin(context.Background(), uuid.NewString(), "u@example.com", "U")
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("expected ErrNoCredentials, got %v", err)
	}
}

// TestBeginRegistration_RequestsDiscoverableCredential guards the passwordless
// login feature: discoverable (usernameless) login can only surface a credential
// that was stored as a resident key, so registration must request one. Without
// residentKey=required the authenticator stores a non-discoverable credential and
// passwordless login silently finds nothing.
func TestBeginRegistration_RequestsDiscoverableCredential(t *testing.T) {
	svc, _ := newTestService(t)
	optsJSON, err := svc.BeginRegistration(context.Background(), uuid.NewString(), "user@example.com", "User")
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}

	var opts struct {
		PublicKey struct {
			AuthenticatorSelection struct {
				ResidentKey        string `json:"residentKey"`
				RequireResidentKey *bool  `json:"requireResidentKey"`
				UserVerification   string `json:"userVerification"`
			} `json:"authenticatorSelection"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(optsJSON, &opts); err != nil {
		t.Fatalf("unmarshal options: %v", err)
	}

	as := opts.PublicKey.AuthenticatorSelection
	if as.ResidentKey != "required" {
		t.Fatalf("expected residentKey=required for passwordless support, got %q", as.ResidentKey)
	}
	if as.RequireResidentKey == nil || !*as.RequireResidentKey {
		t.Fatalf("expected requireResidentKey=true, got %v", as.RequireResidentKey)
	}
	if as.UserVerification != "required" {
		t.Fatalf("expected userVerification=required at enrollment, got %q", as.UserVerification)
	}
}

func TestBeginDiscoverableLogin_ReturnsCeremonyAndOptions(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	optsJSON, err := svc.BeginDiscoverableLogin(ctx)
	if err != nil {
		t.Fatalf("BeginDiscoverableLogin: %v", err)
	}

	var payload struct {
		PublicKey  json.RawMessage `json:"publicKey"`
		CeremonyID string          `json:"ceremony_id"`
	}
	if err := json.Unmarshal(optsJSON, &payload); err != nil {
		t.Fatalf("unmarshal options: %v", err)
	}
	if payload.CeremonyID == "" {
		t.Fatal("expected a ceremony_id in the options")
	}
	if len(payload.PublicKey) == 0 {
		t.Fatal("expected publicKey assertion options")
	}
	// The ceremony session is single-use: a bogus ceremony id is rejected.
	if _, err := svc.FinishDiscoverableLogin(ctx, "does-not-exist", []byte(`{"id":"x"}`)); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound for unknown ceremony, got %v", err)
	}
}

// TestDiscoverableLogin_RoundTrip drives a full passwordless (usernameless) login
// through the real go-webauthn crypto: register a resident credential, then sign in
// with a discoverable assertion whose userHandle identifies the user — no email or
// allowCredentials list. It exercises FinishDiscoverableLogin's happy path.
func TestDiscoverableLogin_RoundTrip(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	userID := uuid.NewString()

	rp := vwa.RelyingParty{ID: "localhost", Name: "ZeroTrust", Origin: "http://localhost:3000"}
	authenticator := vwa.NewAuthenticator()
	cred := vwa.NewCredential(vwa.KeyTypeEC2)

	// --- Register a resident credential ---
	regJSON, err := svc.BeginRegistration(ctx, userID, "user@example.com", "User")
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	attOpts, err := vwa.ParseAttestationOptions(string(regJSON))
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	attResponse := vwa.CreateAttestationResponse(rp, authenticator, cred, *attOpts)
	if err := svc.FinishRegistration(ctx, userID, "user@example.com", "User", "My Passkey", []byte(attResponse)); err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}
	authenticator.AddCredential(cred)

	// The server derives the WebAuthn user handle from the first 16 bytes of the
	// user UUID; the authenticator must echo that handle back on a discoverable
	// assertion so FinishDiscoverableLogin can resolve the user.
	uid := uuid.MustParse(userID)
	handle := make([]byte, 16)
	copy(handle, uid[:])
	authenticator.Options.UserHandle = handle

	// --- Passwordless ceremony ---
	loginJSON, err := svc.BeginDiscoverableLogin(ctx)
	if err != nil {
		t.Fatalf("BeginDiscoverableLogin: %v", err)
	}
	var begin struct {
		CeremonyID string `json:"ceremony_id"`
	}
	if err := json.Unmarshal(loginJSON, &begin); err != nil {
		t.Fatalf("unmarshal begin: %v", err)
	}

	asrOpts, err := vwa.ParseAssertionOptions(string(loginJSON))
	if err != nil {
		t.Fatalf("ParseAssertionOptions: %v", err)
	}
	asrResponse := vwa.CreateAssertionResponse(rp, authenticator, cred, *asrOpts)

	gotUserID, err := svc.FinishDiscoverableLogin(ctx, begin.CeremonyID, []byte(asrResponse))
	if err != nil {
		t.Fatalf("FinishDiscoverableLogin: %v", err)
	}
	if gotUserID != userID {
		t.Fatalf("resolved userID = %q, want %q", gotUserID, userID)
	}

	// The ceremony is single-use — a replay of the same assertion is rejected.
	if _, err := svc.FinishDiscoverableLogin(ctx, begin.CeremonyID, []byte(asrResponse)); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound on replay, got %v", err)
	}
}

func TestFinishLogin_SessionSingleUse(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	userID := uuid.NewString()

	rp := vwa.RelyingParty{ID: "localhost", Name: "ZeroTrust", Origin: "http://localhost:3000"}
	authenticator := vwa.NewAuthenticator()
	cred := vwa.NewCredential(vwa.KeyTypeEC2)

	optsJSON, _ := svc.BeginRegistration(ctx, userID, "u@example.com", "U")
	attOpts, _ := vwa.ParseAttestationOptions(string(optsJSON))
	attResponse := vwa.CreateAttestationResponse(rp, authenticator, cred, *attOpts)
	if err := svc.FinishRegistration(ctx, userID, "u@example.com", "U", "k", []byte(attResponse)); err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}
	authenticator.AddCredential(cred)

	loginOptsJSON, _ := svc.BeginLogin(ctx, userID, "u@example.com", "U")
	asrOpts, _ := vwa.ParseAssertionOptions(string(loginOptsJSON))
	asrResponse := vwa.CreateAssertionResponse(rp, authenticator, cred, *asrOpts)

	if err := svc.FinishLogin(ctx, userID, "u@example.com", "U", []byte(asrResponse)); err != nil {
		t.Fatalf("first FinishLogin: %v", err)
	}
	// The ceremony session is single-use; replaying the same assertion fails
	// because the stored session was consumed.
	if err := svc.FinishLogin(ctx, userID, "u@example.com", "U", []byte(asrResponse)); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound on replay, got %v", err)
	}
}

func TestFinishRegistration_NoSession(t *testing.T) {
	svc, _ := newTestService(t)
	// No BeginRegistration call → no session stored.
	err := svc.FinishRegistration(context.Background(), uuid.NewString(), "u@example.com", "U", "k", []byte(`{}`))
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

// TestFinishLogin_CloneWarningRejected drives a signature-counter regression:
// after a successful login at counter 5, a second assertion at counter 3 makes
// go-webauthn flag CloneWarning, and the login must be rejected (#96).
func TestFinishLogin_CloneWarningRejected(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	userID := uuid.NewString()

	rp := vwa.RelyingParty{ID: "localhost", Name: "ZeroTrust", Origin: "http://localhost:3000"}
	authenticator := vwa.NewAuthenticator()
	cred := vwa.NewCredential(vwa.KeyTypeEC2)

	optsJSON, _ := svc.BeginRegistration(ctx, userID, "u@example.com", "U")
	attOpts, _ := vwa.ParseAttestationOptions(string(optsJSON))
	attResponse := vwa.CreateAttestationResponse(rp, authenticator, cred, *attOpts)
	if err := svc.FinishRegistration(ctx, userID, "u@example.com", "U", "k", []byte(attResponse)); err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}
	authenticator.AddCredential(cred)

	// First login at counter 5 succeeds and stores signCount=5.
	cred.Counter = 5
	loginOptsJSON, _ := svc.BeginLogin(ctx, userID, "u@example.com", "U")
	asrOpts, _ := vwa.ParseAssertionOptions(string(loginOptsJSON))
	asrResponse := vwa.CreateAssertionResponse(rp, authenticator, cred, *asrOpts)
	if err := svc.FinishLogin(ctx, userID, "u@example.com", "U", []byte(asrResponse)); err != nil {
		t.Fatalf("first FinishLogin: %v", err)
	}

	// A cloned authenticator signs at counter 3 — a regression against the
	// stored counter — and must be rejected.
	cred.Counter = 3
	loginOptsJSON2, _ := svc.BeginLogin(ctx, userID, "u@example.com", "U")
	asrOpts2, _ := vwa.ParseAssertionOptions(string(loginOptsJSON2))
	asrResponse2 := vwa.CreateAssertionResponse(rp, authenticator, cred, *asrOpts2)
	if err := svc.FinishLogin(ctx, userID, "u@example.com", "U", []byte(asrResponse2)); !errors.Is(err, ErrCredentialCloneDetected) {
		t.Fatalf("expected ErrCredentialCloneDetected, got %v", err)
	}
}

func TestBeginRegistration_OptionsAreValidJSON(t *testing.T) {
	svc, _ := newTestService(t)
	opts, err := svc.BeginRegistration(context.Background(), uuid.NewString(), "u@example.com", "U")
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(opts, &parsed); err != nil {
		t.Fatalf("options not valid JSON: %v", err)
	}
	if _, ok := parsed["publicKey"]; !ok {
		t.Fatalf("expected publicKey in creation options, got %v", parsed)
	}
}

// A non-UUID userID can't be turned into a WebAuthn user handle, so the
// ceremony-builders must surface the parse error instead of panicking.
func TestBeginRegistration_InvalidUserID(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.BeginRegistration(context.Background(), "not-a-uuid", "u@example.com", "U"); err == nil {
		t.Fatal("expected an error for a non-UUID userID")
	}
}

func TestBeginLogin_InvalidUserID(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.BeginLogin(context.Background(), "not-a-uuid", "u@example.com", "U"); err == nil {
		t.Fatal("expected an error for a non-UUID userID")
	}
}

// When Redis is unreachable the ceremony-begin calls must surface the error from
// persisting the single-use session rather than returning unusable options.
func TestBeginCeremonies_RedisUnavailable(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc, err := NewService(newMemStore(), rdb, Config{
		RPID: "localhost", RPDisplayName: "ZeroTrust", RPOrigins: []string{"http://localhost:3000"},
	}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	mr.Close() // Redis now unreachable → saveSession fails.

	ctx := context.Background()
	if _, err := svc.BeginDiscoverableLogin(ctx); err == nil {
		t.Fatal("expected BeginDiscoverableLogin to fail when Redis is down")
	}
	if _, err := svc.BeginRegistration(ctx, uuid.NewString(), "u@example.com", "U"); err == nil {
		t.Fatal("expected BeginRegistration to fail when Redis is down")
	}
}

// FinishRegistration must reject a malformed attestation body even with a live
// ceremony session (the response can't be parsed into a creation response).
func TestFinishRegistration_MalformedBody(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	userID := uuid.NewString()
	if _, err := svc.BeginRegistration(ctx, userID, "u@example.com", "U"); err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	if err := svc.FinishRegistration(ctx, userID, "u@example.com", "U", "k", []byte(`{"not":"an attestation"}`)); err == nil {
		t.Fatal("expected an error for a malformed attestation body")
	}
}

// FinishLogin must reject a malformed assertion body once a credential exists and
// a login ceremony is in flight.
func TestFinishLogin_MalformedBody(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	userID := uuid.NewString()

	rp := vwa.RelyingParty{ID: "localhost", Name: "ZeroTrust", Origin: "http://localhost:3000"}
	authenticator := vwa.NewAuthenticator()
	cred := vwa.NewCredential(vwa.KeyTypeEC2)

	regJSON, _ := svc.BeginRegistration(ctx, userID, "u@example.com", "U")
	attOpts, _ := vwa.ParseAttestationOptions(string(regJSON))
	attResponse := vwa.CreateAttestationResponse(rp, authenticator, cred, *attOpts)
	if err := svc.FinishRegistration(ctx, userID, "u@example.com", "U", "k", []byte(attResponse)); err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}
	authenticator.AddCredential(cred)

	if _, err := svc.BeginLogin(ctx, userID, "u@example.com", "U"); err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	if err := svc.FinishLogin(ctx, userID, "u@example.com", "U", []byte(`{"not":"an assertion"}`)); err == nil {
		t.Fatal("expected an error for a malformed assertion body")
	}
}

// FinishDiscoverableLogin must reject a malformed assertion body even when the
// ceremony session exists (the response can't be parsed into an assertion).
func TestFinishDiscoverableLogin_MalformedAssertion(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	loginJSON, err := svc.BeginDiscoverableLogin(ctx)
	if err != nil {
		t.Fatalf("BeginDiscoverableLogin: %v", err)
	}
	var begin struct {
		CeremonyID string `json:"ceremony_id"`
	}
	if err := json.Unmarshal(loginJSON, &begin); err != nil {
		t.Fatalf("unmarshal begin: %v", err)
	}

	if _, err := svc.FinishDiscoverableLogin(ctx, begin.CeremonyID, []byte(`{"not":"an assertion"}`)); err == nil {
		t.Fatal("expected an error for a malformed assertion body")
	}
}

type mockSettings struct {
	bools map[string]bool
}

func (m *mockSettings) GetBool(ctx context.Context, key string, defaultVal bool) bool {
	if v, ok := m.bools[key]; ok {
		return v
	}
	return defaultVal
}

func TestHardwareAttestationEnforcement(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := newMemStore()
	
	settings := &mockSettings{bools: map[string]bool{"require_hardware_attestation": true}}
	svc, err := NewService(store, rdb, Config{
		RPID:          "localhost",
		RPDisplayName: "ZeroTrust",
		RPOrigins:     []string{"http://localhost:3000"},
	}, settings)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()
	userID := uuid.NewString()

	rp := vwa.RelyingParty{ID: "localhost", Name: "ZeroTrust", Origin: "http://localhost:3000"}

	// 1. None attestation should be rejected when require_hardware_attestation is true.
	authenticatorNone := vwa.NewAuthenticator()
	authenticatorNone.Aaguid = [16]byte{} // all zeroes
	credNone := vwa.NewCredential(vwa.KeyTypeEC2)

	optsJSON, err := svc.BeginRegistration(ctx, userID, "user@example.com", "User")
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	attOpts, err := vwa.ParseAttestationOptions(string(optsJSON))
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	attResponseNone := vwa.CreateAttestationResponse(rp, authenticatorNone, credNone, *attOpts)
	err = svc.FinishRegistration(ctx, userID, "user@example.com", "User", "None Key", []byte(attResponseNone))
	if !errors.Is(err, ErrHardwareAttestationRequired) {
		t.Fatalf("expected ErrHardwareAttestationRequired for none/software attestation, got %v", err)
	}

	// 2. Packed *self* attestation (basic_surrogate) with a non-zero Aaguid must
	// also be rejected: any software authenticator can forge it (#86). Only
	// certificate-chain-backed attestation (basic_full / attca) is accepted.
	authenticatorSoftware := vwa.NewAuthenticator()
	authenticatorSoftware.Aaguid = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	credSoftware := vwa.NewCredential(vwa.KeyTypeEC2)

	optsJSON2, err := svc.BeginRegistration(ctx, userID, "user@example.com", "User")
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	attOpts2, err := vwa.ParseAttestationOptions(string(optsJSON2))
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	attResponseSoftware := vwa.CreateAttestationResponse(rp, authenticatorSoftware, credSoftware, *attOpts2)
	err = svc.FinishRegistration(ctx, userID, "user@example.com", "User", "Software Key", []byte(attResponseSoftware))
	if !errors.Is(err, ErrHardwareAttestationRequired) {
		t.Fatalf("expected ErrHardwareAttestationRequired for self/software attestation, got %v", err)
	}
}

func TestRenameCredential(t *testing.T) {
	svc, store := newTestService(t)
	ctx := context.Background()
	userID := uuid.NewString()

	// Seed credential.
	store.Insert(ctx, userID, "c1", []byte(`{}`), 0, "Old Key")

	// Empty name rejected.
	if err := svc.RenameCredential(ctx, "c1", userID, "   "); err == nil || err.Error() != "name_cannot_be_empty" {
		t.Fatalf("expected error for empty name, got %v", err)
	}

	// Long name truncated.
	longName := strings.Repeat("a", 150)
	if err := svc.RenameCredential(ctx, "c1", userID, longName); err != nil {
		t.Fatalf("RenameCredential: %v", err)
	}
	metas, _ := svc.ListCredentials(ctx, userID)
	if len(metas[0].Name) != 100 {
		t.Fatalf("expected name to be truncated to 100, got %d", len(metas[0].Name))
	}

	// Successful rename.
	if err := svc.RenameCredential(ctx, "c1", userID, "New Key Name"); err != nil {
		t.Fatalf("RenameCredential: %v", err)
	}
	metas, _ = svc.ListCredentials(ctx, userID)
	if metas[0].Name != "New Key Name" {
		t.Fatalf("expected renamed name to be 'New Key Name', got %q", metas[0].Name)
	}

	// Not found.
	if err := svc.RenameCredential(ctx, "missing", userID, "Name"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing credential, got %v", err)
	}
}

