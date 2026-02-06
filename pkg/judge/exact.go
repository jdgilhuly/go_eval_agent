package judge

import (
	"fmt"
	"strings"
)

// ExactJudge compares agent output against an expected string.
type ExactJudge struct {
	NormalizeWhitespace bool `json:"normalize_whitespace" yaml:"normalize_whitespace"`
}

// Name returns the judge type identifier.
func (j *ExactJudge) Name() string { return "exact" }

// Evaluate checks if the output matches the expected output exactly.
// When NormalizeWhitespace is true, leading/trailing whitespace is trimmed
// and runs of internal whitespace are collapsed to single spaces.
func (j *ExactJudge) Evaluate(input Input) (Result, error) {
	got := input.Output
	want := input.ExpectedOutput

	if j.NormalizeWhitespace {
		got = normalizeWhitespace(got)
		want = normalizeWhitespace(want)
	}

	if got == want {
		return Result{
			Pass:   true,
			Score:  1.0,
			Reason: "output matches expected",
		}, nil
	}

	return Result{
		Pass:   false,
		Score:  0.0,
		Reason: fmt.Sprintf("output does not match expected: got %q, want %q", truncate(got, 100), truncate(want, 100)),
	}, nil
}

func normalizeWhitespace(s string) string {
	s = strings.TrimSpace(s)
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
