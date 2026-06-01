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

func ValidateDPoPProof(tokenStr string, expectedMethod string, expectedURI string) (string, error) {
	if tokenStr == "" {
		return "", errors.New("empty DPoP token")
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
		return "", err
	}

	typ, ok := token.Header["typ"].(string)
	if !ok || !strings.EqualFold(typ, "dpop+jwt") {
		return "", fmt.Errorf("invalid typ header: %s", typ)
	}

	claims, ok := token.Claims.(*DPoPClaims)
	if !ok || !token.Valid {
		return "", errors.New("invalid token claims")
	}

	now := time.Now().Unix()
	// Allow 2-minute skew
	if claims.Iat < now-120 || claims.Iat > now+120 {
		return "", errors.New("DPoP proof expired or issued in the future")
	}

	if !strings.EqualFold(claims.Htm, expectedMethod) {
		return "", fmt.Errorf("htm mismatch: got %q, expected %q", claims.Htm, expectedMethod)
	}

	if !validateHTU(claims.Htu, expectedURI) {
		return "", fmt.Errorf("htu mismatch: got %q, expected path suffix %q", claims.Htu, expectedURI)
	}

	jkt, err := CalculateJKT(jwkHeader)
	if err != nil {
		return "", fmt.Errorf("calculate JKT failed: %w", err)
	}

	return jkt, nil
}

func validateHTU(htu, expectedPath string) bool {
	if strings.Contains(htu, "://") {
		parts := strings.SplitN(htu, "://", 2)
		if len(parts) == 2 {
			slashIdx := strings.Index(parts[1], "/")
			if slashIdx != -1 {
				path := parts[1][slashIdx:]
				// strip query/fragment if any
				if qIdx := strings.IndexAny(path, "?#"); qIdx != -1 {
					path = path[:qIdx]
				}
				return path == expectedPath
			}
		}
	}
	return strings.HasSuffix(htu, expectedPath)
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
