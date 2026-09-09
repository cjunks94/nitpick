// Package review is the one pipeline between a raw unified diff and the
// provider. The CLI, the webhook server, and the eval harness all go
// through Prepare, so what the eval measures is what production sends. It
// was hand-assembled in three places before this, and the eval copy had
// drifted: it sent fixtures unredacted while production redacted them.
package review

import (
	"fmt"

	"github.com/cjunks94/nitpick/internal/config"
	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/secrets"
)

// Prepared is the provider-ready view of a PR diff plus what was done to it.
type Prepared struct {
	Hunks []diff.Hunk
	// IgnoredHunks were dropped by review.ignore_paths.
	IgnoredHunks int
	// RedactedLines and RedactedFiles count what secrets.SanitizeHunks
	// masked. Line numbers are untouched: redaction is line for line.
	RedactedLines int
	RedactedFiles int
	// Model is the resolved model id: cfg.Model, overridden by
	// review.escalate when a reviewed file matches; "" means the caller's
	// default. EscalatedOn names the file that matched, "" if none.
	Model       string
	EscalatedOn string
}

// Prepare parses raw, drops review.ignore_paths, redacts credentials, and
// resolves model escalation on the files that remain. cfg may be nil (the
// eval harness, or serve when no trusted config exists): then only parse
// and redact apply. Redaction is unconditional on every surface: the diff
// goes to the provider verbatim, and ignore_paths is opt-in, so it cannot
// be the only guard.
func Prepare(raw []byte, cfg *config.Config) (Prepared, error) {
	hunks, err := diff.ParseUnifiedDiff(raw)
	if err != nil {
		return Prepared{}, fmt.Errorf("parse diff: %w", err)
	}
	var p Prepared
	if cfg != nil && len(cfg.Review.IgnorePaths) > 0 {
		before := len(hunks)
		hunks = diff.FilterByPath(hunks, func(path string) bool {
			return config.MatchAny(path, cfg.Review.IgnorePaths)
		})
		p.IgnoredHunks = before - len(hunks)
	}
	p.Hunks, p.RedactedLines, p.RedactedFiles = secrets.SanitizeHunks(hunks)
	if cfg != nil {
		p.Model = cfg.Model
		if m, matched := cfg.ModelFor(diff.Files(p.Hunks)); matched != "" {
			p.Model, p.EscalatedOn = m, matched
		}
	}
	return p, nil
}
