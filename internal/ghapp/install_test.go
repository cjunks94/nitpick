package ghapp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cjunks94/nitpick/internal/secrets"
	"github.com/golang-jwt/jwt/v5"
)

// fakeToken builds a syntactically valid GitHub token at runtime.
//
// Deliberately assembled rather than written as a string literal. A literal
// of this shape trips the gitleaks job in CI (correctly — it matches the
// github-app-token rule and clears the entropy threshold), and the obvious
// alternative, a `gitleaks:allow` comment, establishes a pattern in this repo
// that could later be used to hide a real credential. Building the value
// keeps the scanner at full strength over every file with no exemptions,
// while the test still exercises the real regex.
func fakeToken(prefix string) string {
	return prefix + strings.Repeat("x7Kq2Vm9", 4) // 32 chars; rule wants >= 16
}

// The installation-token endpoint's success body is {"token":"ghs_..."}.
// Non-201 responses shouldn't carry one, but this string is logged verbatim,
// so a GitHub change that returned 200 instead of 201 would previously have
// written a live credential straight into the log stream.
func TestRedactTokens(t *testing.T) {
	tests := []struct {
		name  string
		body  func(tok string) string
		token string
	}{
		{
			name:  "installation token",
			token: fakeToken("ghs_"),
			body: func(tok string) string {
				return `{"token":"` + tok + `","expires_at":"2026-01-01T00:00:00Z"}`
			},
		},
		{
			name:  "personal access token",
			token: fakeToken("ghp_"),
			body: func(tok string) string {
				return `{"message":"bad credentials for ` + tok + `"}`
			},
		},
		{
			name:  "oauth token",
			token: fakeToken("gho_"),
			body:  func(tok string) string { return `{"token":"` + tok + `"}` },
		},
		{
			name:  "refresh token",
			token: fakeToken("ghr_"),
			body:  func(tok string) string { return `{"refresh_token":"` + tok + `"}` },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactTokens([]byte(tt.body(tt.token)), 500)
			if strings.Contains(got, tt.token) {
				t.Errorf("token survived redaction: %s", got)
			}
			if !strings.Contains(got, secrets.Placeholder) {
				t.Errorf("expected the %s marker, got: %s", secrets.Placeholder, got)
			}
		})
	}
}

func TestRedactTokens_PreservesOrdinaryErrors(t *testing.T) {
	body := `{"message":"Not Found","documentation_url":"https://docs.github.com/rest"}`
	got := redactTokens([]byte(body), 500)
	if got != body {
		t.Errorf("ordinary error body was altered:\n got: %s\nwant: %s", got, body)
	}
}

func TestRedactTokens_TruncatesOnRuneBoundary(t *testing.T) {
	body := strings.Repeat("日", 200) // 600 bytes
	got := redactTokens([]byte(body), 100)
	if len(got) > 103 { // 100 bytes (rounded down to a boundary) + "..."
		t.Errorf("length = %d, want <= 103", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected a truncation marker, got %q", got)
	}
	trimmed := strings.TrimSuffix(got, "...")
	if len(trimmed)%3 != 0 {
		t.Errorf("truncated mid-rune: %d bytes is not a multiple of 3", len(trimmed))
	}
}

// tokenServer is a fake of POST /app/installations/{id}/access_tokens. It
// counts hits, records the Authorization header, and lets a test choose the
// status, expiry, and body it answers with.
type tokenServer struct {
	srv       *httptest.Server
	hits      atomic.Int32
	mu        sync.Mutex
	lastAuth  string
	lastPath  string
	status    int
	expiresIn time.Duration
	body      func(tok string) string // nil = documented success shape
}

func newTokenServer(t *testing.T) *tokenServer {
	t.Helper()
	ts := &tokenServer{status: http.StatusCreated, expiresIn: time.Hour}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		ts.hits.Add(1)
		ts.mu.Lock()
		ts.lastAuth = r.Header.Get("Authorization")
		ts.lastPath = r.URL.Path
		status, expiresIn, body := ts.status, ts.expiresIn, ts.body
		ts.mu.Unlock()

		tok := fakeToken("ghs_")
		w.WriteHeader(status)
		if body != nil {
			_, _ = io.WriteString(w, body(tok))
			return
		}
		_, _ = fmt.Fprintf(w, `{"token":%q,"expires_at":%q,"permissions":{"pull_requests":"write"}}`,
			tok, time.Now().Add(expiresIn).UTC().Format(time.RFC3339))
	}))
	t.Cleanup(ts.srv.Close)
	return ts
}

