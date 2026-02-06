package provider

import "context"

// Provider defines the interface for LLM API backends.
type Provider interface {
	// Complete sends a completion request and returns the model response.
	Complete(ctx context.Context, req *Request) (*Response, error)

	// Name returns the provider identifier (e.g. "anthropic").
	Name() string
}

// Request represents a completion request to an LLM provider.
type Request struct {
	Model       string    `json:"model"`
	System      string    `json:"system,omitempty"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// Message represents a single message in a conversation.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Tool describes a tool the model can invoke.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolCall represents a tool invocation requested by the model.
type ToolCall struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
}

// Response represents a completion response from an LLM provider.
type Response struct {
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Usage      Usage      `json:"usage"`
	StopReason string     `json:"stop_reason"`
}

// Usage tracks token consumption for a single request.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
