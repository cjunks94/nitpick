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
		hunks   []Hunk
		current *Hunk
		// currentFile is the path findings anchor on: the new-side path, or
		// the old-side path when the new side is /dev/null (a deletion).
		// Both headers are read because every path-keyed guard downstream
		// (secret redaction, ignore_paths, escalate.paths) keys on this,
		// and a deleted .env used to parse with File == "" and slip past
		// all of them.
		currentFile string
		oldFile     string
		// position is per-file. First @@ in a file has position 0 (not commentable);
		// the line right after is position 1. Subsequent @@ within the same file
		// DO increment per GitHub's documented scheme.
		position int
		newLine  int
		oldLine  int
		seenHunk bool
		// oldLeft/newLeft count the lines the current hunk header promised.
		// When both reach zero the hunk is complete and current is cleared,
		// so a following "--- " or "+++ " is unambiguously a file header.
		// Without this, a removed line whose content starts with "-- " (a
		// deleted SQL/Lua/Haskell comment renders as "--- comment") was
		// swallowed as the old-file header and OldLineNum/DiffPosition
		// desynced for the rest of the hunk.
		oldLeft int
		newLeft int
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
			oldFile = ""
			position = 0
			seenHunk = false
		case seenHunk && strings.HasPrefix(line, `\`):
			// "\ No newline at end of file" — counts toward position but has
			// no content kind. Handled here rather than in the content switch
			// because it can follow a hunk that was already flushed on
			// exhaustion, and position must still advance for the next hunk
			// in the same file.
			position++
		case current == nil && strings.HasPrefix(line, "+++ "):
			if p := headerPath(line[4:], "b/"); p != "" {
				currentFile = p
			} else if oldFile != "" {
				currentFile = oldFile // deletion: anchor on the old path
			}
		case current == nil && strings.HasPrefix(line, "--- "):
			oldFile = headerPath(line[4:], "a/")
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
			oldLeft = oldLines
			newLeft = newLines
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
				newLeft--
			case '-':
				current.Lines = append(current.Lines, HunkLine{
					Kind:         LineRemoved,
					Content:      line[1:],
					OldLineNum:   oldLine,
					DiffPosition: position,
				})
				oldLine++
				oldLeft--
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
				newLeft--
				oldLeft--
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
				newLeft--
				oldLeft--
			}
			if oldLeft <= 0 && newLeft <= 0 {
				flush()
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
			newLeft--
			oldLeft--
			if oldLeft <= 0 && newLeft <= 0 {
				flush()
			}
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return hunks, nil
}

// headerPath extracts the repository path from the payload of a "--- " or
// "+++ " header line. It returns "" for /dev/null. Git C-quotes a path that
// contains non-ASCII bytes, quotes, backslashes, or control characters
// (`+++ "b/docs/r\303\251sum\303\251.md"`), and appends a tab after an
// unquoted path that contains spaces; both forms are normalised here. A
// path that does not carry the expected a/ or b/ prefix (--no-prefix diffs)
// is returned as-is.
func headerPath(payload, prefix string) string {
	payload = strings.TrimRight(payload, "\t")
	if strings.HasPrefix(payload, `"`) {
		payload = unquoteGitPath(payload)
	}
	if payload == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(payload, prefix)
}

// unquoteGitPath decodes git's C-style path quoting: a leading and trailing
// double quote, with \\ \" \t \n \r \a \b \f \v and three-digit octal
// byte escapes inside. Input that is not well-formed is returned unchanged
// rather than guessed at.
func unquoteGitPath(q string) string {
	if len(q) < 2 || q[0] != '"' || q[len(q)-1] != '"' {
		return q
	}
	in := q[1 : len(q)-1]
	out := make([]byte, 0, len(in))
	for i := 0; i < len(in); i++ {
		c := in[i]
		if c != '\\' {
			out = append(out, c)
			continue
		}
		i++
		if i >= len(in) {
			return q
		}
		switch e := in[i]; e {
		case '\\', '"':
			out = append(out, e)
		case 't':
			out = append(out, '\t')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 'a':
			out = append(out, '\a')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'v':
			out = append(out, '\v')
		case '0', '1', '2', '3':
			if i+2 >= len(in) {
				return q
			}
			var b byte
			for _, d := range in[i : i+3] {
				if d < '0' || d > '7' {
					return q
				}
				b = b*8 + byte(d-'0')
			}
			out = append(out, b)
			i += 2
		default:
			return q
		}
	}
	return string(out)
}
