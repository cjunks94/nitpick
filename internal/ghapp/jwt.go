// Package ghapp handles GitHub App authentication: minting the App-level JWT
// from the private key, exchanging it for per-installation tokens, and caching
// those tokens. Used by `nitpick serve` to act as the App when receiving
// webhooks and posting reviews back.
package ghapp

import (
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// MintAppJWT creates a short-lived JWT signed with the App's private key.
// The JWT identifies nitpick as a GitHub App and is the first step before
// exchanging for a per-installation access token. Issued for the past 60s
// (clock-skew tolerance per GitHub docs) and valid for the next 9 minutes
// (GitHub allows up to 10 — we leave headroom).
//
// The private key PEM comes from the App's Settings page in GitHub. Stored
// as the GITHUB_APP_PRIVATE_KEY env var on Railway.
func MintAppJWT(appID string, privateKey *rsa.PrivateKey) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
		Issuer:    appID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("sign app JWT: %w", err)
	}
	return signed, nil
}

// ParsePrivateKeyPEM decodes a PEM-encoded RSA private key. Helper for env-
// var loading: Railway lets you paste multi-line secrets so the raw PEM
// (with -----BEGIN/END----- delimiters and newlines) works directly.
func ParsePrivateKeyPEM(pem []byte) (*rsa.PrivateKey, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM(pem)
	if err != nil {
		return nil, fmt.Errorf("parse RSA private key: %w", err)
	}
	return key, nil
}
