package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// VerifySignature checks the GitHub webhook X-Hub-Signature-256 header against
// the shared secret. Returns true if valid. Uses constant-time comparison to
// avoid timing leaks. Header format: "sha256=<hex>".
func VerifySignature(payload []byte, signatureHeader, secret string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(signatureHeader, prefix) {
		return false
	}
	want, err := hex.DecodeString(signatureHeader[len(prefix):])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	got := mac.Sum(nil)
	return hmac.Equal(want, got)
}
