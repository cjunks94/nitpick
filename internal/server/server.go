package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
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
	// AttachContext turns on whole-file context fetching for reviews
	// (NITPICK_CONTEXT_FILES=1). Off by default: measured 2026-09-09 with
	// the eval attaching exactly what serve sends, Sonnet hit 0 of 18
	// labels in three runs at 2.4x the cost, against 2 / 3 / 2 diff-only.
	// See HANDOFF.md. Opt-in until the prompt's context handling is tuned.
	AttachContext bool
}

// Run starts the HTTP server, blocks until SIGTERM/SIGINT, then gracefully
// shuts down in two steps: httpShutdownGrace (10s) for open HTTP
// connections, then reviewDrainGrace (45s) for detached review goroutines.
// Per the project CLAUDE.md, Railway sends SIGTERM before SIGKILL on
// redeploy — wiring the signal handler is load-bearing or in-flight reviews
// are lost on every deploy, and the platform's draining window must cover
// both steps (60s) or the drain never gets to run.
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
	handler.ProviderForModel = MemoizedProviderFactory("anthropic")
	handler.AttachContext = cfg.AttachContext

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
	srv := newHTTPServer(":"+port, withRequestLogging(mux, logger))
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", srv.Addr, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	return serve(ctx, logger, srv, ln, handler, httpShutdownGrace, reviewDrainGrace)
}

// serve runs srv on ln until ctx is cancelled, then shuts the listener down
// and drains the handler's in-flight reviews. Split from Run so a test can
// drive the shutdown ordering through a context and a loopback listener
// instead of a real signal; Run passes the production grace windows.
func serve(ctx context.Context, logger *slog.Logger, srv *http.Server, ln net.Listener, handler *Handler, shutdownGrace, drainGrace time.Duration) error {
	errCh := make(chan error, 1)
	go func() {
		logger.Info("nitpick serve listening", "addr", ln.Addr().String())
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	// Deliberately not an early return. Shutdown's usual error is
	// DeadlineExceeded, raised when one slow client holds a connection open
	// past httpShutdownGrace — and returning there would skip the drain
	// below, killing every in-flight review. That is the exact failure this
	// whole section exists to prevent, so a stuck HTTP client must not be
	// able to cause it.
	shutdownErr := srv.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		logger.Error("http shutdown", "err", shutdownErr)
	}

	// srv.Shutdown only waits for HTTP handlers, and nitpick's handlers return
	// 202 immediately — the actual review runs in a detached goroutine. Before
	// this, Shutdown returned within milliseconds, Run returned, and the
	// process exited with every in-flight review killed mid-LLM-call: tokens
	// billed, nothing posted. Both CLAUDE.md and HANDOFF.md claimed the
	// SIGTERM handler prevented exactly that; it did not until now.
	logger.Info("draining in-flight reviews", "grace_s", int(drainGrace.Seconds()))
	if handler.Drain(drainGrace) {
		logger.Info("all in-flight reviews completed")
	} else {
		logger.Warn("drain window expired; remaining reviews cancelled",
			"grace_s", int(drainGrace.Seconds()))
	}
	logger.Info("shutdown complete")
	return shutdownErr
}

const (
	// httpShutdownGrace bounds waiting for open HTTP connections. Handlers
	// return in milliseconds, so this only matters for slow clients.
	httpShutdownGrace = 10 * time.Second

	// reviewDrainGrace bounds waiting for detached review goroutines. Reviews
	// take 5-30s, so this covers the common case with headroom.
	//
	// IMPORTANT — this grace is only as real as the platform allows. Railway's
	// default gap between SIGTERM and SIGKILL is ZERO seconds: without
	// configuration the process is killed immediately and no drain of any
	// length can run. Set RAILWAY_DEPLOYMENT_DRAINING_SECONDS (or "Draining
	// Time" in service settings / drainingSeconds in railway.json) to at least
	// httpShutdownGrace + reviewDrainGrace, i.e. 60s, or this code is
	// decoration. See DEPLOY.md.
	//
	// If reviews are still routinely cut off, shorten the provider timeout
	// rather than growing this past the platform's kill delay — a drain longer
	// than the SIGKILL window is a drain that never completes.
	reviewDrainGrace = 45 * time.Second
)

// requestIDRE bounds what an inbound correlation id may look like before it
// is echoed and logged: alphanumerics, dash, underscore, at most 128 bytes.
// Anything else is replaced rather than sanitised, so a hostile header cannot
// put newlines or terminal escapes into the log stream or the response.
var requestIDRE = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

type requestIDKey struct{}

// RequestID returns the correlation id withRequestLogging resolved for this
// request, or "" outside the middleware.
func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey{}).(string)
	return v
}

// rngFailedRequestID is the sentinel used when the random source fails, so
// the request's log lines still share one recognisable id.
const rngFailedRequestID = "rng-failed"

// resolveRequestID picks the id for a request: a valid inbound X-Request-ID
// first (so platform and CDN traces flow end to end), then GitHub's own
// X-GitHub-Delivery, else a fresh random id.
func resolveRequestID(r *http.Request) string {
	for _, name := range []string{"X-Request-ID", "X-GitHub-Delivery"} {
		if v := r.Header.Get(name); requestIDRE.MatchString(v) {
			return v
		}
	}
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return rngFailedRequestID
	}
	return hex.EncodeToString(b[:])
}

// withRequestLogging resolves a request id, echoes it in X-Request-ID, puts
// it on the context for handlers to log with, and logs every request at
// INFO with method, path, status, and duration under that id.
func withRequestLogging(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := resolveRequestID(r)
		w.Header().Set("X-Request-ID", id)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		// Don't log /healthz — Railway hits it constantly.
		if r.URL.Path == "/healthz" {
			return
		}
		logger.Info("http",
			"request_id", id,
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

// Connection timeouts for the single public ingress. GitHub delivers a
// webhook in well under a second and the handler answers 202 in
// milliseconds (the review runs detached), so these are generous for any
// legitimate client while bounding what a slow-trickle attacker can hold
// open. ReadHeaderTimeout alone left the body unbounded in time (GH-2):
// the 5 MiB LimitReader caps size, not duration.
const (
	httpReadHeaderTimeout = 10 * time.Second
	httpReadTimeout       = 30 * time.Second  // headers + body
	httpWriteTimeout      = 30 * time.Second  // from end of request read
	httpIdleTimeout       = 120 * time.Second // keep-alive connections between requests
)

// newHTTPServer builds the listener with every timeout set. Kept separate
// from Run so a test can assert the timeouts without binding a port.
func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
	}
}
