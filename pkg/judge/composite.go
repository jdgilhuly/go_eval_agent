package judge

import (
	"fmt"
	"strings"
)

// Status represents the overall evaluation status.
type Status string

const (
	StatusPass   Status = "pass"
	StatusFail   Status = "fail"
	StatusReview Status = "review"
	StatusError  Status = "error"
)

// JudgeScore captures a single judge's contribution to the composite score.
type JudgeScore struct {
	JudgeName string  `json:"judge_name"`
	Pass      bool    `json:"pass"`
	Score     float64 `json:"score"`
	Weight    float64 `json:"weight"`
	Reason    string  `json:"reason"`
	Status    Status  `json:"status"`
}

// CompositeResult holds the aggregated scoring result from all judges.
type CompositeResult struct {
	Status         Status       `json:"status"`
	CompositeScore float64      `json:"composite_score"`
	Pass           bool         `json:"pass"`
	Scores         []JudgeScore `json:"scores"`
	Reason         string       `json:"reason"`
}

// JudgeConfig pairs a judge with its weight for composite scoring.
type JudgeConfig struct {
	Judge  Judge   `json:"-"`
	Weight float64 `json:"weight"`
}

// CompositeScorer combines multiple judge results into a single score.
type CompositeScorer struct {
	Threshold float64 `json:"threshold"` // pass threshold (default 0.5)
}

// NewCompositeScorer creates a CompositeScorer with the given pass threshold.
// If threshold is 0, it defaults to 0.5.
func NewCompositeScorer(threshold float64) *CompositeScorer {
	if threshold == 0 {
		threshold = 0.5
	}
	return &CompositeScorer{Threshold: threshold}
}

// Score evaluates input against all configured judges and returns the
// composite result. Each judge's score is weighted and the composite is
// the weighted average normalized to 0-1.
func (cs *CompositeScorer) Score(input Input, configs []JudgeConfig) CompositeResult {
	var scores []JudgeScore
	var totalWeight float64
	var weightedSum float64
	var hasReview, hasError bool
	var reasons []string

	for _, cfg := range configs {
		w := cfg.Weight
		if w == 0 {
			w = 1.0
		}

		result, err := cfg.Judge.Evaluate(input)

		js := JudgeScore{
			JudgeName: cfg.Judge.Name(),
			Weight:    w,
		}

		if err != nil {
			js.Status = StatusError
			js.Reason = err.Error()
			hasError = true
			reasons = append(reasons, fmt.Sprintf("%s: error: %s", cfg.Judge.Name(), err.Error()))
		} else {
			js.Pass = result.Pass
			js.Score = result.Score
			js.Reason = result.Reason

			if result.Reason == "review" {
				js.Status = StatusReview
				hasReview = true
			} else if result.Pass {
				js.Status = StatusPass
			} else {
				js.Status = StatusFail
			}

			weightedSum += result.Score * w
			totalWeight += w
			reasons = append(reasons, fmt.Sprintf("%s: %s (score=%.2f)", cfg.Judge.Name(), result.Reason, result.Score))
		}

		scores = append(scores, js)
	}

	var composite float64
	if totalWeight > 0 {
		composite = weightedSum / totalWeight
	}

	status := StatusFail
	pass := composite >= cs.Threshold

	if hasError {
		status = StatusError
		pass = false
	} else if hasReview {
		status = StatusReview
		pass = false
	} else if pass {
		status = StatusPass
	}

	return CompositeResult{
		Status:         status,
		CompositeScore: composite,
		Pass:           pass,
		Scores:         scores,
		Reason:         strings.Join(reasons, "; "),
	}
}
