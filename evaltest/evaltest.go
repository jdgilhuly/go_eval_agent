package evaltest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/jdgilhuly/go_eval_agent/pkg/mock"
	"github.com/jdgilhuly/go_eval_agent/pkg/provider"
	"github.com/jdgilhuly/go_eval_agent/pkg/trace"
)

// Config configures the eval test harness.
type Config struct {
	Provider   provider.Provider
	System     string
	Tools      []provider.Tool
	Timeout    time.Duration
	ResultFile string // optional: write results to this JSON file
}

// Harness ties eval cases to a *testing.T for standard go test integration.
type Harness struct {
	t       *testing.T
	cfg     Config
	results []CaseResult
}

// CaseResult captures the outcome of a single eval test case.
type CaseResult struct {
	Name          string              `json:"name"`
	Output        string              `json:"output"`
	ToolCalls     []trace.ToolCallTrace `json:"tool_calls"`
	Duration      time.Duration       `json:"duration"`
	Error         string              `json:"error,omitempty"`
}

// New creates a Harness tied to the given testing.T.
func New(t *testing.T, cfg Config) *Harness {
	t.Helper()
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	h := &Harness{t: t, cfg: cfg}
	if cfg.ResultFile != "" {
		t.Cleanup(func() {
			h.writeResults()
		})
	}
	return h
}

// Run runs a named eval case as a subtest.
func (h *Harness) Run(name string, fn func(tc *TestCase)) {
	h.t.Helper()
	h.t.Run(name, func(t *testing.T) {
		t.Helper()
		tc := &TestCase{
			t:        t,
			harness:  h,
			name:     name,
			registry: mock.NewRegistry(nil),
		}
		fn(tc)
	})
}

// writeResults saves all recorded results to the configured JSON file.
func (h *Harness) writeResults() {
	data, err := json.MarshalIndent(h.results, "", "  ")
	if err != nil {
		h.t.Errorf("evaltest: failed to marshal results: %v", err)
		return
	}
	if err := os.WriteFile(h.cfg.ResultFile, data, 0o644); err != nil {
		h.t.Errorf("evaltest: failed to write results to %s: %v", h.cfg.ResultFile, err)
	}
}

// TestCase provides methods to configure and assert a single eval case.
type TestCase struct {
	t        *testing.T
	harness  *Harness
	name     string
	registry *mock.MockRegistry
	output   string
	trace    *trace.AgentTrace
	executed bool
}

// MockTool registers a mock tool that returns the given response.
func (tc *TestCase) MockTool(name, response string) {
	tc.t.Helper()
	tc.registry.Register(mock.MockConfig{
		ToolName:        name,
		DefaultResponse: &mock.MockResponse{Content: response},
	})
}

// MockToolSequence registers a mock tool with sequential responses.
func (tc *TestCase) MockToolSequence(name string, responses []string) {
	tc.t.Helper()
	mrs := make([]mock.MockResponse, len(responses))
	for i, r := range responses {
		mrs[i] = mock.MockResponse{Content: r}
	}
	tc.registry.Register(mock.MockConfig{
		ToolName:  name,
		Responses: mrs,
	})
}

// MockToolError registers a mock tool that returns an error.
func (tc *TestCase) MockToolError(name, errMsg string) {
	tc.t.Helper()
	tc.registry.Register(mock.MockConfig{
		ToolName:        name,
		DefaultResponse: &mock.MockResponse{Error: errMsg},
	})
}

// Input sets the user message and executes the agent loop against the
// configured provider and mocks.
func (tc *TestCase) Input(text string) {
	tc.t.Helper()

	cfg := tc.harness.cfg
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	tr := trace.New()
	tc.trace = tr

	messages := []provider.Message{
		{Role: "user", Content: text},
	}
	tr.AddMessage("user", text)

	const maxIterations = 20
	for i := 0; i < maxIterations; i++ {
		req := &provider.Request{
			System:   cfg.System,
			Messages: messages,
			Tools:    cfg.Tools,
		}

		resp, err := cfg.Provider.Complete(ctx, req)
		if err != nil {
			tc.t.Errorf("provider error: %v", err)
			tr.Finish()
			tc.recordResult(err.Error())
			return
		}

		tr.AddUsage(resp.Usage.InputTokens, resp.Usage.OutputTokens)

		if len(resp.ToolCalls) == 0 {
			tr.AddMessage("assistant", resp.Content)
			tc.output = resp.Content
			tc.executed = true
			tr.Finish()
			tc.recordResult("")
			return
		}

		tr.AddMessage("assistant", resp.Content)
		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, call := range resp.ToolCalls {
			tcStart := time.Now()
			content, mockErr := tc.registry.Resolve(call.Name, call.Parameters)
			tcDuration := time.Since(tcStart)

			tcTrace := trace.ToolCallTrace{
				ToolName:   call.Name,
				Parameters: call.Parameters,
				Response:   content,
				StartTime:  tcStart,
				EndTime:    time.Now(),
				Duration:   tcDuration,
			}
			if mockErr != nil {
				tcTrace.Error = mockErr.Error()
			}
			tr.AddToolCall(tcTrace)

			toolContent := content
			if mockErr != nil {
				toolContent = fmt.Sprintf("Error: %v", mockErr)
			}
			messages = append(messages, provider.Message{
				Role:       "tool",
				Content:    toolContent,
				ToolCallID: call.ID,
			})
			tr.AddMessage("tool", toolContent)
		}
	}

	tc.t.Error("agent loop exceeded maximum iterations")
	tr.Finish()
	tc.recordResult("max iterations exceeded")
}

func (tc *TestCase) recordResult(errMsg string) {
	var toolCalls []trace.ToolCallTrace
	if tc.trace != nil {
		toolCalls = tc.trace.GetToolCalls()
	}
	result := CaseResult{
		Name:      tc.name,
		Output:    tc.output,
		ToolCalls: toolCalls,
		Error:     errMsg,
	}
	if tc.trace != nil {
		result.Duration = tc.trace.Duration
	}
	tc.harness.results = append(tc.harness.results, result)
}

// Output returns the agent's final output text.
func (tc *TestCase) Output() string {
	tc.t.Helper()
	if !tc.executed {
		tc.t.Error("Output() called before Input()")
	}
	return tc.output
}

// --- Assertion helpers ---

// AssertOutputContains asserts that the output contains the given substring.
func (tc *TestCase) AssertOutputContains(substr string) {
	tc.t.Helper()
	if !tc.executed {
		tc.t.Error("AssertOutputContains called before Input()")
		return
	}
	if !contains(tc.output, substr) {
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

// --- helpers ---

func contains(s, substr string) bool {
	return len(substr) == 0 || len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
