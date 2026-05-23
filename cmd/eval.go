package cmd

import (
	"context"
	"flag"
	"fmt"

	"github.com/cjunks94/nitpick/internal/eval"
	"github.com/cjunks94/nitpick/internal/provider"
)

// Eval runs the nitpick eval subcommand — replays labeled PR cases against a
// provider and writes a markdown report.
func Eval(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("eval", flag.ContinueOnError)
	casesPath := flags.String("cases", "eval/cases/cases.jsonl", "path to cases.jsonl")
	providerName := flags.String("provider", "stub", "stub | deepseek | anthropic")
	model := flags.String("model", "", "model id override (anthropic: claude-haiku-4-5 default, claude-sonnet-4-6 escalation)")
	outPath := flags.String("out", "eval/REPORT.md", "report output path")
	guidelines := flags.Bool("guidelines", false, "load per-repo CLAUDE.md from eval/cases/repos/ as cached context (opt-in; default off after 3v3 A/B showed no win)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	p, err := provider.New(*providerName, *model)
	if err != nil {
		return err
	}
	if err := eval.Run(ctx, *casesPath, *outPath, p, *guidelines); err != nil {
		return err
	}
	fmt.Printf("nitpick: report written to %s\n", *outPath)
	return nil
}
