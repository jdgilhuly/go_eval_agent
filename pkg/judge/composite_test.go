package judge

import (
	"fmt"
	"math"
	"testing"
)

// stubJudge is a test helper that returns a fixed result.
type stubJudge struct {
	name   string
	result Result
	err    error
}

func (s *stubJudge) Name() string                      { return s.name }
func (s *stubJudge) Evaluate(Input) (Result, error) { return s.result, s.err }

func TestCompositeScorer_AllPass(t *testing.T) {
	cs := NewCompositeScorer(0.5)
	result := cs.Score(Input{}, []JudgeConfig{
		{Judge: &stubJudge{name: "a", result: Result{Pass: true, Score: 1.0, Reason: "ok"}}, Weight: 1.0},
		{Judge: &stubJudge{name: "b", result: Result{Pass: true, Score: 1.0, Reason: "ok"}}, Weight: 1.0},
	})

	if result.Status != StatusPass {
		t.Errorf("status = %q, want %q", result.Status, StatusPass)
	}
	if !result.Pass {
		t.Error("expected pass")
	}
	if result.CompositeScore != 1.0 {
		t.Errorf("composite = %v, want 1.0", result.CompositeScore)
	}
	if len(result.Scores) != 2 {
		t.Errorf("expected 2 scores, got %d", len(result.Scores))
	}
}

func TestCompositeScorer_AllFail(t *testing.T) {
	cs := NewCompositeScorer(0.5)
	result := cs.Score(Input{}, []JudgeConfig{
		{Judge: &stubJudge{name: "a", result: Result{Pass: false, Score: 0.0, Reason: "nope"}}, Weight: 1.0},
		{Judge: &stubJudge{name: "b", result: Result{Pass: false, Score: 0.0, Reason: "nope"}}, Weight: 1.0},
	})

	if result.Status != StatusFail {
		t.Errorf("status = %q, want %q", result.Status, StatusFail)
	}
	if result.Pass {
		t.Error("expected fail")
	}
	if result.CompositeScore != 0.0 {
		t.Errorf("composite = %v, want 0.0", result.CompositeScore)
	}
}

func TestCompositeScorer_WeightedAverage(t *testing.T) {
	cs := NewCompositeScorer(0.5)
	result := cs.Score(Input{}, []JudgeConfig{
		{Judge: &stubJudge{name: "high", result: Result{Pass: true, Score: 1.0, Reason: "ok"}}, Weight: 3.0},
		{Judge: &stubJudge{name: "low", result: Result{Pass: false, Score: 0.0, Reason: "nope"}}, Weight: 1.0},
	})

	// Weighted average: (1.0*3 + 0.0*1) / (3+1) = 0.75
	expected := 0.75
	if math.Abs(result.CompositeScore-expected) > 0.001 {
		t.Errorf("composite = %v, want %v", result.CompositeScore, expected)
	}
	if !result.Pass {
		t.Error("expected pass (0.75 >= 0.5)")
	}
}

func TestCompositeScorer_ThresholdBorder(t *testing.T) {
	cs := NewCompositeScorer(0.75)
	result := cs.Score(Input{}, []JudgeConfig{
		{Judge: &stubJudge{name: "a", result: Result{Pass: true, Score: 1.0}}, Weight: 1.0},
		{Judge: &stubJudge{name: "b", result: Result{Pass: false, Score: 0.0}}, Weight: 1.0},
	})

	// Composite = 0.5, threshold = 0.75 -> fail
	if result.Pass {
		t.Error("expected fail (0.5 < 0.75 threshold)")
	}
	if result.Status != StatusFail {
		t.Errorf("status = %q, want %q", result.Status, StatusFail)
	}
}

