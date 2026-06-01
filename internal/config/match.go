package config

import (
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// MatchAny reports whether path matches any of the given doublestar patterns.
// Returns false when patterns is empty. Patterns are assumed to have been
// validated by ValidatePatterns at config load — a runtime match error is
// treated as "no match" so a stray bad pattern can't accidentally block the
// entire review.
//
// Path semantics use forward slashes (GitHub diff paths are always /-separated,
// regardless of host OS). `*` matches within a path segment, `**` matches
// across segments. As a usability shim, a pattern starting with "**/" also
// matches root-level paths — i.e. "**/*.uid" matches both "a/b/x.uid" and
// "x.uid". This matches gitignore / coderabbit precedent and is what users
// reach for first; pure doublestar would require "{,**/}*.uid".
func MatchAny(path string, patterns []string) bool {
	for _, p := range patterns {
		if matchOne(p, path) {
			return true
		}
	}
	return false
}

func matchOne(pattern, path string) bool {
	if matched, err := doublestar.Match(pattern, path); err == nil && matched {
		return true
	}
	if rest, ok := strings.CutPrefix(pattern, "**/"); ok {
		if matched, err := doublestar.Match(rest, path); err == nil && matched {
			return true
		}
	}
	return false
}

// ValidatePatterns returns the first malformed pattern with a wrapped error,
// or nil if every pattern compiles. Called from Parse so a bad glob fails the
// config load instead of silently no-matching at review time.
func ValidatePatterns(patterns []string) error {
	for _, p := range patterns {
		if _, err := doublestar.Match(p, "x"); err != nil {
			return fmt.Errorf("invalid glob %q: %w", p, err)
		}
	}
	return nil
}
