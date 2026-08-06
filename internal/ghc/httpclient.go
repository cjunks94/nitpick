package ghc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cjunks94/nitpick/internal/provider"
)

// ErrFileNotFound is wrapped by FetchFile when the GitHub Contents API
// responds 404 at the requested ref. Callers should use errors.Is to
// distinguish expected absence (no .nitpick.yaml, file deleted in PR)
// from auth, rate-limit, or transport failures that warrant a warning.
var ErrFileNotFound = errors.New("file not found")

// ErrUnsafePath is returned by FetchFile when the requested path contains a
// traversal segment. Repository-relative paths never legitimately need "..",
// and the Contents API resolves them server-side, so we reject rather than
// hope GitHub normalizes the way we expect.
var ErrUnsafePath = errors.New("unsafe repository path")

// escapePath percent-encodes each segment of a repository-relative path so it
// can be interpolated into a Contents API URL. Path segments come from the
// unified diff — i.e. from the PR author — so they must be treated as hostile
// input. Without escaping:
//
//   - a "?" in the filename opens a query string, letting the author override
//     the ?ref= parameter and steer the fetch at an arbitrary git ref;
//   - a "#" (e.g. a "C#/Program.cs" directory, which needs no malice at all)
//     turns the rest of the URL into a fragment, silently dropping ?ref= so
//     GitHub serves the file from the default branch instead of the PR head.
//
// url.PathEscape is applied per segment so "/" separators survive.
func escapePath(p string) (string, error) {
	segments := strings.Split(p, "/")
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg == ".." {
			return "", fmt.Errorf("%q: %w", p, ErrUnsafePath)
		}
		out = append(out, url.PathEscape(seg))
	}
	return strings.Join(out, "/"), nil
}

// HTTPClient calls the GitHub REST API directly using an installation token.
// Used by `nitpick serve` where the gh CLI isn't available (Railway container).
// Distinct from the gh-subprocess functions in pr.go / comments.go which the
// local `nitpick review` command uses.
type HTTPClient struct {
	BaseURL    string // defaults to https://api.github.com
	Token      string // installation token (Authorization: token <Token>)
	HTTPClient *http.Client
}

