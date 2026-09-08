package diff

// Files returns the distinct file paths across hunks, in first-seen order.
// Used by model routing and by callers that need to reason about the PR at
// file granularity after ignore_paths has been applied.
func Files(hunks []Hunk) []string {
	seen := make(map[string]struct{}, len(hunks))
	files := make([]string, 0, len(hunks))
	for _, h := range hunks {
		if _, ok := seen[h.File]; ok {
			continue
		}
		seen[h.File] = struct{}{}
		files = append(files, h.File)
	}
	return files
}
