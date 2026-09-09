package review

import (
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/provider"
	"github.com/cjunks94/nitpick/internal/secrets"
)

// Context-file caps. The model context windows are 200K (Haiku) and 1M
// (Sonnet/Opus), so these are conservative. Token cost matters more than the
// limit: every extra 4K chars is ~1K tokens, roughly $0.001 on Haiku.
const (
	MaxContextFiles      = 5
	MaxContextFileBytes  = 60 * 1024  // skip individual files larger than 60 KiB
	MaxContextTotalBytes = 200 * 1024 // stop once the total exceeds 200 KiB
)

// contextDenyExtensions are file suffixes never fetched as context: they are
// generated, binary metadata, or lockfile churn that adds no review signal
// and wastes the budget. Observed in prod: Godot .uid files (3 bytes of
// "uid://...") ate 40% of a PR's context budget, crowding out the changed
// source files. Lowercase comparison; extensions include the leading dot.
var contextDenyExtensions = []string{
	".uid",     // Godot resource metadata
	".sum",     // go.sum / similar checksum files
	".lock",    // generic lockfile suffix
	".min.js",  // minified bundles
	".min.css", // minified bundles
	".map",     // sourcemaps
	".pb.go",   // generated protobuf (Go)
	".pyc",     // compiled Python
}

// contextDenyFilenames are basenames always skipped regardless of path:
// lockfiles for the major ecosystems.
var contextDenyFilenames = map[string]bool{
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"Gemfile.lock":      true,
	"Cargo.lock":        true,
	"poetry.lock":       true,
	"Pipfile.lock":      true,
	"composer.lock":     true,
	"go.sum":            true,
}

// IsContextDenied reports whether a path is on the don't-fetch list.
// Extensions compare case-insensitively (some repos and OSes uppercase),
// basenames case-sensitively (lockfile names are stable).
func IsContextDenied(path string) bool {
	// Credentials files are dropped from context outright rather than
	// redacted. Context exists to explain surrounding code, and a secrets
	// file explains nothing: all risk, no review signal. (The diff path keeps
	// them, redacted, so the bot can still flag the commit.)
	if secrets.IsSensitivePath(path) {
		return true
	}
	if contextDenyFilenames[filepath.Base(path)] {
		return true
	}
	lower := strings.ToLower(path)
	for _, ext := range contextDenyExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// fileChangeWeight is the number of added+removed lines for a file across
// its hunks: the sort key so the biggest changes get context priority when
// the file-count budget is tight.
func fileChangeWeight(hunks []diff.Hunk, file string) int {
	n := 0
	for _, h := range hunks {
		if h.File != file {
			continue
		}
		for _, line := range h.Lines {
			if line.Kind == diff.LineAdded || line.Kind == diff.LineRemoved {
				n++
			}
		}
	}
	return n
}

// ContextCandidates picks which files touched by the diff are worth fetching
// whole: unique, not deny-listed, sorted by change weight descending, capped
// at MaxContextFiles. Pure, so serve and the eval snapshot select the same
// files for the same diff.
func ContextCandidates(hunks []diff.Hunk) []string {
	seen := make(map[string]bool, len(hunks))
	var paths []string
	for _, h := range hunks {
		if h.File == "" || seen[h.File] {
			continue
		}
		seen[h.File] = true
		if IsContextDenied(h.File) {
			continue
		}
		paths = append(paths, h.File)
	}
	sort.SliceStable(paths, func(i, j int) bool {
		return fileChangeWeight(hunks, paths[i]) > fileChangeWeight(hunks, paths[j])
	})
	if len(paths) > MaxContextFiles {
		paths = paths[:MaxContextFiles]
	}
	return paths
}

// AttachContext fetches each candidate through fetch and applies the byte
// caps and redaction the provider input requires. A fetch error skips the
// file (a new file does not exist at base, a deleted one not at head); an
// oversized file is skipped; the total cap stops the walk. Every attached
// file passes secrets.RedactBytes: the path deny-list catches files that are
// credentials by convention, this catches a key inside ordinary source. log
// may be nil.
func AttachContext(candidates []string, fetch func(path string) ([]byte, error), log *slog.Logger) []provider.ContextFile {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	var (
		out        []provider.ContextFile
		totalBytes int
	)
	for _, p := range candidates {
		content, err := fetch(p)
		if err != nil {
			log.Debug("context file fetch skipped", "path", p, "err", err)
			continue
		}
		if len(content) > MaxContextFileBytes {
			log.Debug("context file too large, skipping",
				"path", p, "bytes", len(content), "cap", MaxContextFileBytes)
			continue
		}
		if totalBytes+len(content) > MaxContextTotalBytes {
			log.Debug("context budget exhausted; stopping fetch",
				"so_far_bytes", totalBytes, "cap", MaxContextTotalBytes, "remaining_files", len(candidates)-len(out))
			break
		}
		content, redacted := secrets.RedactBytes(content)
		if redacted > 0 {
			log.Warn("redacted secrets from context file", "path", p, "lines", redacted)
		}
		out = append(out, provider.ContextFile{Path: p, Content: content})
		totalBytes += len(content)
	}
	log.Info("context fetched",
		"files_attempted", len(candidates),
		"files_attached", len(out),
		"total_bytes", totalBytes)
	return out
}
