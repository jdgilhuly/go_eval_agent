// codegen_test.go - Example programmatic eval tests using the evaltest package.
//
// These tests demonstrate how to write eval cases as standard Go tests.
// Run with: go test ./examples/ -v
//
// Each test uses a mock provider to simulate LLM responses, making tests
// deterministic and runnable without API keys.
package examples

import (
	"testing"

	"github.com/jdgilhuly/go_eval_agent/evaltest"
	"github.com/jdgilhuly/go_eval_agent/pkg/provider"
)

// TestHelloFunction verifies the agent generates a correct Hello function.
// Uses AssertOutputContains for simple substring matching.
func TestHelloFunction(t *testing.T) {
	mock := evaltest.NewMockProvider(
		provider.Response{
			Content:    "Here is the Hello function:\n\nfunc Hello(name string) string {\n\treturn \"Hello, \" + name + \"!\"\n}",
			StopReason: "end_turn",
			Usage:      provider.Usage{InputTokens: 50, OutputTokens: 30},
		},
		// Second response for the second subtest.
		provider.Response{
			Content:    "Here is the Hello function:\n\nfunc Hello(name string) string {\n\treturn \"Hello, \" + name + \"!\"\n}",
			StopReason: "end_turn",
			Usage:      provider.Usage{InputTokens: 50, OutputTokens: 30},
		},
	)

	h := evaltest.New(t,
		evaltest.WithProvider(mock),
		evaltest.WithSystem("You are a Go developer."),
	)

	h.Run("hello-signature", func(tc *evaltest.TestCase) {
		tc.Input("Write a Go function called Hello that takes a name string and returns a greeting string.")
		tc.AssertOutputContains("func Hello(name string) string")
	})

	h.Run("hello-return", func(tc *evaltest.TestCase) {
		tc.Input("Write a Go function called Hello that takes a name string and returns a greeting string.")
		tc.AssertOutputMatches(`(?s)return.*Hello`)
	})
}

// TestToolCallWorkflow verifies the agent reads a file before writing.
// Uses MockTool for tool mocking and AssertToolCalled/AssertToolNotCalled
// for tool call assertions.
func TestToolCallWorkflow(t *testing.T) {
	mock := evaltest.NewMockProvider(
		// First turn: agent calls read_file
		provider.Response{
			ToolCalls: []provider.ToolCall{
				{
					ID:         "call_1",
					Name:       "read_file",
					Parameters: map[string]interface{}{"path": "/app/handler.go"},
				},
			},
			StopReason: "tool_use",
			Usage:      provider.Usage{InputTokens: 80, OutputTokens: 20},
		},
		// Second turn: agent calls write_file with updated code
		provider.Response{
			ToolCalls: []provider.ToolCall{
				{
					ID:   "call_2",
					Name: "write_file",
					Parameters: map[string]interface{}{
						"path":    "/app/handler.go",
						"content": "package app\n\nfunc CreateUser() { /* validated */ }\n",
					},
				},
			},
			StopReason: "tool_use",
			Usage:      provider.Usage{InputTokens: 120, OutputTokens: 60},
		},
		// Third turn: agent provides summary
		provider.Response{
			Content:    "I've added input validation to the CreateUser handler. It now returns 400 if the JSON body is malformed.",
			StopReason: "end_turn",
			Usage:      provider.Usage{InputTokens: 40, OutputTokens: 25},
		},
	)

	h := evaltest.New(t,
		evaltest.WithProvider(mock),
		evaltest.WithSystem("You are a Go developer with file read/write tools."),
	)

	h.Run("read-before-write", func(tc *evaltest.TestCase) {
		// MockTool with variadic args: each arg is a sequential response,
		// with the last one repeated as the default.
		tc.MockTool("read_file", "package app\n\nfunc CreateUser() {}")
		tc.MockTool("write_file", "file written successfully")

		tc.Input("Read /app/handler.go and add input validation to the CreateUser handler.")

		// Verify tool call ordering.
		tc.AssertToolCalled("read_file")
		tc.AssertToolCalled("write_file")
		tc.AssertToolNotCalled("delete_file")

		// Verify parameter matching.
		tc.AssertToolCalledWith("read_file", map[string]interface{}{
			"path": "/app/handler.go",
		})

		// Verify output content.
		tc.AssertOutputContains("validation")
	})
}

