package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cjunks94/nitpick/internal/provider"
)

func TestMatches(t *testing.T) {
	exp := ExpectedFinding{File: "a.js", Line: 65, Keywords: []string{"hsl"}}
	cases := []struct {
		name string
		com  provider.Comment
		want bool
	}{
		{"same line, keyword present", provider.Comment{File: "a.js", Line: 65, Body: "does not handle hsl() values"}, true},
		{"keyword case-insensitive", provider.Comment{File: "a.js", Line: 65, Body: "HSL is unhandled"}, true},
		{"within tolerance", provider.Comment{File: "a.js", Line: 68, Body: "hsl missing"}, true},
		{"outside tolerance", provider.Comment{File: "a.js", Line: 69, Body: "hsl missing"}, false},
		{"wrong file", provider.Comment{File: "b.js", Line: 65, Body: "hsl missing"}, false},
		// The #87 shape: same line, different complaint. Must not be a hit.
		{"same line, different finding", provider.Comment{File: "a.js", Line: 65, Body: "regex is recompiled on every call"}, false},
	}
	for _, tc := range cases {
		if got := matches(exp, tc.com); got != tc.want {
			t.Errorf("%s: matches = %v, want %v", tc.name, got, tc.want)
		}
	}

	// A label without keywords keeps the file+line rule.
	plain := ExpectedFinding{File: "a.js", Line: 65}
	if !matches(plain, provider.Comment{File: "a.js", Line: 65, Body: "anything at all"}) {
		t.Error("label without keywords should match on file+line alone")
	}
}

func TestScore_KeywordRejectsSameLineCoincidence(t *testing.T) {
	c := Case{PR: 87, Expected: []ExpectedFinding{
		{File: "a.js", Line: 65, Severity: "useful", Keywords: []string{"hsl"}},
	}}
	res := provider.ReviewResult{Comments: []provider.Comment{
		{File: "a.js", Line: 65, Body: "regex is recompiled on every call"},
		{File: "a.js", Line: 66, Body: "hsl()/hsla() are not parsed"},
	}}
	cr := score(c, res)
	if len(cr.Hits) != 1 || cr.Hits[0].Line != 66 {
		t.Fatalf("hits = %+v, want only the hsl finding", cr.Hits)
	}
	if len(cr.Misses) != 0 {
		t.Fatalf("misses = %+v, want none", cr.Misses)
	}
	if len(cr.Extras) != 1 || cr.Extras[0].Line != 65 {
		t.Fatalf("extras = %+v, want the same-line coincidence", cr.Extras)
	}
}

