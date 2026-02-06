package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jdgilhuly/go_eval_agent/pkg/mock"
	"github.com/jdgilhuly/go_eval_agent/pkg/prompt"
	"github.com/jdgilhuly/go_eval_agent/pkg/provider"
	"github.com/jdgilhuly/go_eval_agent/pkg/suite"
	"github.com/jdgilhuly/go_eval_agent/pkg/trace"
)

// MaxToolLoopIterations is the maximum number of tool-call round-trips
// per case before the runner stops to prevent infinite loops.
const MaxToolLoopIterations = 20

// CaseResult holds the output from running a single eval case.
type CaseResult struct {
	CaseName      string              `json:"case_name"`
	CaseID        string              `json:"case_id"`
	Prompt        string              `json:"prompt"`
	Model         string              `json:"model"`
	FinalResponse string              `json:"final_response"`
	Trace         *trace.AgentTrace   `json:"trace"`
	Error         string              `json:"error,omitempty"`
	Duration      time.Duration       `json:"duration"`
}

// RunResult holds the output from an entire suite run.
type RunResult struct {
	SuiteName string       `json:"suite_name"`
	StartTime time.Time    `json:"start_time"`
	EndTime   time.Time    `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Cases     []CaseResult `json:"cases"`
}

// Config controls runner behavior.
type Config struct {
	Concurrency int
	Timeout     time.Duration
}

// Runner orchestrates suite execution against one or more provider/prompt
// combinations with bounded concurrency.
type Runner struct {
	cfg Config
}

// New creates a Runner with the given configuration.
func New(cfg Config) *Runner {
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &Runner{cfg: cfg}
}

// ProgressFunc is called after each case completes. Index is 0-based,
// total is the number of cases.
type ProgressFunc func(index, total int, caseName string, elapsed time.Duration, err error)

// Run executes all cases in the suite using the given prompt variant and
// provider. It respects bounded concurrency and per-case timeouts.
// The optional progress callback is invoked after each case completes.
func (r *Runner) Run(ctx context.Context, s *suite.EvalSuite, pv *prompt.PromptVariant, p provider.Provider, progress ProgressFunc) (*RunResult, error) {
	result := &RunResult{
		SuiteName: s.Name,
		StartTime: time.Now(),
		Cases:     make([]CaseResult, len(s.Cases)),
	}

	sem := make(chan struct{}, r.cfg.Concurrency)
	var mu sync.Mutex
	var completed int

	var wg sync.WaitGroup
	for i, c := range s.Cases {
		wg.Add(1)
		go func(idx int, ec suite.EvalCase) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			cr := r.runCase(ctx, ec, pv, p)
			mu.Lock()
			result.Cases[idx] = cr
			completed++
			current := completed
			mu.Unlock()

			if progress != nil {
				var caseErr error
				if cr.Error != "" {
					caseErr = fmt.Errorf("%s", cr.Error)
				}
				progress(current-1, len(s.Cases), ec.Name, time.Since(result.StartTime), caseErr)
			}
		}(i, c)
	}

	wg.Wait()
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result, nil
}

// runCase executes a single eval case through the full agent loop.
func (r *Runner) runCase(ctx context.Context, c suite.EvalCase, pv *prompt.PromptVariant, p provider.Provider) CaseResult {
	start := time.Now()
	cr := CaseResult{
		CaseName: c.Name,
		CaseID:   c.ID,
		Model:    "",
		Prompt:   pv.Name,
	}

	// Per-case timeout.
	timeout := r.cfg.Timeout
	if c.Timeout > 0 {
		timeout = c.Timeout
	}
	caseCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Set up mocks.
	registry := mock.NewRegistry(c.Mocks)

	// Interpolate prompt with case input variables.
	rendered, err := pv.Interpolate(c.Input)
	if err != nil {
		cr.Error = fmt.Sprintf("interpolating prompt: %v", err)
		cr.Duration = time.Since(start)
		return cr
	}

	// Build tools for the provider request.
	tools := make([]provider.Tool, len(rendered.Tools))
	for i, t := range rendered.Tools {
		tools[i] = provider.Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		}
	}

	// Start trace.
	tr := trace.New()
	cr.Trace = tr

	// Build initial messages.
	messages := []provider.Message{
		{Role: "user", Content: rendered.User},
	}
	tr.AddMessage("user", rendered.User)

	// Agent tool-use loop.
	for iteration := 0; iteration < MaxToolLoopIterations; iteration++ {
		req := &provider.Request{
			System:   rendered.System,
			Messages: messages,
			Tools:    tools,
		}

		resp, err := p.Complete(caseCtx, req)
		if err != nil {
			cr.Error = fmt.Sprintf("provider error: %v", err)
			break
		}

		cr.Model = req.Model
		tr.AddUsage(resp.Usage.InputTokens, resp.Usage.OutputTokens)

		// If no tool calls, we have the final response.
		if len(resp.ToolCalls) == 0 {
			tr.AddMessage("assistant", resp.Content)
			cr.FinalResponse = resp.Content
			break
		}

		// Record assistant message with tool calls.
		tr.AddMessage("assistant", resp.Content)

		// Append the assistant message (with tool calls) to the conversation.
		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Resolve each tool call via mocks.
		for _, tc := range resp.ToolCalls {
			tcStart := time.Now()
			content, mockErr := registry.Resolve(tc.Name, tc.Parameters)
			tcDuration := time.Since(tcStart)

			tcTrace := trace.ToolCallTrace{
				ToolName:   tc.Name,
				Parameters: tc.Parameters,
				Response:   content,
				StartTime:  tcStart,
				EndTime:    time.Now(),
				Duration:   tcDuration,
			}
			if mockErr != nil {
				tcTrace.Error = mockErr.Error()
			}
			tr.AddToolCall(tcTrace)

			// Add the tool result as a message for the next turn.
			toolContent := content
			if mockErr != nil {
				toolContent = fmt.Sprintf("Error: %v", mockErr)
			}
			messages = append(messages, provider.Message{
				Role:       "tool",
				Content:    toolContent,
				ToolCallID: tc.ID,
			})
			tr.AddMessage("tool", toolContent)
		}
	}

	tr.Finish()
	cr.Duration = time.Since(start)
	return cr
}

// JSON serializes the RunResult to indented JSON bytes.
func (r *RunResult) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
