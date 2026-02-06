package trace

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestNewTrace(t *testing.T) {
	before := time.Now()
	tr := New()
	after := time.Now()

	if tr.StartTime.Before(before) || tr.StartTime.After(after) {
		t.Error("StartTime should be between before and after New()")
	}
	if len(tr.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(tr.Messages))
	}
	if len(tr.ToolCalls) != 0 {
		t.Errorf("expected 0 tool calls, got %d", len(tr.ToolCalls))
	}
}

func TestAddMessage(t *testing.T) {
	tr := New()

	tr.AddMessage("system", "You are a helpful assistant.")
	tr.AddMessage("user", "Hello")
	tr.AddMessage("assistant", "Hi there!")

	msgs := tr.GetMessages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	cases := []struct {
		role    string
		content string
	}{
		{"system", "You are a helpful assistant."},
		{"user", "Hello"},
		{"assistant", "Hi there!"},
	}
	for i, c := range cases {
		if msgs[i].Role != c.role {
			t.Errorf("message %d: role = %q, want %q", i, msgs[i].Role, c.role)
		}
		if msgs[i].Content != c.content {
			t.Errorf("message %d: content = %q, want %q", i, msgs[i].Content, c.content)
		}
		if msgs[i].Timestamp.IsZero() {
			t.Errorf("message %d: timestamp should not be zero", i)
		}
	}
}

func TestAddToolCall(t *testing.T) {
	tr := New()

	start := time.Now()
	tc := ToolCallTrace{
		ToolName:   "read_file",
		Parameters: map[string]interface{}{"path": "/tmp/test.go"},
		Response:   "package main",
		StartTime:  start,
		EndTime:    start.Add(50 * time.Millisecond),
		Duration:   50 * time.Millisecond,
	}
	tr.AddToolCall(tc)

	tcErr := ToolCallTrace{
		ToolName:   "write_file",
		Parameters: map[string]interface{}{"path": "/tmp/out.go"},
		Error:      "permission denied",
		StartTime:  start,
		EndTime:    start.Add(10 * time.Millisecond),
		Duration:   10 * time.Millisecond,
	}
	tr.AddToolCall(tcErr)

	calls := tr.GetToolCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}
	if calls[0].ToolName != "read_file" {
		t.Errorf("call 0: tool_name = %q, want %q", calls[0].ToolName, "read_file")
	}
	if calls[0].Response != "package main" {
		t.Errorf("call 0: response = %q, want %q", calls[0].Response, "package main")
	}
	if calls[1].Error != "permission denied" {
		t.Errorf("call 1: error = %q, want %q", calls[1].Error, "permission denied")
	}
}

func TestAddUsage(t *testing.T) {
	tr := New()

	tr.AddUsage(100, 50)
	tr.AddUsage(200, 80)

	usage := tr.GetUsage()
	if usage.InputTokens != 300 {
		t.Errorf("input_tokens = %d, want 300", usage.InputTokens)
	}
	if usage.OutputTokens != 130 {
		t.Errorf("output_tokens = %d, want 130", usage.OutputTokens)
	}
	if usage.TotalTokens != 430 {
		t.Errorf("total_tokens = %d, want 430", usage.TotalTokens)
	}
}

func TestFinish(t *testing.T) {
	tr := New()
	time.Sleep(10 * time.Millisecond)
	tr.Finish()

	if tr.EndTime.IsZero() {
		t.Error("EndTime should not be zero after Finish()")
	}
	if tr.Duration < 10*time.Millisecond {
		t.Errorf("Duration = %v, want >= 10ms", tr.Duration)
	}
	if !tr.EndTime.After(tr.StartTime) {
		t.Error("EndTime should be after StartTime")
	}
}

func TestJSONSerialization(t *testing.T) {
	tr := New()
	tr.AddMessage("system", "Be helpful.")
	tr.AddMessage("user", "What is Go?")
	tr.AddToolCall(ToolCallTrace{
		ToolName:   "search",
		Parameters: map[string]interface{}{"query": "golang"},
		Response:   "Go is a programming language",
		StartTime:  tr.StartTime,
		EndTime:    tr.StartTime.Add(100 * time.Millisecond),
		Duration:   100 * time.Millisecond,
	})
	tr.AddUsage(50, 25)
	tr.Finish()

	data, err := tr.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	// Verify it round-trips through json.Unmarshal.
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	msgs, ok := decoded["messages"].([]interface{})
	if !ok {
		t.Fatal("messages field missing or wrong type")
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages in JSON, got %d", len(msgs))
	}

	calls, ok := decoded["tool_calls"].([]interface{})
	if !ok {
		t.Fatal("tool_calls field missing or wrong type")
	}
	if len(calls) != 1 {
		t.Errorf("expected 1 tool call in JSON, got %d", len(calls))
	}

	usage, ok := decoded["usage"].(map[string]interface{})
	if !ok {
		t.Fatal("usage field missing or wrong type")
	}
	if usage["total_tokens"].(float64) != 75 {
		t.Errorf("total_tokens = %v, want 75", usage["total_tokens"])
	}
}

func TestGetMessagesCopySafety(t *testing.T) {
	tr := New()
	tr.AddMessage("user", "original")

	msgs := tr.GetMessages()
	msgs[0].Content = "modified"

	// Verify original is unchanged.
	original := tr.GetMessages()
	if original[0].Content != "original" {
		t.Error("GetMessages should return a copy, not a reference to internal data")
	}
}

func TestGetToolCallsCopySafety(t *testing.T) {
	tr := New()
	tr.AddToolCall(ToolCallTrace{ToolName: "original"})

	calls := tr.GetToolCalls()
	calls[0].ToolName = "modified"

	original := tr.GetToolCalls()
	if original[0].ToolName != "original" {
		t.Error("GetToolCalls should return a copy, not a reference to internal data")
	}
}

func TestConcurrentAccess(t *testing.T) {
	tr := New()

	const goroutines = 50
	const opsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines * 3) // message writers, tool call writers, usage writers

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				tr.AddMessage("user", "concurrent message")
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				tr.AddToolCall(ToolCallTrace{ToolName: "concurrent_tool"})
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				tr.AddUsage(10, 5)
			}
		}()
	}

	wg.Wait()
	tr.Finish()

	msgs := tr.GetMessages()
	expectedMsgs := goroutines * opsPerGoroutine
	if len(msgs) != expectedMsgs {
		t.Errorf("expected %d messages, got %d", expectedMsgs, len(msgs))
	}

	calls := tr.GetToolCalls()
	expectedCalls := goroutines * opsPerGoroutine
	if len(calls) != expectedCalls {
		t.Errorf("expected %d tool calls, got %d", expectedCalls, len(calls))
	}

	usage := tr.GetUsage()
	expectedInput := goroutines * opsPerGoroutine * 10
	if usage.InputTokens != expectedInput {
		t.Errorf("input_tokens = %d, want %d", usage.InputTokens, expectedInput)
	}
}
