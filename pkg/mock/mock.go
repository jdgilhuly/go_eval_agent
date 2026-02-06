package mock

import (
	"fmt"
	"sync"
	"time"
)

// MockConfig defines the mock behavior for a single tool.
type MockConfig struct {
	ToolName        string         `yaml:"tool_name" json:"tool_name"`
	Responses       []MockResponse `yaml:"responses" json:"responses"`
	DefaultResponse *MockResponse  `yaml:"default_response" json:"default_response"`
}

// MockResponse defines a single mock response including optional error and delay.
type MockResponse struct {
	Content string        `yaml:"content" json:"content"`
	Error   string        `yaml:"error" json:"error"`
	Delay   time.Duration `yaml:"delay" json:"delay"`
}

// ToolCallRecord captures a single tool invocation for later inspection.
type ToolCallRecord struct {
	ToolName   string                 `json:"tool_name"`
	Parameters map[string]interface{} `json:"parameters"`
	Response   string                 `json:"response"`
	Error      string                 `json:"error,omitempty"`
	Duration   time.Duration          `json:"duration"`
	Timestamp  time.Time              `json:"timestamp"`
}

// MockRegistry manages mock configurations and records tool calls.
// All methods are safe for concurrent use.
type MockRegistry struct {
	mocks   map[string]*MockConfig
	calls   []ToolCallRecord
	mu      sync.Mutex
	callIdx map[string]int // tracks next response index per tool
}

// NewRegistry creates a MockRegistry pre-loaded with the given configs.
func NewRegistry(configs []MockConfig) *MockRegistry {
	r := &MockRegistry{
		mocks:   make(map[string]*MockConfig),
		callIdx: make(map[string]int),
	}
	for i := range configs {
		c := configs[i]
		r.mocks[c.ToolName] = &c
	}
	return r
}

// Register adds or replaces a mock config for the given tool.
func (r *MockRegistry) Register(config MockConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mocks[config.ToolName] = &config
}

// Resolve simulates a tool call. It returns the next sequential response for
// the tool, falling back to the default response when the sequence is
// exhausted. If no mock is configured for the tool, an error is returned to
// prevent accidental real API calls. Errors defined in the MockResponse are
// returned as Go errors. If a delay is configured, Resolve sleeps for that
// duration before returning.
func (r *MockRegistry) Resolve(toolName string, params map[string]interface{}) (string, error) {
	start := time.Now()

	r.mu.Lock()
	cfg, ok := r.mocks[toolName]
	if !ok {
		r.mu.Unlock()
		return "", fmt.Errorf("no mock configured for tool %q", toolName)
	}

	idx := r.callIdx[toolName]
	var resp *MockResponse
	if idx < len(cfg.Responses) {
		resp = &cfg.Responses[idx]
		r.callIdx[toolName] = idx + 1
	} else if cfg.DefaultResponse != nil {
		resp = cfg.DefaultResponse
	} else {
		r.mu.Unlock()
		return "", fmt.Errorf("mock for tool %q: sequential responses exhausted and no default_response configured", toolName)
	}

	// Copy response fields while still holding the lock so we have a
	// consistent snapshot, then release before sleeping.
	content := resp.Content
	errMsg := resp.Error
	delay := resp.Delay
	r.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}

	duration := time.Since(start)

	record := ToolCallRecord{
		ToolName:   toolName,
		Parameters: params,
		Response:   content,
		Error:      errMsg,
		Duration:   duration,
		Timestamp:  start,
	}

	r.mu.Lock()
	r.calls = append(r.calls, record)
	r.mu.Unlock()

	if errMsg != "" {
		return "", fmt.Errorf("mock error for tool %q: %s", toolName, errMsg)
	}

	return content, nil
}

// GetCalls returns a copy of all recorded tool call records.
func (r *MockRegistry) GetCalls() []ToolCallRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ToolCallRecord, len(r.calls))
	copy(out, r.calls)
	return out
}

// GetCallsForTool returns recorded calls filtered to the given tool name.
func (r *MockRegistry) GetCallsForTool(name string) []ToolCallRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []ToolCallRecord
	for _, c := range r.calls {
		if c.ToolName == name {
			out = append(out, c)
		}
	}
	return out
}
