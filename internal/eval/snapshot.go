package eval

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/review"
	"github.com/cjunks94/nitpick/internal/secrets"
)

// ContextDir is where Snapshot writes and Run (with Options.Context) reads
// each case's whole-file context: <cases dir>/testdata/context/pr-<N>/<path>,
// plus a HEAD file holding the PR head SHA the files were fetched at. Under
// testdata because the snapshots are other repos' source files, Go ones
// included, and the go tool must not treat them as packages.
func ContextDir(casesPath string, pr int) string {
	return filepath.Join(filepath.Dir(casesPath), "testdata", "context", fmt.Sprintf("pr-%d", pr))
}

// Snapshot fetches, for every case, the context files serve would attach
// (review.ContextCandidates on the case's diff, at the PR head SHA) and
// stores them under ContextDir. Fixtures are static, so the eval cannot call
// GitHub at review time the way serve does; this makes the same files
// available offline and commits them next to the diffs. Uses the gh CLI for
// auth, like the local review command. Files over the per-file cap and files
// absent at head (deleted in the PR) are skipped, as serve skips them.
func Snapshot(ctx context.Context, casesPath string, w io.Writer) error {
	cases, err := loadCases(casesPath)
	if err != nil {
		return fmt.Errorf("load cases: %w", err)
	}
	for _, c := range cases {
		raw, err := os.ReadFile(c.DiffPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", c.DiffPath, err)
		}
		hunks, err := diff.ParseUnifiedDiff(raw)
		if err != nil {
			return fmt.Errorf("parse %s: %w", c.DiffPath, err)
		}
		sha, err := headSHA(ctx, c.Repo, c.PR)
		if err != nil {
			return fmt.Errorf("PR #%d (%s): %w", c.PR, c.Repo, err)
		}
		dir := ContextDir(casesPath, c.PR)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte(sha+"\n"), 0o600); err != nil {
			return err
		}
		var stored, skipped int
		for _, p := range review.ContextCandidates(hunks) {
			content, err := fetchFileAtRef(ctx, c.Repo, sha, p)
			if err != nil || len(content) > review.MaxContextFileBytes {
				skipped++
				continue
			}
			// Redacted at write time as well as at load: these files come
			// from other repositories and are committed here, so a key
			// hardcoded in ordinary source must not enter this repo's history.
			content, _ = secrets.RedactBytes(content)
			dest := filepath.Join(dir, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
				return err
			}
			if err := os.WriteFile(dest, content, 0o600); err != nil {
				return err
			}
			stored++
		}
		fmt.Fprintf(w, "nitpick: PR #%d (%s) @ %s: %d context file(s) stored, %d skipped\n",
			c.PR, c.Repo, sha[:min(12, len(sha))], stored, skipped)
	}
	return nil
}

// loadContext returns a fetch func over a case's snapshot directory, for
// review.AttachContext. A missing file reads as an error and is skipped,
// which is also what serve does for a file absent at head.
func loadContext(dir string) func(path string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if strings.Contains(p, "..") {
			return nil, errors.New("unsafe path")
		}
		return os.ReadFile(filepath.Join(dir, filepath.FromSlash(p))) // #nosec G304 -- paths come from the committed diff fixture
	}
}

func headSHA(ctx context.Context, repo string, pr int) (string, error) {
	out, err := runGH(ctx, "pr", "view", fmt.Sprintf("%d", pr), "-R", repo, "--json", "headRefOid", "-q", ".headRefOid")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(out))
	if len(sha) < 7 {
		return "", fmt.Errorf("no head sha in gh output %q", sha)
	}
	return sha, nil
}

func fetchFileAtRef(ctx context.Context, repo, ref, path string) ([]byte, error) {
	segments := strings.Split(path, "/")
	for i, s := range segments {
		if s == ".." {
			return nil, errors.New("unsafe path")
		}
		segments[i] = url.PathEscape(s)
	}
	endpoint := fmt.Sprintf("repos/%s/contents/%s?%s", repo, strings.Join(segments, "/"), url.Values{"ref": {ref}}.Encode())
	return runGH(ctx, "api", "-H", "Accept: application/vnd.github.raw+json", endpoint)
}

func runGH(ctx context.Context, args ...string) ([]byte, error) {
	// #nosec G204 -- args are built here from the committed cases file and
	// GitHub-issued SHAs; the binary is the literal "gh".
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
