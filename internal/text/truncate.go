// Package text holds the one rune-safe truncation the rest of the tree
// shares. It existed three times before, and the copy that fed the API
// request body was not rune-safe: a prefix cut inside a multi-byte rune is
// JSON-encoded as U+FFFD, which the model then sees as noise.
package text

import "unicode/utf8"

// TruncateBytes returns the longest prefix of s that is at most n bytes and
// ends on a rune boundary. n <= 0 yields "".
func TruncateBytes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// Truncate is TruncateBytes with a marker appended when anything was cut.
func Truncate(s string, n int, marker string) string {
	if len(s) <= n {
		return s
	}
	return TruncateBytes(s, n) + marker
}
