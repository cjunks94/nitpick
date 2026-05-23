package provider

import (
	"context"
	"regexp"
	"strings"

	"github.com/cjunks94/nitpick/internal/diff"
)

// Stub is a deterministic, regex-driven reviewer. Zero LLM cost. It serves as
// the eval baseline — every real provider must beat the stub on F1 to be
// worth its tokens. Do not delete it when LLM providers ship; it remains the
// floor against which prompt improvements are measured.
type Stub struct{}

func (Stub) Name() string { return "stub" }

type stubRule struct {
	re            *regexp.Regexp
	severity      Severity
	category      string
	body          string
	skipTestFiles bool
}

var stubRules = []stubRule{
	{
		re:       regexp.MustCompile(`(?i)\b(TODO|FIXME|XXX)\b`),
		severity: SeverityUseful,
		category: "todo",
		body:     "TODO/FIXME marker in changed code — track in an issue or remove before merge.",
	},
	{
		re:            regexp.MustCompile(`\b(console\.log|fmt\.Println|System\.out\.println|print\()`),
		severity:      SeverityNit,
		category:      "debug",
		body:          "Looks like a debug print left in. Use the project logger or remove.",
		skipTestFiles: true,
	},
	{
		re:       regexp.MustCompile(`(?i)(nolint|eslint-disable|noqa)`),
		severity: SeverityUseful,
		category: "suppression",
		body:     "Lint suppression added. Confirm it's intentional and add a justifying comment.",
	},
	{
		re:       regexp.MustCompile(`(?i)(api[_-]?key|secret|password|token|bearer)\s*[:=]\s*["'][A-Za-z0-9+/=_\-]{16,}["']`),
		severity: SeverityCritical,
		category: "secret",
		body:     "Looks like a hard-coded credential. Move to env / secret store before this lands.",
	},
}

func (Stub) Review(_ context.Context, req ReviewRequest) (ReviewResult, error) {
	var out []Comment
	for _, h := range req.Hunks {
		isTest := isTestPath(h.File)
		for _, line := range h.Lines {
			if line.Kind != diff.LineAdded {
				continue
			}
			for _, r := range stubRules {
				if r.skipTestFiles && isTest {
					continue
				}
				if r.re.MatchString(line.Content) {
					out = append(out, Comment{
						File:     h.File,
						Line:     line.NewLineNum,
						Severity: r.severity,
						Category: r.category,
						Body:     r.body,
					})
					break // one rule per added line is plenty
				}
			}
		}
	}
	return ReviewResult{Comments: out}, nil
}

func isTestPath(p string) bool {
	p = strings.ToLower(p)
	return strings.HasSuffix(p, "_test.go") ||
		strings.Contains(p, "/test/") ||
		strings.Contains(p, "/tests/") ||
		strings.HasSuffix(p, ".spec.ts") ||
		strings.HasSuffix(p, ".spec.js") ||
		strings.HasSuffix(p, ".test.ts") ||
		strings.HasSuffix(p, ".test.js") ||
		strings.HasPrefix(p, "test_")
}
