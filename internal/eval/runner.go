// Package eval replays labeled PR cases against a Provider and writes a
// markdown report.
//
// The eval harness is the artifact that distinguishes "wrote an LLM wrapper"
// from "did engineering". Commit REPORT.md on every prompt change; its git
// history is the prompt-tuning log.
package eval

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cjunks94/nitpick/internal/diff"
	"github.com/cjunks94/nitpick/internal/provider"
)

type Case struct {
	PR       int               `json:"pr"`
	Repo     string            `json:"repo"`
	DiffPath string            `json:"diff_path"`
	Expected []ExpectedFinding `json:"expected"`
}

type ExpectedFinding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Category string `json:"category,omitempty"`
	Note     string `json:"note,omitempty"`
}

type CaseResult struct {
	Case    Case
	Hits    []provider.Comment
	Misses  []ExpectedFinding
	Extras  []provider.Comment
	CostUSD float64
}

// Run loads cases, executes the provider against each, and writes a report.
// When loadGuidelines is false, per-repo CLAUDE.md files are skipped — useful
// for measuring baseline variance against the with-guidelines configuration.
func Run(ctx context.Context, casesPath, outPath string, p provider.Provider, loadGuidelines bool) error {
	cases, err := loadCases(casesPath)
	if err != nil {
		return fmt.Errorf("load cases: %w", err)
	}

	reposDir := filepath.Join(filepath.Dir(casesPath), "repos")

	var results []CaseResult
	for _, c := range cases {
		raw, err := os.ReadFile(c.DiffPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", c.DiffPath, err)
		}
		hunks, err := diff.ParseUnifiedDiff(raw)
		if err != nil {
			return fmt.Errorf("parse %s: %w", c.DiffPath, err)
		}
		var guidelines []byte
		if loadGuidelines {
			guidelines, err = loadRepoGuidelines(reposDir, c.Repo)
			if err != nil {
				return fmt.Errorf("load guidelines for %s: %w", c.Repo, err)
			}
		}
		res, err := p.Review(ctx, provider.ReviewRequest{
			Hunks:          hunks,
			RepoGuidelines: guidelines,
		})
		if err != nil {
			// Log but continue — one bad LLM response shouldn't tank a 20-PR
			// sweep that cost real money. The case gets recorded as zero
			// findings, which counts as misses against its expected labels
			// (honest accounting of the failure).
			fmt.Fprintf(os.Stderr, "nitpick: PR #%d (%s) errored, recording zero findings: %v\n", c.PR, c.Repo, err)
			res = provider.ReviewResult{}
		}
		results = append(results, score(c, res))
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	return writeReport(out, p.Name(), results)
}

func score(c Case, res provider.ReviewResult) CaseResult {
	cr := CaseResult{Case: c, CostUSD: res.CostUSD}
	matched := make(map[int]bool)

	for _, exp := range c.Expected {
		hit := false
		for i, com := range res.Comments {
			if matched[i] {
				continue
			}
			if com.File == exp.File && abs(com.Line-exp.Line) <= 3 {
				cr.Hits = append(cr.Hits, com)
				matched[i] = true
				hit = true
				break
			}
		}
		if !hit {
			cr.Misses = append(cr.Misses, exp)
		}
	}
	for i, com := range res.Comments {
		if !matched[i] {
			cr.Extras = append(cr.Extras, com)
		}
	}
	return cr
}

// loadRepoGuidelines returns the contents of repos/<owner>__<repo>.md if it
// exists, nil otherwise. A missing file is not an error — most repos don't
// have a CLAUDE.md, and the provider handles a nil RepoGuidelines fine.
func loadRepoGuidelines(reposDir, repo string) ([]byte, error) {
	sanitized := strings.ReplaceAll(repo, "/", "__")
	path := filepath.Join(reposDir, sanitized+".md")
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

func loadCases(path string) ([]Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Case
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		var c Case
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("parse case: %w", err)
		}
		out = append(out, c)
	}
	return out, sc.Err()
}

func writeReport(w io.Writer, providerName string, results []CaseResult) error {
	totalExpected, totalCritical, totalUseful := 0, 0, 0
	totalHits, totalCriticalHits, totalUsefulHits := 0, 0, 0
	totalExtras := 0
	totalCost := 0.0

	for _, r := range results {
		for _, e := range r.Case.Expected {
			totalExpected++
			switch e.Severity {
			case "critical":
				totalCritical++
			case "useful":
				totalUseful++
			}
		}
		for _, h := range r.Hits {
			totalHits++
			switch h.Severity {
			case provider.SeverityCritical:
				totalCriticalHits++
			case provider.SeverityUseful:
				totalUsefulHits++
			}
		}
		totalExtras += len(r.Extras)
		totalCost += r.CostUSD
	}

	totalProduced := totalHits + totalExtras
	precision := safeDiv(float64(totalHits), float64(totalProduced))
	recall := safeDiv(float64(totalHits), float64(totalExpected))
	criticalRecall := safeDiv(float64(totalCriticalHits), float64(totalCritical))
	usefulRecall := safeDiv(float64(totalUsefulHits), float64(totalUseful))
	noiseRate := safeDiv(float64(totalExtras), float64(totalProduced))
	avgCost := safeDiv(totalCost, float64(len(results)))

	fmt.Fprintf(w, "# Eval report — `%s`\n\n", providerName)
	fmt.Fprintf(w, "Cases: %d  ·  Expected findings: %d  ·  Produced: %d\n\n",
		len(results), totalExpected, totalProduced)
	fmt.Fprintln(w, "| Metric | Value |")
	fmt.Fprintln(w, "|---|---|")
	fmt.Fprintf(w, "| Precision | %.3f |\n", precision)
	fmt.Fprintf(w, "| Recall (all) | %.3f |\n", recall)
	fmt.Fprintf(w, "| Recall (critical) | %.3f |\n", criticalRecall)
	fmt.Fprintf(w, "| Recall (useful) | %.3f |\n", usefulRecall)
	fmt.Fprintf(w, "| Noise rate | %.3f |\n", noiseRate)
	fmt.Fprintf(w, "| Avg $/PR | $%.4f |\n", avgCost)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Per-case")
	fmt.Fprintln(w, "| PR | Repo | Expected | Hits | Misses | Extras | $ |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|")
	for _, r := range results {
		fmt.Fprintf(w, "| #%d | %s | %d | %d | %d | %d | $%.4f |\n",
			r.Case.PR, r.Case.Repo,
			len(r.Case.Expected), len(r.Hits), len(r.Misses), len(r.Extras),
			r.CostUSD)
	}
	return nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
