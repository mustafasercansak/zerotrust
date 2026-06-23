package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"crypto/ed25519"

	"github.com/golang-jwt/jwt/v5"
)

type DPoPClaims struct {
	Jti string `json:"jti"`
	Htm string `json:"htm"`
	Htu string `json:"htu"`
	Iat int64  `json:"iat"`
	jwt.RegisteredClaims
}

// expectedDPoPOrigin, when set (e.g. "https://api.example.com"), requires a
// DPoP proof's htu to carry exactly that scheme+host in addition to the
// expected path. When empty (default), only the path is validated — preserving
// local/dev behavior where the external origin is not known. (ISSUE_LIST #36)
var expectedDPoPOrigin string

// SetExpectedDPoPOrigin configures the required scheme+host for DPoP htu
// validation. Pass an absolute origin with no trailing slash, or "" to disable
// host binding. Intended to be called once at startup.
func SetExpectedDPoPOrigin(origin string) {
	expectedDPoPOrigin = strings.TrimRight(strings.TrimSpace(origin), "/")
}

// ValidateDPoPProof verifies a DPoP proof and returns the JWK thumbprint (jkt).
// It is a thin wrapper over ValidateDPoPProofWithJTI for callers that do not
// need replay protection.
func ValidateDPoPProof(tokenStr string, expectedMethod string, expectedURI string) (string, error) {
	jkt, _, err := ValidateDPoPProofWithJTI(tokenStr, expectedMethod, expectedURI)
	return jkt, err
}

// ValidateDPoPProofWithJTI verifies a DPoP proof and returns both the JWK
// thumbprint (jkt) and the proof's unique identifier (jti). The jti enables
// replay protection at the call site (see Service.ConsumeDPoPProof, ISSUE_LIST #35).
func ValidateDPoPProofWithJTI(tokenStr string, expectedMethod string, expectedURI string) (string, string, error) {
	if tokenStr == "" {
		return "", "", errors.New("empty DPoP token")
	}

	var jwkHeader map[string]any

	token, err := jwt.ParseWithClaims(tokenStr, &DPoPClaims{}, func(t *jwt.Token) (any, error) {
		jwkVal, ok := t.Header["jwk"]
		if !ok {
			return nil, errors.New("missing jwk in header")
		}
		jwkMap, ok := jwkVal.(map[string]any)
		if !ok {
			return nil, errors.New("invalid jwk in header")
		}
		jwkHeader = jwkMap

		kty, _ := jwkMap["kty"].(string)
		switch kty {
		case "OKP":
			crv, _ := jwkMap["crv"].(string)
			xStr, _ := jwkMap["x"].(string)
			if crv != "Ed25519" || xStr == "" {
				return nil, errors.New("invalid OKP parameters")
			}
			xBytes, err := base64.RawURLEncoding.DecodeString(xStr)
			if err != nil {
				return nil, err
			}
			return ed25519.PublicKey(xBytes), nil
		case "EC":
			crv, _ := jwkMap["crv"].(string)
			xStr, _ := jwkMap["x"].(string)
			yStr, _ := jwkMap["y"].(string)
			if xStr == "" || yStr == "" {
				return nil, errors.New("invalid EC parameters")
			}
			xBytes, err := base64.RawURLEncoding.DecodeString(xStr)
			if err != nil {
				return nil, err
			}
			yBytes, err := base64.RawURLEncoding.DecodeString(yStr)
			if err != nil {
				return nil, err
			}
			var curve elliptic.Curve
			switch crv {
			case "P-256":
				curve = elliptic.P256()
			default:
				return nil, fmt.Errorf("unsupported curve: %s", crv)
			}
			pubKey := &ecdsa.PublicKey{
				Curve: curve,
				X:     new(big.Int).SetBytes(xBytes),
				Y:     new(big.Int).SetBytes(yBytes),
			}
			return pubKey, nil
		default:
			return nil, fmt.Errorf("unsupported key type: %s", kty)
		}
	})

	if err != nil {
		return "", "", err
	}

	typ, ok := token.Header["typ"].(string)
	if !ok || !strings.EqualFold(typ, "dpop+jwt") {
		return "", "", fmt.Errorf("invalid typ header: %s", typ)
	}

	claims, ok := token.Claims.(*DPoPClaims)
	if !ok || !token.Valid {
		return "", "", errors.New("invalid token claims")
	}

	if claims.Jti == "" {
		return "", "", errors.New("missing jti claim in DPoP proof")
	}

	now := time.Now().Unix()
	// Allow 2-minute skew
	if claims.Iat < now-120 || claims.Iat > now+120 {
		return "", "", errors.New("DPoP proof expired or issued in the future")
	}

	if !strings.EqualFold(claims.Htm, expectedMethod) {
		return "", "", fmt.Errorf("htm mismatch: got %q, expected %q", claims.Htm, expectedMethod)
	}

	if !validateHTU(claims.Htu, expectedURI, expectedDPoPOrigin) {
		return "", "", fmt.Errorf("htu mismatch: got %q, expected path %q (origin %q)", claims.Htu, expectedURI, expectedDPoPOrigin)
	}

	jkt, err := CalculateJKT(jwkHeader)
	if err != nil {
		return "", "", fmt.Errorf("calculate JKT failed: %w", err)
	}

	return jkt, claims.Jti, nil
}

