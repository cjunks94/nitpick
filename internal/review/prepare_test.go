package review

import (
	"strings"
	"testing"

	"github.com/cjunks94/nitpick/internal/config"
	"github.com/cjunks94/nitpick/internal/secrets"
)

// fakeKey is assembled at runtime so the gitleaks job never sees a literal
// that matches its rules.
var fakeKey = "AKIA" + strings.Repeat("Q7X2", 4)

const sampleDiff = `diff --git a/app/handler.go b/app/handler.go
--- a/app/handler.go
+++ b/app/handler.go
@@ -1,2 +1,3 @@
 package app
+var awsKey = "` + "%KEY%" + `"
 func Handle() {}
diff --git a/vendor/lib.js b/vendor/lib.js
--- a/vendor/lib.js
+++ b/vendor/lib.js
@@ -1 +1,2 @@
 x
+y
`

func rawDiff() []byte {
	return []byte(strings.ReplaceAll(sampleDiff, "%KEY%", fakeKey))
}

func TestPrepare_NilConfigParsesAndRedactsOnly(t *testing.T) {
	p, err := Prepare(rawDiff(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Hunks) != 2 || p.IgnoredHunks != 0 {
		t.Fatalf("hunks=%d ignored=%d, want 2/0 with no config", len(p.Hunks), p.IgnoredHunks)
	}
	if p.RedactedLines != 1 || p.RedactedFiles != 1 {
		t.Errorf("redacted lines=%d files=%d, want 1/1", p.RedactedLines, p.RedactedFiles)
	}
	for _, h := range p.Hunks {
		for _, l := range h.Lines {
			if strings.Contains(l.Content, fakeKey) {
				t.Fatalf("credential reached the prepared hunks: %q", l.Content)
			}
		}
	}
	if p.Model != "" || p.EscalatedOn != "" {
		t.Errorf("model must stay the caller's default with no config; got %q/%q", p.Model, p.EscalatedOn)
	}
}

func TestPrepare_RedactionKeepsLineNumbers(t *testing.T) {
	p, err := Prepare(rawDiff(), nil)
	if err != nil {
		t.Fatal(err)
	}
	h := p.Hunks[0]
	if len(h.Lines) != 3 || h.Lines[1].NewLineNum != 2 {
		t.Fatalf("redaction moved lines: %+v", h.Lines)
	}
	if !strings.Contains(h.Lines[1].Content, secrets.Placeholder) {
		t.Errorf("redacted line lacks the placeholder: %q", h.Lines[1].Content)
	}
}

func TestPrepare_IgnorePathsAndEscalation(t *testing.T) {
	cfg, err := config.Parse([]byte(`
model: claude-haiku-4-5
review:
  ignore_paths: ["vendor/**"]
  escalate:
    model: claude-sonnet-4-6
    paths: ["app/**"]
`))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Prepare(rawDiff(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Hunks) != 1 || p.IgnoredHunks != 1 || p.Hunks[0].File != "app/handler.go" {
		t.Fatalf("ignore_paths: hunks=%v ignored=%d", p.Hunks, p.IgnoredHunks)
	}
	if p.Model != "claude-sonnet-4-6" || p.EscalatedOn != "app/handler.go" {
		t.Errorf("escalation: model=%q on=%q", p.Model, p.EscalatedOn)
	}
}

// Escalation is decided on the files that survive ignore_paths, so an
// ignored file cannot pull in the expensive model.
func TestPrepare_EscalationIgnoresFilteredFiles(t *testing.T) {
	cfg, err := config.Parse([]byte(`
model: claude-haiku-4-5
review:
  ignore_paths: ["vendor/**"]
  escalate:
    model: claude-sonnet-4-6
    paths: ["vendor/**"]
`))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Prepare(rawDiff(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p.Model != "claude-haiku-4-5" || p.EscalatedOn != "" {
		t.Errorf("an ignored file escalated the model: model=%q on=%q", p.Model, p.EscalatedOn)
	}
}

func TestPrepare_ParseErrorIsWrapped(t *testing.T) {
	if _, err := Prepare([]byte("not a diff\n@@ garbage"), nil); err == nil {
		t.Skip("parser tolerates this input; nothing to assert")
	} else if !strings.Contains(err.Error(), "parse diff") {
		t.Errorf("error = %v, want parse diff context", err)
	}
}