func (ts *tokenServer) set(status int, expiresIn time.Duration, body func(string) string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.status, ts.expiresIn, ts.body = status, expiresIn, body
}

func (ts *tokenServer) auth() string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.lastAuth
}

func (ts *tokenServer) path() string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.lastPath
}

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func newTestSource(t *testing.T, ts *tokenServer, key *rsa.PrivateKey) *InstallationTokenSource {
	t.Helper()
	src := NewInstallationTokenSource("12345", key)
	src.BaseURL = ts.srv.URL
	src.HTTPClient = ts.srv.Client()
	return src
}

func TestInstallationTokenSource_CachesUntilNearExpiry(t *testing.T) {
	ts := newTokenServer(t)
	src := newTestSource(t, ts, testKey(t))
	ctx := context.Background()

	first, err := src.Token(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if first != fakeToken("ghs_") {
		t.Fatalf("token = %q, want the server's token", first)
	}
	if got := ts.path(); got != "/app/installations/42/access_tokens" {
		t.Errorf("path = %q, want the installation's access_tokens endpoint", got)
	}

	second, err := src.Token(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Errorf("second call returned %q, want the cached %q", second, first)
	}
	if n := ts.hits.Load(); n != 1 {
		t.Errorf("server hit %d times for two calls on one installation, want 1 (cache miss)", n)
	}
}

// A token that expires inside the 5-minute safety margin is treated as
// already expired: a mid-request expiry is worse than one extra mint.
func TestInstallationTokenSource_RemintsInsideExpiryMargin(t *testing.T) {
	ts := newTokenServer(t)
	ts.set(http.StatusCreated, 4*time.Minute, nil)
	src := newTestSource(t, ts, testKey(t))
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if _, err := src.Token(ctx, 42); err != nil {
			t.Fatal(err)
		}
	}
	if n := ts.hits.Load(); n != 2 {
		t.Errorf("server hit %d times, want 2 — a token expiring in 4m is inside the 5m margin", n)
	}
}

