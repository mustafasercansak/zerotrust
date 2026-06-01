package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func mustGenerateEd25519Key(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return k
}

func writePEMFile(t *testing.T, path string, blockType string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatalf("write pem: %v", err)
	}
}

func TestPublicJWKSExportsAllKeys(t *testing.T) {
	ks := NewKeyStore()
	k1 := mustGenerateEd25519Key(t)
	k2 := mustGenerateEd25519Key(t)
	ks.addKey("kid-1", k1)
	ks.addKey("kid-2", k2)

	jwks := ks.PublicJWKS()
	if len(jwks.Keys) != 2 {
		t.Fatalf("PublicJWKS returned %d keys, want 2", len(jwks.Keys))
	}
	for _, k := range jwks.Keys {
		if k.Kty != "OKP" || k.Crv != "Ed25519" || k.Use != "sig" || k.Alg != "EdDSA" {
			t.Fatalf("unexpected jwk metadata: %+v", k)
		}
		if k.X == "" {
			t.Fatalf("expected x coordinate in jwk: %+v", k)
		}
		if k.Y != "" {
			t.Fatalf("unexpected y coordinate in jwk: %+v", k)
		}
	}
}

func TestLoadKeyFromFilePKCS8(t *testing.T) {
	tmp := t.TempDir()
	key := mustGenerateEd25519Key(t)

	pkcs8DER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pkcs8Path := filepath.Join(tmp, "pkcs8.pem")
	writePEMFile(t, pkcs8Path, "PRIVATE KEY", pkcs8DER)

	loadedPKCS8, err := loadKeyFromFile(pkcs8Path)
	if err != nil {
		t.Fatalf("load pkcs8 key: %v", err)
	}
	if !ed25519.PrivateKey(loadedPKCS8).Public().(ed25519.PublicKey).Equal(key.Public()) {
		t.Fatal("loaded pkcs8 key does not match original public key")
	}
}

func TestLoadKeyFromFileErrors(t *testing.T) {
	_, err := loadKeyFromFile(filepath.Join(t.TempDir(), "missing.pem"))
	if err == nil {
		t.Fatal("expected error for missing key file")
	}

	bad := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(bad, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("write bad file: %v", err)
	}
	_, err = loadKeyFromFile(bad)
	if err == nil {
		t.Fatal("expected error for invalid pem content")
	}
}

func TestLoadKeyFromFileNonEd25519(t *testing.T) {
	tmp := t.TempDir()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatalf("marshal rsa pkcs8: %v", err)
	}
	rsaPath := filepath.Join(tmp, "rsa.pem")
	writePEMFile(t, rsaPath, "PRIVATE KEY", der)

	_, err = loadKeyFromFile(rsaPath)
	if err == nil {
		t.Fatal("expected error for loading non-Ed25519 key, got nil")
	}
}

func TestLoadOrGenerateKeyStoreWithFiles(t *testing.T) {
	tmp := t.TempDir()
	k1 := mustGenerateEd25519Key(t)
	k2 := mustGenerateEd25519Key(t)

	der1, _ := x509.MarshalPKCS8PrivateKey(k1)
	der2, _ := x509.MarshalPKCS8PrivateKey(k2)

	p1 := filepath.Join(tmp, "p1.pem")
	p2 := filepath.Join(tmp, "p2.pem")

	writePEMFile(t, p1, "PRIVATE KEY", der1)
	writePEMFile(t, p2, "PRIVATE KEY", der2)

	// 1. Valid primary only
	ks, err := LoadOrGenerateKeyStore(p1, "")
	if err != nil {
		t.Fatalf("failed to load valid primary: %v", err)
	}
	if ks.PrimaryKID() == "" {
		t.Fatal("expected primary kid to be set")
	}

	// 2. Valid primary and secondary
	ks, err = LoadOrGenerateKeyStore(p1, p2)
	if err != nil {
		t.Fatalf("failed to load valid primary and secondary: %v", err)
	}

	// 3. Valid primary, invalid secondary (should log warning but succeed)
	ks, err = LoadOrGenerateKeyStore(p1, "/nonexistent/sec.pem")
	if err != nil {
		t.Fatalf("failed to load keystore when secondary is invalid: %v", err)
	}

	// 4. Invalid primary (should fail)
	_, err = LoadOrGenerateKeyStore("/nonexistent/primary.pem", "")
	if err == nil {
		t.Fatal("expected error for nonexistent primary key file")
	}
}

func TestEd25519KeyToJWK(t *testing.T) {
	key := mustGenerateEd25519Key(t)
	pub := key.Public().(ed25519.PublicKey)
	jwk := ed25519KeyToJWK("kid-test", pub)

	if jwk.Kid != "kid-test" {
		t.Fatalf("kid=%q want=kid-test", jwk.Kid)
	}
	if jwk.Kty != "OKP" || jwk.Crv != "Ed25519" || jwk.Alg != "EdDSA" {
		t.Fatalf("unexpected jwk metadata: %+v", jwk)
	}
	if jwk.X == "" {
		t.Fatalf("expected non-empty x in jwk: %+v", jwk)
	}
}
