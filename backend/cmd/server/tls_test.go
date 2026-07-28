package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// selfSignedCert generates an in-memory self-signed certificate for TLS tests.
func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "zerotrust-test"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// dialAndGetCurve performs a TLS handshake against a local listener configured
// with serverTLSConfig and returns the negotiated curve.
func dialAndGetCurve(t *testing.T, clientConfig *tls.Config) tls.CurveID {
	t.Helper()

	serverConfig := serverTLSConfig()
	serverConfig.Certificates = []tls.Certificate{selfSignedCert(t)}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverConfig)
	if err != nil {
		t.Fatalf("tls listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Complete the handshake, then hold briefly so the client can read state.
		if tlsConn, ok := conn.(*tls.Conn); ok {
			_ = tlsConn.Handshake()
		}
		time.Sleep(100 * time.Millisecond)
	}()

	conn, err := tls.Dial("tcp", ln.Addr().String(), clientConfig)
	if err != nil {
		t.Fatalf("tls dial: %v", err)
	}
	defer conn.Close()

	return conn.ConnectionState().CurveID
}

// TestHybridTLSNegotiatesMLKEM verifies that the server's TLS configuration
// negotiates the hybrid post-quantum key exchange X25519MLKEM768 (draft-ietf-tls-ecdhe-mlkem)
// with a default Go client.
func TestHybridTLSNegotiatesMLKEM(t *testing.T) {
	curve := dialAndGetCurve(t, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // test-only, no verification needed
	if curve != tls.X25519MLKEM768 {
		t.Fatalf("negotiated curve=%v want X25519MLKEM768(%v)", curve, tls.X25519MLKEM768)
	}
}

// TestHybridTLSFallsBackToClassical verifies that clients without ML-KEM
// support still connect using a classical curve.
func TestHybridTLSFallsBackToClassical(t *testing.T) {
	curve := dialAndGetCurve(t, &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // test-only
		CurvePreferences:   []tls.CurveID{tls.X25519},
	})
	if curve != tls.X25519 {
		t.Fatalf("negotiated curve=%v want X25519(%v)", curve, tls.X25519)
	}
}
