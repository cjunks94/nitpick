package ghc

import (
	"strings"
	"testing"
	"time"

	"github.com/cjunks94/nitpick/internal/provider"
)

func TestBuildStatusCommentBody_silent(t *testing.T) {
	body := BuildStatusCommentBody("anthropic-claude-haiku-4-5", nil, 0.0166, 8100*time.Millisecond)
	checks := []string{
		"no actionable findings",
		"$0.0166",
		"8.1s",
		"claude-haiku-4-5",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("silent body missing %q in: %s", want, body)
		}
	}
	if strings.Contains(body, "anthropic-") {
		t.Errorf("body should strip anthropic- prefix: %s", body)
	}
	if strings.Contains(body, "inline comments") {
		t.Errorf("silent body should not reference inline comments: %s", body)
	}
}

func TestBuildStatusCommentBody_singleFinding(t *testing.T) {
	findings := []provider.Comment{
		{File: "a.go", Line: 1, Severity: provider.SeverityUseful, Body: "x"},
	}
	body := BuildStatusCommentBody("anthropic-claude-sonnet-4-6", findings, 0.012, 18*time.Second)
	if !strings.Contains(body, "1 finding posted") {
		t.Errorf("singular wording missing: %s", body)
	}
	if strings.Contains(body, "1 findings") {
		t.Errorf("body should not pluralize on count of 1: %s", body)
	}
	if !strings.Contains(body, "See inline comments") {
		t.Errorf("findings body should reference inline comments: %s", body)
	}
	if !strings.Contains(body, "claude-sonnet-4-6") {
		t.Errorf("body should include stripped model name: %s", body)
	}
}

func TestBuildStatusCommentBody_multipleFindings(t *testing.T) {
	findings := []provider.Comment{
		{File: "a.go", Line: 1, Severity: provider.SeverityUseful, Body: "x"},
		{File: "b.go", Line: 2, Severity: provider.SeverityCritical, Body: "y"},
		{File: "c.go", Line: 3, Severity: provider.SeverityNit, Body: "z"},
	}
	body := BuildStatusCommentBody("anthropic-claude-haiku-4-5", findings, 0.025, 12*time.Second)
	if !strings.Contains(body, "3 findings posted") {
		t.Errorf("body should pluralize on count of 3: %s", body)
	}
}

func TestBuildStatusCommentBody_nonAnthropicProvider(t *testing.T) {
	body := BuildStatusCommentBody("stub", nil, 0.0, 0)
	if !strings.Contains(body, "`stub`") {
		t.Errorf("non-anthropic provider name should pass through: %s", body)
	}
}
