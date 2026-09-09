package server

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/cjunks94/nitpick/internal/provider"
)

// ProviderFactory returns a provider bound to the given model id. The server
// uses it to honour a repo's review.escalate config, which can name any model
// the provider supports; Handler.Provider stays the default for everything
// that does not match.
type ProviderFactory func(modelID string) (provider.Provider, error)

// MemoizedProviderFactory wraps provider.New so each model id is constructed
// once per process. Construction is cheap but not free (an SDK client each
// time), and a repo that escalates every PR would otherwise rebuild it per
// review. Errors are not cached: a typo in one repo's config must not poison
// the same model id for another repo that spells it correctly later.
func MemoizedProviderFactory(providerName string) ProviderFactory {
	var (
		mu    sync.Mutex
		cache = map[string]provider.Provider{}
	)
	return func(modelID string) (provider.Provider, error) {
		mu.Lock()
		defer mu.Unlock()
		if p, ok := cache[modelID]; ok {
			return p, nil
		}
		p, err := provider.New(providerName, modelID)
		if err != nil {
			return nil, fmt.Errorf("build provider for model %q: %w", modelID, err)
		}
		cache[modelID] = p
		return p, nil
	}
}

// selectProvider applies review.escalate to the post-ignore_paths hunks and
// returns the provider the review should run on. It always returns a usable
// provider: routing failures (no factory wired, unsupported model) fall back
// to the default and are logged, because a misconfigured escalation must not
// silently skip the review.
func (h *Handler) selectProvider(log *slog.Logger, model, matched string) provider.Provider {
	if matched == "" {
		return h.Provider
	}
	if h.ProviderForModel == nil {
		log.Warn("model escalation configured but no ProviderFactory wired; using default provider",
			"model", model, "matched_path", matched)
		return h.Provider
	}
	p, err := h.ProviderForModel(model)
	if err != nil {
		log.Error("model escalation failed; using default provider",
			"model", model, "matched_path", matched, "err", err)
		return h.Provider
	}
	log.Info("model escalated", "model", model, "matched_path", matched, "provider", p.Name())
	return p
}
