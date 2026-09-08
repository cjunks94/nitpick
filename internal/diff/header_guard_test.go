package diff

import "testing"

// A removed line whose content starts with "-- " renders as "--- ..." and
// used to be swallowed as the old-file header, desyncing OldLineNum and
// DiffPosition for the rest of the hunk. Same shape for an added line
// starting with "++ " versus the "+++ " header.
func TestParseUnifiedDiff_RemovedSQLCommentIsNotAHeader(t *testing.T) {
	raw := `diff --git a/schema.sql b/schema.sql
index 1111111..2222222 100644
--- a/schema.sql
+++ b/schema.sql
@@ -10,4 +10,4 @@ CREATE TABLE t (
   id INT,
-  -- old comment
-  name TEXT
+  ++ not a header either
+  name VARCHAR(255)
 );
`
	hunks, err := ParseUnifiedDiff([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("want 1 hunk, got %d", len(hunks))
	}
	h := hunks[0]
	if h.File != "schema.sql" {
		t.Fatalf("file = %q", h.File)
	}
	if len(h.Lines) != 6 {
		t.Fatalf("want 6 lines (1 ctx, 2 removed, 2 added, 1 ctx), got %d: %+v", len(h.Lines), h.Lines)
	}

	type want struct {
		kind LineKind
		old  int
		new  int
		pos  int
	}
	wants := []want{
		{LineContext, 10, 10, 1},
		{LineRemoved, 11, 0, 2},
		{LineRemoved, 12, 0, 3},
		{LineAdded, 0, 11, 4},
		{LineAdded, 0, 12, 5},
		{LineContext, 13, 13, 6},
	}
	for i, w := range wants {
		l := h.Lines[i]
		if l.Kind != w.kind || l.OldLineNum != w.old || l.NewLineNum != w.new || l.DiffPosition != w.pos {
			t.Errorf("line %d: got kind=%v old=%d new=%d pos=%d, want kind=%v old=%d new=%d pos=%d (%q)",
				i, l.Kind, l.OldLineNum, l.NewLineNum, l.DiffPosition, w.kind, w.old, w.new, w.pos, l.Content)
		}
	}
	if h.Lines[1].Content != "  -- old comment" {
		t.Errorf("removed comment content = %q", h.Lines[1].Content)
	}
	if h.Lines[3].Content != "  ++ not a header either" {
		t.Errorf("added content = %q", h.Lines[3].Content)
	}
}

// Hunks are closed when the header's line counts are exhausted, so file
// headers are recognised even when the diff has no "diff --git" line.
func TestParseUnifiedDiff_HeadersWithoutDiffGitLine(t *testing.T) {
	raw := `--- a/one.go
+++ b/one.go
@@ -1,2 +1,2 @@
 package one
-var A = 1
+var A = 2
--- a/two.go
+++ b/two.go
@@ -1,2 +1,2 @@
 package two
-var B = 1
+var B = 2
`
	hunks, err := ParseUnifiedDiff([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(hunks) != 2 {
		t.Fatalf("want 2 hunks, got %d", len(hunks))
	}
	if hunks[0].File != "one.go" || hunks[1].File != "two.go" {
		t.Fatalf("files = %q, %q", hunks[0].File, hunks[1].File)
	}
	if len(hunks[1].Lines) != 3 {
		t.Fatalf("second hunk has %d lines, want 3", len(hunks[1].Lines))
	}
}

// Position is per-file and cumulative across hunks, including the "\ No
// newline" marker between them, even now that hunks close on exhaustion.
func TestParseUnifiedDiff_NoNewlineMarkerStillCountsPosition(t *testing.T) {
	raw := `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -1,1 +1,1 @@
-old
+new
\ No newline at end of file
@@ -5,1 +5,1 @@
-x
+y
`
	hunks, err := ParseUnifiedDiff([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(hunks) != 2 {
		t.Fatalf("want 2 hunks, got %d", len(hunks))
	}
	// hunk 1: @@=0, -old=1, +new=2, \=3; hunk 2: @@=4, -x=5, +y=6
	if got := hunks[1].Lines[0].DiffPosition; got != 5 {
		t.Errorf("second hunk first line position = %d, want 5", got)
	}
	if got := hunks[1].Lines[1].DiffPosition; got != 6 {
		t.Errorf("second hunk second line position = %d, want 6", got)
	}
}
