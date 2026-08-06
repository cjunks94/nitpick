package ghapp

import (
	"strings"
	"testing"
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
			body: func(tok string) string { return `{"token":"` + tok + `"}` },
		},
		{
			name:  "refresh token",
			token: fakeToken("ghr_"),
			body: func(tok string) string { return `{"refresh_token":"` + tok + `"}` },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactTokens([]byte(tt.body(tt.token)), 500)
			if strings.Contains(got, tt.token) {
				t.Errorf("token survived redaction: %s", got)
			}
			if !strings.Contains(got, "[REDACTED]") {
				t.Errorf("expected a [REDACTED] marker, got: %s", got)
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
