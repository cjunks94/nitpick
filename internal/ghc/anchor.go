package ghc

import (
	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/provider"
)

// DropUnanchored splits comments into those GitHub can place on the diff and
// those it cannot. A comment anchors when its file appears in the hunks and
// its line is a new-file line inside one of them (added or context; removed
// lines have no new-file number and we only post on the RIGHT side).
//
// This runs before PostReview on both surfaces because GitHub rejects the
// whole review with 422 if any single comment is off-diff, and the review
// call has already been paid for by then: one hallucinated line number used
// to lose every valid finding in the batch. Line 0 and negatives never
// anchor, so a parser that lets them through is caught here too.
//
// Order is preserved in both slices.
func DropUnanchored(comments []provider.Comment, hunks []diff.Hunk) (kept, dropped []provider.Comment) {
	anchors := make(map[string]map[int]struct{})
	for _, h := range hunks {
		lines := anchors[h.File]
		if lines == nil {
			lines = make(map[int]struct{})
			anchors[h.File] = lines
		}
		for _, l := range h.Lines {
			if l.NewLineNum > 0 {
				lines[l.NewLineNum] = struct{}{}
			}
		}
	}
	for _, c := range comments {
		if _, ok := anchors[c.File][c.Line]; ok {
			kept = append(kept, c)
		} else {
			dropped = append(dropped, c)
		}
	}
	return kept, dropped
}
