package evaltest

import (
	"context"
	"fmt"
	"sync"

	"github.com/jdgilhuly/go_eval_agent/pkg/provider"
)

// MockProvider is a simple provider that returns pre-configured responses
// in sequence. It is safe for concurrent use.
type MockProvider struct {
	responses []provider.Response
	mu        sync.Mutex
	idx       int
}

// NewMockProvider creates a MockProvider that returns the given responses in
// order. Once all responses are consumed, subsequent calls return an error.
func NewMockProvider(responses ...provider.Response) *MockProvider {
	return &MockProvider{
		responses: responses,
	}
}

// Complete returns the next pre-configured response. It ignores the request
// contents entirely; responses are returned in the order they were provided.
func (m *MockProvider) Complete(_ context.Context, _ *provider.Request) (*provider.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.idx >= len(m.responses) {
		return nil, fmt.Errorf("mock provider: no more responses (consumed %d/%d)", m.idx, len(m.responses))
	}

	resp := m.responses[m.idx]
	m.idx++
	return &resp, nil
}

// Name returns "mock".
func (m *MockProvider) Name() string { return "mock" }

// echoProvider is a trivial provider that echoes the last user message.
type echoProvider struct{}

func (echoProvider) Complete(_ context.Context, req *provider.Request) (*provider.Response, error) {
	content := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			content = req.Messages[i].Content
			break
		}
	}
	return &provider.Response{
		Content:    content,
		StopReason: "end_turn",
	}, nil
}

func (echoProvider) Name() string { return "echo" }
