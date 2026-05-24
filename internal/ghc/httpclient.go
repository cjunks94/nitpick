package ghc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cjunks94/nitpick/internal/provider"
)

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
	Number      int
	HeadSHA     string
	Draft       bool
	Additions   int
	Deletions   int
	UserLogin   string
	UserType    string // User | Bot
	BaseRepo    string // owner/name
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
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
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
	}, nil
}

// FetchFile returns the raw contents of a file at a given commit SHA. Used
// to build the context block for the LLM — the diff alone doesn't show
// definitions, return paths, or framework conventions that live outside
// the changed lines. Returns a NotFound-shaped error when the file doesn't
// exist at that ref (e.g. file was deleted in the PR, or it's a new file
// the API resolves differently).
func (c *HTTPClient) FetchFile(ctx context.Context, repo, sha, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/contents/%s?ref=%s", c.BaseURL, repo, path, sha)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
		return nil, fmt.Errorf("file %s not found at %s", path, sha)
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
