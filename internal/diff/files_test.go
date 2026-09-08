package diff

import "testing"

func TestFiles_DistinctInOrder(t *testing.T) {
	hunks := []Hunk{{File: "b.go"}, {File: "a.go"}, {File: "b.go"}, {File: "c.go"}, {File: "a.go"}}
	got := Files(hunks)
	want := []string{"b.go", "a.go", "c.go"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if got := Files(nil); len(got) != 0 {
		t.Fatalf("Files(nil) = %v, want empty", got)
	}
}
