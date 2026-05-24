package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/prompt"
)

// flexInt accepts JSON numbers OR strings convertible to int. Anthropic
// models — especially Sonnet on a verbose prompt — occasionally emit line
// numbers as strings ("80") or as line ranges ("541-543") despite the
// schema asking for int. Without this the whole eval call errors and we
// lose the result. For ranges, the first number wins (closest to the
// start of the change is the most useful comment anchor).
type flexInt int

func (i *flexInt) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*i = flexInt(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("line: not int or string: %s", data)
	}
	s = strings.TrimSpace(s)
	// Handle range form "X-Y" / "X..Y" / "X,Y" by taking the first number.
	for _, sep := range []string{"-", "..", ","} {
		if idx := strings.Index(s, sep); idx > 0 {
			s = s[:idx]
			break
		}
	}
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fmt.Errorf("line %q: not coercible to int", s)
	}
	*i = flexInt(v)
	return nil
}

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
		// Wrapped in <repo-notes> to match the prompt v2.6 contract. Tag
		// name is what the model keys off, not the Go field name (which
		// is still RepoGuidelines for historical reasons — the same field
		// was originally populated from CLAUDE.md before per-repo
		// .nitpick.yaml notes became the canonical source).
		systemBlocks = append(systemBlocks, anthropic.TextBlockParam{
			Text: "<repo-notes source=\".nitpick.yaml\">\n" + string(req.RepoGuidelines) + "\n</repo-notes>",
			CacheControl: anthropic.CacheControlEphemeralParam{
				TTL: anthropic.CacheControlEphemeralTTLTTL1h,
			},
		})
	}

	userText := renderUserMessage(req)

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
// instruction. Strategy: locate the findings-shaped JSON object specifically
// (anchored on the "findings" key) rather than just the first '{', which can
// match prose like "{beforeId: 'aircraft-markers'}". A prose-only response
// with no findings JSON is treated as empty findings, not an error — the
// model declined to flag anything, which is a valid silent review.
func parseFindings(text string) ([]Comment, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	// Anchor on the `"findings"` key; the JSON object surrounding it is the
	// one we want. Falls back to first '{' if anchor not found AND the text
	// starts with '{' (well-formed response with empty object).
	findingsIdx := strings.Index(text, `"findings"`)
	if findingsIdx < 0 {
		if !strings.HasPrefix(text, "{") {
			return nil, nil // prose-only response → silent review
		}
		findingsIdx = 0
	}
	start := strings.LastIndex(text[:findingsIdx+1], "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return nil, nil // findings key but no enclosing braces → silent
	}

	var payload struct {
		Findings []struct {
			File     string  `json:"file"`
			Line     flexInt `json:"line"`
			Severity string  `json:"severity"`
			Category string  `json:"category"`
			Body     string  `json:"body"`
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
			Line:     int(f.Line),
			Severity: sev,
			Category: f.Category,
			Body:     f.Body,
		})
	}
	return out, nil
}

// renderUserMessage builds the full user-side payload: optional context-files
// block (whole-file content for files referenced by the diff, so the model
// can see definitions/return paths/framework conventions outside the diff
// window), followed by the diff itself with line numbers.
//
// Order matters — context first so the model has it loaded when reading the
// diff. The prompt is explicit that ONLY diff lines may be flagged; context
// is read-only background.
func renderUserMessage(req ReviewRequest) string {
	var b strings.Builder
	if len(req.ContextFiles) > 0 {
		b.WriteString("=== CONTEXT FILES (read-only background, do NOT flag issues in these) ===\n")
		b.WriteString("Full content of files referenced by the diff at the PR head SHA. Use these to understand types, return paths, helper definitions, and framework conventions. Findings must still be anchored on lines that appear in the diff below.\n\n")
		for _, cf := range req.ContextFiles {
			fmt.Fprintf(&b, "--- file: %s ---\n%s\n--- end %s ---\n\n", cf.Path, string(cf.Content), cf.Path)
		}
	}
	b.WriteString("=== DIFF (review THESE lines; report findings on added lines only) ===\n")
	b.WriteString("Each changed line is prefixed with its new-file line number. Report findings per the system instructions.\n\n")
	for _, h := range req.Hunks {
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
