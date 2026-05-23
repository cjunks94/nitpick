package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/cjunks94/nitpick/cmd"
)

const version = "v0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}

	ctx := context.Background()
	var err error

	switch os.Args[1] {
	case "review":
		err = cmd.Review(ctx, os.Args[2:])
	case "eval":
		err = cmd.Eval(ctx, os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("nitpick", version)
	case "help", "-h", "--help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "nitpick: unknown subcommand %q\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `nitpick — self-hosted AI code review for GitHub pull requests

Usage:
  nitpick review --pr <number> [flags]
  nitpick eval   [flags]
  nitpick version

Review flags:
  --pr        PR number (required)
  --repo      owner/name (defaults to repo detected by gh)
  --provider  stub | deepseek | anthropic   (default: stub)
  --config    path to .nitpick.yaml (default: ./.nitpick.yaml)
  --dry-run   print findings to stdout instead of posting

Eval flags:
  --cases     path to cases.jsonl (default: eval/cases/cases.jsonl)
  --provider  stub | deepseek | anthropic   (default: stub)
  --out       path to REPORT.md  (default: eval/REPORT.md)

Environment:
  GITHUB_TOKEN       required (gh CLI uses it)
  DEEPSEEK_API_KEY   required when --provider=deepseek
  ANTHROPIC_API_KEY  required when --provider=anthropic
`)
}
