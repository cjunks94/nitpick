package ghc

import "strings"

// ParseRepoArg splits an "owner/name" argument into its two halves.
// The caller MUST pass a string containing exactly one "/" — owner and
// name are both required and must be non-empty. Used by the CLI when
// the --repo flag is provided explicitly (otherwise DetectRepo handles it).
func ParseRepoArg(s string) (owner, name string) {
	parts := strings.SplitN(s, "/", 2)
	return parts[0], parts[1]
}
