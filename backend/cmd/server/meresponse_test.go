package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/zerotrust/backend/internal/user"
)

func TestBuildMeResponse_EscapesSpecialCharacters(t *testing.T) {
	profile := &user.User{
		ID:        "u1",
		Email:     "weird@example.com",
		FirstName: `A"li\ce`,           // quote + backslash
		LastName:  "Bob\tNewline\nEnd", // control characters
		HasAvatar: true,
		Locale:    "en",
		Roles:     []string{"admin"},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(buildMeResponse(profile, []string{"users:read"})); err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	// Must be valid, parseable JSON with the original values intact.
	var got meResponse
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, buf.String())
	}
	if got.FirstName != profile.FirstName {
		t.Errorf("first_name not round-tripped: got %q want %q", got.FirstName, profile.FirstName)
	}
	if got.LastName != profile.LastName {
		t.Errorf("last_name not round-tripped: got %q want %q", got.LastName, profile.LastName)
	}
}

func TestBuildMeResponse_NilSlicesBecomeEmptyArrays(t *testing.T) {
	profile := &user.User{ID: "u1", Email: "e@x.com", Roles: nil}

	b, err := json.Marshal(buildMeResponse(profile, nil))
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	s := string(b)
	if !bytes.Contains(b, []byte(`"roles":[]`)) {
		t.Errorf("expected roles to encode as [], got %s", s)
	}
	if !bytes.Contains(b, []byte(`"permissions":[]`)) {
		t.Errorf("expected permissions to encode as [], got %s", s)
	}
}
