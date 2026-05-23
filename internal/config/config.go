// Package config loads .nitpick.yaml.
package config

import (
	"errors"
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
}

// Load reads .nitpick.yaml. A missing file returns the defaults paired with
// fs.ErrNotExist so callers can distinguish "use defaults" from "parse error".
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return defaults(), err
		}
		return Config{}, err
	}
	cfg := defaults()
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
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
