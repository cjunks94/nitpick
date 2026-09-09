package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/cjunks94/nitpick/internal/diff"
)

// hunkWith builds a one-hunk request whose added lines are the given strings,
// numbered from 10 so a finding's line can be checked against its source.
func hunkWith(file string, added ...string) []diff.Hunk {
	h := diff.Hunk{File: file, NewStart: 10, NewLines: len(added)}
	for i, l := range added {
		h.Lines = append(h.Lines, diff.HunkLine{Kind: diff.LineAdded, Content: l, NewLineNum: 10 + i})
	}
	return []diff.Hunk{h}
}

// fakeSecret assembles a value long enough for the secret rule at runtime so
// the fixture never sits in source as a credential-shaped literal.
func fakeSecret() string {
	return strings.Repeat("Qz9", 8) // 24 chars of [A-Za-z0-9]
}

func TestStub_Name(t *testing.T) {
	if got := (Stub{}).Name(); got != "stub" {
		t.Errorf("Name() = %q, want stub", got)
	}
	p, err := New("", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.(Stub); !ok {
		t.Errorf("New(\"\") = %T, want Stub", p)
	}
}

func TestStub_EachRuleFires(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		wantCategory string
		wantSeverity Severity
	}{
		{"TODO marker", "// TODO: handle the error", "todo", SeverityUseful},
		{"FIXME marker", "# FIXME broken on windows", "todo", SeverityUseful},
		{"XXX marker is word-bounded", "x = XXX", "todo", SeverityUseful},
		{"console.log", "console.log(user)", "debug", SeverityNit},
		{"fmt.Println", "fmt.Println(resp)", "debug", SeverityNit},
		{"python print", "print(df.head())", "debug", SeverityNit},
		{"nolint", "var x = f() //nolint:errcheck", "suppression", SeverityUseful},
		{"eslint-disable", "// eslint-disable-next-line no-console", "suppression", SeverityUseful},
		{"noqa", "import os  # noqa", "suppression", SeverityUseful},
		{"api key", `api_key = "` + fakeSecret() + `"`, "secret", SeverityCritical},
		{"bearer token", `token: '` + fakeSecret() + `'`, "secret", SeverityCritical},
		{"hyphenated upper-case key", `API-KEY = "` + fakeSecret() + `"`, "secret", SeverityCritical},
		{"password", `password="` + fakeSecret() + `"`, "secret", SeverityCritical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Stub{}.Review(context.Background(), ReviewRequest{Hunks: hunkWith("src/app.go", tt.line)})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Comments) != 1 {
				t.Fatalf("got %d comments, want 1: %+v", len(res.Comments), res.Comments)
			}
			c := res.Comments[0]
			if c.Category != tt.wantCategory {
				t.Errorf("category = %q, want %q", c.Category, tt.wantCategory)
			}
			if c.Severity != tt.wantSeverity {
				t.Errorf("severity = %q, want %q", c.Severity, tt.wantSeverity)
			}
			if c.File != "src/app.go" || c.Line != 10 {
				t.Errorf("anchor = %s:%d, want src/app.go:10", c.File, c.Line)
			}
			if c.Body == "" {
				t.Error("body is empty")
			}
		})
	}
}

func TestStub_CleanLinesAreSilent(t *testing.T) {
	res, err := Stub{}.Review(context.Background(), ReviewRequest{Hunks: hunkWith("a.go",
		"x := compute()",
		"return x, nil",
		"// todos are tracked in the issue tracker", // "todos" is not the word TODO
		`api_key = os.Getenv("API_KEY")`,            // not a literal credential
		`token = "short"`,                           // under the 16-char floor
	)})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Comments) != 0 {
		t.Errorf("clean diff produced findings: %+v", res.Comments)
	}
	if res.CostUSD != 0 {
		t.Errorf("CostUSD = %v, want 0", res.CostUSD)
	}
}

// Only added lines are reviewed: a TODO in context or on a removed line is
// not this PR's doing.
func TestStub_OnlyAddedLinesAreFlagged(t *testing.T) {
	h := diff.Hunk{File: "a.go", NewStart: 1, NewLines: 3, Lines: []diff.HunkLine{
		{Kind: diff.LineContext, Content: "// TODO old", NewLineNum: 1},
		{Kind: diff.LineRemoved, Content: "console.log(x)"},
		{Kind: diff.LineAdded, Content: "// FIXME new", NewLineNum: 2},
		{Kind: diff.LineContext, Content: "return", NewLineNum: 3},
	}}
	res, err := Stub{}.Review(context.Background(), ReviewRequest{Hunks: []diff.Hunk{h}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Comments) != 1 || res.Comments[0].Line != 2 {
		t.Fatalf("comments = %+v, want only the added FIXME at line 2", res.Comments)
	}
}

// A line matching several rules yields one finding (first rule wins), and
// each matching line yields its own.
func TestStub_OneFindingPerMatchingLine(t *testing.T) {
	res, err := Stub{}.Review(context.Background(), ReviewRequest{Hunks: hunkWith("a.go",
		"// TODO remove console.log(x)", // todo AND debug
		"fine := 1",
		"console.log(y)",
	)})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Comments) != 2 {
		t.Fatalf("got %d comments, want 2 (one per matching line): %+v", len(res.Comments), res.Comments)
	}
	if res.Comments[0].Line != 10 || res.Comments[0].Category != "todo" {
		t.Errorf("first = %+v, want todo at line 10", res.Comments[0])
	}
	if res.Comments[1].Line != 12 || res.Comments[1].Category != "debug" {
		t.Errorf("second = %+v, want debug at line 12", res.Comments[1])
	}
	if res.CostUSD != 0 {
		t.Errorf("CostUSD = %v, want 0", res.CostUSD)
	}
}

// The debug-print rule is the only one that skips test files: a print in a
// test is often the point. The other rules still apply there.
func TestStub_TestFilesSkipDebugRuleOnly(t *testing.T) {
	for _, file := range []string{
		"internal/x/x_test.go",
		"src/test/helpers.js",
		"src/tests/unit/foo.py",
		"app.spec.ts",
		"app.spec.js",
		"app.test.ts",
		"app.test.js",
		"test_models.py",
		"PKG/Foo_TEST.GO", // case-insensitive
	} {
		t.Run(file, func(t *testing.T) {
			if !isTestPath(file) {
				t.Fatalf("isTestPath(%q) = false", file)
			}
			res, err := Stub{}.Review(context.Background(), ReviewRequest{Hunks: hunkWith(file,
				"fmt.Println(got)",
				"// TODO tighten this assertion",
			)})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Comments) != 1 || res.Comments[0].Category != "todo" || res.Comments[0].Line != 11 {
				t.Errorf("comments = %+v, want only the TODO at line 11", res.Comments)
			}
		})
	}
	// Note "tests/unit/foo.py" (repo-root tests dir, no leading slash) is NOT
	// matched today: the check is Contains("/tests/"). Documented, not asserted.
	for _, file := range []string{"src/app.go", "testing/harness.go", "contest.js", "src/latest.ts"} {
		if isTestPath(file) {
			t.Errorf("isTestPath(%q) = true, want false", file)
		}
	}
}
