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
