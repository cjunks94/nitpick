package config

import "testing"

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
