package prompt

import (
	"strings"
	"testing"
)

// The two ids in provider.priceTable, hard-coded here because provider
// imports prompt and the reverse import would be a cycle. If priceTable
// grows, add the id here too.
var pricedModels = []string{"claude-haiku-4-5", "claude-sonnet-4-6"}

// contractMarkers are the phrases the provider and server rely on the prompt
// carrying: the empty-findings shape parseFindings expects, the <repo-notes>
// block name anthropic.go wraps guidelines in (v2.6), the override strength
// the server's configRef/fork guard assumes (v2.7), and the CONTEXT FILES
// label renderUserMessage emits (v2.3). Losing any of these silently breaks a
// contract that lives on the other side of the eval gate.
var contractMarkers = []string{
	`{"findings":[]}`,
	`"findings":[{"file":`,
	"<repo-notes>",
	"MANDATORY OVERRIDE",
	"CONTEXT FILES",
	"STRICT JSON only",
}

func TestFor_EveryPricedModelGetsThePrompt(t *testing.T) {
	for _, id := range pricedModels {
		t.Run(id, func(t *testing.T) {
			got := For(id)
			if strings.TrimSpace(got) == "" {
				t.Fatalf("For(%q) returned an empty prompt", id)
			}
			for _, m := range contractMarkers {
				if !strings.Contains(got, m) {
					t.Errorf("For(%q) lacks contract marker %q", id, m)
				}
			}
		})
	}
}

// The dispatcher is model-agnostic today (per-model variants were tried in
// 19b1d2d and reverted). An unknown or empty id must fall back to the default
// prompt rather than an empty string, which would send the model no
// instructions at all.
func TestFor_UnknownModelFallsBackToDefault(t *testing.T) {
	def := For(pricedModels[0])
	for _, id := range []string{"", "claude-opus-9", "gpt-4o", "not a model"} {
		got := For(id)
		if got == "" {
			t.Errorf("For(%q) returned an empty prompt", id)
			continue
		}
		if got != def {
			t.Errorf("For(%q) differs from the default prompt; unknown ids should fall back", id)
		}
	}
}

// Sanity floor on size: the prompt is one cached system block, and v2.8
// deliberately compressed it because length cost recall. A prompt that
// balloons past this is a signal to re-run the ablation, not a hard rule.
func TestFor_LengthStaysCompressed(t *testing.T) {
	const maxBytes = 12 << 10
	if n := len(For(pricedModels[0])); n > maxBytes {
		t.Errorf("prompt is %d bytes, over the %d-byte compression floor set in v2.8 — re-run the eval ablation", n, maxBytes)
	}
}