// validateHTU checks a DPoP proof's htu claim against the expected request path
// and, when expectedOrigin is non-empty, the expected scheme+host. The path
// must match exactly (no suffix fallback). When expectedOrigin is set, the htu
// must be an absolute URL whose origin matches exactly. (ISSUE_LIST #36)
func validateHTU(htu, expectedPath, expectedOrigin string) bool {
	gotOrigin, gotPath := splitHTU(htu)
	if gotPath != expectedPath {
		return false
	}
	if expectedOrigin == "" {
		return true
	}
	// Host binding is enabled: a bare-path htu (empty origin) is rejected.
	return strings.EqualFold(gotOrigin, expectedOrigin)
}

// splitHTU separates an htu value into its origin (scheme://host) and path,
// stripping any query string or fragment. A value without a scheme+host is
// treated as a bare path with an empty origin.
func splitHTU(htu string) (origin, path string) {
	u, err := url.Parse(htu)
	if err != nil || u.Scheme == "" || u.Host == "" {
		// Not an absolute URL — treat the whole value as a path, minus any
		// query/fragment.
		path = htu
		if i := strings.IndexAny(path, "?#"); i != -1 {
			path = path[:i]
		}
		return "", path
	}
	path = u.Path
	if path == "" {
		path = "/"
	}
	return u.Scheme + "://" + u.Host, path
}

func CalculateJKT(jwk map[string]any) (string, error) {
	kty, _ := jwk["kty"].(string)
	var jwkJSON string
	switch kty {
	case "OKP":
		crv, _ := jwk["crv"].(string)
		x, _ := jwk["x"].(string)
		jwkJSON = fmt.Sprintf(`{"crv":%q,"kty":%q,"x":%q}`, crv, kty, x)
	case "EC":
		crv, _ := jwk["crv"].(string)
		x, _ := jwk["x"].(string)
		y, _ := jwk["y"].(string)
		jwkJSON = fmt.Sprintf(`{"crv":%q,"kty":%q,"x":%q,"y":%q}`, crv, kty, x, y)
	default:
		return "", fmt.Errorf("unsupported kty for jkt: %s", kty)
	}

	h := sha256.Sum256([]byte(jwkJSON))
	return base64.RawURLEncoding.EncodeToString(h[:]), nil
}

// GenerateDPoPProofForTest generates a valid DPoP proof JWT for test verification.
func GenerateDPoPProofForTest(privKey ed25519.PrivateKey, method, uri string) (string, error) {
	pub := privKey.Public().(ed25519.PublicKey)
	jwk := map[string]any{
		"kty": "OKP",
		"crv": "Ed25519",
		"x":   base64.RawURLEncoding.EncodeToString(pub),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, &DPoPClaims{
		Jti: "test-jti-" + hex.EncodeToString(privKey[:4]),
		Htm: method,
		Htu: uri,
		Iat: time.Now().Unix(),
	})

	token.Header["typ"] = "dpop+jwt"
	token.Header["jwk"] = jwk

	return token.SignedString(privKey)
}

func DPoPRequiredMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dpop := r.Header.Get("DPoP")
		if dpop == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid_dpop_proof"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
