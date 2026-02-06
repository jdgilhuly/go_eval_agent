package provider

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestAnthropicComplete_TextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers.
		if got := r.Header.Get("X-Api-Key"); got != "test-key" {
			t.Errorf("X-Api-Key = %q, want %q", got, "test-key")
		}
		if got := r.Header.Get("Anthropic-Version"); got != defaultAnthropicVersion {
			t.Errorf("Anthropic-Version = %q, want %q", got, defaultAnthropicVersion)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}

		// Verify request body.
		var reqBody anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if reqBody.Model != "claude-3-haiku-20240307" {
			t.Errorf("model = %q, want %q", reqBody.Model, "claude-3-haiku-20240307")
		}
		if reqBody.System != "You are helpful." {
			t.Errorf("system = %q, want %q", reqBody.System, "You are helpful.")
		}
		if len(reqBody.Messages) != 1 {
			t.Fatalf("messages length = %d, want 1", len(reqBody.Messages))
		}

		resp := anthropicResponse{
			ID:   "msg_01",
			Type: "message",
			Role: "assistant",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Hello! How can I help?"},
			},
			StopReason: "end_turn",
		}
		resp.Usage.InputTokens = 15
		resp.Usage.OutputTokens = 8
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key",
		WithBaseURL(server.URL),
		WithMaxRetries(0),
	)

	got, err := p.Complete(context.Background(), &Request{
		Model:    "claude-3-haiku-20240307",
		System:   "You are helpful.",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if got.Content != "Hello! How can I help?" {
		t.Errorf("Content = %q, want %q", got.Content, "Hello! How can I help?")
	}
	if got.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", got.StopReason, "end_turn")
	}
	if got.Usage.InputTokens != 15 {
		t.Errorf("InputTokens = %d, want %d", got.Usage.InputTokens, 15)
	}
	if got.Usage.OutputTokens != 8 {
		t.Errorf("OutputTokens = %d, want %d", got.Usage.OutputTokens, 8)
	}
	if len(got.ToolCalls) != 0 {
		t.Errorf("ToolCalls length = %d, want 0", len(got.ToolCalls))
	}
}

func TestAnthropicComplete_ToolUseResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify tools were sent in request.
		var reqBody anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if len(reqBody.Tools) != 1 {
			t.Fatalf("tools length = %d, want 1", len(reqBody.Tools))
		}
		if reqBody.Tools[0].Name != "get_weather" {
			t.Errorf("tool name = %q, want %q", reqBody.Tools[0].Name, "get_weather")
		}

		resp := anthropicResponse{
			ID:   "msg_02",
			Type: "message",
			Role: "assistant",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Let me check the weather."},
				{
					Type:  "tool_use",
					ID:    "toolu_01",
					Name:  "get_weather",
					Input: map[string]interface{}{"city": "London"},
				},
			},
			StopReason: "tool_use",
		}
		resp.Usage.InputTokens = 50
		resp.Usage.OutputTokens = 30
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key",
		WithBaseURL(server.URL),
		WithMaxRetries(0),
	)

	got, err := p.Complete(context.Background(), &Request{
		Model:    "claude-3-haiku-20240307",
		Messages: []Message{{Role: "user", Content: "What's the weather in London?"}},
		Tools: []Tool{
			{
				Name:        "get_weather",
				Description: "Get current weather for a city",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"city": map[string]interface{}{
							"type":        "string",
							"description": "City name",
						},
					},
					"required": []interface{}{"city"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if got.Content != "Let me check the weather." {
		t.Errorf("Content = %q, want %q", got.Content, "Let me check the weather.")
	}
	if got.StopReason != "tool_use" {
		t.Errorf("StopReason = %q, want %q", got.StopReason, "tool_use")
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("ToolCalls length = %d, want 1", len(got.ToolCalls))
	}
	tc := got.ToolCalls[0]
	if tc.ID != "toolu_01" {
		t.Errorf("ToolCall.ID = %q, want %q", tc.ID, "toolu_01")
	}
	if tc.Name != "get_weather" {
		t.Errorf("ToolCall.Name = %q, want %q", tc.Name, "get_weather")
	}
	if city, ok := tc.Parameters["city"].(string); !ok || city != "London" {
		t.Errorf("ToolCall.Parameters[city] = %v, want %q", tc.Parameters["city"], "London")
	}
}

func TestAnthropicComplete_RetryOn429(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(anthropicErrorResponse{
				Type: "error",
				Error: struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				}{
					Type:    "rate_limit_error",
					Message: "rate limited",
				},
			})
			return
		}

		resp := anthropicResponse{
			ID:   "msg_03",
			Type: "message",
			Role: "assistant",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Success after retry"},
			},
			StopReason: "end_turn",
		}
		resp.Usage.InputTokens = 10
		resp.Usage.OutputTokens = 5
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key",
		WithBaseURL(server.URL),
		WithMaxRetries(3),
	)

	got, err := p.Complete(context.Background(), &Request{
		Model:    "claude-3-haiku-20240307",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if got.Content != "Success after retry" {
		t.Errorf("Content = %q, want %q", got.Content, "Success after retry")
	}

	if n := attempts.Load(); n != 3 {
		t.Errorf("attempts = %d, want 3", n)
	}
}

func TestAnthropicComplete_RetryOn500(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"type":"error","error":{"type":"server_error","message":"internal error"}}`))
			return
		}

		resp := anthropicResponse{
			ID:   "msg_04",
			Type: "message",
			Role: "assistant",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Recovered"},
			},
			StopReason: "end_turn",
		}
		resp.Usage.InputTokens = 10
		resp.Usage.OutputTokens = 3
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key",
		WithBaseURL(server.URL),
		WithMaxRetries(2),
	)

	got, err := p.Complete(context.Background(), &Request{
		Model:    "claude-3-haiku-20240307",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if got.Content != "Recovered" {
		t.Errorf("Content = %q, want %q", got.Content, "Recovered")
	}

	if n := attempts.Load(); n != 2 {
		t.Errorf("attempts = %d, want 2", n)
	}
}

func TestAnthropicComplete_ExhaustedRetries(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"rate limited"}}`))
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key",
		WithBaseURL(server.URL),
		WithMaxRetries(2),
	)

	_, err := p.Complete(context.Background(), &Request{
		Model:    "claude-3-haiku-20240307",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("Complete() expected error, got nil")
	}

	// Should have attempted 1 initial + 2 retries = 3 total.
	if n := attempts.Load(); n != 3 {
		t.Errorf("attempts = %d, want 3", n)
	}
}

