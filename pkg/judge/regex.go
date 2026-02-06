package judge

import (
	"fmt"
	"regexp"
)

// RegexJudge matches agent output against a regular expression pattern.
type RegexJudge struct {
	Pattern string `json:"pattern" yaml:"pattern"`
}

// Name returns the judge type identifier.
func (j *RegexJudge) Name() string { return "regex" }

// Evaluate checks if the output matches the configured regex pattern.
func (j *RegexJudge) Evaluate(input Input) (Result, error) {
	re, err := regexp.Compile(j.Pattern)
	if err != nil {
		return Result{}, fmt.Errorf("invalid regex pattern %q: %w", j.Pattern, err)
	}

	if re.MatchString(input.Output) {
		return Result{
			Pass:   true,
			Score:  1.0,
			Reason: fmt.Sprintf("output matches pattern %q", j.Pattern),
		}, nil
	}

	return Result{
		Pass:   false,
		Score:  0.0,
		Reason: fmt.Sprintf("output does not match pattern %q", j.Pattern),
	}, nil
}
