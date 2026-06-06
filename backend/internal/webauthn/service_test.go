package webauthn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

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
	})
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
