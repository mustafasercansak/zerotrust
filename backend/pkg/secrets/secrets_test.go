package secrets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestNewClientConfiguration(t *testing.T) {
	// Set environment variables to mock settings
	os.Setenv("BAO_ADDR", "http://bao-test.local:8200")
	os.Setenv("BAO_TOKEN", "bao-test-token")
	defer func() {
		os.Unsetenv("BAO_ADDR")
		os.Unsetenv("BAO_TOKEN")
	}()

	client, err := NewClient("test-key")
	if err != nil {
		t.Fatalf("expected NewClient to succeed, got %v", err)
	}

	if client.keyName != "test-key" {
		t.Errorf("expected keyName to be 'test-key', got %s", client.keyName)
	}

	if client.vaultClient.Address() != "http://bao-test.local:8200" {
		t.Errorf("expected Address to be 'http://bao-test.local:8200', got %s", client.vaultClient.Address())
	}
}

func TestEncryptData(t *testing.T) {
	// 1. Test empty plaintext returns empty ciphertext immediately without calling server
	client, _ := NewClient("test-key")
	ct, err := client.EncryptData(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error for empty string: %v", err)
	}
	if ct != "" {
		t.Fatalf("expected empty ciphertext, got %q", ct)
	}

	// Setup mock server
	var responseStatus int
	var responseBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(responseStatus)
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()

	os.Setenv("BAO_ADDR", server.URL)
	os.Setenv("BAO_TOKEN", "test-token")
	defer func() {
		os.Unsetenv("BAO_ADDR")
		os.Unsetenv("BAO_TOKEN")
	}()

	client, err = NewClient("test-key")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// 2. Test successful encryption
	responseStatus = http.StatusOK
	respMap := map[string]any{
		"data": map[string]any{
			"ciphertext": "vault:v1:encrypteddata",
		},
	}
	responseBody, _ = json.Marshal(respMap)

	ct, err = client.EncryptData(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}
	if ct != "vault:v1:encrypteddata" {
		t.Errorf("unexpected ciphertext: %q", ct)
	}

	// 3. Test API server error response (e.g., 500)
	responseStatus = http.StatusInternalServerError
	responseBody = []byte("internal server error")
	_, err = client.EncryptData(context.Background(), "hello")
	if err == nil {
		t.Error("expected error from server failure, got nil")
	}

	// 4. Test missing ciphertext key in response
	responseStatus = http.StatusOK
	respMap = map[string]any{
		"data": map[string]any{
			"something_else": "here",
		},
	}
	responseBody, _ = json.Marshal(respMap)
	_, err = client.EncryptData(context.Background(), "hello")
	if err == nil {
		t.Error("expected error for missing ciphertext key, got nil")
	}
}

func TestDecryptData(t *testing.T) {
	// 1. Test empty ciphertext returns empty plaintext immediately without calling server
	client, _ := NewClient("test-key")
	pt, err := client.DecryptData(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error for empty string: %v", err)
	}
	if pt != "" {
		t.Fatalf("expected empty plaintext, got %q", pt)
	}

	// Setup mock server
	var responseStatus int
	var responseBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(responseStatus)
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()

	os.Setenv("BAO_ADDR", server.URL)
	os.Setenv("BAO_TOKEN", "test-token")
	defer func() {
		os.Unsetenv("BAO_ADDR")
		os.Unsetenv("BAO_TOKEN")
	}()

	client, err = NewClient("test-key")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// 2. Test successful decryption
	responseStatus = http.StatusOK
	respMap := map[string]any{
		"data": map[string]any{
			"plaintext": base64.StdEncoding.EncodeToString([]byte("hello")),
		},
	}
	responseBody, _ = json.Marshal(respMap)

	pt, err = client.DecryptData(context.Background(), "vault:v1:encrypteddata")
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}
	if pt != "hello" {
		t.Errorf("unexpected plaintext: %q", pt)
	}

	// 3. Test API server error response (e.g., 500)
	responseStatus = http.StatusInternalServerError
	responseBody = []byte("internal server error")
	_, err = client.DecryptData(context.Background(), "vault:v1:encrypteddata")
	if err == nil {
		t.Error("expected error from server failure, got nil")
	}

	// 4. Test missing plaintext key in response
	responseStatus = http.StatusOK
	respMap = map[string]any{
		"data": map[string]any{
			"something_else": "here",
		},
	}
	responseBody, _ = json.Marshal(respMap)
	_, err = client.DecryptData(context.Background(), "vault:v1:encrypteddata")
	if err == nil {
		t.Error("expected error for missing plaintext key, got nil")
	}

	// 5. Test invalid base64 encoding in response
	responseStatus = http.StatusOK
	respMap = map[string]any{
		"data": map[string]any{
			"plaintext": "not-valid-base64-!!!",
		},
	}
	responseBody, _ = json.Marshal(respMap)
	_, err = client.DecryptData(context.Background(), "vault:v1:encrypteddata")
	if err == nil {
		t.Error("expected error for invalid base64 encoding, got nil")
	}
}
