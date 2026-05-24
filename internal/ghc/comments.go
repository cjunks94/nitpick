package ghc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"

	"github.com/cjunks94/nitpick/internal/provider"
)

// BuildReviewBody marshals comments into the GitHub review API JSON payload.
// One review event with an inline comment per finding (path + line + side=RIGHT
// for the modern review-comment API). Sorted by file then line so the timeline
// reads top-to-bottom of the diff. Shared between the gh-subprocess path (used
// by local `nitpick review`) and the HTTP path (used by `nitpick serve`).
func BuildReviewBody(comments []provider.Comment) ([]byte, error) {
	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].File != comments[j].File {
			return comments[i].File < comments[j].File
		}
		return comments[i].Line < comments[j].Line
	})
	apiComments := make([]map[string]any, 0, len(comments))
	for _, c := range comments {
		apiComments = append(apiComments, map[string]any{
			"path": c.File,
			"line": c.Line,
			"side": "RIGHT",
			"body": renderBody(c),
		})
	}
	return json.Marshal(map[string]any{
		"event":    "COMMENT",
		"body":     renderReviewSummary(len(comments)),
		"comments": apiComments,
	})
}

// renderReviewSummary builds the top-level review body shown above the inline
// comments on GitHub. ASCII-FAQ styling deliberate: monospace code block reads
// as a single visual unit and doesn't fight GitHub's markdown formatting.
func renderReviewSummary(n int) string {
	plural := "s"
	if n == 1 {
		plural = ""
	}
	return fmt.Sprintf("```\n"+
		"============================================================\n"+
		"  nitpick review — %d finding%s\n"+
		"============================================================\n"+
		"\n"+
		"  SEVERITY\n"+
		"    [CRITICAL]  real bug / security — would break production\n"+
		"    [USEFUL]    real idiom / perf / maintainability worth fixing\n"+
		"    [NIT]       taste / style — usually safe to ignore\n"+
		"\n"+
		"  SCOPE       complements CodeRabbit; targets contract drift,\n"+
		"              unenforced security gates, perf concerns tied to\n"+
		"              this repo's data shape. Skips style/formatting.\n"+
		"\n"+
		"  SOURCE      github.com/cjunks94/nitpick\n"+
		"============================================================\n"+
		"```", n, plural)
}

// PostReview posts a single PR review with inline comments via `gh api`.
// Uses GitHub's modern review-comment fields (`line` + `side=RIGHT`) so the
// diff parser's NewLineNum is what we send. The whole batch is one review
// event, not N individual comments — keeps the PR timeline clean.
func PostReview(ctx context.Context, repo string, pr int, comments []provider.Comment) error {
	if len(comments) == 0 {
		fmt.Println("nitpick: no findings")
		return nil
	}
	if repo == "" {
		return fmt.Errorf("repo is required to post a review")
	}

	body, err := BuildReviewBody(comments)
	if err != nil {
		return err
	}

	args := []string{
		"api",
		"-X", "POST",
		fmt.Sprintf("/repos/%s/pulls/%d/reviews", repo, pr),
		"--input", "-",
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stdin = bytes.NewReader(body)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh api: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	fmt.Printf("nitpick: posted %d finding(s) to %s#%d\n", len(comments), repo, pr)
	return nil
}

// PrintComments writes findings to w in a human-readable form. Used by --dry-run.
func PrintComments(w io.Writer, comments []provider.Comment, costUSD float64) error {
	if len(comments) == 0 {
		fmt.Fprintln(w, "nitpick: no findings")
		fmt.Fprintf(w, "cost: $%.4f\n", costUSD)
		return nil
	}
	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].File != comments[j].File {
			return comments[i].File < comments[j].File
		}
		return comments[i].Line < comments[j].Line
	})
	for _, c := range comments {
		fmt.Fprintf(w, "[%s/%s] %s:%d\n  %s\n\n",
			c.Severity, c.Category, c.File, c.Line,
			strings.ReplaceAll(c.Body, "\n", "\n  "))
	}
	fmt.Fprintf(w, "cost: $%.4f\n", costUSD)
	return nil
}

// renderBody is the per-finding inline comment body. Short header line with
// severity + category in ASCII brackets, then the LLM-produced explanation,
// then a plain-text signature. No emoji; no markdown bold; nothing that
// fights with the source-code context the comment is anchored to.
func renderBody(c provider.Comment) string {
	var sev string
	switch c.Severity {
	case provider.SeverityCritical:
		sev = "CRITICAL"
	case provider.SeverityUseful:
		sev = "USEFUL"
	default:
		sev = "NIT"
	}
	cat := ""
	if c.Category != "" {
		cat = " · " + c.Category
	}
	return fmt.Sprintf("`[%s%s]`\n\n%s\n\n`— nitpick`", sev, cat, c.Body)
}
