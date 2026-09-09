# ADR-005: `gh` CLI for the local path, raw HTTP for the server, one body builder

Date: 2026-09-09

## Status
Accepted

## Context
`nitpick review` runs on a developer's machine or in an Action step, where a user token (or the Action's `GITHUB_TOKEN`) is already available through the `gh` CLI. `nitpick serve` runs as a GitHub App and authenticates with short-lived installation tokens minted from an App JWT. Unifying on one HTTP client would mean re-implementing `gh`'s auth discovery for the local path or forcing the server to shell out inside a container.

## Decision
`internal/ghc` has two transports. `pr.go` and `comments.go` wrap `gh` subprocesses for the CLI; `httpclient.go` calls the REST API directly with installation tokens for the server. Review-body construction is shared through `BuildReviewBody`, and the status comment through `BuildStatusCommentBody`, so the two surfaces post byte-identical content.

## Consequences
- Each surface uses the credential model native to its environment; the container image needs `gh` only for the Action path.
- Body shape cannot drift between surfaces, and is covered by tests once.
- Two code paths exist for fetch-diff and post-review, so a GitHub API change may need two fixes.
- Fork PRs in the Action path get a read-only token from GitHub; that limitation belongs to the transport, not to nitpick.
