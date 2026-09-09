package ghc

import (
	"testing"

	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/provider"
)

func anchorHunks() []diff.Hunk {
	return []diff.Hunk{
		{
			File: "a.go", NewStart: 10,
			Lines: []diff.HunkLine{
				{Kind: diff.LineContext, NewLineNum: 10, OldLineNum: 10},
				{Kind: diff.LineRemoved, NewLineNum: 0, OldLineNum: 11},
				{Kind: diff.LineAdded, NewLineNum: 11, OldLineNum: 0},
				{Kind: diff.LineAdded, NewLineNum: 12, OldLineNum: 0},
				{Kind: diff.LineContext, NewLineNum: 13, OldLineNum: 12},
			},
		},
		{
			File: "b.go", NewStart: 1,
			Lines: []diff.HunkLine{
				{Kind: diff.LineAdded, NewLineNum: 1},
			},
		},
	}
}

func TestDropUnanchored(t *testing.T) {
	tests := []struct {
		name string
		c    provider.Comment
		keep bool
	}{
		{"added line", provider.Comment{File: "a.go", Line: 11}, true},
		{"context line", provider.Comment{File: "a.go", Line: 10}, true},
		{"last context line", provider.Comment{File: "a.go", Line: 13}, true},
		{"other file added line", provider.Comment{File: "b.go", Line: 1}, true},
		{"line just past the hunk", provider.Comment{File: "a.go", Line: 14}, false},
		{"line just before the hunk", provider.Comment{File: "a.go", Line: 9}, false},
		{"file not in diff", provider.Comment{File: "c.go", Line: 1}, false},
		{"right line wrong file", provider.Comment{File: "b.go", Line: 11}, false},
		{"line zero", provider.Comment{File: "a.go", Line: 0}, false},
		{"negative line", provider.Comment{File: "a.go", Line: -3}, false},
		{"empty file", provider.Comment{File: "", Line: 11}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept, dropped := DropUnanchored([]provider.Comment{tt.c}, anchorHunks())
			if tt.keep && (len(kept) != 1 || len(dropped) != 0) {
				t.Errorf("want kept, got kept=%d dropped=%d", len(kept), len(dropped))
			}
			if !tt.keep && (len(kept) != 0 || len(dropped) != 1) {
				t.Errorf("want dropped, got kept=%d dropped=%d", len(kept), len(dropped))
			}
		})
	}
}

// A removed line has no new-file number, and reviews post on the RIGHT side
// only, so a comment "on" the old line number is not placeable.
func TestDropUnanchored_RemovedLineDoesNotAnchor(t *testing.T) {
	hunks := []diff.Hunk{{File: "a.go", Lines: []diff.HunkLine{
		{Kind: diff.LineRemoved, OldLineNum: 5},
	}}}
	kept, dropped := DropUnanchored([]provider.Comment{{File: "a.go", Line: 5}}, hunks)
	if len(kept) != 0 || len(dropped) != 1 {
		t.Errorf("kept=%d dropped=%d, want 0/1", len(kept), len(dropped))
	}
}

func TestDropUnanchored_PreservesOrderAndSplits(t *testing.T) {
	in := []provider.Comment{
		{File: "a.go", Line: 11, Body: "first"},
		{File: "zzz.go", Line: 1, Body: "off-diff"},
		{File: "b.go", Line: 1, Body: "second"},
		{File: "a.go", Line: 99, Body: "off-diff too"},
		{File: "a.go", Line: 10, Body: "third"},
	}
	kept, dropped := DropUnanchored(in, anchorHunks())
	want := []string{"first", "second", "third"}
	if len(kept) != len(want) {
		t.Fatalf("kept %d, want %d", len(kept), len(want))
	}
	for i, w := range want {
		if kept[i].Body != w {
			t.Errorf("kept[%d] = %q, want %q", i, kept[i].Body, w)
		}
	}
	if len(dropped) != 2 || dropped[0].Body != "off-diff" || dropped[1].Body != "off-diff too" {
		t.Errorf("dropped = %+v", dropped)
	}
}

func TestDropUnanchored_NoHunksDropsEverything(t *testing.T) {
	kept, dropped := DropUnanchored([]provider.Comment{{File: "a.go", Line: 1}}, nil)
	if len(kept) != 0 || len(dropped) != 1 {
		t.Errorf("kept=%d dropped=%d, want 0/1", len(kept), len(dropped))
	}
	kept, dropped = DropUnanchored(nil, anchorHunks())
	if len(kept) != 0 || len(dropped) != 0 {
		t.Errorf("nil comments should yield nothing; kept=%d dropped=%d", len(kept), len(dropped))
	}
}
