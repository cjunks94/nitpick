package diff

// FilterByPath returns a new slice containing only hunks whose File field
// does not satisfy the keep predicate's drop case. Used by callers to apply
// .nitpick.yaml ignore_paths before sending hunks to the provider.
//
// The predicate-based shape keeps the diff package free of any glob library
// dependency — config owns the matcher, diff owns the slice mechanics.
func FilterByPath(hunks []Hunk, drop func(path string) bool) []Hunk {
	if drop == nil || len(hunks) == 0 {
		return hunks
	}
	out := make([]Hunk, 0, len(hunks))
	for _, h := range hunks {
		if drop(h.File) {
			continue
		}
		out = append(out, h)
	}
	return out
}
