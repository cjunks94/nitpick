package eval

import (
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
