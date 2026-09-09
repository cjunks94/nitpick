package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func serveThrough(t *testing.T, req *http.Request) (*httptest.ResponseRecorder, []map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(RequestID(r.Context())))
	})
	rec := httptest.NewRecorder()
	withRequestLogging(inner, logger).ServeHTTP(rec, req)
	var lines []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("log line is not JSON: %q", l)
		}
		lines = append(lines, m)
	}
	return rec, lines
}

// The workspace standard: honour a valid inbound X-Request-ID so platform and
// CDN traces flow end to end, echo it, and put it on the access log line.
func TestRequestID_ValidInboundIsHonouredEchoedAndLogged(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Header.Set("X-Request-ID", "trace-Abc_123")
	rec, lines := serveThrough(t, req)

	if got := rec.Header().Get("X-Request-ID"); got != "trace-Abc_123" {
		t.Errorf("echoed id = %q, want the inbound one", got)
	}
	if rec.Body.String() != "trace-Abc_123" {
		t.Errorf("handler saw id %q via context, want the inbound one", rec.Body.String())
	}
	if len(lines) != 1 || lines[0]["request_id"] != "trace-Abc_123" {
		t.Errorf("access log line = %v, want request_id=trace-Abc_123", lines)
	}
}

func TestRequestID_InvalidInboundIsReplaced(t *testing.T) {
	hexID := regexp.MustCompile(`^[0-9a-f]{24}$`)
	for name, bad := range map[string]string{
		"newline":  "abc\ndef",
		"too long": strings.Repeat("x", 129),
		"spaces":   "has space",
		"escape":   "\x1b[31mred",
		"empty":    "",
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
			req.Header.Set("X-Request-ID", bad)
			rec, lines := serveThrough(t, req)
			got := rec.Header().Get("X-Request-ID")
			if !hexID.MatchString(got) {
				t.Errorf("replacement id = %q, want 24 hex chars", got)
			}
			if got == bad {
				t.Error("invalid inbound id must never be echoed")
			}
			if len(lines) != 1 || lines[0]["request_id"] != got {
				t.Errorf("log request_id = %v, want %q", lines, got)
			}
		})
	}
}

// GitHub's own delivery GUID is the natural id when no platform trace id is
// present, so the access line and the handler's delivery_id lines join up.
func TestRequestID_FallsBackToGitHubDelivery(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Header.Set("X-GitHub-Delivery", "72d3162e-cc78-11e3-81ab-4c9367dc0958")
	rec, _ := serveThrough(t, req)
	if got := rec.Header().Get("X-Request-ID"); got != "72d3162e-cc78-11e3-81ab-4c9367dc0958" {
		t.Errorf("id = %q, want the delivery GUID", got)
	}
}

// /healthz is polled constantly and stays out of the log, but the id is
// still resolved and echoed so a probe can be correlated if needed.
func TestRequestID_HealthzEchoesButDoesNotLog(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec, lines := serveThrough(t, req)
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("healthz response lacks X-Request-ID")
	}
	if len(lines) != 0 {
		t.Errorf("healthz was logged: %v", lines)
	}
}

// A Handler with no secret must not verify anything: HMAC over "" would
// accept any payload signed with "", i.e. an unauthenticated webhook.
func TestServeHTTP_EmptySecretRefuses(t *testing.T) {
	h := minimalHandler("")
	req := signedRequest(t, "", "ping", []byte(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for an unconfigured secret", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Error("response body should not describe the misconfiguration")
	}
}
