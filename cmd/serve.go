package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/cjunks94/nitpick/internal/server"
)

// Serve runs the nitpick HTTP server — receives GitHub App webhooks and
// reviews PRs out of band. Designed for Railway / Fly / any container host.
// Reads config from env vars (Railway convention); flag is for the port
// override during local testing.
func Serve(_ context.Context, args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := flags.String("port", "", "bind port (defaults to $PORT, then 8080)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg := server.Config{
		Port:             firstNonEmpty(*port, os.Getenv("PORT")),
		AnthropicAPIKey:  os.Getenv("ANTHROPIC_API_KEY"),
		GitHubAppID:      os.Getenv("GITHUB_APP_ID"),
		GitHubPrivateKey: []byte(os.Getenv("GITHUB_APP_PRIVATE_KEY")),
		WebhookSecret:    os.Getenv("GITHUB_WEBHOOK_SECRET"),
		Model:            os.Getenv("NITPICK_MODEL"),
		AttachContext:    envTrue("NITPICK_CONTEXT_FILES"),
	}
	if cfg.AnthropicAPIKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY is required")
	}
	return server.Run(cfg)
}

// envTrue reads a boolean env var: 1, true, yes (any case) are on; unset or
// anything else is off, so the default is the cheap, measured behaviour.
func envTrue(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
