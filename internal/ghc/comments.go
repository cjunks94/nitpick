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

	body, err := json.Marshal(map[string]any{
		"event":    "COMMENT",
		"body":     fmt.Sprintf("nitpick — %d finding(s)", len(comments)),
		"comments": apiComments,
	})
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

func renderBody(c provider.Comment) string {
	var sev string
	switch c.Severity {
	case provider.SeverityCritical:
		sev = "**[critical]**"
	case provider.SeverityUseful:
		sev = "**[useful]**"
	default:
		sev = "**[nit]**"
	}
	cat := ""
	if c.Category != "" {
		cat = fmt.Sprintf(" `%s`", c.Category)
	}
	return fmt.Sprintf("%s%s %s\n\n<sub>posted by nitpick</sub>", sev, cat, c.Body)
}
