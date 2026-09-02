// Package secrets keeps credentials out of the payload nitpick sends to the
// LLM provider.
//
// Two distinct exposures, both of which existed before this package:
//
//   - The DIFF. Every changed line goes to the provider. A PR that commits a
//     .env sends its contents verbatim. The only guard was review.ignore_paths
//     in .nitpick.yaml, which is opt-in — so the default configuration leaked.
//   - CONTEXT FILES. `serve` fetches whole files touched by the diff. The deny
//     list there was built for generated-file noise (.uid, lockfiles); it says
//     nothing about credentials.
//
// The policy is deliberately asymmetric:
//
//   - In the DIFF, a secrets-shaped file keeps its structure with contents
//     replaced. "You committed a .env" is arguably the single most valuable
//     finding nitpick can produce, and dropping the file silently would throw
//     that away. The model sees the path and the shape; it never sees the
//     values.
//   - In CONTEXT, a secrets-shaped file is dropped outright. Context exists to
//     explain surrounding code, and a credentials file explains nothing — it
//     is all risk and no review signal.
//
// Redaction is strictly line-by-line and never adds or removes a line.
// Findings anchor on new-file line numbers, so any transformation that shifted
// them would move every comment below it onto the wrong code.
package secrets

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cjunks94/nitpick/internal/diff"
)

// Placeholder replaces a matched secret. Recognisable in a prompt and in logs.
const Placeholder = "[REDACTED-BY-NITPICK]"

// sensitiveBasenames are files whose entire purpose is to hold credentials.
// Matched case-insensitively against the path's final element.
var sensitiveBasenames = map[string]bool{
	".env":             true,
	".envrc":           true,
	".netrc":           true,
	"_netrc":           true,
	".npmrc":           true,
	".pypirc":          true,
	".htpasswd":        true,
	".pgpass":          true,
	"credentials":      true, // ~/.aws/credentials
	"secrets.yaml":     true,
	"secrets.yml":      true,
	"secrets.json":     true,
	"id_rsa":           true,
	"id_dsa":           true,
	"id_ecdsa":         true,
	"id_ed25519":       true,
	"terraform.tfvars": true,
}

// envSuffixes are deployment-environment names that make a dotless "env.X"
// file a credentials file rather than a source file. Kept deliberately narrow:
// treating any "env.*" as sensitive would mask env.go, env.ts, env.py and
// friends, costing real review signal on every PR that touches them.
var envSuffixes = map[string]bool{
	"local":       true,
	"development": true,
	"dev":         true,
	"test":        true,
	"testing":     true,
	"staging":     true,
	"stage":       true,
	"production":  true,
	"prod":        true,
	"ci":          true,
	"secret":      true,
	"secrets":     true,
}

// sensitiveExtensions are suffixes that carry key material. ".crt"/".cer" are
// deliberately absent: certificates are public by design, and denying them
// would cost review signal for no gain.
var sensitiveExtensions = []string{
	".pem",
	".key",
	".p12",
	".pfx",
	".jks",
	".keystore",
	".asc",
	".gpg",
	".kdbx",
}

// IsSensitivePath reports whether a repository path is a credentials file by
// convention. Case-insensitive; forward-slash separated (GitHub diff paths
// always are, regardless of host OS).
//
// The ".env" family is prefix-matched so ".env.local", ".env.production", and
// ".env.staging.local" are all covered, while a source file that merely starts
// with "env" (e.g. "environment.go") is not.
func IsSensitivePath(path string) bool {
	if path == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(filepath.FromSlash(path)))

	if sensitiveBasenames[base] {
		return true
	}
	// .env, .env.local, .env.production.local, ...
	//
	// .env.example is included even though it is committed on purpose: the
	// convention is placeholders, but real values land in it often enough
	// that masking is the right default. The cost is one template file's
	// values, which carry no review signal anyway.
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	// The dotless form (env.production) is a real convention, but it cannot
	// be matched on the "env." prefix alone — that would swallow env.go,
	// env.ts, env.py and every other source file named for the package it
	// holds. Only recognised environment names count.
	if rest, ok := strings.CutPrefix(base, "env."); ok && envSuffixes[rest] {
		return true
	}
	for _, ext := range sensitiveExtensions {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}
	// Anything under a .ssh/ or .gnupg/ directory.
	lower := strings.ToLower(path)
	if strings.Contains(lower, ".ssh/") || strings.Contains(lower, ".gnupg/") {
		return true
	}
	return false
}

// pattern is one secret shape plus a human label for logging.
type pattern struct {
	name string
	re   *regexp.Regexp
}

