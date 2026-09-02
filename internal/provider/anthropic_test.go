package provider

import "testing"

// Real-world model outputs we lost eval runs to in early Sonnet sweeps.
// Each entry is a transcript-derived response; the parser must handle them
// without erroring (silence-on-prose, range-on-multiline) since one bad parse
// historically tanked a whole 20-PR run.
func TestParseFindings(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLen   int
		wantFirst Comment // sparse — only fields we care about
	}{
		{
			name:    "well-formed empty",
			input:   `{"findings":[]}`,
			wantLen: 0,
		},
		{
			name: "well-formed single finding",
			input: `{"findings":[{"file":"a.go","line":42,"severity":"useful",` +
				`"category":"perf","body":"unbounded loop"}]}`,
			wantLen:   1,
			wantFirst: Comment{File: "a.go", Line: 42, Severity: SeverityUseful},
		},
		{
			name: "fenced JSON",
			input: "```json\n{\"findings\":[{\"file\":\"x.py\",\"line\":1," +
				"\"severity\":\"critical\",\"category\":\"sec\",\"body\":\"\"}]}\n```",
			wantLen:   1,
			wantFirst: Comment{File: "x.py", Line: 1, Severity: SeverityCritical},
		},
		{
			name: "line as string (Sonnet quirk)",
			input: `{"findings":[{"file":"a.go","line":"80","severity":"useful",` +
				`"category":"x","body":"y"}]}`,
			wantLen:   1,
			wantFirst: Comment{File: "a.go", Line: 80},
		},
		{
			name: "line as range (Sonnet multi-line quirk)",
			input: `{"findings":[{"file":"a.go","line":"541-543","severity":"useful",` +
				`"category":"x","body":"y"}]}`,
			wantLen:   1,
			wantFirst: Comment{File: "a.go", Line: 541},
		},
		{
			name:    "prose-only response → silent review",
			input:   "Looking at this diff, the key change is moving the aircraft block. Nothing to flag.",
			wantLen: 0,
		},
		{
			name: "prose with code reference containing braces, no findings JSON",
			input: "Looking at this diff: the {beforeId: 'aircraft-markers'} prop is fine. " +
				"I don't see issues worth flagging.",
			wantLen: 0,
		},
		{
			name: "prose before JSON",
			input: `Here is my review: {"findings":[{"file":"a.go","line":1,` +
				`"severity":"useful","category":"x","body":"y"}]}`,
			wantLen:   1,
			wantFirst: Comment{File: "a.go", Line: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFindings(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("len=%d want %d, got=%+v", len(got), tt.wantLen, got)
			}
			if tt.wantLen > 0 {
				if got[0].File != tt.wantFirst.File {
					t.Errorf("File=%q want %q", got[0].File, tt.wantFirst.File)
				}
				if got[0].Line != tt.wantFirst.Line {
					t.Errorf("Line=%d want %d", got[0].Line, tt.wantFirst.Line)
				}
				if tt.wantFirst.Severity != "" && got[0].Severity != tt.wantFirst.Severity {
					t.Errorf("Severity=%q want %q", got[0].Severity, tt.wantFirst.Severity)
				}
			}
		})
	}
}

// The response object used to be delimited by strings.LastIndex(text, "}"),
// which grabbed the last brace anywhere in the reply. Trailing prose is common
// enough that this discarded whole reviews the operator had already paid for.
func TestParseFindings_TrailingProseWithBraces(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantLen  int
		wantFile string
	}{
		{
			name: "trailing prose containing braces",
			text: `{"findings":[{"file":"a.go","line":10,"severity":"useful",` +
				`"category":"perf","body":"N+1 query"}]}` +
				"\n\nLet me know if you'd like {more detail} on any of these.",
			wantLen:  1,
			wantFile: "a.go",
		},
		{
			name: "preamble prose then findings",
			text: "I reviewed the diff. Here are my findings:\n\n" +
				`{"findings":[{"file":"b.go","line":3,"severity":"critical",` +
				`"category":"bug","body":"nil deref"}]}`,
			wantLen:  1,
			wantFile: "b.go",
		},
		{
			name: "brace inside a finding body does not close early",
			text: `{"findings":[{"file":"c.go","line":7,"severity":"useful",` +
				`"category":"style","body":"prefer map[string]any{} over the literal"}]}`,
			wantLen:  1,
			wantFile: "c.go",
		},
		{
			name: "escaped quote inside body",
			text: `{"findings":[{"file":"d.go","line":1,"severity":"useful",` +
				`"category":"bug","body":"the \"key\" is unchecked}"}]}`,
			wantLen:  1,
			wantFile: "d.go",
		},
		{
			name:    "empty findings stays empty",
			text:    `{"findings":[]}`,
			wantLen: 0,
		},
		{
			name:    "prose-only reply is a silent review, not an error",
			text:    "Nothing here worth flagging.",
			wantLen: 0,
		},
		{
			name:    "unterminated object degrades to silence",
			text:    `{"findings":[{"file":"e.go",`,
			wantLen: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFindings(tt.text)
			if err != nil {
				t.Fatalf("parseFindings returned error (a paid review would be discarded): %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("got %d findings, want %d: %+v", len(got), tt.wantLen, got)
			}
			if tt.wantLen > 0 && got[0].File != tt.wantFile {
				t.Errorf("file = %q, want %q", got[0].File, tt.wantFile)
			}
		})
	}
}

func TestMatchingBrace(t *testing.T) {
	tests := []struct {
		in    string
		start int
		want  int
	}{
		{`{}`, 0, 1},
		{`{"a":{"b":1}}`, 0, 12},
		{`{"a":"}"}`, 0, 8},          // brace inside a string literal
		{`{"a":"\""}`, 0, 9},         // escaped quote does not end the string
		{`{"a":1} trailing }`, 0, 6}, // stops at the real close, not the last brace
		{`{"a":1`, 0, -1},            // unterminated
	}
	for _, tt := range tests {
		if got := matchingBrace(tt.in, tt.start); got != tt.want {
			t.Errorf("matchingBrace(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
