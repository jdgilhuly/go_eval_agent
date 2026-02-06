package judge

import (
	"github.com/jdgilhuly/go_eval_agent/pkg/trace"
)

// Result captures the outcome of a judge evaluation.
type Result struct {
	Pass   bool    `json:"pass"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

// Input provides all the data a judge needs to evaluate an agent run.
type Input struct {
	Output         string                   `json:"output"`
	ExpectedOutput string                   `json:"expected_output,omitempty"`
	ToolCalls      []trace.ToolCallTrace    `json:"tool_calls,omitempty"`
}

// Judge defines the interface for evaluating agent outputs.
type Judge interface {
	// Evaluate scores the agent's output and returns a result.
	Evaluate(input Input) (Result, error)

	// Name returns the judge type identifier.
	Name() string
}
