package secrets

import (
	"strings"
	"testing"

	"github.com/cjunks94/nitpick/internal/diff"
)

// "Remove the committed .env" is a common PR. The deletion hunk used to parse
// with File == "" (only "+++ b/" set the path), so IsSensitivePath never
// fired and the removed lines went to the provider verbatim.
func TestSanitizeHunks_DeletedEnvFileIsMasked(t *testing.T) {
	raw := `diff --git a/.env b/.env
deleted file mode 100644
--- a/.env
+++ /dev/null
@@ -1,2 +0,0 @@
-DB_PASSWORD=hunter2hunter2hunter2
-API_URL=https://example.test
`
	hunks, err := diff.ParseUnifiedDiff([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	out, lines, files := SanitizeHunks(hunks)
	if files != 1 || lines == 0 {
		t.Fatalf("redacted files=%d lines=%d, want the .env hunk masked", files, lines)
	}
	if len(out) != 1 || len(out[0].Lines) != 2 {
		t.Fatalf("line count changed: %+v", out)
	}
	for _, l := range out[0].Lines {
		if strings.Contains(l.Content, "hunter2") {
			t.Fatalf("secret survived sanitisation: %q", l.Content)
		}
	}
}
