package secrets

import (
	"strings"
	"testing"

	"github.com/cjunks94/nitpick/internal/diff"
)

// tok assembles credential fixtures at runtime. Written as literals they trip
// the gitleaks job in CI, and an inline `gitleaks:allow` would establish a
// pattern for silencing the scanner that could later hide a real secret.
func tok(prefix, body string) string { return prefix + body }

// pemBegin/pemEnd assemble PEM markers for the same reason as tok: written
// whole they match gitleaks' private-key rule, and this file's whole job is to
// hold credential-shaped fixtures.
func pemBegin(kind string) string {
	if kind != "" {
		kind += " "
	}
	return "-----BEGIN " + kind + "PRIVATE" + " KEY-----"
}

func pemEnd(kind string) string {
	if kind != "" {
		kind += " "
	}
	return "-----END " + kind + "PRIVATE" + " KEY-----"
}

func TestIsSensitivePath(t *testing.T) {
	sensitive := []string{
		".env",
		".env.local",
		".env.production.local",
		"app/.env",
		"config/env.staging",
		".envrc",
		"deploy/id_rsa",
		"certs/server.key",
		"certs/private.pem",
		"keystore.jks",
		"secrets.yaml",
		"infra/terraform.tfvars",
		"home/.ssh/config",
		".npmrc",
		"CREDENTIALS",      // case-insensitive
		"Certs/Server.PEM", // case-insensitive extension
	}
	for _, p := range sensitive {
		if !IsSensitivePath(p) {
			t.Errorf("IsSensitivePath(%q) = false, want true", p)
		}
	}

	// False positives here cost real review signal on every PR, so the
	// near-misses matter as much as the hits.
	safe := []string{
		"",
		"main.go",
		"internal/env/env.go",
		"environment.go",
		"envelope.ts",
		"env.go",
		"config/env.ts",
		"internal/env/env.py",
		"src/environment/config.ts",
		"README.md",
		"certs/server.crt", // certificates are public by design
		"certs/ca.cer",
		"docs/keyboard.md",
		"pkg/monkey.go", // ends in "key" but not ".key"
		".github/workflows/security.yml",
	}
	for _, p := range safe {
		if IsSensitivePath(p) {
			t.Errorf("IsSensitivePath(%q) = true, want false", p)
		}
	}
}

func TestRedactLine_KnownCredentialFormats(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		secret string
	}{
		{"github token", `token := "` + tok("ghp_", "abcdefghijklmnopqrstuvwxyz0123456789") + `"`, tok("ghp_", "abcdefghijklmnopqrstuvwxyz0123456789")},
		{"anthropic key", tok("sk-ant-", "api03-AAAABBBBCCCCDDDDEEEEFFFF"), tok("sk-ant-", "api03-AAAABBBBCCCCDDDDEEEEFFFF")},
		{"aws access key", `aws_access_key_id = ` + tok("AKIA", "IOSFODNN7EXAMPLE"), tok("AKIA", "IOSFODNN7EXAMPLE")},
		{"google api key", `key: ` + tok("AIza", "SyD-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), tok("AIza", "SyD-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")},
		{"slack token", tok("xoxb-", "1234567890-abcdefghijkl"), tok("xoxb-", "1234567890-abcdefghijkl")},
		{"stripe live key", tok("sk_live_", "4eC39HqLyjWDarjtT1zdp7dc"), tok("sk_live_", "4eC39HqLyjWDarjtT1zdp7dc")},
		{"npm token", tok("npm_", "abcdefghijklmnopqrstuvwxyz0123456789"), tok("npm_", "abcdefghijklmnopqrstuvwxyz0123456789")},
		{"basic auth in url", `db := "postgres://user:hunter2pass@localhost/db"`, "hunter2pass"},
		{"generic password assignment", `password = "correcthorsebatterystaple"`, "correcthorsebatterystaple"},
		{"generic api_key assignment", `api_key: "` + tok("abcdef1234", "567890abcdef") + `"`, tok("abcdef1234", "567890abcdef")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := RedactLine(tt.line)
			if !changed {
				t.Fatalf("no redaction applied to %q", tt.line)
			}
			if strings.Contains(got, tt.secret) {
				t.Errorf("secret survived redaction:\n in: %s\nout: %s", tt.line, got)
			}
			if !strings.Contains(got, Placeholder) {
				t.Errorf("expected a placeholder, got: %s", got)
			}
		})
	}
}

