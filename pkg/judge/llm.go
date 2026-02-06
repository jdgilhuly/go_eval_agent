package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jdgilhuly/go_eval_agent/pkg/provider"
)

const judgeSystemPrompt = `You are an expert evaluator grading an AI agent's output. You will be given:
1. The original input/question
2. The agent's output
3. A rubric describing how to evaluate

Grade the output on a scale of 1-5:
  1 = Completely wrong or irrelevant
  2 = Mostly wrong with minor correct elements
  3 = Partially correct but significant issues
  4 = Mostly correct with minor issues
  5 = Fully correct and complete

You MUST respond with ONLY a JSON object in this exact format, no other text:
{"score": <1-5>, "pass": <true/false>, "reasoning": "<your explanation>"}

Set "pass" to true if score >= 4, false otherwise.`

// scorePattern matches a standalone integer 1-5 in text as a fallback.
var scorePattern = regexp.MustCompile(`\b([1-5])\b`)

// LLMJudge uses an LLM provider to evaluate agent outputs against a rubric.
type LLMJudge struct {
	Provider provider.Provider
	Model    string
	Rubric   string
	Ctx      context.Context

	// Usage tracks token consumption from judge calls separately.
	Usage provider.Usage
}

// Name returns "llm".
func (j *LLMJudge) Name() string { return "llm" }

// Evaluate sends the agent input and output to the judge model for grading.
func (j *LLMJudge) Evaluate(input Input) (Result, error) {
	ctx := j.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	userMsg := buildJudgePrompt(j.Rubric, input)

	resp, err := j.Provider.Complete(ctx, &provider.Request{
		Model:     j.Model,
		System:    judgeSystemPrompt,
		Messages:  []provider.Message{{Role: "user", Content: userMsg}},
		MaxTokens: 1024,
	})
	if err != nil {
		return Result{}, fmt.Errorf("llm judge call failed: %w", err)
	}

	// Track judge usage separately.
	j.Usage.InputTokens += resp.Usage.InputTokens
	j.Usage.OutputTokens += resp.Usage.OutputTokens

	result, err := parseJudgeResponse(resp.Content)
	if err != nil {
		return Result{}, fmt.Errorf("parsing judge response: %w", err)
	}

	return result, nil
}

// GetUsage returns the accumulated token usage from judge calls.
func (j *LLMJudge) GetUsage() provider.Usage {
	return j.Usage
}

func buildJudgePrompt(rubric string, input Input) string {
	var b strings.Builder

	if input.ExpectedOutput != "" {
		b.WriteString("## Expected Output\n")
		b.WriteString(input.ExpectedOutput)
		b.WriteString("\n\n")
	}

	b.WriteString("## Agent Output\n")
	b.WriteString(input.Output)
	b.WriteString("\n\n")

	if len(input.ToolCalls) > 0 {
		b.WriteString("## Tool Calls Made\n")
		for i, tc := range input.ToolCalls {
			params, _ := json.Marshal(tc.Parameters)
			fmt.Fprintf(&b, "%d. %s(%s)\n", i+1, tc.ToolName, string(params))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Rubric\n")
	b.WriteString(rubric)

	return b.String()
}

// judgeOutput is the expected JSON response format from the judge model.
type judgeOutput struct {
	Score     int    `json:"score"`
	Pass      bool   `json:"pass"`
	Reasoning string `json:"reasoning"`
}

func parseJudgeResponse(content string) (Result, error) {
	content = strings.TrimSpace(content)

	// Try structured JSON parse first.
	var out judgeOutput
	if err := json.Unmarshal([]byte(content), &out); err == nil {
		if out.Score >= 1 && out.Score <= 5 {
			return Result{
				Pass:   out.Pass,
				Score:  float64(out.Score) / 5.0,
				Reason: out.Reasoning,
			}, nil
		}
	}

	// Try to extract JSON from within markdown code fences or surrounding text.
	if idx := strings.Index(content, "{"); idx >= 0 {
		if end := strings.LastIndex(content, "}"); end > idx {
			jsonStr := content[idx : end+1]
			if err := json.Unmarshal([]byte(jsonStr), &out); err == nil {
				if out.Score >= 1 && out.Score <= 5 {
					return Result{
						Pass:   out.Pass,
						Score:  float64(out.Score) / 5.0,
						Reason: out.Reasoning,
					}, nil
				}
			}
		}
	}

	// Fallback: extract a score 1-5 from the text.
	if matches := scorePattern.FindStringSubmatch(content); len(matches) > 1 {
		score, _ := strconv.Atoi(matches[1])
		return Result{
			Pass:   score >= 4,
			Score:  float64(score) / 5.0,
			Reason: "score extracted from text (malformed JSON): " + truncate(content, 200),
		}, nil
	}

	return Result{}, fmt.Errorf("could not parse judge response: %s", truncate(content, 200))
}

