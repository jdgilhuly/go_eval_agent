package judge

// HumanReviewJudge marks cases for human review instead of auto-grading.
// When used as part of a composite scorer, it sets the status to "review"
// so the eval review command can present these cases for manual grading.
type HumanReviewJudge struct {
	Reason string
}

// Name returns "human_review".
func (j *HumanReviewJudge) Name() string { return "human_review" }

// Evaluate always returns a result with the "review" reason, signaling
// that the case requires human evaluation.
func (j *HumanReviewJudge) Evaluate(_ Input) (Result, error) {
	reason := j.Reason
	if reason == "" {
		reason = "review"
	}
	return Result{
		Pass:   false,
		Score:  0,
		Reason: reason,
	}, nil
}
