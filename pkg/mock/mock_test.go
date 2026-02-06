package mock

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSequentialResponses(t *testing.T) {
	reg := NewRegistry([]MockConfig{
		{
			ToolName: "read_file",
			Responses: []MockResponse{
				{Content: "first"},
				{Content: "second"},
				{Content: "third"},
			},
		},
	})

	for _, want := range []string{"first", "second", "third"} {
		got, err := reg.Resolve("read_file", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestDefaultFallback(t *testing.T) {
	reg := NewRegistry([]MockConfig{
		{
			ToolName: "search",
			Responses: []MockResponse{
				{Content: "first hit"},
			},
			DefaultResponse: &MockResponse{Content: "default hit"},
		},
	})

	// First call returns sequential response.
	got, err := reg.Resolve("search", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "first hit" {
		t.Errorf("got %q, want %q", got, "first hit")
	}

	// Subsequent calls fall back to default.
	for i := 0; i < 3; i++ {
		got, err = reg.Resolve("search", nil)
		if err != nil {
			t.Fatalf("unexpected error on fallback call %d: %v", i, err)
		}
		if got != "default hit" {
			t.Errorf("fallback call %d: got %q, want %q", i, got, "default hit")
		}
	}
}

func TestNoMockConfigured(t *testing.T) {
	reg := NewRegistry(nil)

	_, err := reg.Resolve("unknown_tool", nil)
	if err == nil {
		t.Fatal("expected error for unconfigured tool, got nil")
	}
	if !strings.Contains(err.Error(), "no mock configured") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSequentialExhaustedNoDefault(t *testing.T) {
	reg := NewRegistry([]MockConfig{
		{
			ToolName: "one_shot",
			Responses: []MockResponse{
				{Content: "only"},
			},
		},
	})

	_, err := reg.Resolve("one_shot", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = reg.Resolve("one_shot", nil)
	if err == nil {
		t.Fatal("expected error when sequential responses exhausted with no default")
	}
	if !strings.Contains(err.Error(), "exhausted") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestErrorSimulation(t *testing.T) {
	reg := NewRegistry([]MockConfig{
		{
			ToolName: "fail_tool",
			Responses: []MockResponse{
				{Error: "permission denied"},
			},
		},
	})

	_, err := reg.Resolve("fail_tool", nil)
	if err == nil {
		t.Fatal("expected error from error simulation, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDelaySimulation(t *testing.T) {
	delay := 50 * time.Millisecond
	reg := NewRegistry([]MockConfig{
		{
			ToolName: "slow_tool",
			Responses: []MockResponse{
				{Content: "done", Delay: delay},
			},
		},
	})

	start := time.Now()
	got, err := reg.Resolve("slow_tool", nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "done" {
		t.Errorf("got %q, want %q", got, "done")
	}
	if elapsed < delay {
		t.Errorf("expected at least %v delay, got %v", delay, elapsed)
	}
}

func TestCallRecording(t *testing.T) {
	reg := NewRegistry([]MockConfig{
		{
			ToolName:        "tool_a",
			DefaultResponse: &MockResponse{Content: "a"},
		},
		{
			ToolName:        "tool_b",
			DefaultResponse: &MockResponse{Content: "b"},
		},
	})

	params := map[string]interface{}{"key": "value"}

	reg.Resolve("tool_a", params)
	reg.Resolve("tool_b", nil)
	reg.Resolve("tool_a", nil)

	all := reg.GetCalls()
	if len(all) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(all))
	}
	if all[0].ToolName != "tool_a" {
		t.Errorf("call 0: got tool %q, want %q", all[0].ToolName, "tool_a")
	}
	if all[0].Parameters["key"] != "value" {
		t.Errorf("call 0: parameters not recorded correctly")
	}
	if all[1].ToolName != "tool_b" {
		t.Errorf("call 1: got tool %q, want %q", all[1].ToolName, "tool_b")
	}

	aCalls := reg.GetCallsForTool("tool_a")
	if len(aCalls) != 2 {
		t.Errorf("expected 2 calls for tool_a, got %d", len(aCalls))
	}

	bCalls := reg.GetCallsForTool("tool_b")
	if len(bCalls) != 1 {
		t.Errorf("expected 1 call for tool_b, got %d", len(bCalls))
	}
}

func TestRegister(t *testing.T) {
	reg := NewRegistry(nil)

	reg.Register(MockConfig{
		ToolName:        "late_add",
		DefaultResponse: &MockResponse{Content: "registered"},
	})

	got, err := reg.Resolve("late_add", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "registered" {
		t.Errorf("got %q, want %q", got, "registered")
	}
}

func TestConcurrentAccess(t *testing.T) {
	reg := NewRegistry([]MockConfig{
		{
			ToolName:        "concurrent",
			DefaultResponse: &MockResponse{Content: "ok"},
		},
	})

	const goroutines = 50
	const callsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				got, err := reg.Resolve("concurrent", map[string]interface{}{"i": j})
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if got != "ok" {
					t.Errorf("got %q, want %q", got, "ok")
					return
				}
			}
		}()
	}

	wg.Wait()

	calls := reg.GetCalls()
	expected := goroutines * callsPerGoroutine
	if len(calls) != expected {
		t.Errorf("expected %d recorded calls, got %d", expected, len(calls))
	}
}
