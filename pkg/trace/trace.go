package trace

import (
	"encoding/json"
	"sync"
	"time"
)

// AgentTrace captures the full execution trace of an agent run, including
// all messages, tool calls, token usage, and timing information.
type AgentTrace struct {
	Messages  []Message       `json:"messages"`
	ToolCalls []ToolCallTrace `json:"tool_calls"`
	Usage     TokenUsage      `json:"usage"`
	StartTime time.Time       `json:"start_time"`
	EndTime   time.Time       `json:"end_time"`
	Duration  time.Duration   `json:"duration"`

	mu sync.Mutex
}

// Message records a single message in the conversation.
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ToolCallTrace records a single tool invocation with its parameters,
// response, and timing.
type ToolCallTrace struct {
	ToolName   string                 `json:"tool_name"`
	Parameters map[string]interface{} `json:"parameters"`
	Response   string                 `json:"response"`
	Error      string                 `json:"error,omitempty"`
	StartTime  time.Time              `json:"start_time"`
	EndTime    time.Time              `json:"end_time"`
	Duration   time.Duration          `json:"duration"`
}

// TokenUsage tracks total token consumption across all API calls in a trace.
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// New creates a new AgentTrace and marks the start time.
func New() *AgentTrace {
	return &AgentTrace{
		StartTime: time.Now(),
	}
}

// AddMessage appends a message to the trace.
func (t *AgentTrace) AddMessage(role, content string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Messages = append(t.Messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
}

// AddToolCall appends a tool call record to the trace.
func (t *AgentTrace) AddToolCall(tc ToolCallTrace) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ToolCalls = append(t.ToolCalls, tc)
}

// AddUsage accumulates token usage from a single API call into the trace totals.
func (t *AgentTrace) AddUsage(input, output int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Usage.InputTokens += input
	t.Usage.OutputTokens += output
	t.Usage.TotalTokens += input + output
}

// Finish marks the trace as complete and records the end time and duration.
func (t *AgentTrace) Finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.EndTime = time.Now()
	t.Duration = t.EndTime.Sub(t.StartTime)
}

// GetMessages returns a copy of all recorded messages.
func (t *AgentTrace) GetMessages() []Message {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Message, len(t.Messages))
	copy(out, t.Messages)
	return out
}

// GetToolCalls returns a copy of all recorded tool calls.
func (t *AgentTrace) GetToolCalls() []ToolCallTrace {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]ToolCallTrace, len(t.ToolCalls))
	copy(out, t.ToolCalls)
	return out
}

// GetUsage returns the current token usage totals.
func (t *AgentTrace) GetUsage() TokenUsage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Usage
}

// JSON serializes the trace to indented JSON bytes.
func (t *AgentTrace) JSON() ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return json.MarshalIndent(t, "", "  ")
}
