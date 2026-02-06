package runner

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jdgilhuly/go_eval_agent/pkg/mock"
	"github.com/jdgilhuly/go_eval_agent/pkg/prompt"
	"github.com/jdgilhuly/go_eval_agent/pkg/provider"
	"github.com/jdgilhuly/go_eval_agent/pkg/suite"
)

// fakeProvider is a test double that implements provider.Provider.
type fakeProvider struct {
	responses []provider.Response
	callIdx   int
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Complete(_ context.Context, _ *provider.Request) (*provider.Response, error) {
	if f.callIdx >= len(f.responses) {
		return nil, fmt.Errorf("no more responses configured")
	}
	resp := f.responses[f.callIdx]
	f.callIdx++
	return &resp, nil
}

// errorProvider always returns an error.
type errorProvider struct{}

func (e *errorProvider) Name() string { return "error" }
func (e *errorProvider) Complete(_ context.Context, _ *provider.Request) (*provider.Response, error) {
	return nil, fmt.Errorf("provider error: connection refused")
}

func simplePrompt() *prompt.PromptVariant {
	return &prompt.PromptVariant{
		Name:   "test-prompt",
		System: "You are a test assistant.",
		User:   "Question: {{.question}}",
	}
}

func simpleSuite() *suite.EvalSuite {
	return &suite.EvalSuite{
		Name: "test-suite",
		Cases: []suite.EvalCase{
			{
				ID:   "c1",
				Name: "simple-case",
				Input: map[string]interface{}{
					"question": "What is 2+2?",
				},
			},
		},
	}
}

func TestRun_SimpleCase(t *testing.T) {
	fp := &fakeProvider{
		responses: []provider.Response{
			{Content: "4", StopReason: "end_turn", Usage: provider.Usage{InputTokens: 10, OutputTokens: 1}},
		},
	}

	r := New(Config{Concurrency: 1, Timeout: 5 * time.Second})
	result, err := r.Run(context.Background(), simpleSuite(), simplePrompt(), fp, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.SuiteName != "test-suite" {
		t.Errorf("SuiteName = %q, want %q", result.SuiteName, "test-suite")
	}
	if len(result.Cases) != 1 {
		t.Fatalf("len(Cases) = %d, want 1", len(result.Cases))
	}

	cr := result.Cases[0]
	if cr.FinalResponse != "4" {
		t.Errorf("FinalResponse = %q, want %q", cr.FinalResponse, "4")
	}
	if cr.Error != "" {
		t.Errorf("Error = %q, want empty", cr.Error)
	}
	if cr.CaseName != "simple-case" {
		t.Errorf("CaseName = %q, want %q", cr.CaseName, "simple-case")
	}
	if cr.Trace == nil {
		t.Fatal("Trace is nil")
	}
	usage := cr.Trace.GetUsage()
	if usage.InputTokens != 10 || usage.OutputTokens != 1 {
		t.Errorf("Usage = %+v, want input=10, output=1", usage)
	}
}

func TestRun_WithToolCalls(t *testing.T) {
	fp := &fakeProvider{
		responses: []provider.Response{
			{
				Content:    "",
				StopReason: "tool_use",
				ToolCalls: []provider.ToolCall{
					{ID: "tc1", Name: "calculator", Parameters: map[string]interface{}{"expr": "2+2"}},
				},
				Usage: provider.Usage{InputTokens: 20, OutputTokens: 5},
			},
			{
				Content:    "The answer is 4.",
				StopReason: "end_turn",
				Usage:      provider.Usage{InputTokens: 30, OutputTokens: 10},
			},
		},
	}

	s := &suite.EvalSuite{
		Name: "tool-suite",
		Cases: []suite.EvalCase{
			{
				ID:   "tc1",
				Name: "tool-case",
				Input: map[string]interface{}{
					"question": "Calculate 2+2",
				},
				Mocks: []mock.MockConfig{
					{
						ToolName:        "calculator",
						DefaultResponse: &mock.MockResponse{Content: "4"},
					},
				},
			},
		},
	}

	pv := &prompt.PromptVariant{
		Name:   "tool-prompt",
		System: "Use tools when needed.",
		User:   "{{.question}}",
		Tools: []prompt.ToolDefinition{
			{Name: "calculator", Description: "Do math"},
		},
	}

	r := New(Config{Concurrency: 1, Timeout: 5 * time.Second})
	result, err := r.Run(context.Background(), s, pv, fp, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	cr := result.Cases[0]
	if cr.FinalResponse != "The answer is 4." {
		t.Errorf("FinalResponse = %q, want %q", cr.FinalResponse, "The answer is 4.")
	}

	toolCalls := cr.Trace.GetToolCalls()
	if len(toolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(toolCalls))
	}
	if toolCalls[0].ToolName != "calculator" {
		t.Errorf("ToolCalls[0].ToolName = %q, want %q", toolCalls[0].ToolName, "calculator")
	}
	if toolCalls[0].Response != "4" {
		t.Errorf("ToolCalls[0].Response = %q, want %q", toolCalls[0].Response, "4")
	}

	usage := cr.Trace.GetUsage()
	if usage.InputTokens != 50 {
		t.Errorf("InputTokens = %d, want 50", usage.InputTokens)
	}
	if usage.OutputTokens != 15 {
		t.Errorf("OutputTokens = %d, want 15", usage.OutputTokens)
	}
}

func TestRun_ProviderError(t *testing.T) {
	r := New(Config{Concurrency: 1, Timeout: 5 * time.Second})
	result, err := r.Run(context.Background(), simpleSuite(), simplePrompt(), &errorProvider{}, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	cr := result.Cases[0]
	if cr.Error == "" {
		t.Fatal("expected case Error to be set for provider error")
	}
	if cr.FinalResponse != "" {
		t.Errorf("FinalResponse = %q, want empty", cr.FinalResponse)
	}
}

func TestRun_InterpolationError(t *testing.T) {
	pv := &prompt.PromptVariant{
		Name:   "bad-prompt",
		System: "Hello {{.undefined}}",
		User:   "test",
	}

	s := &suite.EvalSuite{
		Name: "interp-suite",
		Cases: []suite.EvalCase{
			{
				Name:  "interp-case",
				Input: map[string]interface{}{},
			},
		},
	}

	fp := &fakeProvider{
		responses: []provider.Response{
			{Content: "should not reach"},
		},
	}

	r := New(Config{Concurrency: 1, Timeout: 5 * time.Second})
	result, err := r.Run(context.Background(), s, pv, fp, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	cr := result.Cases[0]
	if cr.Error == "" {
		t.Fatal("expected interpolation error")
	}
}

func TestRun_BoundedConcurrency(t *testing.T) {
	var maxConcurrent atomic.Int32
	var current atomic.Int32

	// Provider that tracks concurrent calls.
	type concurrentProvider struct {
		fakeProvider
	}
	cp := &concurrentProvider{
		fakeProvider: fakeProvider{},
	}
	// We'll use a custom approach - create a separate provider per case.
	// Instead, let's just verify timing with a slow provider.

	slowProvider := &slowFakeProvider{
		delay: 50 * time.Millisecond,
		response: provider.Response{
			Content: "ok", StopReason: "end_turn",
			Usage: provider.Usage{InputTokens: 1, OutputTokens: 1},
		},
		maxConcurrent: &maxConcurrent,
		current:       &current,
	}
	_ = cp // unused

	s := &suite.EvalSuite{
		Name: "concurrency-suite",
		Cases: []suite.EvalCase{
			{Name: "c1", Input: map[string]interface{}{"question": "a"}},
			{Name: "c2", Input: map[string]interface{}{"question": "b"}},
			{Name: "c3", Input: map[string]interface{}{"question": "c"}},
			{Name: "c4", Input: map[string]interface{}{"question": "d"}},
		},
	}

	r := New(Config{Concurrency: 2, Timeout: 5 * time.Second})
	result, err := r.Run(context.Background(), s, simplePrompt(), slowProvider, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if len(result.Cases) != 4 {
		t.Fatalf("len(Cases) = %d, want 4", len(result.Cases))
	}

	// With concurrency=2 and 50ms delay, max concurrent should be <= 2.
	if maxConcurrent.Load() > 2 {
		t.Errorf("maxConcurrent = %d, want <= 2", maxConcurrent.Load())
	}
}

type slowFakeProvider struct {
	delay         time.Duration
	response      provider.Response
	maxConcurrent *atomic.Int32
	current       *atomic.Int32
}

func (s *slowFakeProvider) Name() string { return "slow-fake" }

func (s *slowFakeProvider) Complete(_ context.Context, _ *provider.Request) (*provider.Response, error) {
	c := s.current.Add(1)
	for {
		old := s.maxConcurrent.Load()
		if c <= old || s.maxConcurrent.CompareAndSwap(old, c) {
			break
		}
	}
	time.Sleep(s.delay)
	s.current.Add(-1)
	return &s.response, nil
}

func TestRun_ProgressCallback(t *testing.T) {
	fp := &fakeProvider{
		responses: []provider.Response{
			{Content: "a", StopReason: "end_turn"},
			{Content: "b", StopReason: "end_turn"},
		},
	}

	s := &suite.EvalSuite{
		Name: "progress-suite",
		Cases: []suite.EvalCase{
			{Name: "p1", Input: map[string]interface{}{"question": "x"}},
			{Name: "p2", Input: map[string]interface{}{"question": "y"}},
		},
	}

	var callCount int
	r := New(Config{Concurrency: 1, Timeout: 5 * time.Second})
	_, err := r.Run(context.Background(), s, simplePrompt(), fp, func(index, total int, name string, elapsed time.Duration, err error) {
		callCount++
		if total != 2 {
			t.Errorf("progress total = %d, want 2", total)
		}
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if callCount != 2 {
		t.Errorf("progress called %d times, want 2", callCount)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	// Provider that checks context.
	ctxProvider := &contextAwareProvider{}

	r := New(Config{Concurrency: 1, Timeout: 5 * time.Second})
	result, err := r.Run(ctx, simpleSuite(), simplePrompt(), ctxProvider, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	cr := result.Cases[0]
	if cr.Error == "" {
		t.Fatal("expected error from cancelled context")
	}
}

type contextAwareProvider struct{}

func (c *contextAwareProvider) Name() string { return "ctx-aware" }
func (c *contextAwareProvider) Complete(ctx context.Context, _ *provider.Request) (*provider.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &provider.Response{Content: "ok"}, nil
}

func TestRunResult_JSON(t *testing.T) {
	result := &RunResult{
		SuiteName: "json-test",
		StartTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2025, 1, 1, 0, 0, 1, 0, time.UTC),
		Duration:  time.Second,
		Cases: []CaseResult{
			{CaseName: "c1", FinalResponse: "hello"},
		},
	}

	data, err := result.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("JSON() returned empty")
	}
}
