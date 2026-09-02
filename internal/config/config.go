// Package config loads .nitpick.yaml.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Provider string       `yaml:"provider"`
	Model    string       `yaml:"model"`
	Review   ReviewConfig `yaml:"review"`
}

type ReviewConfig struct {
	SeverityThreshold string   `yaml:"severity_threshold"`
	IgnorePaths       []string `yaml:"ignore_paths"`
	CategoriesEnabled []string `yaml:"categories_enabled"`
	// CodeRabbit configures interop with CodeRabbit, which many repos run
	// alongside nitpick. See CodeRabbitConfig.
	CodeRabbit CodeRabbitConfig `yaml:"coderabbit"`
	// ContextNotes is free-form text injected into the reviewer's system
	// prompt as a cached <repo-notes> block. Put repo-specific things the
	// bot should know: language conventions (e.g. "GDScript class_name
	// declarations are repo-globally resolved — don't flag missing imports
	// for repo-local classes"), framework conventions (e.g. "Test framework
	// is GdUnit4 — use before_test/after_test"), and patterns the team
	// doesn't want flagged. Keep it short — bullets, not essays.
	ContextNotes string `yaml:"context_notes"`
}

// Load reads .nitpick.yaml from disk. A missing file returns the defaults
// paired with fs.ErrNotExist so callers can distinguish "use defaults" from
// "parse error".
func Load(path string) (Config, error) {
	// #nosec G304 -- callers pass the literal repoConfigPath constant
	// (".nitpick.yaml"); never user-controlled input at runtime.
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return defaults(), err
		}
		return Config{}, err
	}
	return Parse(b)
}

// Parse decodes config from raw bytes. Used by the server, which fetches the
// .nitpick.yaml via HTTP and doesn't have a filesystem path.
func Parse(b []byte) (Config, error) {
	cfg := defaults()
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse .nitpick.yaml: %w", err)
	}
	if err := ValidatePatterns(cfg.Review.IgnorePaths); err != nil {
		return Config{}, fmt.Errorf("parse .nitpick.yaml: review.ignore_paths: %w", err)
	}
	return cfg, nil
}

// CodeRabbitConfig controls how nitpick behaves when CodeRabbit reviews the
// same pull requests.
//
// nitpick's prompt has always claimed to complement CodeRabbit rather than
// duplicate it, but that was an abstract instruction — the model was told to
// skip "anything CodeRabbit would also flag" without ever being shown what
// CodeRabbit actually said. This config turns that into real per-PR knowledge.
type CodeRabbitConfig struct {
	// Enabled fetches CodeRabbit's existing comments on the PR and shows them
	// to the reviewer as already-covered ground. Default true; it costs one
	// GitHub call and reliably reduces duplicate findings.
	Enabled *bool `yaml:"enabled"`

	// Bots are the logins treated as CodeRabbit. Configurable because
	// self-hosted and enterprise installs use different account names.
	Bots []string `yaml:"bots"`

	// Wait blocks the review until CodeRabbit has posted on this PR (or
	// WaitTimeout elapses). Off by default: it trades latency for dedup
	// quality, and on a repo where CodeRabbit isn't installed it would add
	// WaitTimeout to every single review before giving up.
	//
	// Turn this on when CodeRabbit is reliably installed and you care more
	// about non-overlapping comments than about nitpick posting first.
	Wait bool `yaml:"wait"`

	// WaitTimeout bounds the wait. On expiry nitpick reviews anyway with
	// whatever CodeRabbit comments exist — never skips the review.
	WaitTimeout Duration `yaml:"wait_timeout"`

	// PollInterval is how often to re-check for CodeRabbit's comments.
	PollInterval Duration `yaml:"poll_interval"`
}

// IsEnabled reports whether comment fetching is on, treating an omitted
// field as true.
func (c CodeRabbitConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// BotLogins returns the configured bot logins, or the default set.
func (c CodeRabbitConfig) BotLogins() []string {
	if len(c.Bots) > 0 {
		return c.Bots
	}
	return []string{"coderabbitai[bot]", "coderabbitai"}
}

// Duration wraps time.Duration with YAML support for Go duration strings
// ("5m", "30s"). gopkg.in/yaml.v3 has no native time.Duration handling; it
// would decode "5m" into an int and fail.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("duration must be a string like \"5m\": %w", err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		// yaml decodes an unquoted 300 into the string "300", which
		// ParseDuration rejects for want of a unit. Read a bare number as
		// seconds rather than failing the whole config load — that is
		// unambiguously what someone writing "wait_timeout: 300" means.
		if n, nerr := strconv.ParseInt(s, 10, 64); nerr == nil {
			if n < 0 {
				return fmt.Errorf("duration %q must not be negative", s)
			}
			// time.Duration is int64 nanoseconds; multiplying an unchecked
			// int64 by time.Second wraps negative past ~292 years and would
			// pass the negative check above only by accident of ordering.
			if n > math.MaxInt64/int64(time.Second) {
				return fmt.Errorf("duration %q is too large", s)
			}
			*d = Duration(time.Duration(n) * time.Second)
			return nil
		}
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	if parsed < 0 {
		return fmt.Errorf("duration %q must not be negative", s)
	}
	*d = Duration(parsed)
	return nil
}

// Or returns the duration, falling back to def when unset (zero).
func (d Duration) Or(def time.Duration) time.Duration {
	if d == 0 {
		return def
	}
	return time.Duration(d)
}

func defaults() Config {
	return Config{
		Provider: "stub",
		Review: ReviewConfig{
			SeverityThreshold: "nit",
		},
	}
}
