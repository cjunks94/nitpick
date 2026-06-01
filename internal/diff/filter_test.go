package diff

import "testing"

func TestFilterByPath(t *testing.T) {
	hunks := []Hunk{
		{File: "main.go"},
		{File: "vendor/lib.go"},
		{File: "scripts/items/slot.gd.uid"},
		{File: "go.sum"},
		{File: "internal/server/handler.go"},
	}

	t.Run("nil predicate is a passthrough", func(t *testing.T) {
		out := FilterByPath(hunks, nil)
		if len(out) != len(hunks) {
			t.Fatalf("nil predicate dropped hunks: got %d, want %d", len(out), len(hunks))
		}
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		out := FilterByPath(nil, func(string) bool { return true })
		if len(out) != 0 {
			t.Fatalf("got %d hunks, want 0", len(out))
		}
	})

	t.Run("drops matching files preserving order", func(t *testing.T) {
		drop := func(path string) bool {
			return path == "vendor/lib.go" || path == "scripts/items/slot.gd.uid" || path == "go.sum"
		}
		out := FilterByPath(hunks, drop)
		want := []string{"main.go", "internal/server/handler.go"}
		if len(out) != len(want) {
			t.Fatalf("got %d hunks, want %d", len(out), len(want))
		}
		for i, h := range out {
			if h.File != want[i] {
				t.Errorf("[%d] got %q, want %q", i, h.File, want[i])
			}
		}
	})

	t.Run("does not mutate input slice", func(t *testing.T) {
		input := []Hunk{{File: "a"}, {File: "b"}}
		_ = FilterByPath(input, func(p string) bool { return p == "a" })
		if input[0].File != "a" || input[1].File != "b" {
			t.Errorf("input mutated: %+v", input)
		}
	})
}
