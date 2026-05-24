package ghc

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cjunks94/nitpick/internal/provider"
)

func TestRenderReviewSummary(t *testing.T) {
	tests := []struct {
		name     string
		n        int
		contains []string // every substring must appear in the output
	}{
		{
			name: "single finding (singular)",
			n:    1,
			contains: []string{
				"nitpick review — 1 finding",
				"[CRITICAL]",
				"[USEFUL]",
				"[NIT]",
				"complements CodeRabbit",
				"github.com/cjunks94/nitpick",
			},
		},
		{
			name: "multiple findings (plural)",
			n:    3,
			contains: []string{
				"nitpick review — 3 findings",
				"SEVERITY",
				"SCOPE",
				"SOURCE",
			},
		},
		{
			name: "zero (defensive; called only when n>0 in practice)",
			n:    0,
			contains: []string{
				"nitpick review — 0 findings",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderReviewSummary(tt.n)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("summary missing %q.\nGot:\n%s", want, got)
				}
			}
			if !strings.HasPrefix(got, "```\n") || !strings.HasSuffix(got, "\n```") {
				t.Errorf("summary should be wrapped in a fenced code block; got:\n%s", got)
			}
		})
	}
}

func TestRenderBody(t *testing.T) {
	tests := []struct {
		name string
		in   provider.Comment
		want string
	}{
		{
			name: "critical with category",
			in:   provider.Comment{Severity: provider.SeverityCritical, Category: "security", Body: "SQL injection risk."},
			want: "`[CRITICAL · security]`\n\nSQL injection risk.\n\n`— nitpick`",
		},
		{
			name: "useful no category",
			in:   provider.Comment{Severity: provider.SeverityUseful, Body: "Missing nil guard."},
			want: "`[USEFUL]`\n\nMissing nil guard.\n\n`— nitpick`",
		},
		{
			name: "nit default for unknown severity",
			in:   provider.Comment{Severity: "weird", Body: "what."},
			want: "`[NIT]`\n\nwhat.\n\n`— nitpick`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderBody(tt.in)
			if got != tt.want {
				t.Errorf("renderBody() mismatch.\nwant: %q\ngot:  %q", tt.want, got)
			}
		})
	}
}

func TestBuildReviewBody_Structure(t *testing.T) {
	// Smoke check: confirm the JSON payload still has the keys GitHub's
	// review API expects, after the rendering refactor.
	body, err := BuildReviewBody([]provider.Comment{
		{File: "a.go", Line: 10, Severity: provider.SeverityUseful, Body: "x"},
	})
	if err != nil {
		t.Fatalf("BuildReviewBody: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["event"] != "COMMENT" {
		t.Errorf("event = %v, want COMMENT", got["event"])
	}
	if _, ok := got["body"].(string); !ok {
		t.Errorf("body should be a string")
	}
	comments, ok := got["comments"].([]any)
	if !ok || len(comments) != 1 {
		t.Fatalf("comments should be a 1-element array, got %v", got["comments"])
	}
	c := comments[0].(map[string]any)
	if c["path"] != "a.go" || c["side"] != "RIGHT" {
		t.Errorf("comment payload missing required fields: %v", c)
	}
}
