package ghc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Accept media types. GitHub selects the representation by header: the same
// /pulls/{n} URL returns JSON or a unified diff depending on this.
const (
	acceptJSON = "application/vnd.github+json"
	acceptRaw  = "application/vnd.github.raw+json"
	acceptDiff = "application/vnd.github.diff"
)

// Response body caps. The server caps inbound webhook bodies at 5 MiB; these
// bound the other direction so a pathological response cannot grow memory
// without limit. A body over its cap is an error, never a truncated success.
const (
	maxDiffBytes = 8 << 20 // a PR under MaxLinesPerPR can still carry long lines
	maxFileBytes = 1 << 20 // the server applies its own tighter context cap after
	maxJSONBytes = 1 << 20
)

const (
	defaultMaxAttempts  = 3
	defaultRetryBackoff = 500 * time.Millisecond
	// maxRetryAfter bounds how long a Retry-After or rate-limit reset is
	// honoured. Past this the call fails rather than parking a review slot.
	maxRetryAfter = 30 * time.Second
	userAgent     = "nitpick (+https://github.com/cjunks94/nitpick)"
)

// ErrRateLimited is wrapped by do when GitHub still answers 429, or 403 with
// rate-limit headers, after every retry. Callers use errors.Is to keep it
// apart from a permission denial, which also arrives as 403.
var ErrRateLimited = errors.New("github rate limited")

// ErrBodyTooLarge is wrapped by do when a response exceeds its cap.
var ErrBodyTooLarge = errors.New("response body exceeds cap")

func (c *HTTPClient) attempts() int {
	if c.MaxAttempts > 0 {
		return c.MaxAttempts
	}
	return defaultMaxAttempts
}

// delay returns the backoff before retry number attempt (1-based): base,
// then 4x per step, so the defaults give 500ms then 2s.
func (c *HTTPClient) delay(attempt int) time.Duration {
	base := c.RetryBackoff
	if base <= 0 {
		base = defaultRetryBackoff
	}
	d := base
	for i := 1; i < attempt; i++ {
		d *= 4
	}
	return d
}

// do performs one GitHub API call with the client's auth headers and returns
// the status and the complete body. It is the only place in this package that
// builds a request, so the read-error check, the body cap, the User-Agent, and
// the retry policy cannot drift between endpoints the way they did when every
// method carried its own copy.
//
// Retry policy, bounded by MaxAttempts:
//   - rate limited (429, or 403 carrying Retry-After or X-RateLimit-Remaining:
//     0): retried for any method, since the request was not processed; the
//     wait honours Retry-After / X-RateLimit-Reset up to maxRetryAfter.
//   - transport error or 5xx: retried for GET only. A POST that reached GitHub
//     may have been applied even if the response was lost, and a duplicated
//     review is worse than a missing one.
//   - anything else is returned as is; the caller maps status to meaning.
//
// A body that cannot be read to the end is an error, never a short success:
// a truncated diff parses cleanly as a smaller diff and would be reviewed,
// billed, and posted with findings anchored on an incomplete view.
func (c *HTTPClient) do(ctx context.Context, method, u, accept string, body []byte, maxBody int64) (int, []byte, error) {
	for attempt := 1; ; attempt++ {
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rdr)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", "token "+c.Token)
		req.Header.Set("Accept", accept)
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("User-Agent", userAgent)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		more := attempt < c.attempts()
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			if method == http.MethodGet && more && ctx.Err() == nil {
				if werr := sleepCtx(ctx, c.delay(attempt)); werr == nil {
					continue
				}
			}
			return 0, nil, err
		}
		respBody, readErr := readCapped(resp.Body, maxBody)
		_ = resp.Body.Close()
		if readErr != nil {
			if errors.Is(readErr, ErrBodyTooLarge) {
				return resp.StatusCode, nil, readErr
			}
			if method == http.MethodGet && more && ctx.Err() == nil {
				if werr := sleepCtx(ctx, c.delay(attempt)); werr == nil {
					continue
				}
			}
			return resp.StatusCode, nil, fmt.Errorf("read response: %w", readErr)
		}

		if limited, wait := rateLimited(resp); limited {
			if more && wait <= maxRetryAfter {
				if werr := sleepCtx(ctx, wait); werr == nil {
					continue
				}
			}
			return resp.StatusCode, respBody, fmt.Errorf("HTTP %d: %w", resp.StatusCode, ErrRateLimited)
		}
		if resp.StatusCode >= 500 && method == http.MethodGet && more {
			if werr := sleepCtx(ctx, c.delay(attempt)); werr == nil {
				continue
			}
		}
		return resp.StatusCode, respBody, nil
	}
}

// readCapped reads r to the end, failing if it holds more than max bytes.
func readCapped(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%d+ bytes, cap %d: %w", len(b), max, ErrBodyTooLarge)
	}
	return b, nil
}

// rateLimited reports whether resp is a rate-limit response and how long
// GitHub asks the client to wait. 429 is unambiguous; 403 is also what a
// permission denial looks like, so it counts only with the rate-limit
// headers present. With no usable header the wait is one backoff step.
func rateLimited(resp *http.Response) (bool, time.Duration) {
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
	case http.StatusForbidden:
		if resp.Header.Get("Retry-After") == "" && resp.Header.Get("X-RateLimit-Remaining") != "0" {
			return false, 0
		}
	default:
		return false, 0
	}
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs >= 0 {
			return true, time.Duration(secs) * time.Second
		}
	}
	if s := resp.Header.Get("X-RateLimit-Reset"); s != "" {
		if unix, err := strconv.ParseInt(s, 10, 64); err == nil {
			if wait := time.Until(time.Unix(unix, 0)); wait > 0 {
				return true, wait
			}
			return true, 0
		}
	}
	return true, defaultRetryBackoff
}

// sleepCtx waits for d or until ctx is done, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
