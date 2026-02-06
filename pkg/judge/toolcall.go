package judge

import (
	"fmt"
	"strings"
)

// ExpectedToolCall describes a tool call assertion for the ToolCallJudge.
type ExpectedToolCall struct {
	ToolName   string                 `json:"tool_name" yaml:"tool_name"`
	Parameters map[string]interface{} `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Negate     bool                   `json:"negate,omitempty" yaml:"negate,omitempty"`
	MatchMode  string                 `json:"match_mode,omitempty" yaml:"match_mode,omitempty"` // "exact" or "subset" (default: "subset")
}

// ToolCallJudge asserts that expected tool calls were made (or not made)
// in order, with parameter matching.
type ToolCallJudge struct {
	Expected []ExpectedToolCall `json:"expected" yaml:"expected"`
}

// Name returns the judge type identifier.
func (j *ToolCallJudge) Name() string { return "toolcall" }

// Evaluate checks tool calls against expectations. Positive assertions
// are checked in order against the actual call sequence. Negative assertions
// verify that the tool was NOT called at all.
func (j *ToolCallJudge) Evaluate(input Input) (Result, error) {
	var failures []string

	// Separate positive and negative assertions.
	var positives []ExpectedToolCall
	var negatives []ExpectedToolCall
	for _, exp := range j.Expected {
		if exp.Negate {
			negatives = append(negatives, exp)
		} else {
			positives = append(positives, exp)
		}
	}

	// Check negative assertions: tool must NOT appear in calls.
	for _, neg := range negatives {
		for _, call := range input.ToolCalls {
			if call.ToolName == neg.ToolName {
				failures = append(failures, fmt.Sprintf("tool %q was called but should not have been", neg.ToolName))
				break
			}
		}
	}

	// Check positive assertions in order.
	callIdx := 0
	for _, exp := range positives {
		found := false
		for callIdx < len(input.ToolCalls) {
			call := input.ToolCalls[callIdx]
			callIdx++
			if call.ToolName == exp.ToolName && paramsMatch(exp.Parameters, call.Parameters, exp.MatchMode) {
				found = true
				break
			}
		}
		if !found {
			failures = append(failures, fmt.Sprintf("expected tool call %q not found in sequence", exp.ToolName))
		}
	}

	if len(failures) == 0 {
		return Result{
			Pass:   true,
			Score:  1.0,
			Reason: "all tool call assertions passed",
		}, nil
	}

	return Result{
		Pass:   false,
		Score:  0.0,
		Reason: strings.Join(failures, "; "),
	}, nil
}

// paramsMatch checks whether actual parameters satisfy expected parameters.
// In "exact" mode, the maps must have identical keys and values.
// In "subset" mode (default), every key in expected must be present in actual
// with the same value, but actual may have additional keys.
func paramsMatch(expected, actual map[string]interface{}, mode string) bool {
	if len(expected) == 0 {
		return true
	}

	if mode == "exact" {
		return mapsEqual(expected, actual)
	}

	// Default: subset match.
	return isSubset(expected, actual)
}

func mapsEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok || fmt.Sprintf("%v", v) != fmt.Sprintf("%v", bv) {
			return false
		}
	}
	return true
}

func isSubset(subset, superset map[string]interface{}) bool {
	for k, v := range subset {
		sv, ok := superset[k]
		if !ok || fmt.Sprintf("%v", v) != fmt.Sprintf("%v", sv) {
			return false
		}
	}
	return true
}
