package config

import "testing"

func TestMatchAny(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		patterns []string
		want     bool
	}{
		{"empty patterns never match", "any/path.go", nil, false},
		{"empty patterns slice", "any/path.go", []string{}, false},
		{"exact match", "go.sum", []string{"go.sum"}, true},
		{"single-segment star at root", "main.go", []string{"*.go"}, true},
		{"single-segment star does not span dirs", "internal/main.go", []string{"*.go"}, false},
		{"doublestar spans dirs", "internal/foo/main.go", []string{"**/*.go"}, true},
		{"doublestar matches at root too", "main.go", []string{"**/*.go"}, true},
		{"directory prefix with doublestar", "vendor/x/y.go", []string{"vendor/**"}, true},
		{"directory prefix doublestar also matches dir itself", "vendor", []string{"vendor/**"}, true},
		{"multiple patterns, second matches", "x.uid", []string{"vendor/**", "**/*.uid"}, true},
		{"multiple patterns, none match", "src/main.go", []string{"vendor/**", "**/*.uid"}, false},
		{"godot .uid recursive", "scripts/items/slot.gd.uid", []string{"**/*.uid"}, true},
		{"generated.go pattern", "internal/api/types.generated.go", []string{"**/*.generated.go"}, true},
		{"pb.go at depth", "proto/gen/foo.pb.go", []string{"**/*.pb.go"}, true},
		{"testdata anywhere", "internal/server/testdata/fixture.json", []string{"**/testdata/**"}, true},
		{"bad pattern returns no match, not error", "x.go", []string{"[unclosed"}, false},
		{"bad pattern does not poison good pattern", "x.uid", []string{"[unclosed", "**/*.uid"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchAny(tt.path, tt.patterns)
			if got != tt.want {
				t.Errorf("MatchAny(%q, %v) = %v, want %v", tt.path, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestValidatePatterns(t *testing.T) {
	t.Run("nil is valid", func(t *testing.T) {
		if err := ValidatePatterns(nil); err != nil {
			t.Errorf("nil patterns: unexpected error %v", err)
		}
	})
	t.Run("all valid", func(t *testing.T) {
		err := ValidatePatterns([]string{"vendor/**", "**/*.uid", "*.lock"})
		if err != nil {
			t.Errorf("valid patterns: unexpected error %v", err)
		}
	})
	t.Run("first bad pattern is reported", func(t *testing.T) {
		err := ValidatePatterns([]string{"vendor/**", "[unclosed"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