// patterns are high-confidence, provider-specific credential formats. Vendor
// prefixes make these nearly false-positive-free, which matters: an
// over-eager redactor that mangles ordinary code costs review quality on every
// PR, whereas a missed exotic secret costs nothing that wasn't already broken.
//
// The generic key=value rule at the end is the one judgement call; it requires
// a credential-ish key, a quoted value, and >=12 characters of it.
var patterns = []pattern{
	{"github-token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{16,}`)},
	{"github-fine-grained", regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`)},
	{"anthropic-key", regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]{20,}`)},
	{"openai-key", regexp.MustCompile(`sk-(?:proj-)?[A-Za-z0-9_\-]{32,}`)},
	{"aws-access-key-id", regexp.MustCompile(`(?:AKIA|ASIA|AGPA|AIDA)[0-9A-Z]{16}`)},
	{"google-api-key", regexp.MustCompile(`AIza[0-9A-Za-z_\-]{35}`)},
	{"slack-token", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9\-]{10,}`)},
	{"stripe-key", regexp.MustCompile(`(?:sk|rk)_live_[A-Za-z0-9]{16,}`)},
	{"sendgrid-key", regexp.MustCompile(`SG\.[A-Za-z0-9_\-]{20,}\.[A-Za-z0-9_\-]{20,}`)},
	{"npm-token", regexp.MustCompile(`npm_[A-Za-z0-9]{36}`)},
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`)},
	{"basic-auth-url", regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.\-]*://[^\s/:@]+:[^\s/@]{4,}@`)},
	{
		"generic-credential-assignment",
		regexp.MustCompile(`(?i)\b(?:api[_\-]?key|apikey|secret|password|passwd|token|access[_\-]?key|private[_\-]?key|client[_\-]?secret)\b\s*[:=]\s*["'][^"'\n]{12,}["']`),
	},
}

// beginPrivateKey opens a PEM private-key block. The body is base64 spread
// over many lines, so it is handled by a line-state machine rather than a
// multiline regex — collapsing it to one line would shift every line number
// below it.
var (
	beginPrivateKey = regexp.MustCompile(`(?i)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)
	endPrivateKey   = regexp.MustCompile(`(?i)-----END [A-Z0-9 ]*PRIVATE KEY-----`)
)

// RedactLine masks any credential found in a single line and reports whether
// anything was replaced. The line count is unchanged by construction.
//
// Not sufficient on its own for PEM blocks, whose body spans lines with no
// individually-recognisable content; use Redactor for multi-line input.
func RedactLine(line string) (string, bool) {
	out := line
	for _, p := range patterns {
		if p.re.MatchString(out) {
			out = p.re.ReplaceAllString(out, Placeholder)
		}
	}
	return out, out != line
}

// Redactor walks lines in order, carrying the state needed for constructs that
// span lines (PEM blocks). Use one per file or per hunk; do not share across
// unrelated inputs, since an unterminated block would bleed into the next.
type Redactor struct {
	inPrivateKey bool
	// Count is the number of lines altered, for logging. Not a secret count —
	// one line may hold several.
	Count int
}

// Line redacts a single line, honouring any PEM block currently open.
func (r *Redactor) Line(line string) string {
	if r.inPrivateKey {
		if endPrivateKey.MatchString(line) {
			r.inPrivateKey = false
		}
		r.Count++
		return Placeholder
	}
	if beginPrivateKey.MatchString(line) {
		// A single-line "BEGIN...END" (rare but legal) closes immediately.
		if !endPrivateKey.MatchString(line) {
			r.inPrivateKey = true
		}
		r.Count++
		return Placeholder
	}
	out, changed := RedactLine(line)
	if changed {
		r.Count++
	}
	return out
}

// RedactBytes redacts a whole file, preserving line structure and the presence
// or absence of a trailing newline. Returns the result and the number of lines
// altered.
func RedactBytes(b []byte) ([]byte, int) {
	if len(b) == 0 {
		return b, 0
	}
	s := string(b)
	hadTrailingNewline := strings.HasSuffix(s, "\n")
	if hadTrailingNewline {
		s = strings.TrimSuffix(s, "\n")
	}
	var r Redactor
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = r.Line(line)
	}
	out := strings.Join(lines, "\n")
	if hadTrailingNewline {
		out += "\n"
	}
	return []byte(out), r.Count
}

// SanitizeHunks redacts credentials from diff content in place of the caller's
// slice, returning the cleaned hunks, the number of lines altered, and the
// number of distinct files affected.
//
// Line counts are never changed. Findings anchor on new-file line numbers, so
// adding or removing a line here would silently move every comment below it
// onto the wrong code.
//
// A fresh Redactor is used per file so an unterminated PEM block in
// one file cannot bleed into the next.
func SanitizeHunks(hunks []diff.Hunk) ([]diff.Hunk, int, int) {
	if len(hunks) == 0 {
		return hunks, 0, 0
	}
	out := make([]diff.Hunk, 0, len(hunks))
	totalLines := 0
	affected := make(map[string]bool)

	redactors := make(map[string]*Redactor, len(hunks))
	for _, h := range hunks {
		sensitive := IsSensitivePath(h.File)
		r := redactors[h.File]
		if r == nil {
			r = &Redactor{}
			redactors[h.File] = r
		}

		lines := make([]diff.HunkLine, len(h.Lines))
		copy(lines, h.Lines)
		for i := range lines {
			before := lines[i].Content
			if sensitive {
				// Whole file is credentials: keep the line so the model can
				// still flag that the file was committed, drop the value.
				if strings.TrimSpace(before) != "" {
					lines[i].Content = Placeholder
				}
			} else {
				lines[i].Content = r.Line(before)
			}
			if lines[i].Content != before {
				totalLines++
				affected[h.File] = true
			}
		}
		h.Lines = lines
		out = append(out, h)
	}
	return out, totalLines, len(affected)
}
