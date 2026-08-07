package config

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(t *testing.T, c Config)
	}{
		{
			name: "full config",
			yaml: `
provider: anthropic
model: claude-sonnet-4-6
review:
  severity_threshold: critical
  context_notes: |
    GDScript: class_name is repo-globally resolved.
`,
			check: func(t *testing.T, c Config) {
				if c.Provider != "anthropic" {
					t.Errorf("Provider = %q", c.Provider)
				}
				if c.Model != "claude-sonnet-4-6" {
					t.Errorf("Model = %q", c.Model)
				}
				if c.Review.SeverityThreshold != "critical" {
					t.Errorf("SeverityThreshold = %q", c.Review.SeverityThreshold)
				}
				if c.Review.ContextNotes == "" {
					t.Error("ContextNotes should be populated")
				}
			},
		},
		{
			name: "empty yaml uses defaults",
			yaml: ``,
			check: func(t *testing.T, c Config) {
				if c.Provider != "stub" {
					t.Errorf("default Provider = %q, want stub", c.Provider)
				}
				if c.Review.SeverityThreshold != "nit" {
					t.Errorf("default SeverityThreshold = %q, want nit", c.Review.SeverityThreshold)
				}
				if c.Review.ContextNotes != "" {
					t.Errorf("default ContextNotes should be empty, got %q", c.Review.ContextNotes)
				}
			},
		},
		{
			name:    "malformed yaml returns error",
			yaml:    "provider: anthropic\nreview: [this is not a map",
			wantErr: true,
		},
		{
			name: "ignore_paths populates and parses globs",
			yaml: `
review:
  ignore_paths:
    - "vendor/**"
    - "**/*.uid"
    - "**/*.generated.go"
`,
			check: func(t *testing.T, c Config) {
				want := []string{"vendor/**", "**/*.uid", "**/*.generated.go"}
				if len(c.Review.IgnorePaths) != len(want) {
					t.Fatalf("IgnorePaths len = %d, want %d", len(c.Review.IgnorePaths), len(want))
				}
				for i, p := range want {
					if c.Review.IgnorePaths[i] != p {
						t.Errorf("IgnorePaths[%d] = %q, want %q", i, c.Review.IgnorePaths[i], p)
					}
				}
			},
		},
		{
			name: "malformed ignore_paths glob fails Parse",
			yaml: `
review:
  ignore_paths:
    - "[unclosed"
`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Parse([]byte(tt.yaml))
			if tt.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if tt.check != nil {
				tt.check(t, c)
			}
		})
	}
}

func TestParse_CodeRabbitDefaults(t *testing.T) {
	// An empty config, or one that omits the coderabbit block entirely, must
	// leave dedup ON — it costs one GitHub call and is the whole point of the
	// interop. Only `wait` is opt-in.
	for _, in := range []string{``, "review:\n  severity_threshold: useful\n"} {
		cfg, err := Parse([]byte(in))
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", in, err)
		}
		cr := cfg.Review.CodeRabbit
		if !cr.IsEnabled() {
			t.Errorf("Parse(%q): dedup should default to enabled", in)
		}
		if cr.Wait {
			t.Errorf("Parse(%q): wait should default to off", in)
		}
		if got := cr.BotLogins(); len(got) == 0 {
			t.Errorf("Parse(%q): BotLogins should fall back to a default set", in)
		}
	}
}

func TestParse_CodeRabbitExplicit(t *testing.T) {
	cfg, err := Parse([]byte(`
review:
  coderabbit:
    enabled: false
    bots: ["coderabbit-enterprise"]
    wait: true
    wait_timeout: 3m
    poll_interval: 20s
`))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cr := cfg.Review.CodeRabbit
	if cr.IsEnabled() {
		t.Error("enabled: false should disable dedup")
	}
	if !cr.Wait {
		t.Error("wait: true was not read")
	}
	if got := cr.WaitTimeout.Or(time.Minute); got != 3*time.Minute {
		t.Errorf("WaitTimeout = %v, want 3m", got)
	}
	if got := cr.PollInterval.Or(time.Minute); got != 20*time.Second {
		t.Errorf("PollInterval = %v, want 20s", got)
	}
	if got := cr.BotLogins(); len(got) != 1 || got[0] != "coderabbit-enterprise" {
		t.Errorf("BotLogins = %v, want [coderabbit-enterprise]", got)
	}
}

// Setting only one field must not wipe the defaults for the others — yaml.v3
// decodes into the pre-populated struct, and this pins that behaviour.
func TestParse_CodeRabbitPartialKeepsDefaults(t *testing.T) {
	cfg, err := Parse([]byte("review:\n  coderabbit:\n    wait: true\n"))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cr := cfg.Review.CodeRabbit
	if !cr.Wait {
		t.Error("wait: true was not read")
	}
	if !cr.IsEnabled() {
		t.Error("omitting `enabled` should leave dedup on")
	}
	if got := cr.WaitTimeout.Or(defaultTestTimeout); got != defaultTestTimeout {
		t.Errorf("omitted WaitTimeout should fall through to the caller's default, got %v", got)
	}
}

const defaultTestTimeout = 5 * time.Minute

func TestDuration_Unmarshal(t *testing.T) {
	tests := []struct {
		yaml    string
		want    time.Duration
		wantErr bool
	}{
		{"review:\n  coderabbit:\n    wait_timeout: 90s\n", 90 * time.Second, false},
		{"review:\n  coderabbit:\n    wait_timeout: 2m30s\n", 150 * time.Second, false},
		{"review:\n  coderabbit:\n    wait_timeout: 1h\n", time.Hour, false},
		// A bare number is read as seconds rather than rejected — that's what
		// someone writing "wait_timeout: 300" means.
		{"review:\n  coderabbit:\n    wait_timeout: 300\n", 300 * time.Second, false},
		{"review:\n  coderabbit:\n    wait_timeout: soon\n", 0, true},
		{"review:\n  coderabbit:\n    wait_timeout: -5m\n", 0, true},
	}
	for _, tt := range tests {
		cfg, err := Parse([]byte(tt.yaml))
		if tt.wantErr {
			if err == nil {
				t.Errorf("Parse(%q) should have failed", tt.yaml)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.yaml, err)
			continue
		}
		if got := cfg.Review.CodeRabbit.WaitTimeout.Or(0); got != tt.want {
			t.Errorf("Parse(%q) WaitTimeout = %v, want %v", tt.yaml, got, tt.want)
		}
	}
}
