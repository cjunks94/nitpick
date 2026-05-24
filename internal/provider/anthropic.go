package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/prompt"
)

// Anthropic is the production provider. Single-shot call per PR with the
// static review prompt + repo CLAUDE.md cached as ephemeral 1h-TTL system
// blocks — the first call for a given repo writes the cache (~2× input price
// on the prefix), every subsequent call within the TTL reads it (~0.1×).
// That's the cost story for the resume bullet; verify cache_read_input_tokens
// rises on repeat calls before claiming the number.
type Anthropic struct {
	client anthropic.Client
	model  anthropic.Model
}

// pricePerMTok is USD per 1M tokens. Sourced from the model catalog cached at
// 2026-04-15 in the claude-api skill. Re-verify with a live price check before
// publishing cost-savings claims.
type pricePerMTok struct {
	input, output float64
}

var priceTable = map[anthropic.Model]pricePerMTok{
	anthropic.ModelClaudeHaiku4_5:  {input: 1.00, output: 5.00},
	anthropic.ModelClaudeSonnet4_6: {input: 3.00, output: 15.00},
}

// NewAnthropic returns a provider. modelID overrides the default; empty
// string picks the Haiku tier (cheap, sufficient for most PRs).
func NewAnthropic(modelID string) (Anthropic, error) {
	model := anthropic.ModelClaudeHaiku4_5
	if modelID != "" {
		model = anthropic.Model(modelID)
		if _, ok := priceTable[model]; !ok {
			return Anthropic{}, fmt.Errorf("unsupported model %q: add it to priceTable before use", modelID)
		}
	}
	return Anthropic{
		client: anthropic.NewClient(), // reads ANTHROPIC_API_KEY env var
		model:  model,
	}, nil
}

func (a Anthropic) Name() string { return "anthropic-" + string(a.model) }

func (a Anthropic) Review(ctx context.Context, req ReviewRequest) (ReviewResult, error) {
	systemBlocks := []anthropic.TextBlockParam{
		{
			Text: prompt.For(string(a.model)),
			CacheControl: anthropic.CacheControlEphemeralParam{
				TTL: anthropic.CacheControlEphemeralTTLTTL1h,
			},
		},
	}
	if len(req.RepoGuidelines) > 0 {
		systemBlocks = append(systemBlocks, anthropic.TextBlockParam{
			Text: "<repo-guidelines>\n" + string(req.RepoGuidelines) + "\n</repo-guidelines>",
			CacheControl: anthropic.CacheControlEphemeralParam{
				TTL: anthropic.CacheControlEphemeralTTLTTL1h,
			},
		})
	}

	userText := renderHunks(req.Hunks)

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     a.model,
		MaxTokens: 4096,
		System:    systemBlocks,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userText)),
		},
	})
	if err != nil {
		return ReviewResult{}, fmt.Errorf("anthropic Messages.New: %w", err)
	}

	text := extractText(resp)
	comments, err := parseFindings(text)
	if err != nil {
		return ReviewResult{}, fmt.Errorf("parse findings: %w (raw: %s)", err, truncate(text, 200))
	}

	usage := TokenUsage{
		Input:       int(resp.Usage.InputTokens) + int(resp.Usage.CacheCreationInputTokens),
		Output:      int(resp.Usage.OutputTokens),
		CachedInput: int(resp.Usage.CacheReadInputTokens),
	}

	return ReviewResult{
		Comments: comments,
		Tokens:   usage,
		CostUSD:  a.cost(resp.Usage),
	}, nil
}

// cost computes USD from the four token buckets. Cache writes are billed at
// ~2× input price for 1h TTL; cache reads at ~0.1×. Verify against the live
// pricing page before quoting savings — these multipliers shift occasionally.
func (a Anthropic) cost(u anthropic.Usage) float64 {
	p := priceTable[a.model]
	const million = 1_000_000.0
	const cacheWrite1hMultiplier = 2.0
	const cacheReadMultiplier = 0.1
	return (float64(u.InputTokens)*p.input +
		float64(u.CacheCreationInputTokens)*p.input*cacheWrite1hMultiplier +
		float64(u.CacheReadInputTokens)*p.input*cacheReadMultiplier +
		float64(u.OutputTokens)*p.output) / million
}

func extractText(m *anthropic.Message) string {
	var b strings.Builder
	for _, block := range m.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(tb.Text)
		}
	}
	return b.String()
}

// parseFindings handles JSON the model may have wrapped in prose despite the
// instruction. Strips a leading ```json fence if present, then locates the
// first { and parses through the matching }.
func parseFindings(text string) ([]Comment, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON object found")
	}

	var payload struct {
		Findings []struct {
			File     string `json:"file"`
			Line     int    `json:"line"`
			Severity string `json:"severity"`
			Category string `json:"category"`
			Body     string `json:"body"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &payload); err != nil {
		return nil, err
	}

	out := make([]Comment, 0, len(payload.Findings))
	for _, f := range payload.Findings {
		sev := Severity(f.Severity)
		if sev != SeverityCritical && sev != SeverityUseful && sev != SeverityNit {
			sev = SeverityUseful // be charitable on unknown labels
		}
		out = append(out, Comment{
			File:     f.File,
			Line:     f.Line,
			Severity: sev,
			Category: f.Category,
			Body:     f.Body,
		})
	}
	return out, nil
}

// renderHunks formats parsed hunks for the user message. Includes new-file
// line numbers in the gutter so the model can cite them back accurately.
func renderHunks(hunks []diff.Hunk) string {
	var b strings.Builder
	b.WriteString("Review the following PR diff and report findings per the system instructions. Each changed line is prefixed with its new-file line number.\n\n")
	for _, h := range hunks {
		fmt.Fprintf(&b, "=== %s ===\n", h.File)
		fmt.Fprintf(&b, "@@ +%d,%d @@\n", h.NewStart, h.NewLines)
		for _, line := range h.Lines {
			switch line.Kind {
			case diff.LineAdded:
				fmt.Fprintf(&b, "%5d + %s\n", line.NewLineNum, line.Content)
			case diff.LineRemoved:
				fmt.Fprintf(&b, "    - - %s\n", line.Content)
			default:
				fmt.Fprintf(&b, "%5d   %s\n", line.NewLineNum, line.Content)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
