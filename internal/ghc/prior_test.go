package ghc

import "testing"

func TestToPriorFindings(t *testing.T) {
	hits := []ExistingComment{
		{Author: "coderabbitai[bot]", Path: "a.go", Line: 3, Body: "inline one"},
		{Author: "coderabbitai[bot]", Path: "b.go", Line: 9, Body: "inline two"},
		{Author: "coderabbitai[bot]", Body: "walkthrough"},
	}

	out, dropped := ToPriorFindings(hits, 25)
	if len(out) != 3 || dropped != 0 {
		t.Fatalf("len=%d dropped=%d, want 3/0", len(out), dropped)
	}
	if out[0].Path != "a.go" || out[0].Line != 3 || out[1].Body != "inline two" {
		t.Errorf("order or fields changed: %+v", out)
	}
	if out[2].Path != "" || out[2].Line != 0 || out[2].Body != "walkthrough" {
		t.Errorf("top-level comment must carry no anchor: %+v", out[2])
	}

	out, dropped = ToPriorFindings(hits, 2)
	if len(out) != 2 || dropped != 1 || out[1].Body != "inline two" {
		t.Errorf("cap: len=%d dropped=%d last=%q", len(out), dropped, out[len(out)-1].Body)
	}

	out, dropped = ToPriorFindings(nil, 25)
	if len(out) != 0 || dropped != 0 {
		t.Errorf("nil hits: len=%d dropped=%d", len(out), dropped)
	}
	out, dropped = ToPriorFindings(hits, 0)
	if len(out) != 0 || dropped != 3 {
		t.Errorf("zero cap: len=%d dropped=%d", len(out), dropped)
	}
}
