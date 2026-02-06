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

func TestOpenAIComplete_TextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers.
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}

		// Verify request body structure.
		var reqBody openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if reqBody.Model != "gpt-4o" {
			t.Errorf("model = %q, want %q", reqBody.Model, "gpt-4o")
		}
		// System prompt should be the first message.
		if len(reqBody.Messages) < 2 {
			t.Fatalf("messages length = %d, want >= 2", len(reqBody.Messages))
		}
		if reqBody.Messages[0].Role != "system" {
			t.Errorf("messages[0].role = %q, want %q", reqBody.Messages[0].Role, "system")
		}
		if reqBody.Messages[0].Content == nil || *reqBody.Messages[0].Content != "You are helpful." {
			t.Errorf("messages[0].content = %v, want %q", reqBody.Messages[0].Content, "You are helpful.")
		}

		resp := openaiResponse{
			ID:     "chatcmpl-01",
			Object: "chat.completion",
			Choices: []openaiChoice{
				{
					Index: 0,
					Message: openaiMessage{
						Role:    "assistant",
						Content: strPtr("Hello! How can I help?"),
					},
					FinishReason: "stop",
				},
			},
		}
		resp.Usage.PromptTokens = 15
		resp.Usage.CompletionTokens = 8
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key",
		WithOpenAIBaseURL(server.URL),
		WithOpenAIMaxRetries(0),
	)

	got, err := p.Complete(context.Background(), &Request{
		Model:    "gpt-4o",
		System:   "You are helpful.",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if got.Content != "Hello! How can I help?" {
		t.Errorf("Content = %q, want %q", got.Content, "Hello! How can I help?")
	}
	if got.StopReason != "stop" {
		t.Errorf("StopReason = %q, want %q", got.StopReason, "stop")
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

func TestOpenAIComplete_ToolUseResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}

		// Verify tools sent in OpenAI function calling format.
		if len(reqBody.Tools) != 1 {
			t.Fatalf("tools length = %d, want 1", len(reqBody.Tools))
		}
		if reqBody.Tools[0].Type != "function" {
			t.Errorf("tool type = %q, want %q", reqBody.Tools[0].Type, "function")
		}
		if reqBody.Tools[0].Function.Name != "get_weather" {
			t.Errorf("tool name = %q, want %q", reqBody.Tools[0].Function.Name, "get_weather")
		}

		resp := openaiResponse{
			ID:     "chatcmpl-02",
			Object: "chat.completion",
			Choices: []openaiChoice{
				{
					Index: 0,
					Message: openaiMessage{
						Role:    "assistant",
						Content: strPtr("Let me check the weather."),
						ToolCalls: []openaiToolCall{
							{
								ID:   "call_01",
								Type: "function",
								Function: openaiCallFunction{
									Name:      "get_weather",
									Arguments: `{"city":"London"}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
		}
		resp.Usage.PromptTokens = 50
		resp.Usage.CompletionTokens = 30
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key",
		WithOpenAIBaseURL(server.URL),
		WithOpenAIMaxRetries(0),
	)

	got, err := p.Complete(context.Background(), &Request{
		Model:    "gpt-4o",
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
	if got.StopReason != "tool_calls" {
		t.Errorf("StopReason = %q, want %q", got.StopReason, "tool_calls")
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("ToolCalls length = %d, want 1", len(got.ToolCalls))
	}
	tc := got.ToolCalls[0]
	if tc.ID != "call_01" {
		t.Errorf("ToolCall.ID = %q, want %q", tc.ID, "call_01")
	}
	if tc.Name != "get_weather" {
		t.Errorf("ToolCall.Name = %q, want %q", tc.Name, "get_weather")
	}
	if city, ok := tc.Parameters["city"].(string); !ok || city != "London" {
		t.Errorf("ToolCall.Parameters[city] = %v, want %q", tc.Parameters["city"], "London")
	}
}

func TestOpenAIComplete_RetryOn429(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(openaiErrorResponse{
				Error: struct {
					Message string `json:"message"`
					Type    string `json:"type"`
					Code    string `json:"code"`
				}{
					Message: "Rate limit exceeded",
					Type:    "rate_limit_error",
				},
			})
			return
		}

		resp := openaiResponse{
			ID:     "chatcmpl-03",
			Object: "chat.completion",
			Choices: []openaiChoice{
				{
					Index: 0,
					Message: openaiMessage{
						Role:    "assistant",
						Content: strPtr("Success after retry"),
					},
					FinishReason: "stop",
				},
			},
		}
		resp.Usage.PromptTokens = 10
		resp.Usage.CompletionTokens = 5
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key",
		WithOpenAIBaseURL(server.URL),
		WithOpenAIMaxRetries(3),
	)

	got, err := p.Complete(context.Background(), &Request{
		Model:    "gpt-4o",
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

func TestOpenAIComplete_RetryOn500(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":{"message":"internal error","type":"server_error"}}`))
			return
		}

		resp := openaiResponse{
			ID:     "chatcmpl-04",
			Object: "chat.completion",
			Choices: []openaiChoice{
				{
					Index: 0,
					Message: openaiMessage{
						Role:    "assistant",
						Content: strPtr("Recovered"),
					},
					FinishReason: "stop",
				},
			},
		}
		resp.Usage.PromptTokens = 10
		resp.Usage.CompletionTokens = 3
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key",
		WithOpenAIBaseURL(server.URL),
		WithOpenAIMaxRetries(2),
	)

	got, err := p.Complete(context.Background(), &Request{
		Model:    "gpt-4o",
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

func TestOpenAIComplete_NonRetryableError(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"invalid model","type":"invalid_request_error"}}`))
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key",
		WithOpenAIBaseURL(server.URL),
		WithOpenAIMaxRetries(3),
	)

	_, err := p.Complete(context.Background(), &Request{
		Model:    "invalid-model",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("Complete() expected error, got nil")
	}
	if n := attempts.Load(); n != 1 {
		t.Errorf("attempts = %d, want 1 (should not retry 400)", n)
	}
}

func TestOpenAIComplete_ExhaustedRetries(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key",
		WithOpenAIBaseURL(server.URL),
		WithOpenAIMaxRetries(2),
	)

	_, err := p.Complete(context.Background(), &Request{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("Complete() expected error, got nil")
	}
	// 1 initial + 2 retries = 3 total.
	if n := attempts.Load(); n != 3 {
		t.Errorf("attempts = %d, want 3", n)
	}
}

func TestOpenAICostEstimation(t *testing.T) {
	tests := []struct {
		name  string
		model string
		usage Usage
		want  float64
	}{
		{
			name:  "gpt-4o",
			model: "gpt-4o",
			usage: Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			want:  12.50, // 2.50 + 10.0
		},
		{
			name:  "gpt-4o-mini",
			model: "gpt-4o-mini",
			usage: Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			want:  0.75, // 0.15 + 0.60
		},
		{
			name:  "gpt-4-turbo",
			model: "gpt-4-turbo",
			usage: Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			want:  40.0, // 10 + 30
		},
		{
			name:  "gpt-4",
			model: "gpt-4",
			usage: Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			want:  90.0, // 30 + 60
		},
		{
			name:  "gpt-4o partial usage",
			model: "gpt-4o",
			usage: Usage{InputTokens: 500_000, OutputTokens: 100_000},
			want:  2.25, // (0.5 * 2.50) + (0.1 * 10.0)
		},
		{
			name:  "o3-mini",
			model: "o3-mini",
			usage: Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
			want:  5.50, // 1.10 + 4.40
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

func TestOpenAIProviderName(t *testing.T) {
	p := NewOpenAIProvider("key")
	if got := p.Name(); got != "openai" {
		t.Errorf("Name() = %q, want %q", got, "openai")
	}
}

func TestConvertToOpenAIMessages(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "What's the weather?"},
		{
			Role:    "assistant",
			Content: "Let me check.",
			ToolCalls: []ToolCall{
				{ID: "call_01", Name: "weather", Parameters: map[string]interface{}{"city": "NYC"}},
			},
		},
		{Role: "tool", ToolCallID: "call_01", Content: `{"temp": 72}`},
	}

	got := convertToOpenAIMessages("Be helpful.", msgs)

	// System + 3 messages = 4 total.
	if len(got) != 4 {
		t.Fatalf("convertToOpenAIMessages length = %d, want 4", len(got))
	}

	// System message first.
	if got[0].Role != "system" {
		t.Errorf("msg[0].Role = %q, want %q", got[0].Role, "system")
	}
	if got[0].Content == nil || *got[0].Content != "Be helpful." {
		t.Errorf("msg[0].Content = %v, want %q", got[0].Content, "Be helpful.")
	}

	// User message.
	if got[1].Role != "user" {
		t.Errorf("msg[1].Role = %q, want %q", got[1].Role, "user")
	}

	// Assistant with tool calls.
	if got[2].Role != "assistant" {
		t.Errorf("msg[2].Role = %q, want %q", got[2].Role, "assistant")
	}
	if len(got[2].ToolCalls) != 1 {
		t.Fatalf("msg[2] tool_calls length = %d, want 1", len(got[2].ToolCalls))
	}
	if got[2].ToolCalls[0].Type != "function" {
		t.Errorf("msg[2] tool_call type = %q, want %q", got[2].ToolCalls[0].Type, "function")
	}
	if got[2].ToolCalls[0].Function.Name != "weather" {
		t.Errorf("msg[2] tool_call function name = %q, want %q", got[2].ToolCalls[0].Function.Name, "weather")
	}

	// Tool result.
	if got[3].Role != "tool" {
		t.Errorf("msg[3].Role = %q, want %q", got[3].Role, "tool")
	}
	if got[3].ToolCallID != "call_01" {
		t.Errorf("msg[3].ToolCallID = %q, want %q", got[3].ToolCallID, "call_01")
	}
}

func strPtr(s string) *string { return &s }
