package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// minimalHandler builds a Handler stripped of LLM/auth dependencies — just
// enough to exercise the issue_comment routing logic. TokenSource and
// Provider are nil; tests that touch them won't reach those paths.
func minimalHandler(secret string) *Handler {
	return &Handler{
		WebhookSecret:  secret,
		MaxLinesPerPR:  1000,
		SkipUserLogins: []string{"dependabot[bot]"},
		Logger:         silentLogger(),
		seen:           make(map[string]time.Time),
	}
}

func signedRequest(t *testing.T, secret string, eventType string, payload []byte) *http.Request {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", eventType)
	req.Header.Set("X-GitHub-Delivery", "test-delivery")
	return req
}

// commentPayload builds an issue_comment webhook body. action / body / userType
// are the dimensions tests vary; everything else is reasonable defaults.
func commentPayload(action, commentBody, userType string, isPR bool) []byte {
	prField := "null"
	if isPR {
		prField = `{"url":"https://api.github.com/repos/x/y/pulls/1"}`
	}
	return []byte(`{
		"action": "` + action + `",
		"comment": {
			"body": "` + strings.ReplaceAll(commentBody, `"`, `\"`) + `",
			"user": {"login": "alice", "type": "` + userType + `"}
		},
		"issue": {
			"number": 42,
			"pull_request": ` + prField + `
		},
		"repository": {"full_name": "owner/repo"},
		"installation": {"id": 12345}
	}`)
}

func TestIssueComment_TriggerPhrases(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantFires bool
	}{
		{"exact slash command", "/nitpick", true},
		{"with verb", "/nitpick review", true},
		{"case insensitive", "/Nitpick PLEASE", true},
		{"inline in sentence", "hey, please /nitpick this", true},
		{"no trigger", "lgtm!", false},
		{"only mentions name without slash", "@nitpick can you review?", false},
		{"empty body", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := "topsecret"
			h := minimalHandler(secret)

			payload := commentPayload("created", tt.body, "User", true)
			req := signedRequest(t, secret, "issue_comment", payload)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			// Status is always 202 (we don't surface trigger-yes vs trigger-no
			// to GitHub — both are valid Acceptance from its POV). The signal
			// we check is the response body: if trigger fired, the response
			// JSON includes "trigger":"comment". If not, it's a bare ack.
			fired := strings.Contains(rec.Body.String(), `"trigger":"comment"`)
			if fired != tt.wantFires {
				t.Errorf("body=%q: fired=%v, want %v (response: %q)",
					tt.body, fired, tt.wantFires, rec.Body.String())
			}
		})
	}
}

func TestIssueComment_SkipsBotComments(t *testing.T) {
	// Even if a bot uses the trigger phrase, don't fire — prevents loops if
	// nitpick or another bot ever quotes "/nitpick" in its own output.
	secret := "topsecret"
	h := minimalHandler(secret)

	payload := commentPayload("created", "/nitpick review", "Bot", true)
	req := signedRequest(t, secret, "issue_comment", payload)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), `"trigger":"comment"`) {
		t.Error("Bot-authored comment should not fire trigger; got body: " + rec.Body.String())
	}
}

func TestIssueComment_SkipsIssueComments(t *testing.T) {
	// Comments on issues (not PRs) include pull_request: null. Ignore.
	secret := "topsecret"
	h := minimalHandler(secret)

	payload := commentPayload("created", "/nitpick", "User", false)
	req := signedRequest(t, secret, "issue_comment", payload)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), `"trigger":"comment"`) {
		t.Error("issue (non-PR) comment should not fire trigger")
	}
}

func TestIssueComment_SkipsNonCreatedActions(t *testing.T) {
	// Edited comments don't re-trigger; otherwise typo-fixes would spam reviews.
	secret := "topsecret"
	h := minimalHandler(secret)

	payload := commentPayload("edited", "/nitpick", "User", true)
	req := signedRequest(t, secret, "issue_comment", payload)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), `"trigger":"comment"`) {
		t.Error("edited action should not fire trigger")
	}
}

// Sanity check that the handler returns fast and doesn't block on the async
// goroutine (which would try to mint tokens against nil TokenSource).
func TestIssueComment_ReturnsFastWithoutBlocking(t *testing.T) {
	secret := "topsecret"
	h := minimalHandler(secret)

	payload := commentPayload("created", "/nitpick", "User", true)
	req := signedRequest(t, secret, "issue_comment", payload)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ServeHTTP didn't return within 2s — likely blocking on goroutine")
	}
}

// Suppress unused-import warnings if the file becomes the only one using these.
var _ atomic.Int32