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

func runGH(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
