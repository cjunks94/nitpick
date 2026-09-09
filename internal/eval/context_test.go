package eval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cjunks94/nitpick/internal/provider"
)

// captureProvider captures the requests a sweep sends.
type captureProvider struct {
	reqs []provider.ReviewRequest
}

func (r *captureProvider) Name() string { return "recording" }
func (r *captureProvider) Review(_ context.Context, req provider.ReviewRequest) (provider.ReviewResult, error) {
	r.reqs = append(r.reqs, req)
	return provider.ReviewResult{}, nil
}

const contextCaseDiff = `diff --git a/src/app.go b/src/app.go
--- a/src/app.go
+++ b/src/app.go
@@ -1,2 +1,3 @@
 package app
+var x = 1
 func F() {}
`

// With Options.Context the sweep attaches the snapshot files serve would
// have fetched, through the same selection and caps, and says so in the
// report header. Without it, or with no snapshot, the case runs diff-only.
func TestRunWithOptions_AttachesSnapshotContext(t *testing.T) {
	dir := t.TempDir()
	cases := filepath.Join(dir, "cases.jsonl")
	diffPath := filepath.Join(dir, "pr-7.diff")
	if err := os.WriteFile(diffPath, []byte(contextCaseDiff), 0o600); err != nil {
		t.Fatal(err)
	}
	line := `{"pr":7,"repo":"o/r","diff_path":"` + strings.ReplaceAll(diffPath, `\`, `\\`) + `","expected":[]}` + "\n"
	if err := os.WriteFile(cases, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(ContextDir(cases, 7), "src")
	if err := os.MkdirAll(snap, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snap, "app.go"), []byte("package app\n\nfunc F() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("context on", func(t *testing.T) {
		rp := &captureProvider{}
		out := filepath.Join(t.TempDir(), "REPORT.md")
		if err := RunWithOptions(context.Background(), cases, out, rp, Options{Context: true}); err != nil {
			t.Fatal(err)
		}
		if len(rp.reqs) != 1 || len(rp.reqs[0].ContextFiles) != 1 || rp.reqs[0].ContextFiles[0].Path != "src/app.go" {
			t.Fatalf("provider saw context %+v, want src/app.go", rp.reqs)
		}
		report, _ := os.ReadFile(out)
		if !strings.Contains(string(report), "Context: on — 1 whole file(s) attached across 1 case(s)") {
			t.Errorf("report header does not state the context:\n%s", report)
		}
	})

	t.Run("context off", func(t *testing.T) {
		rp := &captureProvider{}
		out := filepath.Join(t.TempDir(), "REPORT.md")
		if err := RunWithOptions(context.Background(), cases, out, rp, Options{}); err != nil {
			t.Fatal(err)
		}
		if len(rp.reqs[0].ContextFiles) != 0 {
			t.Fatalf("context attached without the option: %+v", rp.reqs[0].ContextFiles)
		}
		report, _ := os.ReadFile(out)
		if !strings.Contains(string(report), "Context: off") {
			t.Errorf("report header should say context is off:\n%s", report)
		}
	})

	t.Run("no snapshot runs diff-only", func(t *testing.T) {
		if err := os.RemoveAll(ContextDir(cases, 7)); err != nil {
			t.Fatal(err)
		}
		rp := &captureProvider{}
		if err := RunWithOptions(context.Background(), cases, filepath.Join(t.TempDir(), "R.md"), rp, Options{Context: true}); err != nil {
			t.Fatal(err)
		}
		if len(rp.reqs[0].ContextFiles) != 0 {
			t.Fatalf("a missing snapshot must not attach anything: %+v", rp.reqs[0].ContextFiles)
		}
	})
}

func TestLoadContext_RejectsTraversal(t *testing.T) {
	fetch := loadContext(t.TempDir())
	if _, err := fetch("../outside.go"); err == nil {
		t.Error("a traversal path must not be read")
	}
}
