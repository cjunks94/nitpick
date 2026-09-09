package review

import (
	"errors"
	"strings"
	"testing"

	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/secrets"
)

func hunkWith(file string, added int) diff.Hunk {
	h := diff.Hunk{File: file}
	for i := 0; i < added; i++ {
		h.Lines = append(h.Lines, diff.HunkLine{Kind: diff.LineAdded, NewLineNum: i + 1})
	}
	return h
}

func TestContextCandidates_OrderDenyCap(t *testing.T) {
	hunks := []diff.Hunk{
		hunkWith("small.go", 1),
		hunkWith("big.go", 9),
		hunkWith("big.go", 3), // second hunk, same file: counted once, weight 12
		hunkWith("go.sum", 50),
		hunkWith(".env", 2),
		hunkWith("bundle.min.js", 100),
		hunkWith("mid.go", 5),
		hunkWith("a.go", 4),
		hunkWith("b.go", 4),
		hunkWith("c.go", 4),
		hunkWith("d.go", 2),
	}
	got := ContextCandidates(hunks)
	want := []string{"big.go", "mid.go", "a.go", "b.go", "c.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("candidates = %v, want %v (weight desc, deny-listed removed, capped at %d)", got, want, MaxContextFiles)
	}
}

func TestIsContextDenied(t *testing.T) {
	for path, want := range map[string]bool{
		"src/app.go":            false,
		"Gemfile.lock":          true,
		"vendor/Gemfile.lock":   true,
		"assets/app.MIN.JS":     true,
		"proto/api.pb.go":       true,
		"config/secrets.yml":    true,
		"docs/README.md":        false,
		"scene.uid":             true,
		"package-lock.json":     true,
		"src/lockfile_utils.go": false,
	} {
		if got := IsContextDenied(path); got != want {
			t.Errorf("IsContextDenied(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestAttachContext_CapsSkipsRedacts(t *testing.T) {
	key := "AKIA" + strings.Repeat("Q7X2", 4)
	sixtyK := []byte(strings.Repeat("y", MaxContextFileBytes))
	files := map[string][]byte{
		"a.go":    []byte("package a\nvar k = \"" + key + "\"\n"),
		"big.go":  []byte(strings.Repeat("x", MaxContextFileBytes+1)),
		"b.go":    sixtyK,
		"c.go":    sixtyK,
		"d.go":    sixtyK,
		"e.go":    sixtyK, // a.go + b..d is 180 KiB; e would cross the 200 KiB total
		"late.go": []byte("never reached"),
	}
	fetch := func(p string) ([]byte, error) {
		if p == "gone.go" {
			return nil, errors.New("404")
		}
		return files[p], nil
	}
	got := AttachContext([]string{"a.go", "big.go", "gone.go", "b.go", "c.go", "d.go", "e.go", "late.go"}, fetch, nil)

	paths := make([]string, 0, len(got))
	for _, f := range got {
		paths = append(paths, f.Path)
	}
	if strings.Join(paths, ",") != "a.go,b.go,c.go,d.go" {
		t.Fatalf("attached %v, want a,b,c,d (oversize and missing skipped; the total cap stops the walk at e)", paths)
	}
	if strings.Contains(string(got[0].Content), key) || !strings.Contains(string(got[0].Content), secrets.Placeholder) {
		t.Errorf("credential in a.go was not redacted: %q", got[0].Content)
	}
}
