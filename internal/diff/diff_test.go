package diff

import "testing"

const sampleDiff = `diff --git a/foo.go b/foo.go
index 1234567..89abcde 100644
--- a/foo.go
+++ b/foo.go
@@ -1,4 +1,5 @@
 package foo

-func Old() {}
+func New() {
+	return
+}
`

func TestParseUnifiedDiff_basic(t *testing.T) {
	hunks, err := ParseUnifiedDiff([]byte(sampleDiff))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("want 1 hunk, got %d", len(hunks))
	}
	h := hunks[0]
	if h.File != "foo.go" {
		t.Errorf("file = %q, want foo.go", h.File)
	}
	if h.NewStart != 1 || h.OldStart != 1 {
		t.Errorf("start: new=%d old=%d", h.NewStart, h.OldStart)
	}

	var added, removed, ctx int
	for _, l := range h.Lines {
		switch l.Kind {
		case LineAdded:
			added++
		case LineRemoved:
			removed++
		case LineContext:
			ctx++
		}
	}
	if added != 3 {
		t.Errorf("added = %d, want 3", added)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if ctx != 2 {
		t.Errorf("context = %d, want 2", ctx)
	}
}

func TestParseUnifiedDiff_lineNumbers(t *testing.T) {
	hunks, err := ParseUnifiedDiff([]byte(sampleDiff))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	h := hunks[0]
	// Added lines are the new func body — should be at NewLineNum 3, 4, 5.
	var addedLines []int
	for _, l := range h.Lines {
		if l.Kind == LineAdded {
			addedLines = append(addedLines, l.NewLineNum)
		}
	}
	want := []int{3, 4, 5}
	if len(addedLines) != len(want) {
		t.Fatalf("added line numbers: got %v, want %v", addedLines, want)
	}
	for i, n := range addedLines {
		if n != want[i] {
			t.Errorf("added[%d] line = %d, want %d", i, n, want[i])
		}
	}
}

const multiFileDiff = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,1 +1,2 @@
 a
+aa
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1,1 +1,2 @@
 b
+bb
`

func TestParseUnifiedDiff_multiFile(t *testing.T) {
	hunks, err := ParseUnifiedDiff([]byte(multiFileDiff))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(hunks) != 2 {
		t.Fatalf("want 2 hunks, got %d", len(hunks))
	}
	if hunks[0].File != "a.go" || hunks[1].File != "b.go" {
		t.Errorf("files: got %q, %q", hunks[0].File, hunks[1].File)
	}
	// Position resets per file — both added lines should be position 2
	// (position 1 is the "a"/"b" context line right after @@).
	for i, h := range hunks {
		var addedPos int
		for _, l := range h.Lines {
			if l.Kind == LineAdded {
				addedPos = l.DiffPosition
			}
		}
		if addedPos != 2 {
			t.Errorf("hunk %d added position = %d, want 2", i, addedPos)
		}
	}
}
