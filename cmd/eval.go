package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/cjunks94/nitpick/internal/eval"
	"github.com/cjunks94/nitpick/internal/provider"
)

// Eval runs the nitpick eval subcommand — replays labeled PR cases against a
// provider and writes a markdown report.
func Eval(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("eval", flag.ContinueOnError)
	casesPath := flags.String("cases", "eval/cases/cases.jsonl", "path to cases.jsonl")
	providerName := flags.String("provider", "stub", "stub | anthropic")
	model := flags.String("model", "", "model id override (anthropic: claude-haiku-4-5 default; also claude-sonnet-4-6, claude-sonnet-5, claude-opus-5)")
	outPath := flags.String("out", "eval/REPORT.md", "report output path")
	guidelines := flags.Bool("guidelines", false, "load per-repo CLAUDE.md from eval/cases/repos/ as cached context (opt-in; default off after 3v3 A/B showed no win)")
	withContext := flags.Bool("context", false, "attach the whole-file context serve would fetch, from the snapshots under eval/cases/testdata/context/ (run --snapshot-context first)")
	snapshot := flags.Bool("snapshot-context", false, "fetch each case's context files at the PR head SHA via gh into eval/cases/testdata/context/ and exit")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *snapshot {
		return eval.Snapshot(ctx, *casesPath, os.Stdout)
	}
	p, err := provider.New(*providerName, *model)
	if err != nil {
		return err
	}
	if err := eval.RunWithOptions(ctx, *casesPath, *outPath, p, eval.Options{Guidelines: *guidelines, Context: *withContext}); err != nil {
		return err
	}
	fmt.Printf("nitpick: report written to %s\n", *outPath)
	return nil
}
