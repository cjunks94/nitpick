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
	outPath := flags.String("out", "eval/REPORT.md", "report output path")
	noGuidelines := flags.Bool("no-guidelines", false, "skip loading per-repo CLAUDE.md (for variance baseline)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	p, err := provider.New(*providerName)
	if err != nil {
		return err
	}
	if err := eval.Run(ctx, *casesPath, *outPath, p, !*noGuidelines); err != nil {
		return err
	}
	fmt.Printf("nitpick: report written to %s\n", *outPath)
	return nil
}
