package diff

import "testing"

// A deletion has "+++ /dev/null"; the hunk must carry the old-side path so
// path-keyed guards (secret redaction, ignore_paths, escalate.paths) still
// see the file. Previously File was "" and a removed .env went to the
// provider unredacted.
func TestParseUnifiedDiff_DeletedFileKeepsPath(t *testing.T) {
	raw := `diff --git a/.env b/.env
deleted file mode 100644
index 1111111..0000000
--- a/.env
+++ /dev/null
@@ -1,2 +0,0 @@
-DB_PASSWORD=hunter2hunter2hunter2
-API_URL=https://example.test
diff --git a/README.md b/README.md
index 2222222..3333333 100644
--- a/README.md
+++ b/README.md
@@ -1 +1 @@
-old
+new
`
	hunks, err := ParseUnifiedDiff([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(hunks) != 2 {
		t.Fatalf("want 2 hunks, got %d", len(hunks))
	}
	if hunks[0].File != ".env" {
		t.Errorf("deleted file path = %q, want .env", hunks[0].File)
	}
	if hunks[1].File != "README.md" {
		t.Errorf("second file path = %q, want README.md (old path must not leak across files)", hunks[1].File)
	}
	if len(hunks[0].Lines) != 2 || hunks[0].Lines[0].Kind != LineRemoved {
		t.Errorf("deleted hunk lines = %+v", hunks[0].Lines)
	}
}

func TestParseUnifiedDiff_NewFileStillUsesNewPath(t *testing.T) {
	raw := `diff --git a/new.go b/new.go
new file mode 100644
--- /dev/null
+++ b/new.go
@@ -0,0 +1 @@
+package new
`
	hunks, err := ParseUnifiedDiff([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(hunks) != 1 || hunks[0].File != "new.go" {
		t.Fatalf("hunks = %+v, want one hunk for new.go", hunks)
	}
}

// Git C-quotes paths with non-ASCII bytes, quotes, or backslashes. The
// quoted form must decode to the real path or findings post with File ""
// and GitHub rejects the whole review.
func TestParseUnifiedDiff_QuotedPaths(t *testing.T) {
	raw := "diff --git \"a/docs/r\\303\\251sum\\303\\251.md\" \"b/docs/r\\303\\251sum\\303\\251.md\"\n" +
		"--- \"a/docs/r\\303\\251sum\\303\\251.md\"\n" +
		"+++ \"b/docs/r\\303\\251sum\\303\\251.md\"\n" +
		"@@ -1 +1 @@\n-a\n+b\n" +
		"diff --git \"a/we\\\"ird\\\\name.txt\" \"b/we\\\"ird\\\\name.txt\"\n" +
		"--- \"a/we\\\"ird\\\\name.txt\"\n" +
		"+++ \"b/we\\\"ird\\\\name.txt\"\n" +
		"@@ -1 +1 @@\n-a\n+b\n" +
		"diff --git a/docs/my file.md b/docs/my file.md\n" +
		"--- a/docs/my file.md\t\n" +
		"+++ b/docs/my file.md\t\n" +
		"@@ -1 +1 @@\n-a\n+b\n"
	hunks, err := ParseUnifiedDiff([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{"docs/résumé.md", `we"ird\name.txt`, "docs/my file.md"}
	if len(hunks) != len(want) {
		t.Fatalf("want %d hunks, got %d: %+v", len(want), len(hunks), hunks)
	}
	for i, w := range want {
		if hunks[i].File != w {
			t.Errorf("hunk %d file = %q, want %q", i, hunks[i].File, w)
		}
	}
}

func TestParseUnifiedDiff_NoPrefixDiff(t *testing.T) {
	raw := "--- x.go\n+++ x.go\n@@ -1 +1 @@\n-a\n+b\n"
	hunks, err := ParseUnifiedDiff([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(hunks) != 1 || hunks[0].File != "x.go" {
		t.Fatalf("hunks = %+v, want one hunk for x.go", hunks)
	}
}

func TestUnquoteGitPath(t *testing.T) {
	cases := map[string]string{
		`"b/plain.txt"`:               "b/plain.txt",
		`"b/r\303\251sum\303\251.md"`: "b/résumé.md",
		`"b/tab\there.txt"`:           "b/tab\there.txt",
		`"b/q\"uote\\back.txt"`:       `b/q"uote\back.txt`,
		`b/unquoted.txt`:              "b/unquoted.txt",
		`"b/unterminated`:             `"b/unterminated`,
		`"b/bad\9escape.txt"`:         `"b/bad\9escape.txt"`,
		`"b/short\30"`:                `"b/short\30"`,
		`"b/trailing\"`:               `"b/trailing\"`,
	}
	for in, want := range cases {
		if got := unquoteGitPath(in); got != want {
			t.Errorf("unquoteGitPath(%s) = %q, want %q", in, got, want)
		}
	}
}
