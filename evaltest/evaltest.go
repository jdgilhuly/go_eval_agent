package evaltest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jdgilhuly/go_eval_agent/pkg/config"
	"github.com/jdgilhuly/go_eval_agent/pkg/mock"
	"github.com/jdgilhuly/go_eval_agent/pkg/provider"
	"github.com/jdgilhuly/go_eval_agent/pkg/trace"
)

// maxToolIterations is the maximum number of tool-call round-trips per case
// to prevent infinite loops.
const maxToolIterations = 20

// Option configures a Harness.
type Option func(*Harness)

// WithProvider sets a custom provider on the harness. If not set, a default
// echo provider is used that returns the user input as output.
func WithProvider(p provider.Provider) Option {
	return func(h *Harness) {
		h.provider = p
	}
}

// WithConfig sets the eval framework config on the harness.
func WithConfig(c *config.Config) Option {
	return func(h *Harness) {
		h.config = c
	}
}

// WithSystem sets the system prompt used for all cases in this harness.
func WithSystem(system string) Option {
	return func(h *Harness) {
		h.system = system
	}
}

// WithTools sets the tools available to the agent for all cases.
func WithTools(tools []provider.Tool) Option {
	return func(h *Harness) {
		h.tools = tools
	}
}

// WithTimeout sets the per-case timeout. Defaults to 30 seconds.
func WithTimeout(d time.Duration) Option {
	return func(h *Harness) {
		h.timeout = d
	}
}

// WithResultFile configures the harness to write test results to a JSON file
// when all cases are complete.
func WithResultFile(path string) Option {
	return func(h *Harness) {
		h.resultFile = path
	}
}

// CaseResult captures the outcome of a single eval test case.
type CaseResult struct {
	Name      string                `json:"name"`
	Output    string                `json:"output"`
	ToolCalls []trace.ToolCallTrace `json:"tool_calls"`
	Duration  time.Duration         `json:"duration"`
	Error     string                `json:"error,omitempty"`
}

// Harness provides the scaffolding for running eval cases as standard Go
// tests. It is tied to a *testing.T and manages shared configuration such
// as the LLM provider.
type Harness struct {
	t          *testing.T
	provider   provider.Provider
	config     *config.Config
	system     string
	tools      []provider.Tool
	timeout    time.Duration
	resultFile string
	results    []CaseResult
}

// New creates a Harness bound to the given *testing.T. Options can be used
// to override the provider, config, and other settings. Sensible defaults
// are applied for anything not configured.
func New(t *testing.T, opts ...Option) *Harness {
	t.Helper()
	h := &Harness{
		t:        t,
		provider: echoProvider{},
		config:   config.Default(),
		timeout:  30 * time.Second,
	}
	for _, opt := range opts {
		opt(h)
	}
	if h.resultFile != "" {
		t.Cleanup(func() {
			h.writeResults()
		})
	}
	return h
}

// Run executes a named eval case as a subtest. The provided function receives
// a *TestCase with helpers for mocking tools, sending input, and making
// assertions.
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
	if err := os.WriteFile(h.resultFile, data, 0o644); err != nil {
		h.t.Errorf("evaltest: failed to write results to %s: %v", h.resultFile, err)
	}
}

// TestCase provides methods to configure and assert a single eval case.
type TestCase struct {
	t         *testing.T
	harness   *Harness
	name      string
	registry  *mock.MockRegistry
	output    string
	trace     *trace.AgentTrace
	toolCalls []provider.ToolCall
	executed  bool
}

// MockTool registers mock responses for a tool. Responses are returned in
// order; the last response is repeated once all sequential responses are
// consumed.
func (tc *TestCase) MockTool(name string, responses ...string) {
	tc.t.Helper()
	mockResponses := make([]mock.MockResponse, len(responses))
	for i, r := range responses {
		mockResponses[i] = mock.MockResponse{Content: r}
	}
	cfg := mock.MockConfig{
		ToolName:  name,
		Responses: mockResponses,
	}
	if len(responses) > 0 {
		last := mock.MockResponse{Content: responses[len(responses)-1]}
		cfg.DefaultResponse = &last
	}
	tc.registry.Register(cfg)
}

// MockToolError registers a mock for a tool that always returns an error.
func (tc *TestCase) MockToolError(name string, errMsg string) {
	tc.t.Helper()
	tc.registry.Register(mock.MockConfig{
		ToolName:        name,
		DefaultResponse: &mock.MockResponse{Error: errMsg},
	})
}

// Input sends the user message to the agent via the configured provider and
// executes the agent loop (processing tool calls via mocks). It returns the
// final agent output text.
func (tc *TestCase) Input(text string) string {
	tc.t.Helper()

	h := tc.harness
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()

	tr := trace.New()
	tc.trace = tr

	messages := []provider.Message{
		{Role: "user", Content: text},
	}
	tr.AddMessage("user", text)

	for i := 0; i < maxToolIterations; i++ {
		req := &provider.Request{
			System:   h.system,
			Messages: messages,
			Tools:    h.tools,
		}

		resp, err := h.provider.Complete(ctx, req)
		if err != nil {
			tc.t.Errorf("provider error: %v", err)
			tr.Finish()
			tc.recordResult(err.Error())
			return ""
		}

		tr.AddUsage(resp.Usage.InputTokens, resp.Usage.OutputTokens)

		if len(resp.ToolCalls) == 0 {
			tr.AddMessage("assistant", resp.Content)
			tc.output = resp.Content
			tc.executed = true
			tr.Finish()
			tc.recordResult("")
			return tc.output
		}

		tc.toolCalls = append(tc.toolCalls, resp.ToolCalls...)
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
	return ""
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

// Trace returns the agent execution trace for inspection.
func (tc *TestCase) Trace() *trace.AgentTrace {
	return tc.trace
}

// ToolCallRecords returns all tool calls made by the provider during
// the agent loop.
func (tc *TestCase) ToolCallRecords() []provider.ToolCall {
	return tc.toolCalls
}