// An over-eager redactor mangles ordinary code on every PR, which costs more
// than it saves. These must pass through untouched.
func TestRedactLine_LeavesOrdinaryCodeAlone(t *testing.T) {
	safe := []string{
		`func TokenSource(ctx context.Context) (string, error) {`,
		`// the api_key is read from the environment`,
		`password := os.Getenv("DB_PASSWORD")`,
		`if err != nil { return fmt.Errorf("token: %w", err) }`,
		`secret = ""`,
		`token: cfg.Token,`,
		`const maxTokens = 4096`,
		`api_key = "short"`, // below the 12-char floor
		`https://api.github.com/repos/owner/name`,
		`sk-too-short`,
		`Authorization: Bearer <token>`,
	}
	for _, line := range safe {
		got, changed := RedactLine(line)
		if changed {
			t.Errorf("ordinary code was redacted:\n in: %s\nout: %s", line, got)
		}
	}
}

// Line numbers are the anchor for every finding. A redaction that added or
// removed a line would move every comment below it onto the wrong code.
func TestRedactBytes_PreservesLineCount(t *testing.T) {
	in := "package main\n" +
		"\n" +
		"const key = \"" + tok("ghp_", "abcdefghijklmnopqrstuvwxyz0123456789") + "\"\n" +
		"\n" +
		pemBegin("RSA") + "\n" +
		"MIIEowIBAAKCAQEAxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n" +
		"yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy\n" +
		pemEnd("RSA") + "\n" +
		"func main() {}\n"

	out, n := RedactBytes([]byte(in))
	if n == 0 {
		t.Fatal("expected redactions")
	}
	inLines := strings.Count(in, "\n")
	outLines := strings.Count(string(out), "\n")
	if inLines != outLines {
		t.Errorf("line count changed: in=%d out=%d — findings would anchor on the wrong lines", inLines, outLines)
	}
	s := string(out)
	if strings.Contains(s, "MIIEowIBAAKCAQEA") {
		t.Error("PEM key body survived redaction")
	}
	if !strings.Contains(s, "package main") || !strings.Contains(s, "func main() {}") {
		t.Error("surrounding code was destroyed; only secrets should be replaced")
	}
}

func TestRedactBytes_PreservesTrailingNewlinePresence(t *testing.T) {
	withNL, _ := RedactBytes([]byte("a\nb\n"))
	if string(withNL) != "a\nb\n" {
		t.Errorf("got %q, want %q", withNL, "a\nb\n")
	}
	withoutNL, _ := RedactBytes([]byte("a\nb"))
	if string(withoutNL) != "a\nb" {
		t.Errorf("got %q, want %q", withoutNL, "a\nb")
	}
	empty, n := RedactBytes(nil)
	if len(empty) != 0 || n != 0 {
		t.Errorf("empty input should pass through, got %q / %d", empty, n)
	}
}

// A PEM block that never closes must not bleed into the next file.
func TestRedactor_UnterminatedPEMIsScopedToItsRedactor(t *testing.T) {
	var r1 Redactor
	r1.Line(pemBegin(""))
	r1.Line("AAAABBBBCCCC")
	if got := r1.Line("this is still swallowed"); got != Placeholder {
		t.Errorf("inside an open PEM block, got %q", got)
	}

	var r2 Redactor // fresh redactor == fresh file
	if got := r2.Line("package main"); got != "package main" {
		t.Errorf("new Redactor inherited PEM state: %q", got)
	}
}

