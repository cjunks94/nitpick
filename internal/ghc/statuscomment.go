package ghc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/cjunks94/nitpick/internal/provider"
)

// BuildStatusCommentBody formats the one-line summary that nitpick posts to
// every PR after a review. Always visible — silent reviews get the same
// treatment as finding-heavy reviews. New comment per review (no edit-in-
// place); the comment history is the audit trail. N pushes to a PR =
// N status comments.
func BuildStatusCommentBody(providerName string, findings []provider.Comment, costUSD float64, duration time.Duration) string {
	model := strings.TrimPrefix(providerName, "anthropic-")
	cost := fmt.Sprintf("$%.4f", costUSD)
	dur := fmt.Sprintf("%.1fs", duration.Seconds())

	if len(findings) == 0 {
		return fmt.Sprintf("**nitpick** — no actionable findings · %s · %s · `%s`", cost, dur, model)
	}

	plural := "s"
	if len(findings) == 1 {
		plural = ""
	}
	return fmt.Sprintf("**nitpick** — %d finding%s posted · %s · %s · `%s`\n\nSee inline comments below.",
		len(findings), plural, cost, dur, model)
}

// PostIssueComment posts a top-level PR comment via the gh CLI. Used by the
// local `nitpick review` path; mirrors PostReview's gh-subprocess shape so
// the two transports share an auth model.
func PostIssueComment(ctx context.Context, repo string, pr int, body string) error {
	if body == "" {
		return nil
	}
	if repo == "" {
		return fmt.Errorf("repo is required to post a status comment")
	}
	cmd := exec.CommandContext(ctx, "gh", "pr", "comment", fmt.Sprintf("%d", pr), "--repo", repo, "--body", body)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh pr comment: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// PostIssueComment posts a top-level PR comment via the REST API. Used by
// `nitpick serve`. Returns nil on empty body (no-op). The GitHub App needs
// pull_requests:write — already required for review posting, so no new
// permission needed.
func (c *HTTPClient) PostIssueComment(ctx context.Context, repo string, pr int, body string) error {
	if body == "" {
		return nil
	}
	payload, err := json.Marshal(map[string]any{"body": body})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/repos/%s/issues/%d/comments", c.BaseURL, repo, pr)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("post status comment: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("post status comment: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}
	return nil
}
