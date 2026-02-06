package evaltest

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jdgilhuly/go_eval_agent/pkg/judge"
)

// AssertOutputContains asserts that the output contains the given substring.
func (tc *TestCase) AssertOutputContains(substr string) {
	tc.t.Helper()
	if !tc.executed {
		tc.t.Error("AssertOutputContains called before Input()")
		return
	}
	if !strings.Contains(tc.output, substr) {
		tc.t.Errorf("output does not contain %q\n  output: %s", substr, truncate(tc.output, 200))
	}
}

// AssertOutputMatches asserts that the output matches the given regex pattern.
func (tc *TestCase) AssertOutputMatches(pattern string) {
	tc.t.Helper()
	if !tc.executed {
		tc.t.Error("AssertOutputMatches called before Input()")
		return
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		tc.t.Errorf("invalid regex pattern %q: %v", pattern, err)
		return
	}
	if !re.MatchString(tc.output) {
		tc.t.Errorf("output does not match pattern %q\n  output: %s", pattern, truncate(tc.output, 200))
	}
}

// AssertToolCalled asserts that the named tool was called at least once.
func (tc *TestCase) AssertToolCalled(toolName string) {
	tc.t.Helper()
	if tc.trace == nil {
		tc.t.Error("AssertToolCalled called before Input()")
		return
	}
	for _, call := range tc.trace.GetToolCalls() {
		if call.ToolName == toolName {
			return
		}
	}
	tc.t.Errorf("tool %q was not called", toolName)
}

// AssertToolNotCalled asserts that the named tool was never called.
func (tc *TestCase) AssertToolNotCalled(toolName string) {
	tc.t.Helper()
	if tc.trace == nil {
		tc.t.Error("AssertToolNotCalled called before Input()")
		return
	}
	for _, call := range tc.trace.GetToolCalls() {
		if call.ToolName == toolName {
			tc.t.Errorf("tool %q was called but should not have been", toolName)
			return
		}
	}
}

// AssertToolCalledWith asserts the named tool was called with parameters
// that are a superset of the given params (subset match).
func (tc *TestCase) AssertToolCalledWith(toolName string, params map[string]interface{}) {
	tc.t.Helper()
	if tc.trace == nil {
		tc.t.Error("AssertToolCalledWith called before Input()")
		return
	}
	for _, call := range tc.trace.GetToolCalls() {
		if call.ToolName == toolName && isSubset(params, call.Parameters) {
			return
		}
	}
	tc.t.Errorf("tool %q was not called with params %v", toolName, params)
}

// AssertLLMJudge runs the given LLM judge with the specified rubric and
// checks that the resulting score matches the provided ScoreMatcher. This
// requires that a real LLM provider is configured on the harness or that
// a mock provider is set up to return judge-formatted responses.
func (tc *TestCase) AssertLLMJudge(rubric string, matcher ScoreMatcher) {
	tc.t.Helper()
	if !tc.executed {
		tc.t.Error("AssertLLMJudge called before Input()")
		return
	}

	j := &judge.LLMJudge{
		Provider: tc.harness.provider,
		Rubric:   rubric,
	}

	input := judge.Input{
		Output: tc.output,
	}
	if tc.trace != nil {
		input.ToolCalls = tc.trace.GetToolCalls()
	}

	result, err := j.Evaluate(input)
	if err != nil {
		tc.t.Errorf("LLM judge evaluation failed: %v", err)
		return
	}

	if !matcher.Match(result.Score) {
		tc.t.Errorf("LLM judge score %.2f does not satisfy %s (reason: %s)", result.Score, matcher, result.Reason)
	}
}

// isSubset checks whether every key/value in subset exists in superset
// with the same value (compared via fmt.Sprintf).
func isSubset(subset, superset map[string]interface{}) bool {
	for k, v := range subset {
		sv, ok := superset[k]
		if !ok || fmt.Sprintf("%v", v) != fmt.Sprintf("%v", sv) {
			return false
		}
	}
	return true
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
