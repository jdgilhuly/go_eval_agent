package review

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jdgilhuly/go_eval_agent/pkg/result"
)

// Filter determines which cases are shown for review.
type Filter string

const (
	FilterReview Filter = "review"
	FilterFail   Filter = "fail"
	FilterAll    Filter = "all"
)

// ParseFilter converts a string to a Filter, defaulting to FilterReview.
func ParseFilter(s string) Filter {
	switch strings.ToLower(s) {
	case "fail", "failed":
		return FilterFail
	case "all":
		return FilterAll
	default:
		return FilterReview
	}
}

// Reviewer handles interactive review of eval results.
type Reviewer struct {
	In  io.Reader
	Out io.Writer
}

// Review presents filtered cases for human grading and returns the updated
// summary with grades applied. Returns the number of cases reviewed.
func (r *Reviewer) Review(summary *result.RunSummary, filter Filter) (int, error) {
	indices := filterCases(summary.Results, filter)
	if len(indices) == 0 {
		fmt.Fprintf(r.Out, "No cases match filter %q.\n", string(filter))
		return 0, nil
	}

	scanner := bufio.NewScanner(r.In)
	reviewed := 0

	for i, idx := range indices {
		cr := &summary.Results[idx]
		fmt.Fprintf(r.Out, "\n--- Case %d of %d ---\n", i+1, len(indices))
		printCase(r.Out, cr)

		fmt.Fprintf(r.Out, "\nGrade [pass/fail/1-5/skip]: ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if input == "" || input == "skip" || input == "s" {
			fmt.Fprintf(r.Out, "  Skipped.\n")
			continue
		}

		applyGrade(cr, input)
		reviewed++
		fmt.Fprintf(r.Out, "  Graded: status=%s score=%.1f\n", cr.Status, cr.Score)
	}

	// Recompute stats after grading.
	summary.Stats = result.ComputeStats(summary.Results)

	return reviewed, scanner.Err()
}

func filterCases(results []result.CaseResult, filter Filter) []int {
	var indices []int
	for i, cr := range results {
		switch filter {
		case FilterReview:
			if cr.Status == "review" {
				indices = append(indices, i)
			}
		case FilterFail:
			if cr.Status == "fail" || cr.Status == "review" {
				indices = append(indices, i)
			}
		case FilterAll:
			indices = append(indices, i)
		}
	}
	return indices
}

func printCase(w io.Writer, cr *result.CaseResult) {
	fmt.Fprintf(w, "Name:     %s\n", cr.CaseName)
	fmt.Fprintf(w, "Status:   %s\n", cr.Status)
	if cr.Prompt != "" {
		fmt.Fprintf(w, "Prompt:   %s\n", truncateStr(cr.Prompt, 200))
	}
	fmt.Fprintf(w, "Output:   %s\n", truncateStr(cr.FinalResponse, 500))
	if cr.Error != "" {
		fmt.Fprintf(w, "Error:    %s\n", cr.Error)
	}
}

func applyGrade(cr *result.CaseResult, input string) {
	switch input {
	case "pass", "p":
		cr.Status = "pass"
		cr.Pass = true
		cr.Score = 1.0
	case "fail", "f":
		cr.Status = "fail"
		cr.Pass = false
		cr.Score = 0.0
	default:
		// Try numeric score 1-5.
		if score, err := strconv.Atoi(input); err == nil && score >= 1 && score <= 5 {
			cr.Score = float64(score) / 5.0
			cr.Pass = score >= 4
			if cr.Pass {
				cr.Status = "pass"
			} else {
				cr.Status = "fail"
			}
		}
	}
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
