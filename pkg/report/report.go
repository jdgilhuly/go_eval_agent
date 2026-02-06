package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jdgilhuly/go_eval_agent/pkg/result"
)

// ANSI color codes for terminal output.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

// StatusLabel returns a colored status string for terminal display.
func StatusLabel(cr result.CaseResult) string {
	if cr.Error != "" {
		return colorRed + "ERROR" + colorReset
	}
	if cr.Pass {
		return colorGreen + "PASS" + colorReset
	}
	return colorRed + "FAIL" + colorReset
}

// StatusLabelPlain returns an uncolored status string.
func StatusLabelPlain(cr result.CaseResult) string {
	if cr.Error != "" {
		return "ERROR"
	}
	if cr.Pass {
		return "PASS"
	}
	return "FAIL"
}

// FormatDuration formats a duration for table display.
func FormatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dus", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// PrintSummaryTable writes a formatted summary table of run results.
func PrintSummaryTable(w io.Writer, summary *result.RunSummary, color bool) {
	// Header.
	sep := strings.Repeat("-", 78)
	fmt.Fprintf(w, "%s\n", sep)
	fmt.Fprintf(w, "  %-30s  %-7s  %8s  %8s\n", "CASE", "STATUS", "SCORE", "LATENCY")
	fmt.Fprintf(w, "%s\n", sep)

	// Case rows.
	for _, cr := range summary.Results {
		name := truncate(cr.CaseName, 30)
		var status string
		if color {
			status = StatusLabel(cr)
		} else {
			status = StatusLabelPlain(cr)
		}
		fmt.Fprintf(w, "  %-30s  %-7s  %8.2f  %8s\n",
			name, status, cr.Score, FormatDuration(cr.Duration))
	}

	// Footer.
	fmt.Fprintf(w, "%s\n", sep)
	s := summary.Stats
	if color {
		fmt.Fprintf(w, "  %s%d passed%s  %s%d failed%s  %s%d errored%s  | avg %.2f | %s total\n",
			colorGreen, s.PassedCases, colorReset,
			colorRed, s.FailedCases, colorReset,
			colorYellow, s.ErroredCases, colorReset,
			s.AvgScore, FormatDuration(summary.Duration))
	} else {
		fmt.Fprintf(w, "  %d passed  %d failed  %d errored  | avg %.2f | %s total\n",
			s.PassedCases, s.FailedCases, s.ErroredCases,
			s.AvgScore, FormatDuration(summary.Duration))
	}
	fmt.Fprintf(w, "  p50 %s | p95 %s | tokens: %d in / %d out\n",
		FormatDuration(s.LatencyP50), FormatDuration(s.LatencyP95),
		s.TotalInputTokens, s.TotalOutputTokens)
	fmt.Fprintf(w, "%s\n", sep)
}

// PrintVerbose writes detailed per-case output including full responses.
func PrintVerbose(w io.Writer, summary *result.RunSummary, color bool) {
	PrintSummaryTable(w, summary, color)

	fmt.Fprintf(w, "\n--- Detailed Results ---\n\n")

	for _, cr := range summary.Results {
		var status string
		if color {
			status = StatusLabel(cr)
		} else {
			status = StatusLabelPlain(cr)
		}

		fmt.Fprintf(w, "Case: %s [%s]\n", cr.CaseName, status)
		fmt.Fprintf(w, "  ID:       %s\n", cr.CaseID)
		fmt.Fprintf(w, "  Prompt:   %s\n", cr.Prompt)
		fmt.Fprintf(w, "  Model:    %s\n", cr.Model)
		fmt.Fprintf(w, "  Score:    %.2f\n", cr.Score)
		fmt.Fprintf(w, "  Latency:  %s\n", FormatDuration(cr.Duration))
		fmt.Fprintf(w, "  Tokens:   %d in / %d out\n", cr.InputTokens, cr.OutputTokens)

		if cr.Error != "" {
			fmt.Fprintf(w, "  Error:    %s\n", cr.Error)
		}

		if cr.FinalResponse != "" {
			fmt.Fprintf(w, "  Response:\n")
			for _, line := range strings.Split(cr.FinalResponse, "\n") {
				fmt.Fprintf(w, "    %s\n", line)
			}
		}
		fmt.Fprintln(w)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
