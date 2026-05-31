package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func mustGenerateECDSAKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
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
	k1 := mustGenerateECDSAKey(t)
	k2 := mustGenerateECDSAKey(t)
	ks.addKey("kid-1", k1)
	ks.addKey("kid-2", k2)

	jwks := ks.PublicJWKS()
	if len(jwks.Keys) != 2 {
		t.Fatalf("PublicJWKS returned %d keys, want 2", len(jwks.Keys))
	}
	for _, k := range jwks.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" || k.Use != "sig" || k.Alg != "ES256" {
			t.Fatalf("unexpected jwk metadata: %+v", k)
		}
		if k.X == "" || k.Y == "" {
			t.Fatalf("expected x/y coordinates in jwk: %+v", k)
		}
	}
}

func TestLoadKeyFromFilePKCS8AndSEC1(t *testing.T) {
	tmp := t.TempDir()
	key := mustGenerateECDSAKey(t)

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
	if loadedPKCS8.PublicKey.X.Cmp(key.PublicKey.X) != 0 || loadedPKCS8.PublicKey.Y.Cmp(key.PublicKey.Y) != 0 {
		t.Fatal("loaded pkcs8 key does not match original public key")
	}

	sec1DER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal sec1: %v", err)
	}
	sec1Path := filepath.Join(tmp, "sec1.pem")
	writePEMFile(t, sec1Path, "EC PRIVATE KEY", sec1DER)

	loadedSEC1, err := loadKeyFromFile(sec1Path)
	if err != nil {
		t.Fatalf("load sec1 key: %v", err)
	}
	if loadedSEC1.PublicKey.X.Cmp(key.PublicKey.X) != 0 || loadedSEC1.PublicKey.Y.Cmp(key.PublicKey.Y) != 0 {
		t.Fatal("loaded sec1 key does not match original public key")
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

func TestEcKeyToJWKAndPadTo32(t *testing.T) {
	key := mustGenerateECDSAKey(t)
	jwk := ecKeyToJWK("kid-test", &key.PublicKey)

	if jwk.Kid != "kid-test" {
		t.Fatalf("kid=%q want=kid-test", jwk.Kid)
	}
	if jwk.X == "" || jwk.Y == "" {
		t.Fatalf("expected non-empty coordinates in jwk: %+v", jwk)
	}

	in := []byte{1, 2, 3}
	out := padTo32(in)
	if len(out) != 32 {
		t.Fatalf("padTo32 len=%d want=32", len(out))
	}
	if out[29] != 1 || out[30] != 2 || out[31] != 3 {
		t.Fatalf("padTo32 unexpected suffix: %v", out[29:32])
	}

	exact := make([]byte, 32)
	same := padTo32(exact)
	if &same[0] != &exact[0] {
		t.Fatal("padTo32 should return original slice when length is exactly 32")
	}
}
