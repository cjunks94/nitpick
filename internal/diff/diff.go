// Package diff parses unified diffs into hunks. The parser tracks both
// new-file line numbers (used by GitHub's modern review-comment API via the
// `line` parameter with `side=RIGHT`) and per-file diff positions (used by
// the legacy `position` parameter). Most callers should prefer NewLineNum;
// DiffPosition is preserved so the next provider can fall back if needed.
package diff

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Hunk represents a single @@ block of a unified diff.
type Hunk struct {
	File     string
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []HunkLine
}

// HunkLine is one line within a hunk. NewLineNum is 0 for removed lines,
// OldLineNum is 0 for added lines. DiffPosition is per-file 1-indexed,
// counting the line immediately after the first @@ as position 1.
type HunkLine struct {
	Kind         LineKind
	Content      string
	NewLineNum   int
	OldLineNum   int
	DiffPosition int
}

type LineKind int

const (
	LineContext LineKind = iota
	LineAdded
	LineRemoved
)

func (k LineKind) String() string {
	switch k {
	case LineAdded:
		return "added"
	case LineRemoved:
		return "removed"
	default:
		return "context"
	}
}

var hunkHeaderRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// ParseUnifiedDiff parses a unified diff into hunks.
func ParseUnifiedDiff(raw []byte) ([]Hunk, error) {
	var (
		hunks       []Hunk
		current     *Hunk
		currentFile string
		// position is per-file. First @@ in a file has position 0 (not commentable);
		// the line right after is position 1. Subsequent @@ within the same file
		// DO increment per GitHub's documented scheme.
		position int
		newLine  int
		oldLine  int
		seenHunk bool
	)

	flush := func() {
		if current != nil {
			hunks = append(hunks, *current)
			current = nil
		}
	}

	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<20)

	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			currentFile = ""
			position = 0
			seenHunk = false
		case strings.HasPrefix(line, "+++ b/"):
			currentFile = strings.TrimPrefix(line, "+++ b/")
		case strings.HasPrefix(line, "+++ "):
			// /dev/null or other; ignore for file path
		case strings.HasPrefix(line, "--- "):
			// ignore old-file header
		case strings.HasPrefix(line, "@@"):
			flush()
			m := hunkHeaderRE.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("unparseable hunk header: %q", line)
			}
			oldStart, _ := strconv.Atoi(m[1])
			newStart, _ := strconv.Atoi(m[3])
			oldLines := 1
			newLines := 1
			if m[2] != "" {
				oldLines, _ = strconv.Atoi(m[2])
			}
			if m[4] != "" {
				newLines, _ = strconv.Atoi(m[4])
			}
			if seenHunk {
				position++
			}
			seenHunk = true
			newLine = newStart
			oldLine = oldStart
			current = &Hunk{
				File:     currentFile,
				OldStart: oldStart,
				OldLines: oldLines,
				NewStart: newStart,
				NewLines: newLines,
			}
		case current != nil && len(line) > 0:
			position++
			switch line[0] {
			case '+':
				current.Lines = append(current.Lines, HunkLine{
					Kind:         LineAdded,
					Content:      line[1:],
					NewLineNum:   newLine,
					DiffPosition: position,
				})
				newLine++
			case '-':
				current.Lines = append(current.Lines, HunkLine{
					Kind:         LineRemoved,
					Content:      line[1:],
					OldLineNum:   oldLine,
					DiffPosition: position,
				})
				oldLine++
			case ' ':
				current.Lines = append(current.Lines, HunkLine{
					Kind:         LineContext,
					Content:      line[1:],
					NewLineNum:   newLine,
					OldLineNum:   oldLine,
					DiffPosition: position,
				})
				newLine++
				oldLine++
			case '\\':
				// "\ No newline at end of file" — counts toward position but
				// has no content kind.
			default:
				// Anomalous; treat as context to stay permissive.
				current.Lines = append(current.Lines, HunkLine{
					Kind:         LineContext,
					Content:      line,
					NewLineNum:   newLine,
					OldLineNum:   oldLine,
					DiffPosition: position,
				})
				newLine++
				oldLine++
			}
		case current != nil && line == "":
			position++
			current.Lines = append(current.Lines, HunkLine{
				Kind:         LineContext,
				Content:      "",
				NewLineNum:   newLine,
				OldLineNum:   oldLine,
				DiffPosition: position,
			})
			newLine++
			oldLine++
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return hunks, nil
}
