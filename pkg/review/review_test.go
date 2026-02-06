package review

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jdgilhuly/go_eval_agent/pkg/result"
)

func testSummary() *result.RunSummary {
	return &result.RunSummary{
		SuiteName: "test-suite",
		Results: []result.CaseResult{
			{CaseID: "1", CaseName: "case-pass", Status: "pass", Pass: true, Score: 1.0, FinalResponse: "correct answer"},
			{CaseID: "2", CaseName: "case-review", Status: "review", Pass: false, Score: 0.0, FinalResponse: "needs human check"},
			{CaseID: "3", CaseName: "case-fail", Status: "fail", Pass: false, Score: 0.0, FinalResponse: "wrong answer"},
			{CaseID: "4", CaseName: "case-review2", Status: "review", Pass: false, Score: 0.0, FinalResponse: "another review"},
		},
	}
}

func TestReviewer_ReviewFilter(t *testing.T) {
	summary := testSummary()
	r := &Reviewer{
		In:  strings.NewReader("pass\nfail\n"),
		Out: &bytes.Buffer{},
	}

	reviewed, err := r.Review(summary, FilterReview)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only review-status cases shown: case-review and case-review2.
	if reviewed != 2 {
		t.Errorf("reviewed = %d, want 2", reviewed)
	}

	// case-review graded pass.
	if summary.Results[1].Status != "pass" {
		t.Errorf("case-review status = %q, want %q", summary.Results[1].Status, "pass")
	}
	if !summary.Results[1].Pass {
		t.Error("case-review Pass = false, want true")
	}

	// case-review2 graded fail.
	if summary.Results[3].Status != "fail" {
		t.Errorf("case-review2 status = %q, want %q", summary.Results[3].Status, "fail")
	}
}

func TestReviewer_ReviewNumericScore(t *testing.T) {
	summary := testSummary()
	r := &Reviewer{
		In:  strings.NewReader("4\n2\n"),
		Out: &bytes.Buffer{},
	}

	reviewed, err := r.Review(summary, FilterReview)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reviewed != 2 {
		t.Errorf("reviewed = %d, want 2", reviewed)
	}

	// Score 4 -> pass, score 0.8.
	if summary.Results[1].Score != 0.8 {
		t.Errorf("case-review score = %f, want 0.8", summary.Results[1].Score)
	}
	if !summary.Results[1].Pass {
		t.Error("case-review with score 4 should pass")
	}

	// Score 2 -> fail, score 0.4.
	if summary.Results[3].Score != 0.4 {
		t.Errorf("case-review2 score = %f, want 0.4", summary.Results[3].Score)
	}
	if summary.Results[3].Pass {
		t.Error("case-review2 with score 2 should fail")
	}
}

func TestReviewer_SkipCases(t *testing.T) {
	summary := testSummary()
	r := &Reviewer{
		In:  strings.NewReader("skip\npass\n"),
		Out: &bytes.Buffer{},
	}

	reviewed, err := r.Review(summary, FilterReview)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only one was actually graded (skip doesn't count).
	if reviewed != 1 {
		t.Errorf("reviewed = %d, want 1", reviewed)
	}

	// Skipped case stays as review.
	if summary.Results[1].Status != "review" {
		t.Errorf("skipped case status = %q, want %q", summary.Results[1].Status, "review")
	}

	// Second case graded pass.
	if summary.Results[3].Status != "pass" {
		t.Errorf("graded case status = %q, want %q", summary.Results[3].Status, "pass")
	}
}

func TestReviewer_FilterFail(t *testing.T) {
	summary := testSummary()
	r := &Reviewer{
		// 3 cases: case-review, case-fail, case-review2.
		In:  strings.NewReader("pass\npass\npass\n"),
		Out: &bytes.Buffer{},
	}

	reviewed, err := r.Review(summary, FilterFail)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reviewed != 3 {
		t.Errorf("reviewed = %d, want 3", reviewed)
	}
}

func TestReviewer_FilterAll(t *testing.T) {
	summary := testSummary()
	r := &Reviewer{
		In:  strings.NewReader("skip\nskip\nskip\nskip\n"),
		Out: &bytes.Buffer{},
	}

	reviewed, err := r.Review(summary, FilterAll)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All skipped, none actually graded.
	if reviewed != 0 {
		t.Errorf("reviewed = %d, want 0", reviewed)
	}
}

func TestReviewer_NoCases(t *testing.T) {
	summary := &result.RunSummary{Results: []result.CaseResult{
		{CaseID: "1", CaseName: "pass", Status: "pass"},
	}}
	out := &bytes.Buffer{}
	r := &Reviewer{
		In:  strings.NewReader(""),
		Out: out,
	}

	reviewed, err := r.Review(summary, FilterReview)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reviewed != 0 {
		t.Errorf("reviewed = %d, want 0", reviewed)
	}
	if !strings.Contains(out.String(), "No cases") {
		t.Error("expected 'No cases' message in output")
	}
}

func TestReviewer_ProgressIndicator(t *testing.T) {
	summary := testSummary()
	out := &bytes.Buffer{}
	r := &Reviewer{
		In:  strings.NewReader("pass\npass\n"),
		Out: out,
	}

	r.Review(summary, FilterReview)

	output := out.String()
	if !strings.Contains(output, "Case 1 of 2") {
		t.Error("missing progress indicator 'Case 1 of 2'")
	}
	if !strings.Contains(output, "Case 2 of 2") {
		t.Error("missing progress indicator 'Case 2 of 2'")
	}
}

func TestParseFilter(t *testing.T) {
	tests := []struct {
		input string
		want  Filter
	}{
		{"review", FilterReview},
		{"fail", FilterFail},
		{"failed", FilterFail},
		{"all", FilterAll},
		{"", FilterReview},
		{"unknown", FilterReview},
	}

	for _, tt := range tests {
		got := ParseFilter(tt.input)
		if got != tt.want {
			t.Errorf("ParseFilter(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHumanReviewJudge(t *testing.T) {
	// Import judge types aren't available here, so test via the review package's integration.
	// The HumanReviewJudge is in pkg/judge/review.go and tested via judge_test.go patterns.
	// This test verifies filter matches review status correctly.
	indices := filterCases([]result.CaseResult{
		{Status: "pass"},
		{Status: "review"},
		{Status: "fail"},
		{Status: "review"},
	}, FilterReview)

	if len(indices) != 2 {
		t.Errorf("review filter matched %d cases, want 2", len(indices))
	}
	if indices[0] != 1 || indices[1] != 3 {
		t.Errorf("review filter indices = %v, want [1, 3]", indices)
	}
}