func TestInstallationTokenSource_CacheIsPerInstallation(t *testing.T) {
	ts := newTokenServer(t)
	src := newTestSource(t, ts, testKey(t))
	ctx := context.Background()

	if _, err := src.Token(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Token(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if got := ts.path(); got != "/app/installations/2/access_tokens" {
		t.Errorf("second mint hit %q, want installation 2's endpoint", got)
	}
	if n := ts.hits.Load(); n != 2 {
		t.Errorf("server hit %d times, want 2 — installations must not share a cache slot", n)
	}
	// Both stay cached independently.
	if _, err := src.Token(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if n := ts.hits.Load(); n != 2 {
		t.Errorf("server hit %d times after re-reading installation 1, want 2", n)
	}
}

// A 200 with a token-shaped body is not a success: the error string is logged,
// so the token must be masked before it becomes part of the message.
func TestInstallationTokenSource_Non201WithTokenIsRedacted(t *testing.T) {
	ts := newTokenServer(t)
	ts.set(http.StatusOK, time.Hour, nil)
	src := newTestSource(t, ts, testKey(t))

	tok, err := src.Token(context.Background(), 42)
	if err == nil {
		t.Fatalf("expected an error for HTTP 200, got token %q", tok)
	}
	if tok != "" {
		t.Errorf("token = %q, want empty on error", tok)
	}
	msg := err.Error()
	if !strings.Contains(msg, "HTTP 200") {
		t.Errorf("error should name the status, got: %s", msg)
	}
	if !strings.Contains(msg, secrets.Placeholder) {
		t.Errorf("error should carry a redaction marker, got: %s", msg)
	}
	if strings.Contains(msg, fakeToken("ghs_")) {
		t.Errorf("live token leaked into the error string: %s", msg)
	}
	// Nothing is cached from a failed exchange.
	if _, err := src.Token(context.Background(), 42); err == nil {
		t.Error("second call succeeded; a failed exchange must not populate the cache")
	}
}

func TestInstallationTokenSource_BadBodies(t *testing.T) {
	tests := []struct {
		name    string
		body    func(tok string) string
		wantErr string
	}{
		{
			name:    "missing token field",
			body:    func(string) string { return `{"expires_at":"2030-01-01T00:00:00Z"}` },
			wantErr: "missing token field",
		},
		{
			name:    "empty token",
			body:    func(string) string { return `{"token":"","expires_at":"2030-01-01T00:00:00Z"}` },
			wantErr: "missing token field",
		},
		{
			name:    "malformed JSON",
			body:    func(string) string { return `{"token":` },
			wantErr: "parse installation token response",
		},
		{
			name: "body over the 64 KiB cap",
			body: func(tok string) string {
				return `{"token":"` + tok + `","padding":"` + strings.Repeat("x", maxTokenResponseBytes) + `"}`
			},
			wantErr: fmt.Sprintf("exceeds %d bytes", maxTokenResponseBytes),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTokenServer(t)
			ts.set(http.StatusCreated, time.Hour, tt.body)
			src := newTestSource(t, ts, testKey(t))

			tok, err := src.Token(context.Background(), 42)
			if err == nil {
				t.Fatalf("expected an error, got token %q", tok)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), fakeToken("ghs_")) {
				t.Errorf("token leaked into the error string: %v", err)
			}
		})
	}
}

// The exchange must be authenticated with an App JWT: RS256, signed by the
// App's private key, with the App ID as issuer.
func TestInstallationTokenSource_SendsSignedAppJWT(t *testing.T) {
	ts := newTokenServer(t)
	key := testKey(t)
	src := newTestSource(t, ts, key)

	if _, err := src.Token(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
	auth := ts.auth()
	if !strings.HasPrefix(auth, "Bearer ") {
		t.Fatalf("Authorization = %q, want a Bearer token", auth)
	}
	raw := strings.TrimPrefix(auth, "Bearer ")

	var claims jwt.RegisteredClaims
	parsed, err := jwt.ParseWithClaims(raw, &claims, func(tok *jwt.Token) (any, error) {
		return &key.PublicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}))
	if err != nil {
		t.Fatalf("JWT does not verify against the App key: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("JWT reported invalid")
	}
	if claims.Issuer != "12345" {
		t.Errorf("iss = %q, want the App ID", claims.Issuer)
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Fatalf("iat/exp missing: %+v", claims)
	}
	if ttl := claims.ExpiresAt.Sub(claims.IssuedAt.Time); ttl > 10*time.Minute {
		t.Errorf("JWT lifetime %v exceeds GitHub's 10-minute ceiling", ttl)
	}

	// A different key must NOT verify — the signature is load-bearing.
	other := testKey(t)
	if _, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{}, func(tok *jwt.Token) (any, error) {
		return &other.PublicKey, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()})); err == nil {
		t.Error("JWT verified against an unrelated key")
	}
}

func TestInstallationTokenSource_TransportError(t *testing.T) {
	ts := newTokenServer(t)
	src := newTestSource(t, ts, testKey(t))
	ts.srv.Close() // connection refused from here on

	_, err := src.Token(context.Background(), 42)
	if err == nil || !strings.Contains(err.Error(), "exchange JWT for installation token") {
		t.Fatalf("err = %v, want a wrapped transport error", err)
	}
}

func TestParsePrivateKeyPEM(t *testing.T) {
	key := testKey(t)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	got, err := ParsePrivateKeyPEM(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(key) {
		t.Error("parsed key differs from the generated one")
	}
	if _, err := ParsePrivateKeyPEM([]byte("not a pem")); err == nil {
		t.Error("expected an error for garbage input")
	}
}