func TestAnthropicComplete_NonRetryableError(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`))
	}))
	defer server.Close()

	p := NewAnthropicProvider("test-key",
		WithBaseURL(server.URL),
		WithMaxRetries(3),
	)

	_, err := p.Complete(context.Background(), &Request{
		Model:    "claude-3-haiku-20240307",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("Complete() expected error, got nil")
	}

	// Non-retryable errors should not be retried.
	if n := attempts.Load(); n != 1 {
		t.Errorf("attempts = %d, want 1 (should not retry 400)", n)
	}
}

func TestEstimateCost(t *testing.T) {
	tests := []struct {
		name  string
		model string
		usage Usage
		want  float64
	}{
		{
			name:  "claude-3-opus",
			model: "claude-3-opus-20240229",
			usage: Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			want:  90.0, // 15 + 75
		},
		{
			name:  "claude-3-sonnet",
			model: "claude-3-sonnet-20240229",
			usage: Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			want:  18.0, // 3 + 15
		},
		{
			name:  "claude-3-haiku",
			model: "claude-3-haiku-20240307",
			usage: Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			want:  1.5, // 0.25 + 1.25
		},
		{
			name:  "claude-3-5-sonnet",
			model: "claude-3-5-sonnet-20241022",
			usage: Usage{InputTokens: 500_000, OutputTokens: 100_000},
			want:  3.0, // (0.5 * 3) + (0.1 * 15)
		},
		{
			name:  "claude-sonnet-4-5",
			model: "claude-sonnet-4-5-20250929",
			usage: Usage{InputTokens: 100_000, OutputTokens: 50_000},
			want:  1.05, // (0.1 * 3) + (0.05 * 15)
		},
		{
			name:  "unknown model",
			model: "unknown-model-xyz",
			usage: Usage{InputTokens: 1000, OutputTokens: 1000},
			want:  0,
		},
		{
			name:  "zero usage",
			model: "claude-3-haiku-20240307",
			usage: Usage{InputTokens: 0, OutputTokens: 0},
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateCost(tt.model, tt.usage)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("EstimateCost(%q, %+v) = %f, want %f", tt.model, tt.usage, got, tt.want)
			}
		})
	}
}

func TestAnthropicProviderName(t *testing.T) {
	p := NewAnthropicProvider("key")
	if got := p.Name(); got != "anthropic" {
		t.Errorf("Name() = %q, want %q", got, "anthropic")
	}
}

func TestConvertMessages_ToolResult(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "What's the weather?"},
		{
			Role:    "assistant",
			Content: "Let me check.",
			ToolCalls: []ToolCall{
				{ID: "tc_01", Name: "weather", Parameters: map[string]interface{}{"city": "NYC"}},
			},
		},
		{Role: "tool", ToolCallID: "tc_01", Content: `{"temp": 72}`},
	}

	got := convertMessages(msgs)

	if len(got) != 3 {
		t.Fatalf("convertMessages length = %d, want 3", len(got))
	}

	// User message: simple string content.
	if got[0].Role != "user" {
		t.Errorf("msg[0].Role = %q, want %q", got[0].Role, "user")
	}

	// Assistant message with tool calls: content blocks.
	if got[1].Role != "assistant" {
		t.Errorf("msg[1].Role = %q, want %q", got[1].Role, "assistant")
	}
	blocks, ok := got[1].Content.([]map[string]interface{})
	if !ok {
		t.Fatalf("msg[1].Content type = %T, want []map[string]interface{}", got[1].Content)
	}
	if len(blocks) != 2 {
		t.Fatalf("msg[1] blocks length = %d, want 2", len(blocks))
	}
	if blocks[0]["type"] != "text" {
		t.Errorf("msg[1] block[0] type = %v, want text", blocks[0]["type"])
	}
	if blocks[1]["type"] != "tool_use" {
		t.Errorf("msg[1] block[1] type = %v, want tool_use", blocks[1]["type"])
	}

	// Tool result message: converted to user with tool_result content.
	if got[2].Role != "user" {
		t.Errorf("msg[2].Role = %q, want %q", got[2].Role, "user")
	}
	resultBlocks, ok := got[2].Content.([]map[string]interface{})
	if !ok {
		t.Fatalf("msg[2].Content type = %T, want []map[string]interface{}", got[2].Content)
	}
	if len(resultBlocks) != 1 {
		t.Fatalf("msg[2] blocks length = %d, want 1", len(resultBlocks))
	}
	if resultBlocks[0]["type"] != "tool_result" {
		t.Errorf("msg[2] block type = %v, want tool_result", resultBlocks[0]["type"])
	}
	if resultBlocks[0]["tool_use_id"] != "tc_01" {
		t.Errorf("msg[2] tool_use_id = %v, want tc_01", resultBlocks[0]["tool_use_id"])
	}
}
