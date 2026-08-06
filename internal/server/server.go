package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cjunks94/nitpick/internal/ghapp"
	"github.com/cjunks94/nitpick/internal/provider"
)

// Config is everything the server needs to run. Loaded from env vars in cmd.
type Config struct {
	Port             string // bind port (Railway sets PORT; default 8080)
	AnthropicAPIKey  string // for the LLM provider (provider reads ANTHROPIC_API_KEY directly)
	GitHubAppID      string // App ID from the App settings page
	GitHubPrivateKey []byte // PEM-encoded RSA key from the App settings page
	WebhookSecret    string // shared secret configured on the App webhook
	Model            string // anthropic model id, empty = Haiku default
}

// Run starts the HTTP server, blocks until SIGTERM/SIGINT, then gracefully
// shuts down with a 30s grace window. Per the project CLAUDE.md, Railway
// sends SIGTERM before SIGKILL on redeploy — wiring the signal handler is
// load-bearing or in-flight reviews are lost on every deploy.
func Run(cfg Config) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if cfg.GitHubAppID == "" || len(cfg.GitHubPrivateKey) == 0 || cfg.WebhookSecret == "" {
		return errors.New("missing required config: need GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY, GITHUB_WEBHOOK_SECRET")
	}
	key, err := ghapp.ParsePrivateKeyPEM(cfg.GitHubPrivateKey)
	if err != nil {
		return fmt.Errorf("parse GITHUB_APP_PRIVATE_KEY: %w", err)
	}

	p, err := provider.New("anthropic", cfg.Model)
	if err != nil {
		return fmt.Errorf("init provider: %w", err)
	}

	tokenSource := ghapp.NewInstallationTokenSource(cfg.GitHubAppID, key)
	handler := NewHandler(cfg.WebhookSecret, tokenSource, p, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.Handle("/webhook", handler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nitpick — POST /webhook for GitHub App events; GET /healthz for health", http.StatusNotFound)
	})

	port := cfg.Port
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           withRequestLogging(mux, logger),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("nitpick serve listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
		return err
	}

	// srv.Shutdown only waits for HTTP handlers, and nitpick's handlers return
	// 202 immediately — the actual review runs in a detached goroutine. Before
	// this, Shutdown returned within milliseconds, Run returned, and the
	// process exited with every in-flight review killed mid-LLM-call: tokens
	// billed, nothing posted. Both CLAUDE.md and HANDOFF.md claimed the
	// SIGTERM handler prevented exactly that; it did not until now.
	logger.Info("draining in-flight reviews", "grace_s", int(reviewDrainGrace.Seconds()))
	if handler.Drain(reviewDrainGrace) {
		logger.Info("all in-flight reviews completed")
	} else {
		logger.Warn("drain window expired; remaining reviews cancelled",
			"grace_s", int(reviewDrainGrace.Seconds()))
	}
	logger.Info("shutdown complete")
	return nil
}

const (
	// httpShutdownGrace bounds waiting for open HTTP connections. Handlers
	// return in milliseconds, so this only matters for slow clients.
	httpShutdownGrace = 10 * time.Second

	// reviewDrainGrace bounds waiting for detached review goroutines. Reviews
	// take 5-30s, so this covers the common case with headroom.
	//
	// Railway sends SIGTERM then SIGKILL; the window between them is finite,
	// so this plus httpShutdownGrace should stay under it. If reviews are
	// routinely cut off on redeploy, shorten the provider timeout rather than
	// growing this past the platform's kill delay — a drain longer than the
	// SIGKILL window is a drain that never completes.
	reviewDrainGrace = 45 * time.Second
)

// withRequestLogging logs every request at INFO with method, path, status,
// and duration. Lightweight — skipping the full request-ID-middleware
// pattern (X-Request-ID echo, regex validation) since GitHub already sends
// X-GitHub-Delivery which serves the same correlation purpose; the webhook
// handler attaches that to its own log context.
func withRequestLogging(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		// Don't log /healthz — Railway hits it constantly.
		if r.URL.Path == "/healthz" {
			return
		}
		logger.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
