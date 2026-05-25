package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/cjunks94/nitpick/internal/config"
	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/ghc"
	"github.com/cjunks94/nitpick/internal/provider"
)

// Review runs the nitpick review subcommand against a single PR.
func Review(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("review", flag.ContinueOnError)
	pr := flags.Int("pr", 0, "PR number (required)")
	repo := flags.String("repo", "", "owner/name (defaults to gh-detected)")
	providerName := flags.String("provider", "stub", "stub | deepseek | anthropic")
	configPath := flags.String("config", ".nitpick.yaml", "config path")
	dryRun := flags.Bool("dry-run", false, "print findings to stdout instead of posting")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *pr == 0 {
		return fmt.Errorf("--pr is required")
	}

	cfg, err := config.Load(*configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load config: %w", err)
	}

	if *repo == "" {
		detected, derr := ghc.DetectRepo(ctx)
		if derr != nil {
			return fmt.Errorf("detect repo (pass --repo to override): %w", derr)
		}
		*repo = detected
	} else if _, _, perr := ghc.ParseRepoArg(*repo); perr != nil {
		return perr
	}

	rawDiff, err := ghc.FetchDiff(ctx, *repo, *pr)
	if err != nil {
		return fmt.Errorf("fetch diff: %w", err)
	}

	hunks, err := diff.ParseUnifiedDiff(rawDiff)
	if err != nil {
		return fmt.Errorf("parse diff: %w", err)
	}

	p, err := provider.New(*providerName, cfg.Model)
	if err != nil {
		return err
	}

	start := time.Now()
	result, err := p.Review(ctx, provider.ReviewRequest{
		Hunks:  hunks,
		Config: cfg.Review,
	})
	if err != nil {
		return fmt.Errorf("review: %w", err)
	}
	duration := time.Since(start)

	if *dryRun {
		return ghc.PrintComments(os.Stdout, result.Comments, result.CostUSD)
	}
	if err := ghc.PostReview(ctx, *repo, *pr, result.Comments); err != nil {
		return err
	}
	statusBody := ghc.BuildStatusCommentBody(p.Name(), result.Comments, result.CostUSD, duration)
	if err := ghc.PostIssueComment(ctx, *repo, *pr, statusBody); err != nil {
		fmt.Fprintf(os.Stderr, "warning: post status comment: %v\n", err)
	}
	return nil
}
