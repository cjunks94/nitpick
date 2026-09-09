package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/cjunks94/nitpick/internal/config"
	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/provider"
)

// namedProvider is a provider that only knows its name; enough to tell which
// one selectProvider handed back.
type namedProvider struct{ name string }

func (n namedProvider) Name() string { return n.name }
func (n namedProvider) Review(context.Context, provider.ReviewRequest) (provider.ReviewResult, error) {
	return provider.ReviewResult{}, nil
}

func escalatingConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(`
model: claude-haiku-4-5
review:
  escalate:
    model: claude-sonnet-4-6
    paths: ["auth/**", "migrations/**"]
`))
	if err != nil {
		t.Fatal(err)
	}
	return &cfg
}

func hunksFor(files ...string) []diff.Hunk {
	out := make([]diff.Hunk, 0, len(files))
	for _, f := range files {
		out = append(out, diff.Hunk{File: f})
	}
	return out
}

// resolved mirrors what review.Prepare hands selectProvider: the escalation
// model and the file that matched, or "" when nothing did.
func resolved(cfg *config.Config, files ...string) (model, matched string) {
	if cfg == nil {
		return "", ""
	}
	return cfg.ModelFor(diff.Files(hunksFor(files...)))
}

func selectFor(h *Handler, log *slog.Logger, cfg *config.Config, files ...string) provider.Provider {
	model, matched := resolved(cfg, files...)
	return h.selectProvider(log, model, matched)
}

func TestSelectProvider(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	def := namedProvider{"default"}

	t.Run("no repo config uses default", func(t *testing.T) {
		h := &Handler{Provider: def}
		if got := selectFor(h, log, nil, "auth/x.go"); got != def {
			t.Fatalf("got %s, want default", got.Name())
		}
	})

	t.Run("no matching path uses default without calling factory", func(t *testing.T) {
		h := &Handler{Provider: def, ProviderForModel: func(string) (provider.Provider, error) {
			t.Fatal("factory must not be called when nothing matches")
			return nil, nil
		}}
		if got := selectFor(h, log, escalatingConfig(t), "README.md", "web/app.js"); got != def {
			t.Fatalf("got %s, want default", got.Name())
		}
	})

	t.Run("matching path routes to the escalation model", func(t *testing.T) {
		var asked string
		h := &Handler{Provider: def, ProviderForModel: func(m string) (provider.Provider, error) {
			asked = m
			return namedProvider{"escalated"}, nil
		}}
		got := selectFor(h, log, escalatingConfig(t), "README.md", "migrations/1.sql")
		if got.Name() != "escalated" {
			t.Fatalf("got %s, want escalated", got.Name())
		}
		if asked != "claude-sonnet-4-6" {
			t.Fatalf("factory asked for %q, want claude-sonnet-4-6", asked)
		}
	})

	t.Run("factory error falls back to default rather than skipping the review", func(t *testing.T) {
		h := &Handler{Provider: def, ProviderForModel: func(string) (provider.Provider, error) {
			return nil, errors.New("unsupported model")
		}}
		if got := selectFor(h, log, escalatingConfig(t), "auth/x.go"); got != def {
			t.Fatalf("got %s, want default", got.Name())
		}
	})

	t.Run("no factory wired falls back to default", func(t *testing.T) {
		h := &Handler{Provider: def}
		if got := selectFor(h, log, escalatingConfig(t), "auth/x.go"); got != def {
			t.Fatalf("got %s, want default", got.Name())
		}
	})
}

func TestMemoizedProviderFactory(t *testing.T) {
	f := MemoizedProviderFactory("stub")
	a, err := f("")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f("")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("same model id should return the cached provider")
	}

	if _, err := MemoizedProviderFactory("no-such-provider")("m"); err == nil {
		t.Fatal("unknown provider should error, not be cached as nil")
	}
}
