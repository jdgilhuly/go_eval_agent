package diff

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/jdgilhuly/go_eval_agent/pkg/result"
)

// Category classifies a case comparison.
type Category string

const (
	Improved  Category = "improved"
	Regressed Category = "regressed"
	Unchanged Category = "unchanged"
	New       Category = "new"
	Removed   Category = "removed"
)

// CaseDiff represents the comparison of a single case between two runs.
type CaseDiff struct {
	CaseName   string   `json:"case_name"`
	Category   Category `json:"category"`
	ScoreA     float64  `json:"score_a"`
	ScoreB     float64  `json:"score_b"`
	ScoreDelta float64  `json:"score_delta"`
	StatusA    string   `json:"status_a"`
	StatusB    string   `json:"status_b"`
}

// DiffResult holds the full comparison between two runs.
type DiffResult struct {
	RunA  string     `json:"run_a"`
	RunB  string     `json:"run_b"`
	Cases []CaseDiff `json:"cases"`
	Summary
}

// Summary holds counts by category.
type Summary struct {
	Improved  int `json:"improved"`
	Regressed int `json:"regressed"`
	Unchanged int `json:"unchanged"`
	New       int `json:"new"`
	Removed   int `json:"removed"`
}

// Compare produces a diff between two run summaries. Cases are matched by
// case_name. A threshold controls the minimum absolute score delta to
// classify a case as improved or regressed (below threshold = unchanged).
func Compare(a, b *result.RunSummary, threshold float64) *DiffResult {
	dr := &DiffResult{
		RunA: a.RunID,
		RunB: b.RunID,
	}

	// Index cases from run A by name.
	aMap := make(map[string]result.CaseResult, len(a.Results))
	for _, cr := range a.Results {
		aMap[cr.CaseName] = cr
	}

	// Index cases from run B by name.
	bMap := make(map[string]result.CaseResult, len(b.Results))
	for _, cr := range b.Results {
		bMap[cr.CaseName] = cr
	}

	// Process all cases in B (may be matched from A, or new).
	seen := make(map[string]bool, len(b.Results))
	for _, crB := range b.Results {
		seen[crB.CaseName] = true

		crA, inA := aMap[crB.CaseName]
		cd := CaseDiff{
			CaseName: crB.CaseName,
			ScoreB:   crB.Score,
			StatusB:  statusStr(crB),
		}

		if !inA {
			cd.Category = New
			dr.Summary.New++
		} else {
			cd.ScoreA = crA.Score
			cd.StatusA = statusStr(crA)
			cd.ScoreDelta = crB.Score - crA.Score

			if math.Abs(cd.ScoreDelta) <= threshold {
				cd.Category = Unchanged
				dr.Summary.Unchanged++
			} else if cd.ScoreDelta > 0 {
				cd.Category = Improved
				dr.Summary.Improved++
			} else {
				cd.Category = Regressed
				dr.Summary.Regressed++
			}
		}

		dr.Cases = append(dr.Cases, cd)
	}

	// Cases in A but not in B are removed.
	for _, crA := range a.Results {
		if !seen[crA.CaseName] {
			dr.Cases = append(dr.Cases, CaseDiff{
				CaseName: crA.CaseName,
				Category: Removed,
				ScoreA:   crA.Score,
				StatusA:  statusStr(crA),
			})
			dr.Summary.Removed++
		}
	}

	return dr
}

// Filter returns a new DiffResult with only cases matching the given
// categories. Pass nil to include all.
func (dr *DiffResult) Filter(categories []Category) *DiffResult {
	if len(categories) == 0 {
		return dr
	}

	catSet := make(map[Category]bool, len(categories))
	for _, c := range categories {
		catSet[c] = true
	}

	filtered := &DiffResult{
		RunA: dr.RunA,
		RunB: dr.RunB,
	}
	for _, cd := range dr.Cases {
		if catSet[cd.Category] {
			filtered.Cases = append(filtered.Cases, cd)
		}
	}
	filtered.Summary = dr.Summary
	return filtered
}

// JSON serializes the diff result.
func (dr *DiffResult) JSON() ([]byte, error) {
	return json.MarshalIndent(dr, "", "  ")
}

// PrintTable writes a formatted diff table.
func (dr *DiffResult) PrintTable(w io.Writer) {
	sep := strings.Repeat("-", 82)
	fmt.Fprintf(w, "%s\n", sep)
	fmt.Fprintf(w, "  %-25s  %-10s  %8s  %8s  %8s\n", "CASE", "CHANGE", "SCORE A", "SCORE B", "DELTA")
	fmt.Fprintf(w, "%s\n", sep)

	for _, cd := range dr.Cases {
		name := cd.CaseName
		if len(name) > 25 {
			name = name[:22] + "..."
		}

		var delta string
		switch cd.Category {
		case New:
			delta = "new"
		case Removed:
			delta = "removed"
		default:
			delta = fmt.Sprintf("%+.2f", cd.ScoreDelta)
		}

		fmt.Fprintf(w, "  %-25s  %-10s  %8.2f  %8.2f  %8s\n",
			name, string(cd.Category), cd.ScoreA, cd.ScoreB, delta)
	}

	fmt.Fprintf(w, "%s\n", sep)
	fmt.Fprintf(w, "  %d improved  %d regressed  %d unchanged  %d new  %d removed\n",
		dr.Summary.Improved, dr.Summary.Regressed, dr.Summary.Unchanged,
		dr.Summary.New, dr.Summary.Removed)
	fmt.Fprintf(w, "%s\n", sep)
}

func statusStr(cr result.CaseResult) string {
	if cr.Error != "" {
		return "error"
	}
	if cr.Pass {
		return "pass"
	}
	return "fail"
}