// TestSequentialMocks demonstrates using MockTool with multiple responses
// for tools that are called multiple times.
func TestSequentialMocks(t *testing.T) {
	mock := evaltest.NewMockProvider(
		// Agent searches twice
		provider.Response{
			ToolCalls:  []provider.ToolCall{{ID: "s1", Name: "search", Parameters: map[string]interface{}{"query": "user model"}}},
			StopReason: "tool_use",
		},
		provider.Response{
			ToolCalls:  []provider.ToolCall{{ID: "s2", Name: "search", Parameters: map[string]interface{}{"query": "user handler"}}},
			StopReason: "tool_use",
		},
		// Agent provides final response
		provider.Response{
			Content:    "Based on the search results, the User model is defined in models/user.go and the handler is in handlers/user.go.",
			StopReason: "end_turn",
		},
	)

	h := evaltest.New(t, evaltest.WithProvider(mock))
	h.Run("multi-search", func(tc *evaltest.TestCase) {
		// Each search call returns a different result. The last response
		// becomes the default for any additional calls.
		tc.MockTool("search",
			"models/user.go: type User struct { Name string; Email string }",
			"handlers/user.go: func GetUser(w http.ResponseWriter, r *http.Request) {}",
		)

		tc.Input("Find the User model and its handler.")
		tc.AssertOutputContains("models/user.go")
		tc.AssertOutputContains("handlers/user.go")
	})
}

// TestErrorSimulation demonstrates testing agent behavior when tools fail.
func TestErrorSimulation(t *testing.T) {
	mock := evaltest.NewMockProvider(
		// Agent tries to write
		provider.Response{
			ToolCalls:  []provider.ToolCall{{ID: "w1", Name: "write_file", Parameters: map[string]interface{}{"path": "/etc/config"}}},
			StopReason: "tool_use",
		},
		// Agent handles the error gracefully
		provider.Response{
			Content:    "I was unable to write to /etc/config due to a permission error. Try using a path in your home directory instead.",
			StopReason: "end_turn",
		},
	)

	h := evaltest.New(t, evaltest.WithProvider(mock))
	h.Run("graceful-error-handling", func(tc *evaltest.TestCase) {
		tc.MockToolError("write_file", "permission denied: /etc/config")

		tc.Input("Write the config to /etc/config")
		tc.AssertOutputContains("permission")
		tc.AssertToolCalled("write_file")
	})
}

// TestResultOutput demonstrates writing results to a JSON file.
// Useful for later analysis with 'eval diff' or custom tooling.
func TestResultOutput(t *testing.T) {
	mock := evaltest.NewMockProvider(
		provider.Response{
			Content:    "func Add(a, b int) int { return a + b }",
			StopReason: "end_turn",
		},
	)

	// Results are written to a file on test cleanup.
	h := evaltest.New(t,
		evaltest.WithProvider(mock),
		evaltest.WithResultFile(t.TempDir()+"/eval_results.json"),
	)

	h.Run("add-function", func(tc *evaltest.TestCase) {
		tc.Input("Write an Add function")
		tc.AssertOutputContains("func Add")
		tc.AssertOutputMatches(`return a \+ b`)
	})
}

// TestScoreMatchers demonstrates the score matcher API for LLM judge
// integration. These run without an actual LLM call.
func TestScoreMatchers(t *testing.T) {
	// ScoreAbove(0.7) passes for scores strictly > 0.7
	above := evaltest.ScoreAbove(0.7)
	if !above.Match(0.9) {
		t.Error("ScoreAbove(0.7) should match 0.9")
	}
	if above.Match(0.7) {
		t.Error("ScoreAbove(0.7) should not match 0.7 (strict)")
	}

	// ScoreExact(1.0) passes for scores equal to 1.0
	exact := evaltest.ScoreExact(1.0)
	if !exact.Match(1.0) {
		t.Error("ScoreExact(1.0) should match 1.0")
	}

	// ScoreAtLeast(0.5) passes for scores >= 0.5
	atLeast := evaltest.ScoreAtLeast(0.5)
	if !atLeast.Match(0.5) {
		t.Error("ScoreAtLeast(0.5) should match 0.5")
	}
	if atLeast.Match(0.4) {
		t.Error("ScoreAtLeast(0.5) should not match 0.4")
	}
}