func TestCompositeScorer_DefaultWeight(t *testing.T) {
	cs := NewCompositeScorer(0.5)
	result := cs.Score(Input{}, []JudgeConfig{
		{Judge: &stubJudge{name: "a", result: Result{Pass: true, Score: 1.0}}, Weight: 0},
		{Judge: &stubJudge{name: "b", result: Result{Pass: false, Score: 0.0}}, Weight: 0},
	})

	// Weight 0 defaults to 1.0, so average = 0.5. At threshold 0.5, pass.
	expected := 0.5
	if math.Abs(result.CompositeScore-expected) > 0.001 {
		t.Errorf("composite = %v, want %v", result.CompositeScore, expected)
	}
	if !result.Pass {
		t.Error("expected pass with default weights at threshold boundary")
	}
}

func TestCompositeScorer_ReviewOverride(t *testing.T) {
	cs := NewCompositeScorer(0.5)
	result := cs.Score(Input{}, []JudgeConfig{
		{Judge: &stubJudge{name: "pass", result: Result{Pass: true, Score: 1.0, Reason: "ok"}}, Weight: 1.0},
		{Judge: &stubJudge{name: "review", result: Result{Pass: true, Score: 0.8, Reason: "review"}}, Weight: 1.0},
	})

	if result.Status != StatusReview {
		t.Errorf("status = %q, want %q", result.Status, StatusReview)
	}
	if result.Pass {
		t.Error("expected fail when any judge returns review")
	}
}

func TestCompositeScorer_ErrorOverride(t *testing.T) {
	cs := NewCompositeScorer(0.5)
	result := cs.Score(Input{}, []JudgeConfig{
		{Judge: &stubJudge{name: "pass", result: Result{Pass: true, Score: 1.0, Reason: "ok"}}, Weight: 1.0},
		{Judge: &stubJudge{name: "broken", err: fmt.Errorf("judge crashed")}, Weight: 1.0},
	})

	if result.Status != StatusError {
		t.Errorf("status = %q, want %q", result.Status, StatusError)
	}
	if result.Pass {
		t.Error("expected fail when any judge errors")
	}
}

func TestCompositeScorer_ErrorTakesPrecedenceOverReview(t *testing.T) {
	cs := NewCompositeScorer(0.5)
	result := cs.Score(Input{}, []JudgeConfig{
		{Judge: &stubJudge{name: "review", result: Result{Pass: true, Score: 0.8, Reason: "review"}}, Weight: 1.0},
		{Judge: &stubJudge{name: "broken", err: fmt.Errorf("judge crashed")}, Weight: 1.0},
	})

	if result.Status != StatusError {
		t.Errorf("status = %q, want %q (error should override review)", result.Status, StatusError)
	}
}

func TestCompositeScorer_PerJudgeScoresPreserved(t *testing.T) {
	cs := NewCompositeScorer(0.5)
	result := cs.Score(Input{}, []JudgeConfig{
		{Judge: &stubJudge{name: "exact", result: Result{Pass: true, Score: 1.0, Reason: "matched"}}, Weight: 2.0},
		{Judge: &stubJudge{name: "regex", result: Result{Pass: false, Score: 0.0, Reason: "no match"}}, Weight: 1.0},
	})

	if len(result.Scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(result.Scores))
	}

	if result.Scores[0].JudgeName != "exact" || result.Scores[0].Score != 1.0 || result.Scores[0].Weight != 2.0 {
		t.Errorf("score 0 unexpected: %+v", result.Scores[0])
	}
	if result.Scores[1].JudgeName != "regex" || result.Scores[1].Score != 0.0 || result.Scores[1].Weight != 1.0 {
		t.Errorf("score 1 unexpected: %+v", result.Scores[1])
	}
}

func TestCompositeScorer_EmptyJudges(t *testing.T) {
	cs := NewCompositeScorer(0.5)
	result := cs.Score(Input{}, nil)

	if result.CompositeScore != 0.0 {
		t.Errorf("composite = %v, want 0.0 for no judges", result.CompositeScore)
	}
	if result.Status != StatusFail {
		t.Errorf("status = %q, want %q for no judges", result.Status, StatusFail)
	}
}

func TestCompositeScorer_DefaultThreshold(t *testing.T) {
	cs := NewCompositeScorer(0) // should default to 0.5
	if cs.Threshold != 0.5 {
		t.Errorf("threshold = %v, want 0.5 as default", cs.Threshold)
	}
}
