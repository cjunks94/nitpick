package ghc

import "github.com/cjunks94/nitpick/internal/provider"

// ToPriorFindings converts comments already on the PR into what the prompt
// shows the model as covered ground, keeping the first max and reporting how
// many were dropped. Callers pass inline comments first: a comment anchored
// to a diff line is far more likely to collide with one of nitpick's
// findings than a walkthrough summary is, so it should win the budget.
// Top-level comments carry no path or line. Shared by the CLI and serve so
// the cap rule cannot drift between them again.
func ToPriorFindings(hits []ExistingComment, max int) (out []provider.PriorFinding, dropped int) {
	if max < 0 {
		max = 0
	}
	n := len(hits)
	if n > max {
		n = max
	}
	out = make([]provider.PriorFinding, 0, n)
	for _, c := range hits[:n] {
		pf := provider.PriorFinding{Author: c.Author, Body: c.Body}
		if c.Path != "" {
			pf.Path, pf.Line = c.Path, c.Line
		}
		out = append(out, pf)
	}
	return out, len(hits) - n
}
