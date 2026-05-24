// Package config loads .nitpick.yaml.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

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
	return cfg, nil
}

func defaults() Config {
	return Config{
		Provider: "stub",
		Review: ReviewConfig{
			SeverityThreshold: "nit",
		},
	}
}
