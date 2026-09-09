package text

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateBytes_RuneSafe(t *testing.T) {
	s := strings.Repeat("日", 10) // 30 bytes, 3 per rune
	for n := 0; n <= 32; n++ {
		got := TruncateBytes(s, n)
		if len(got) > n {
			t.Errorf("n=%d: len %d exceeds cap", n, len(got))
		}
		if !utf8.ValidString(got) {
			t.Errorf("n=%d: result is not valid UTF-8: %q", n, got)
		}
		if n >= 30 && got != s {
			t.Errorf("n=%d: a string within the cap must be returned whole", n)
		}
	}
	if got := TruncateBytes("abc", 5); got != "abc" {
		t.Errorf("short ASCII changed: %q", got)
	}
	if got := TruncateBytes("abcdef", 3); got != "abc" {
		t.Errorf("ASCII cut = %q, want abc", got)
	}
}

func TestTruncate_MarkerOnlyWhenCut(t *testing.T) {
	if got := Truncate("abc", 3, "…"); got != "abc" {
		t.Errorf("no cut should add no marker: %q", got)
	}
	if got := Truncate("abcd", 3, "…"); got != "abc…" {
		t.Errorf("cut = %q, want abc…", got)
	}
	if got := Truncate("日本語", 4, "..."); got != "日..." {
		t.Errorf("multi-byte cut = %q, want 日...", got)
	}
}