func TestSanitizeHunks_SecretsFileKeepsShapeLosesValues(t *testing.T) {
	hunks := []diff.Hunk{{
		File: ".env",
		Lines: []diff.HunkLine{
			{Kind: diff.LineAdded, Content: "STRIPE_KEY=" + tok("sk_live_", "4eC39HqLyjWDarjtT1zdp7dc"), NewLineNum: 1},
			{Kind: diff.LineAdded, Content: "DEBUG=true", NewLineNum: 2},
			{Kind: diff.LineContext, Content: "", NewLineNum: 3},
		},
	}}

	got, lines, files := SanitizeHunks(hunks)
	if lines != 2 || files != 1 {
		t.Errorf("lines=%d files=%d, want 2 and 1", lines, files)
	}
	if len(got[0].Lines) != 3 {
		t.Fatalf("line count changed: %d, want 3", len(got[0].Lines))
	}
	// The path survives so the model can still flag "you committed a .env" —
	// the most valuable finding available on this diff.
	if got[0].File != ".env" {
		t.Errorf("file path was lost: %q", got[0].File)
	}
	for i, l := range got[0].Lines[:2] {
		if l.Content != Placeholder {
			t.Errorf("line %d not redacted: %q", i, l.Content)
		}
	}
	// Even non-secret values in a secrets file are masked; DEBUG=true is
	// harmless but the file is untrusted wholesale.
	if strings.Contains(got[0].Lines[1].Content, "DEBUG") {
		t.Error("non-secret line in a secrets file should still be masked")
	}
	// Blank lines stay blank rather than becoming placeholders.
	if got[0].Lines[2].Content != "" {
		t.Errorf("blank line became %q", got[0].Lines[2].Content)
	}
	// Line numbers must be untouched.
	for i, l := range got[0].Lines {
		if l.NewLineNum != i+1 {
			t.Errorf("line %d: NewLineNum = %d, want %d", i, l.NewLineNum, i+1)
		}
	}
}

func TestSanitizeHunks_OrdinaryFileRedactsOnlySecrets(t *testing.T) {
	hunks := []diff.Hunk{{
		File: "main.go",
		Lines: []diff.HunkLine{
			{Kind: diff.LineAdded, Content: `const k = "` + tok("ghp_", "abcdefghijklmnopqrstuvwxyz0123456789") + `"`, NewLineNum: 1},
			{Kind: diff.LineAdded, Content: "func main() {}", NewLineNum: 2},
		},
	}}

	got, lines, files := SanitizeHunks(hunks)
	if lines != 1 || files != 1 {
		t.Errorf("lines=%d files=%d, want 1 and 1", lines, files)
	}
	if strings.Contains(got[0].Lines[0].Content, "ghp_") {
		t.Errorf("token survived: %q", got[0].Lines[0].Content)
	}
	if got[0].Lines[1].Content != "func main() {}" {
		t.Errorf("ordinary line altered: %q", got[0].Lines[1].Content)
	}
}

// SanitizeHunks must not mutate the caller's slice — reviewPR logs counts from
// the original and a surprise in-place edit would be a nasty aliasing bug.
func TestSanitizeHunks_DoesNotMutateInput(t *testing.T) {
	original := `password = "correcthorsebatterystaple"`
	hunks := []diff.Hunk{{
		File:  "config.go",
		Lines: []diff.HunkLine{{Kind: diff.LineAdded, Content: original, NewLineNum: 1}},
	}}

	_, _, _ = SanitizeHunks(hunks)

	if hunks[0].Lines[0].Content != original {
		t.Errorf("input slice was mutated: %q", hunks[0].Lines[0].Content)
	}
}

func TestSanitizeHunks_Empty(t *testing.T) {
	got, lines, files := SanitizeHunks(nil)
	if got != nil || lines != 0 || files != 0 {
		t.Errorf("nil input should pass through, got %v / %d / %d", got, lines, files)
	}
}
