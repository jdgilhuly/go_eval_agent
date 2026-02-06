package evaltest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdgilhuly/go_eval_agent/pkg/provider"
)

func TestHarness_SimpleOutput(t *testing.T) {
	fp := NewMockProvider(provider.Response{
		Content:    "Hello, world!",
		StopReason: "end_turn",
		Usage:      provider.Usage{InputTokens: 10, OutputTokens: 5},
	})

	h := New(t, WithProvider(fp), WithSystem("Be helpful."))
	h.Run("greeting", func(tc *TestCase) {
		tc.Input("Say hello")
		tc.AssertOutputContains("Hello")
		tc.AssertOutputMatches(`(?i)hello.*world`)
	})
}

func TestHarness_ToolCallFlow(t *testing.T) {
	fp := NewMockProvider(
		provider.Response{
			Content: "",
			ToolCalls: []provider.ToolCall{
				{ID: "tc1", Name: "read_file", Parameters: map[string]interface{}{"path": "/tmp/test.go"}},
			},
			StopReason: "tool_use",
			Usage:      provider.Usage{InputTokens: 20, OutputTokens: 10},
		},
		provider.Response{
			Content:    "The file contains Go code.",
			StopReason: "end_turn",
			Usage:      provider.Usage{InputTokens: 30, OutputTokens: 15},
		},
	)

	h := New(t, WithProvider(fp), WithSystem("You are a code assistant."))
	h.Run("tool-use", func(tc *TestCase) {
		tc.MockTool("read_file", "package main\n\nfunc main() {}")
		tc.Input("Read the file")
		tc.AssertOutputContains("Go code")
		tc.AssertToolCalled("read_file")
		tc.AssertToolCalledWith("read_file", map[string]interface{}{"path": "/tmp/test.go"})
		tc.AssertToolNotCalled("write_file")
	})
}

func TestHarness_MockToolSequence(t *testing.T) {
	fp := NewMockProvider(
		provider.Response{
			ToolCalls:  []provider.ToolCall{{ID: "tc1", Name: "search"}},
			StopReason: "tool_use",
		},
		provider.Response{
			ToolCalls:  []provider.ToolCall{{ID: "tc2", Name: "search"}},
			StopReason: "tool_use",
		},
		provider.Response{
			Content:    "Found two results",
			StopReason: "end_turn",
		},
	)

	h := New(t, WithProvider(fp))
	h.Run("sequence", func(tc *TestCase) {
		tc.MockTool("search", "result1", "result2")
		tc.Input("Search twice")
		tc.AssertOutputContains("two results")
	})
}

func TestHarness_MockToolError(t *testing.T) {
	fp := NewMockProvider(
		provider.Response{
			ToolCalls:  []provider.ToolCall{{ID: "tc1", Name: "write_file"}},
			StopReason: "tool_use",
		},
		provider.Response{
			Content:    "Write failed due to permissions.",
			StopReason: "end_turn",
		},
	)

	h := New(t, WithProvider(fp))
	h.Run("error-mock", func(tc *TestCase) {
		tc.MockToolError("write_file", "permission denied")
		tc.Input("Try to write")
		tc.AssertOutputContains("failed")
		tc.AssertToolCalled("write_file")
	})
}

func TestHarness_ResultFile(t *testing.T) {
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "results.json")

	fp := NewMockProvider(provider.Response{
		Content:    "done",
		StopReason: "end_turn",
	})

	h := New(t, WithProvider(fp), WithResultFile(resultPath))
	h.Run("result-output", func(tc *TestCase) {
		tc.Input("Do something")
		tc.AssertOutputContains("done")
	})

	// Force cleanup to write results.
	h.writeResults()

	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("failed to read result file: %v", err)
	}
	if len(data) == 0 {
		t.Error("result file is empty")
	}
	if !strings.Contains(string(data), "result-output") {
		t.Error("result file does not contain case name")
	}
}

func TestHarness_OutputMethod(t *testing.T) {
	fp := NewMockProvider(provider.Response{
		Content:    "the answer is 42",
		StopReason: "end_turn",
	})

	h := New(t, WithProvider(fp))
	h.Run("output-access", func(tc *TestCase) {
		tc.Input("What is the answer?")
		out := tc.Output()
		if out != "the answer is 42" {
			t.Errorf("output = %q, want %q", out, "the answer is 42")
		}
	})
}

func TestHarness_InputReturnsOutput(t *testing.T) {
	fp := NewMockProvider(provider.Response{
		Content:    "returned value",
		StopReason: "end_turn",
	})

	h := New(t, WithProvider(fp))
	h.Run("input-return", func(tc *TestCase) {
		got := tc.Input("test")
		if got != "returned value" {
			t.Errorf("Input() returned %q, want %q", got, "returned value")
		}
	})
}

func TestHarness_MultipleSubtests(t *testing.T) {
	fp := NewMockProvider(
		provider.Response{Content: "alpha", StopReason: "end_turn"},
		provider.Response{Content: "beta", StopReason: "end_turn"},
	)

	h := New(t, WithProvider(fp))
	h.Run("first", func(tc *TestCase) {
		tc.Input("Give me alpha")
		tc.AssertOutputContains("alpha")
	})
	h.Run("second", func(tc *TestCase) {
		tc.Input("Give me beta")
		tc.AssertOutputContains("beta")
	})
}

func TestMockProvider(t *testing.T) {
	mp := NewMockProvider(
		provider.Response{Content: "first", StopReason: "end_turn"},
		provider.Response{Content: "second", StopReason: "end_turn"},
	)

	h := New(t, WithProvider(mp))
	h.Run("mock-provider-first", func(tc *TestCase) {
		out := tc.Input("msg1")
		if out != "first" {
			t.Errorf("expected %q, got %q", "first", out)
		}
	})
	h.Run("mock-provider-second", func(tc *TestCase) {
		out := tc.Input("msg2")
		if out != "second" {
			t.Errorf("expected %q, got %q", "second", out)
		}
	})
}

func TestEchoProvider(t *testing.T) {
	h := New(t)
	h.Run("echo", func(tc *TestCase) {
		out := tc.Input("echo this back")
		if out != "echo this back" {
			t.Errorf("expected echo, got %q", out)
		}
	})
}

func TestScoreMatchers(t *testing.T) {
	above := ScoreAbove(0.7)
	if !above.Match(0.8) {
		t.Error("ScoreAbove(0.7) should match 0.8")
	}
	if above.Match(0.5) {
		t.Error("ScoreAbove(0.7) should not match 0.5")
	}

	exact := ScoreExact(1.0)
	if !exact.Match(1.0) {
		t.Error("ScoreExact(1.0) should match 1.0")
	}
	if exact.Match(0.9) {
		t.Error("ScoreExact(1.0) should not match 0.9")
	}

	atLeast := ScoreAtLeast(0.5)
	if !atLeast.Match(0.5) {
		t.Error("ScoreAtLeast(0.5) should match 0.5")
	}
	if !atLeast.Match(0.8) {
		t.Error("ScoreAtLeast(0.5) should match 0.8")
	}
	if atLeast.Match(0.3) {
		t.Error("ScoreAtLeast(0.5) should not match 0.3")
	}
}

func TestAssertToolNotCalled_Negative(t *testing.T) {
	fp := NewMockProvider(
		provider.Response{Content: "no tools used", StopReason: "end_turn"},
	)

	h := New(t, WithProvider(fp))
	h.Run("no-tools", func(tc *TestCase) {
		tc.Input("Just respond")
		tc.AssertToolNotCalled("any_tool")
	})
}
