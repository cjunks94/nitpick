// Package ghc wraps the GitHub CLI for the bits of PR interaction nitpick
// needs. Using `gh` as a subprocess piggybacks on existing auth (GITHUB_TOKEN
// in Actions, the user's gh login locally) and keeps v0 simple. Swap to the
// raw REST API when finer control is required.
package ghc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// FetchDiff returns the unified diff for the given PR.
func FetchDiff(ctx context.Context, repo string, pr int) ([]byte, error) {
	args := []string{"pr", "diff", fmt.Sprintf("%d", pr)}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	return runGH(ctx, args...)
}

// DetectRepo returns owner/name for the current working directory.
func DetectRepo(ctx context.Context) (string, error) {
	out, err := runGH(ctx, "repo", "view", "--json", "nameWithOwner")
	if err != nil {
		return "", err
	}
	var v struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", fmt.Errorf("parse gh repo view: %w", err)
	}
	if v.NameWithOwner == "" {
		return "", fmt.Errorf("gh repo view returned empty nameWithOwner")
	}
	return v.NameWithOwner, nil
}

// HeadSHA returns the head commit SHA of a PR. Needed by the inline-comment
// REST endpoint if we ever swap off `gh api`.
func HeadSHA(ctx context.Context, repo string, pr int) (string, error) {
	args := []string{"pr", "view", fmt.Sprintf("%d", pr), "--json", "headRefOid"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := runGH(ctx, args...)
	if err != nil {
		return "", err
	}
	var v struct {
		HeadRefOid string `json:"headRefOid"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", err
	}
	return v.HeadRefOid, nil
}

// ListPRComments returns the existing comments on a PR via `gh api`, both
// inline review comments and top-level issue comments. The gh-subprocess twin
// of HTTPClient.ListReviewComments / ListIssueComments — same ExistingComment
// shape so the CodeRabbit dedup logic works identically on both surfaces.
//
// --paginate makes gh follow Link headers; --slurp merges the pages into one
// JSON array rather than emitting one array per page (which isn't valid JSON
// as a whole document).
func ListPRComments(ctx context.Context, repo string, pr int) ([]ExistingComment, error) {
	endpoints := []string{
		fmt.Sprintf("/repos/%s/pulls/%d/comments", repo, pr),
		fmt.Sprintf("/repos/%s/issues/%d/comments", repo, pr),
	}
	var out []ExistingComment
	for _, ep := range endpoints {
		raw, err := runGH(ctx, "api", "--paginate", "--slurp", ep)
		if err != nil {
			return nil, fmt.Errorf("list comments %s: %w", ep, err)
		}
		// With --slurp the result is [[...], [...]] — one inner array per page.
		var pages [][]struct {
			Body string `json:"body"`
			Path string `json:"path"`
			Line int    `json:"line"`
			User struct {
				Login string `json:"login"`
			} `json:"user"`
			CreatedAt time.Time `json:"created_at"`
		}
		if err := json.Unmarshal(raw, &pages); err != nil {
			return nil, fmt.Errorf("parse comments %s: %w", ep, err)
		}
		for _, page := range pages {
			for _, c := range page {
				if len(out) >= maxListedComments {
					return out, nil
				}
				out = append(out, ExistingComment{
					Author:    c.User.Login,
					Path:      c.Path,
					Line:      c.Line,
					Body:      c.Body,
					CreatedAt: c.CreatedAt,
				})
			}
		}
	}
	return out, nil
}

func runGH(ctx context.Context, args ...string) ([]byte, error) {
	// #nosec G204 -- args are constructed internally by callers in this
	// package; never sourced from untrusted input. The binary name is the
	// literal "gh" so there's no command-injection surface.
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
