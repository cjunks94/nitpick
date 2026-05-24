package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifySignature(t *testing.T) {
	secret := "topsecret"
	payload := []byte(`{"action":"opened"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validHeader := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{"valid", validHeader, true},
		{"bad signature", "sha256=" + hex.EncodeToString(make([]byte, 32)), false},
		{"wrong prefix", "sha1=" + hex.EncodeToString(mac.Sum(nil)), false},
		{"no prefix", hex.EncodeToString(mac.Sum(nil)), false},
		{"non-hex", "sha256=zzz", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifySignature(payload, tt.header, secret); got != tt.want {
				t.Errorf("VerifySignature(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}
