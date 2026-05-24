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
	// ContextFiles is the full content of files referenced by the diff at the
	// PR head SHA. Lets the reviewer see definitions, return paths, and
	// framework conventions that don't appear in the changed lines. Empty in
	// the local CLI path; populated by `nitpick serve` after fetching via the
	// installation token. The provider renders these in the user message as a
	// labeled context block; the prompt instructs the model to flag only diff
	// lines, treating context as read-only background.
	ContextFiles []ContextFile
}

// ContextFile is one whole-file context entry: path relative to repo root +
// raw content. Capped upstream so the user message stays under model context.
type ContextFile struct {
	Path    string
	Content []byte
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
// modelID is honored only by providers that take a model (anthropic, deepseek);
// pass "" to use the provider's default.
func New(name, modelID string) (Provider, error) {
	switch name {
	case "", "stub":
		return Stub{}, nil
	case "deepseek":
		return nil, fmt.Errorf("deepseek provider not yet implemented — see HANDOFF.md")
	case "anthropic":
		return NewAnthropic(modelID)
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