// NewHTTPClient returns a client wired with reasonable defaults.
func NewHTTPClient(token string) *HTTPClient {
	return &HTTPClient{
		BaseURL: "https://api.github.com",
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// PRDetails is the subset of the GitHub PR resource nitpick needs to decide
// whether to review a PR triggered by something other than the pull_request
// webhook (e.g. a /nitpick comment). All fields nitpick keys off live here;
// extending requires updating both the struct and the JSON shape below.
type PRDetails struct {
	Number    int
	HeadSHA   string
	Draft     bool
	Additions int
	Deletions int
	UserLogin string
	UserType  string // User | Bot
	BaseRepo  string // owner/name
	// BaseRef is the branch the PR targets ("main"). Used as the trusted ref
	// for reading .nitpick.yaml when the PR comes from a fork.
	BaseRef string
	// HeadRepo is the owner/name the head branch lives in. Differs from
	// BaseRepo exactly when the PR originates from a fork, which is the
	// signal nitpick uses to decide whether head-SHA config is trustworthy.
	HeadRepo string
}

// IsFork reports whether the PR's head branch lives in a different repository
// than its base — i.e. whether the head commit is authored by someone without
// write access to the base repo. Config and prompt-influencing files read from
// a fork head must be treated as untrusted.
func (p PRDetails) IsFork() bool {
	return p.HeadRepo != "" && p.BaseRepo != "" && p.HeadRepo != p.BaseRepo
}

// FetchPR returns the current state of a PR. Used by triggers that don't
// carry full PR data (issue_comment) — fetches the same fields the
// pull_request webhook would have given us.
func (c *HTTPClient) FetchPR(ctx context.Context, repo string, pr int) (PRDetails, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls/%d", c.BaseURL, repo, pr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return PRDetails{}, err
	}
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return PRDetails{}, fmt.Errorf("fetch PR: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return PRDetails{}, fmt.Errorf("fetch PR: HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var raw struct {
		Number    int  `json:"number"`
		Draft     bool `json:"draft"`
		Additions int  `json:"additions"`
		Deletions int  `json:"deletions"`
		Head      struct {
			SHA  string `json:"sha"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
		Base struct {
			Ref  string `json:"ref"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"base"`
		User struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return PRDetails{}, fmt.Errorf("parse PR response: %w", err)
	}
	return PRDetails{
		Number:    raw.Number,
		HeadSHA:   raw.Head.SHA,
		Draft:     raw.Draft,
		Additions: raw.Additions,
		Deletions: raw.Deletions,
		UserLogin: raw.User.Login,
		UserType:  raw.User.Type,
		BaseRepo:  raw.Base.Repo.FullName,
		BaseRef:   raw.Base.Ref,
		HeadRepo:  raw.Head.Repo.FullName,
	}, nil
}

// Permission levels returned by the collaborator-permission endpoint, ordered
// least to most privileged. GitHub collapses "maintain" and "triage" into this
// same field, so the set below is what the API can actually emit.
const (
	PermNone   = "none"
	PermRead   = "read"
	PermTriage = "triage"
	PermWrite  = "write"
	PermAdmin  = "admin"
)

// CanWrite reports whether a permission string grants push access. Used to
// gate the /nitpick command: anyone can *comment* on a public repo's PR, but
// only someone who could push should be able to spend the operator's LLM
// budget.
func CanWrite(permission string) bool {
	switch permission {
	case PermWrite, PermAdmin, "maintain":
		return true
	default:
		return false
	}
}

// RepoPermission returns the permission level a user holds on a repository:
// one of none | read | triage | write | admin. A 403/404 from this endpoint
// means the installation can't see the collaborator list or the user isn't a
// collaborator; both are reported as PermNone rather than an error so the
// caller fails closed.
func (c *HTTPClient) RepoPermission(ctx context.Context, repo, username string) (string, error) {
	if username == "" {
		return PermNone, nil
	}
	u := fmt.Sprintf("%s/repos/%s/collaborators/%s/permission",
		c.BaseURL, repo, url.PathEscape(username))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return PermNone, err
	}
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return PermNone, fmt.Errorf("fetch repo permission: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		// Not a collaborator, or the App lacks the metadata scope. Fail closed.
		return PermNone, nil
	}
	if resp.StatusCode != http.StatusOK {
		return PermNone, fmt.Errorf("fetch repo permission: HTTP %d: %s",
			resp.StatusCode, truncate(string(body), 300))
	}
	var raw struct {
		Permission string `json:"permission"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return PermNone, fmt.Errorf("parse permission response: %w", err)
	}
	if raw.Permission == "" {
		return PermNone, nil
	}
	return raw.Permission, nil
}

// FetchFile returns the raw contents of a file at a given commit SHA. Used
// to build the context block for the LLM — the diff alone doesn't show
// definitions, return paths, or framework conventions that live outside
// the changed lines. Returns a NotFound-shaped error when the file doesn't
// exist at that ref (e.g. file was deleted in the PR, or it's a new file
// the API resolves differently).
func (c *HTTPClient) FetchFile(ctx context.Context, repo, sha, path string) ([]byte, error) {
	escaped, err := escapePath(path)
	if err != nil {
		return nil, err
	}
	// Query built with url.Values rather than string concatenation so a "?"
	// or "&" surviving inside a segment can't graft extra parameters on.
	u := fmt.Sprintf("%s/repos/%s/contents/%s?%s",
		c.BaseURL, repo, escaped, url.Values{"ref": {sha}}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch file %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("file %s not found at %s: %w", path, sha, ErrFileNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch file %s: HTTP %d: %s", path, resp.StatusCode, truncate(string(body), 300))
	}
	return body, nil
}

// FetchDiff returns the unified diff for a PR via the REST API. Equivalent
// to `gh pr diff <n>` but uses the installation token. The media type header
// is what makes GitHub return raw diff text rather than the JSON resource.
func (c *HTTPClient) FetchDiff(ctx context.Context, repo string, pr int) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls/%d", c.BaseURL, repo, pr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Accept", "application/vnd.github.diff")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch diff: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch diff: HTTP %d: %s", resp.StatusCode, truncate(string(body), 500))
	}
	return body, nil
}

// PostReview posts a single PR review with inline comments via the REST API.
// Equivalent to comments.go:PostReview but uses the installation token. The
// body shape is identical (shared via BuildReviewBody) — only the transport
// differs.
func (c *HTTPClient) PostReview(ctx context.Context, repo string, pr int, comments []provider.Comment) error {
	if len(comments) == 0 {
		return nil
	}
	body, err := BuildReviewBody(comments)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/repos/%s/pulls/%d/reviews", c.BaseURL, repo, pr)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("post review: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("post review: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}
	return nil
}

// truncate caps s at n bytes without splitting a multi-byte rune. These
// strings end up in error messages that are JSON-encoded into slog output; a
// half-rune would render as U+FFFD noise at best.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return TruncateBytes(s, n) + "..."
}

// TruncateBytes returns the longest prefix of s that is at most n bytes and
// ends on a rune boundary. Exported because the server applies the same cap to
// .nitpick.yaml context_notes before handing them to the provider.
func TruncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
