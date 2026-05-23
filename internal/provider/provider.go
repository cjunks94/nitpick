// Package provider defines the Review interface implemented by each LLM (or
// stub) backend. Add new backends by writing a new file in this package and
// registering it in New().
package provider

import (
	"context"
	"fmt"

	"github.com/cjunks94/nitpick/internal/config"
	"github.com/cjunks94/nitpick/internal/diff"
)

type Provider interface {
	Name() string
	Review(ctx context.Context, req ReviewRequest) (ReviewResult, error)
}

type ReviewRequest struct {
	Hunks  []diff.Hunk
	Config config.ReviewConfig
	// RepoGuidelines is the optional repo CLAUDE.md / contributor guide. The
	// Anthropic provider should pass this as a cache-controlled system block.
	RepoGuidelines []byte
}

type ReviewResult struct {
	Comments []Comment
	CostUSD  float64
	Tokens   TokenUsage
}

type TokenUsage struct {
	Input       int
	Output      int
	CachedInput int // cache-hit input tokens (Anthropic)
}

type Comment struct {
	File     string
	Line     int // 1-indexed, new-file line
	Severity Severity
	Category string
	Body     string
}

type Severity string

const (
	SeverityNit      Severity = "nit"
	SeverityUseful   Severity = "useful"
	SeverityCritical Severity = "critical"
)

// New returns a provider by name. Register new implementations here.
func New(name string) (Provider, error) {
	switch name {
	case "", "stub":
		return Stub{}, nil
	case "deepseek":
		return nil, fmt.Errorf("deepseek provider not yet implemented — see HANDOFF.md")
	case "anthropic":
		return NewAnthropic("")
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