func TestLoadCases_KeywordsParse(t *testing.T) {
	cases, err := loadCases("../../eval/cases/cases.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	labeled, keyworded := 0, 0
	for _, c := range cases {
		for _, e := range c.Expected {
			labeled++
			if len(e.Keywords) > 0 {
				keyworded++
			}
		}
	}
	if labeled == 0 {
		t.Fatal("no labeled findings loaded")
	}
	if keyworded != labeled {
		t.Errorf("%d of %d labels carry keywords; every label should, or the matcher is inconsistent across cases", keyworded, labeled)
	}
}

// errOnSecondCall is a provider that fails exactly one Review call — the
// second — and otherwise returns whatever comments the test wired for the
// file under review. It stands in for the "one bad LLM response" that must
// not tank a paid multi-PR sweep.
type errOnSecondCall struct {
	calls    int
	comments map[string][]provider.Comment // keyed by hunk file
}

func (p *errOnSecondCall) Name() string { return "fake" }

func (p *errOnSecondCall) Review(_ context.Context, req provider.ReviewRequest) (provider.ReviewResult, error) {
	p.calls++
	if p.calls == 2 {
		return provider.ReviewResult{}, errors.New("simulated 529 overloaded")
	}
	var out []provider.Comment
	for _, h := range req.Hunks {
		out = append(out, p.comments[h.File]...)
	}
	return provider.ReviewResult{Comments: out, CostUSD: 0.01}, nil
}

// writeOneHunkDiff writes a minimal unified diff that adds one line to file
// at new-file line 2 and returns its path.
func writeOneHunkDiff(t *testing.T, dir, file string) string {
	t.Helper()
	body := "diff --git a/" + file + " b/" + file + "\n" +
		"--- a/" + file + "\n" +
		"+++ b/" + file + "\n" +
		"@@ -1,2 +1,3 @@\n" +
		" package main\n" +
		"+var x = 1\n" +
		" func main() {}\n"
	p := filepath.Join(dir, file+".diff")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// perCaseRows parses the "## Per-case" table of a report into its rows, each
// row being the trimmed cell values (PR, Repo, Expected, Hits, Misses, Extras, $).
func perCaseRows(t *testing.T, report string) [][]string {
	t.Helper()
	var rows [][]string
	for _, line := range strings.Split(report, "\n") {
		if !strings.HasPrefix(line, "| #") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows = append(rows, cells)
	}
	return rows
}

// metric returns the value cell for a "| <name> | <value> |" row.
func metric(t *testing.T, report, name string) string {
	t.Helper()
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(line, "| "+name+" |") {
			cells := strings.Split(strings.Trim(line, "|"), "|")
			if len(cells) != 2 {
				t.Fatalf("metric row %q has %d cells", line, len(cells))
			}
			return strings.TrimSpace(cells[1])
		}
	}
	t.Fatalf("metric %q not found in report:\n%s", name, report)
	return ""
}

// A provider error on one PR must be recorded as zero findings for that PR
// and must not abort the sweep — the other PRs already cost real money.
func TestRun_IsolatesPerPRErrors(t *testing.T) {
	dir := t.TempDir()
	files := []string{"a.go", "b.go", "c.go"}
	lines := []string{"// header comment must be skipped"}
	for i, f := range files {
		p := writeOneHunkDiff(t, dir, f)
		c := Case{
			PR:       i + 1,
			Repo:     "owner/repo",
			DiffPath: p,
			Expected: []ExpectedFinding{
				{File: f, Line: 2, Severity: "critical", Keywords: []string{"unused"}},
				{File: f, Line: 2, Severity: "useful", Keywords: []string{"naming"}},
			},
		}
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(b))
	}
	casesPath := filepath.Join(dir, "cases.jsonl")
	if err := os.WriteFile(casesPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := &errOnSecondCall{comments: map[string][]provider.Comment{}}
	for _, f := range files {
		p.comments[f] = []provider.Comment{
			{File: f, Line: 2, Severity: provider.SeverityCritical, Body: "x is unused"},
			{File: f, Line: 2, Severity: provider.SeverityUseful, Body: "single-letter naming"},
		}
	}

	outPath := filepath.Join(dir, "REPORT.md")
	if err := Run(context.Background(), casesPath, outPath, p, false); err != nil {
		t.Fatalf("Run returned %v; a single provider error must not abort the sweep", err)
	}
	if p.calls != 3 {
		t.Fatalf("provider called %d times, want 3 (every case must still run)", p.calls)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("REPORT.md not written: %v", err)
	}
	report := string(raw)
	if !strings.Contains(report, "# Eval report — `fake`") {
		t.Errorf("report header missing provider name:\n%s", report)
	}

	rows := perCaseRows(t, report)
	if len(rows) != 3 {
		t.Fatalf("per-case table has %d rows, want 3:\n%s", len(rows), report)
	}
	// Cells: PR, Repo, Expected, Hits, Misses, Extras, $.
	want := [][]string{
		{"#1", "owner/repo", "2", "2", "0", "0", "$0.0100"},
		{"#2", "owner/repo", "2", "0", "2", "0", "$0.0000"},
		{"#3", "owner/repo", "2", "2", "0", "0", "$0.0100"},
	}
	for i, w := range want {
		for j, cell := range w {
			if rows[i][j] != cell {
				t.Errorf("row %d cell %d = %q, want %q (row: %v)", i, j, rows[i][j], cell, rows[i])
			}
		}
	}
	// The errored case shows up as misses in the detail section, not silently.
	if !strings.Contains(report, "### #2 owner/repo") || !strings.Contains(report, "- MISS `b.go:2` [critical/]") {
		t.Errorf("errored case should be recorded as misses in the detail section:\n%s", report)
	}
}

func TestRun_MissingDiffIsAnError(t *testing.T) {
	dir := t.TempDir()
	casesPath := filepath.Join(dir, "cases.jsonl")
	c, err := json.Marshal(Case{PR: 1, Repo: "o/r", DiffPath: filepath.Join(dir, "nope.diff")})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(casesPath, append(c, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	err = Run(context.Background(), casesPath, filepath.Join(dir, "REPORT.md"), &errOnSecondCall{}, false)
	if err == nil || !strings.Contains(err.Error(), "nope.diff") {
		t.Fatalf("Run = %v, want a read error naming the missing diff", err)
	}
}

type recordingProvider struct {
	onReview func(provider.ReviewRequest)
}

func (recordingProvider) Name() string { return "recording" }

func (p recordingProvider) Review(_ context.Context, req provider.ReviewRequest) (provider.ReviewResult, error) {
	p.onReview(req)
	return provider.ReviewResult{}, nil
}

// Guidelines come from repos/<owner>__<repo>.md next to the cases file; a
// missing file is not an error, and the flag can switch them off entirely.
func TestRun_LoadsRepoGuidelinesWhenAsked(t *testing.T) {
	dir := t.TempDir()
	diffPath := writeOneHunkDiff(t, dir, "a.go")
	if err := os.MkdirAll(filepath.Join(dir, "repos"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "repos", "owner__repo.md"), []byte("# rules\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var content []byte
	for _, c := range []Case{
		{PR: 1, Repo: "owner/repo", DiffPath: diffPath},
		{PR: 2, Repo: "other/repo", DiffPath: diffPath},
	} {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		content = append(content, append(b, '\n')...)
	}
	casesPath := filepath.Join(dir, "cases.jsonl")
	if err := os.WriteFile(casesPath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, load := range []bool{true, false} {
		var seen [][]byte
		p := recordingProvider{onReview: func(req provider.ReviewRequest) {
			seen = append(seen, req.RepoGuidelines)
		}}
		if err := Run(context.Background(), casesPath, filepath.Join(dir, "REPORT.md"), p, load); err != nil {
			t.Fatal(err)
		}
		if len(seen) != 2 {
			t.Fatalf("load=%v: provider saw %d requests, want 2", load, len(seen))
		}
		if load {
			if string(seen[0]) != "# rules\n" {
				t.Errorf("guidelines for owner/repo = %q, want file contents", seen[0])
			}
			if seen[1] != nil {
				t.Errorf("guidelines for other/repo = %q, want nil (no file)", seen[1])
			}
		} else if seen[0] != nil || seen[1] != nil {
			t.Errorf("load=false: guidelines should be nil, got %q / %q", seen[0], seen[1])
		}
	}
}

// Metrics are checked against hand-computed values so a change to the
// formulas (or to which hits count toward which recall) fails loudly.
func TestWriteReport_MetricsMatchHandComputedValues(t *testing.T) {
	results := []CaseResult{
		{
			Case: Case{PR: 1, Repo: "o/r", Expected: []ExpectedFinding{
				{File: "a.go", Line: 1, Severity: "critical", Keywords: []string{"nil"}},
				{File: "a.go", Line: 9, Severity: "useful", Keywords: []string{"loop"}},
			}},
			Hits:    []provider.Comment{{File: "a.go", Line: 1, Severity: provider.SeverityCritical, Body: "nil deref"}},
			Misses:  []ExpectedFinding{{File: "a.go", Line: 9, Severity: "useful"}},
			Extras:  []provider.Comment{{File: "a.go", Line: 20, Severity: provider.SeverityNit, Body: "extra"}},
			CostUSD: 0.01,
		},
		{
			Case: Case{PR: 2, Repo: "o/r", Expected: []ExpectedFinding{
				{File: "b.go", Line: 4, Severity: "useful"},
			}},
			Hits: []provider.Comment{{File: "b.go", Line: 4, Severity: provider.SeverityUseful, Body: "hit"}},
			Extras: []provider.Comment{
				{File: "b.go", Line: 30, Severity: provider.SeverityUseful, Body: "extra 1"},
				{File: "b.go", Line: 31, Severity: provider.SeverityUseful, Body: "extra 2"},
			},
			CostUSD: 0.03,
		},
	}
	// expected=3 (1 critical, 2 useful), hits=2 (1 critical, 1 useful),
	// extras=3, produced=5, cost=0.04 over 2 cases.
	var buf bytes.Buffer
	if err := writeReport(&buf, "fake", results); err != nil {
		t.Fatal(err)
	}
	report := buf.String()

	want := map[string]string{
		"Precision":         "0.400", // 2/5
		"Recall (all)":      "0.667", // 2/3
		"Recall (critical)": "1.000", // 1/1
		"Recall (useful)":   "0.500", // 1/2
		"Noise rate":        "0.600", // 3/5
		"Avg $/PR":          "$0.0200",
	}
	for name, w := range want {
		if got := metric(t, report, name); got != w {
			t.Errorf("%s = %q, want %q", name, got, w)
		}
	}
	if !strings.Contains(report, "Cases: 2  ·  Expected findings: 3  ·  Produced: 5") {
		t.Errorf("summary line wrong:\n%s", report)
	}
	if !strings.Contains(report, "(2 of 3 labels carry keywords)") {
		t.Errorf("keyword count wrong:\n%s", report)
	}
	for _, line := range []string{
		"- HIT `a.go:1` [critical/] nil deref",
		"- MISS `a.go:9` [useful/]",
		"- EXTRA `b.go:31` [useful/] extra 2",
	} {
		if !strings.Contains(report, line) {
			t.Errorf("detail line %q missing:\n%s", line, report)
		}
	}
}

// Division by zero must render as 0.000, never NaN — a sweep of pure silence
// cases has no expected findings and no produced comments.
func TestWriteReport_ZeroExpectedIsNotNaN(t *testing.T) {
	tests := []struct {
		name    string
		results []CaseResult
	}{
		{"no cases at all", nil},
		{"silence case, silent provider", []CaseResult{{Case: Case{PR: 1, Repo: "o/r"}}}},
		{"silence case, noisy provider", []CaseResult{{
			Case:   Case{PR: 1, Repo: "o/r"},
			Extras: []provider.Comment{{File: "a.go", Line: 1, Body: "noise"}},
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeReport(&buf, "stub", tt.results); err != nil {
				t.Fatal(err)
			}
			report := buf.String()
			if strings.Contains(report, "NaN") || strings.Contains(report, "Inf") {
				t.Fatalf("report contains NaN/Inf:\n%s", report)
			}
			for _, name := range []string{"Recall (all)", "Recall (critical)", "Recall (useful)"} {
				if got := metric(t, report, name); got != "0.000" {
					t.Errorf("%s = %q, want 0.000", name, got)
				}
			}
			if got := metric(t, report, "Avg $/PR"); got != "$0.0000" {
				t.Errorf("Avg $/PR = %q, want $0.0000", got)
			}
		})
	}
}

func TestLoadCases_Parser(t *testing.T) {
	valid := `{"pr":7,"repo":"o/r","diff_path":"x.diff","expected":[{"file":"a.go","line":3,"severity":"useful","keywords":["hsl","hsla"]}]}`
	tests := []struct {
		name      string
		content   string
		wantCases int
		wantErr   string
	}{
		{
			name:      "comment lines and blank lines are skipped",
			content:   "// preamble\n\n   \n" + valid + "\n// trailing\n",
			wantCases: 1,
		},
		{
			name:      "leading whitespace before a case is tolerated",
			content:   "   " + valid + "\n",
			wantCases: 1,
		},
		{
			name:      "two cases",
			content:   valid + "\n" + valid + "\n",
			wantCases: 2,
		},
		{
			name:    "malformed line errors",
			content: valid + "\n{\"pr\": not-json}\n",
			wantErr: "parse case",
		},
		{
			name:    "hash is not a comment marker",
			content: "# not a comment\n" + valid + "\n",
			wantErr: "parse case",
		},
		{
			name:      "empty file",
			content:   "",
			wantCases: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "cases.jsonl")
			if err := os.WriteFile(p, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadCases(p)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.wantCases {
				t.Fatalf("got %d cases, want %d", len(got), tt.wantCases)
			}
			if tt.wantCases == 0 {
				return
			}
			c := got[0]
			if c.PR != 7 || c.Repo != "o/r" || c.DiffPath != "x.diff" {
				t.Errorf("case fields = %+v", c)
			}
			if len(c.Expected) != 1 {
				t.Fatalf("expected = %+v", c.Expected)
			}
			e := c.Expected[0]
			if e.File != "a.go" || e.Line != 3 || e.Severity != "useful" {
				t.Errorf("expected finding = %+v", e)
			}
			if len(e.Keywords) != 2 || e.Keywords[0] != "hsl" || e.Keywords[1] != "hsla" {
				t.Errorf("keywords = %v, want [hsl hsla]", e.Keywords)
			}
		})
	}
}

func TestLoadCases_MissingFile(t *testing.T) {
	if _, err := loadCases(filepath.Join(t.TempDir(), "absent.jsonl")); err == nil {
		t.Fatal("expected an error for a missing cases file")
	}
}
