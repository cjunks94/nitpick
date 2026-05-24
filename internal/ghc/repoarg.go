package ghc

import (
	"fmt"
	"strings"
)

// ParseRepoArg splits an "owner/name" argument into its two halves and
// validates that both are non-empty. Returns an error for malformed input
// rather than panicking — the CLI calls this when --repo is set explicitly
// so the user gets a clear message instead of an index-out-of-range later
// in FetchDiff or PostReview.
func ParseRepoArg(s string) (owner, name string, err error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid --repo %q: expected owner/name", s)
	}
	return parts[0], parts[1], nil
}
