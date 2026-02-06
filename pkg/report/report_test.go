package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/jdgilhuly/go_eval_agent/pkg/result"
)

func sampleSummary() *result.RunSummary {
	return &result.RunSummary{
		RunID:     "test-run",
		SuiteName: "test-suite",
		Duration:  3 * time.Second,
		Stats: result.Stats{
			TotalCases:        3,
			PassedCases:       1,
			FailedCases:       1,
			ErroredCases:      1,
			PassRate:          0.5,
			AvgScore:          0.6,
			LatencyP50:        200 * time.Millisecond,
			LatencyP95:        900 * time.Millisecond,
			TotalInputTokens:  100,
			TotalOutputTokens: 50,
		},
		Results: []result.CaseResult{
			{CaseID: "c1", CaseName: "pass-case", Pass: true, Score: 1.0, Duration: 100 * time.Millisecond},
			{CaseID: "c2", CaseName: "fail-case", Pass: false, Score: 0.3, Duration: 200 * time.Millisecond},
			{CaseID: "c3", CaseName: "error-case", Error: "timeout", Score: 0.0, Duration: 900 * time.Millisecond},
		},
	}
}

func TestStatusLabel(t *testing.T) {
	tests := []struct {
		name   string
		cr     result.CaseResult
		expect string
	}{
		{"pass", result.CaseResult{Pass: true}, "PASS"},
		{"fail", result.CaseResult{Pass: false}, "FAIL"},
		{"error", result.CaseResult{Error: "err"}, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label := StatusLabelPlain(tt.cr)
			if label != tt.expect {
				t.Errorf("StatusLabelPlain() = %q, want %q", label, tt.expect)
			}

			colored := StatusLabel(tt.cr)
			if !strings.Contains(colored, tt.expect) {
				t.Errorf("StatusLabel() = %q, should contain %q", colored, tt.expect)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Microsecond, "500us"},
		{150 * time.Millisecond, "150ms"},
		{2500 * time.Millisecond, "2.5s"},
		{0, "0us"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatDuration(tt.d)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestPrintSummaryTable_Plain(t *testing.T) {
	var buf bytes.Buffer
	PrintSummaryTable(&buf, sampleSummary(), false)
	output := buf.String()

	for _, want := range []string{
		"CASE", "STATUS", "SCORE", "LATENCY",
		"pass-case", "PASS",
		"fail-case", "FAIL",
		"error-case", "ERROR",
		"1 passed", "1 failed", "1 errored",
		"avg 0.60",
		"p50 200ms", "p95 900ms",
		"tokens: 100 in / 50 out",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestPrintSummaryTable_Colored(t *testing.T) {
	var buf bytes.Buffer
	PrintSummaryTable(&buf, sampleSummary(), true)
	output := buf.String()

	if !strings.Contains(output, colorGreen) {
		t.Error("colored output missing green ANSI code")
	}
	if !strings.Contains(output, colorRed) {
		t.Error("colored output missing red ANSI code")
	}
}

func TestPrintVerbose(t *testing.T) {
	summary := sampleSummary()
	summary.Results[0].FinalResponse = "The answer is 42."
	summary.Results[0].Prompt = "default"
	summary.Results[0].Model = "test-model"

	var buf bytes.Buffer
	PrintVerbose(&buf, summary, false)
	output := buf.String()

	for _, want := range []string{
		"Detailed Results",
		"Case: pass-case",
		"Response:",
		"The answer is 42.",
		"Case: error-case",
		"Error:    timeout",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("verbose output missing %q", want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"a-very-long-case-name-that-exceeds", 20, "a-very-long-case-..."},
		{"exact", 5, "exact"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.max)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}
