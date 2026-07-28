package auth

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
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
	ks.addKey("kid-1", keyEntry{signer: k1, alg: AlgEdDSA})
	ks.addKey("kid-2", keyEntry{signer: k2, alg: AlgEdDSA})

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
	if loadedPKCS8.alg != AlgEdDSA {
		t.Fatalf("alg=%q want=%q", loadedPKCS8.alg, AlgEdDSA)
	}
	if !loadedPKCS8.signer.(ed25519.PrivateKey).Public().(ed25519.PublicKey).Equal(key.Public()) {
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

func TestLoadKeyFromFileNonP256ECDSA(t *testing.T) {
	tmp := t.TempDir()
	p384Key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("generate p384 key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(p384Key)
	if err != nil {
		t.Fatalf("marshal p384 pkcs8: %v", err)
	}
	p384Path := filepath.Join(tmp, "p384.pem")
	writePEMFile(t, p384Path, "PRIVATE KEY", der)

	_, err = loadKeyFromFile(p384Path)
	if err == nil {
		t.Fatal("expected error for loading non-P-256 ECDSA key, got nil")
	}
}

func TestLoadKeyFromFileWeakRSA(t *testing.T) {
	tmp := t.TempDir()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 1024)
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
		t.Fatal("expected error for loading <2048-bit RSA key, got nil")
	}
}

func TestLoadKeyFromFileAlgorithms(t *testing.T) {
	tmp := t.TempDir()

	cases := []struct {
		name    string
		gen     func(t *testing.T) any
		wantAlg string
	}{
		{"Ed25519", func(t *testing.T) any { return mustGenerateEd25519Key(t) }, AlgEdDSA},
		{"ECDSA-P256", func(t *testing.T) any {
			k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatalf("generate ecdsa key: %v", err)
			}
			return k
		}, AlgES256},
		{"RSA-2048", func(t *testing.T) any {
			k, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatalf("generate rsa key: %v", err)
			}
			return k
		}, AlgRS256},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			der, err := x509.MarshalPKCS8PrivateKey(tc.gen(t))
			if err != nil {
				t.Fatalf("marshal pkcs8: %v", err)
			}
			path := filepath.Join(tmp, tc.name+".pem")
			writePEMFile(t, path, "PRIVATE KEY", der)

			entry, err := loadKeyFromFile(path)
			if err != nil {
				t.Fatalf("load key: %v", err)
			}
			if entry.alg != tc.wantAlg {
				t.Fatalf("alg=%q want=%q", entry.alg, tc.wantAlg)
			}
		})
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
	ks, err := LoadOrGenerateKeyStore(p1, "", AlgEdDSA)
	if err != nil {
		t.Fatalf("failed to load valid primary: %v", err)
	}
	if ks.PrimaryKID() == "" {
		t.Fatal("expected primary kid to be set")
	}

	// 2. Valid primary and secondary
	ks, err = LoadOrGenerateKeyStore(p1, p2, AlgEdDSA)
	if err != nil {
		t.Fatalf("failed to load valid primary and secondary: %v", err)
	}

	// 3. Valid primary, invalid secondary (should log warning but succeed)
	ks, err = LoadOrGenerateKeyStore(p1, "/nonexistent/sec.pem", AlgEdDSA)
	if err != nil {
		t.Fatalf("failed to load keystore when secondary is invalid: %v", err)
	}

	// 4. Invalid primary (should fail)
	_, err = LoadOrGenerateKeyStore("/nonexistent/primary.pem", "", AlgEdDSA)
	if err == nil {
		t.Fatal("expected error for nonexistent primary key file")
	}
}

func TestLoadOrGenerateKeyStoreEphemeralAlgorithms(t *testing.T) {
	for _, alg := range []string{AlgEdDSA, AlgES256, AlgRS256} {
		t.Run(alg, func(t *testing.T) {
			ks, err := LoadOrGenerateKeyStore("", "", alg)
			if err != nil {
				t.Fatalf("generate ephemeral %s keystore: %v", alg, err)
			}
			if ks.PrimaryAlg() != alg {
				t.Fatalf("PrimaryAlg=%q want=%q", ks.PrimaryAlg(), alg)
			}
			if ks.SigningMethod().Alg() != alg {
				t.Fatalf("SigningMethod=%q want=%q", ks.SigningMethod().Alg(), alg)
			}
		})
	}

	if _, err := LoadOrGenerateKeyStore("", "", "HS256"); err == nil {
		t.Fatal("expected error for unsupported algorithm")
	}
}

func TestKeyToJWK(t *testing.T) {
	t.Run("Ed25519", func(t *testing.T) {
		key := mustGenerateEd25519Key(t)
		jwk := keyToJWK("kid-test", keyEntry{signer: key, alg: AlgEdDSA})

		if jwk.Kid != "kid-test" {
			t.Fatalf("kid=%q want=kid-test", jwk.Kid)
		}
		if jwk.Kty != "OKP" || jwk.Crv != "Ed25519" || jwk.Alg != "EdDSA" {
			t.Fatalf("unexpected jwk metadata: %+v", jwk)
		}
		if jwk.X == "" {
			t.Fatalf("expected non-empty x in jwk: %+v", jwk)
		}
	})

	t.Run("ECDSA-P256", func(t *testing.T) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("generate ecdsa key: %v", err)
		}
		jwk := keyToJWK("kid-ec", keyEntry{signer: key, alg: AlgES256})

		if jwk.Kty != "EC" || jwk.Crv != "P-256" || jwk.Alg != "ES256" {
			t.Fatalf("unexpected jwk metadata: %+v", jwk)
		}
		if jwk.X == "" || jwk.Y == "" {
			t.Fatalf("expected x and y coordinates in jwk: %+v", jwk)
		}
	})

	t.Run("RSA", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate rsa key: %v", err)
		}
		jwk := keyToJWK("kid-rsa", keyEntry{signer: key, alg: AlgRS256})

		if jwk.Kty != "RSA" || jwk.Alg != "RS256" {
			t.Fatalf("unexpected jwk metadata: %+v", jwk)
		}
		if jwk.N == "" || jwk.E == "" {
			t.Fatalf("expected n and e in jwk: %+v", jwk)
		}
	})
}
