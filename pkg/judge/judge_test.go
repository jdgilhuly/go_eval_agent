package judge

import (
	"testing"

	"github.com/jdgilhuly/go_eval_agent/pkg/trace"
)

// --- Exact Judge ---

func TestExactJudge_Pass(t *testing.T) {
	j := &ExactJudge{}
	r, err := j.Evaluate(Input{Output: "hello world", ExpectedOutput: "hello world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Pass || r.Score != 1.0 {
		t.Errorf("expected pass with score 1.0, got pass=%v score=%v", r.Pass, r.Score)
	}
}

func TestExactJudge_Fail(t *testing.T) {
	j := &ExactJudge{}
	r, err := j.Evaluate(Input{Output: "hello", ExpectedOutput: "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Pass || r.Score != 0.0 {
		t.Errorf("expected fail with score 0.0, got pass=%v score=%v", r.Pass, r.Score)
	}
}

func TestExactJudge_WhitespaceNormalization(t *testing.T) {
	j := &ExactJudge{NormalizeWhitespace: true}
	r, err := j.Evaluate(Input{
		Output:         "  hello   world  \n",
		ExpectedOutput: "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Pass {
		t.Errorf("expected pass with whitespace normalization, got fail: %s", r.Reason)
	}
}

func TestExactJudge_WhitespaceMatters(t *testing.T) {
	j := &ExactJudge{NormalizeWhitespace: false}
	r, err := j.Evaluate(Input{
		Output:         "hello  world",
		ExpectedOutput: "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Pass {
		t.Error("expected fail without whitespace normalization")
	}
}

func TestExactJudge_Name(t *testing.T) {
	j := &ExactJudge{}
	if j.Name() != "exact" {
		t.Errorf("name = %q, want %q", j.Name(), "exact")
	}
}

// --- Regex Judge ---

func TestRegexJudge_Pass(t *testing.T) {
	j := &RegexJudge{Pattern: `\d{3}-\d{4}`}
	r, err := j.Evaluate(Input{Output: "Call 555-1234 for info"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Pass {
		t.Errorf("expected pass, got fail: %s", r.Reason)
	}
}

func TestRegexJudge_Fail(t *testing.T) {
	j := &RegexJudge{Pattern: `\d{3}-\d{4}`}
	r, err := j.Evaluate(Input{Output: "no phone number here"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Pass {
		t.Error("expected fail for non-matching output")
	}
}

func TestRegexJudge_InvalidPattern(t *testing.T) {
	j := &RegexJudge{Pattern: `[invalid`}
	_, err := j.Evaluate(Input{Output: "anything"})
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

func TestRegexJudge_Name(t *testing.T) {
	j := &RegexJudge{}
	if j.Name() != "regex" {
		t.Errorf("name = %q, want %q", j.Name(), "regex")
	}
}

// --- Schema Judge ---

func TestSchemaJudge_Pass(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "integer"}
		},
		"required": ["name", "age"]
	}`

	j := &SchemaJudge{Schema: schema}
	r, err := j.Evaluate(Input{Output: `{"name": "Alice", "age": 30}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Pass {
		t.Errorf("expected pass, got fail: %s", r.Reason)
	}
}

func TestSchemaJudge_Fail_MissingField(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "integer"}
		},
		"required": ["name", "age"]
	}`

	j := &SchemaJudge{Schema: schema}
	r, err := j.Evaluate(Input{Output: `{"name": "Alice"}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Pass {
		t.Error("expected fail for missing required field")
	}
}

func TestSchemaJudge_Fail_WrongType(t *testing.T) {
	schema := `{
		"type": "object",
		"properties": {
			"age": {"type": "integer"}
		},
		"required": ["age"]
	}`

	j := &SchemaJudge{Schema: schema}
	r, err := j.Evaluate(Input{Output: `{"age": "not a number"}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Pass {
		t.Error("expected fail for wrong type")
	}
}

func TestSchemaJudge_Fail_InvalidJSON(t *testing.T) {
	schema := `{"type": "object"}`
	j := &SchemaJudge{Schema: schema}
	r, err := j.Evaluate(Input{Output: "not json at all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Pass {
		t.Error("expected fail for non-JSON output")
	}
}

func TestSchemaJudge_InvalidSchema(t *testing.T) {
	j := &SchemaJudge{Schema: "not valid json"}
	_, err := j.Evaluate(Input{Output: "{}"})
	if err == nil {
		t.Error("expected error for invalid schema")
	}
}

func TestSchemaJudge_Name(t *testing.T) {
	j := &SchemaJudge{}
	if j.Name() != "schema" {
		t.Errorf("name = %q, want %q", j.Name(), "schema")
	}
}

// --- ToolCall Judge ---

func TestToolCallJudge_Pass_InOrder(t *testing.T) {
	j := &ToolCallJudge{
		Expected: []ExpectedToolCall{
			{ToolName: "read_file"},
			{ToolName: "write_file"},
		},
	}

	r, err := j.Evaluate(Input{
		ToolCalls: []trace.ToolCallTrace{
			{ToolName: "read_file"},
			{ToolName: "write_file"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Pass {
		t.Errorf("expected pass, got fail: %s", r.Reason)
	}
}

func TestToolCallJudge_Pass_SubsetParams(t *testing.T) {
	j := &ToolCallJudge{
		Expected: []ExpectedToolCall{
			{
				ToolName:   "read_file",
				Parameters: map[string]interface{}{"path": "/tmp/test"},
			},
		},
	}

	r, err := j.Evaluate(Input{
		ToolCalls: []trace.ToolCallTrace{
			{
				ToolName:   "read_file",
				Parameters: map[string]interface{}{"path": "/tmp/test", "encoding": "utf-8"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Pass {
		t.Errorf("expected pass with subset param match, got fail: %s", r.Reason)
	}
}

func TestToolCallJudge_Fail_ExactParams(t *testing.T) {
	j := &ToolCallJudge{
		Expected: []ExpectedToolCall{
			{
				ToolName:   "read_file",
				Parameters: map[string]interface{}{"path": "/tmp/test"},
				MatchMode:  "exact",
			},
		},
	}

	r, err := j.Evaluate(Input{
		ToolCalls: []trace.ToolCallTrace{
			{
				ToolName:   "read_file",
				Parameters: map[string]interface{}{"path": "/tmp/test", "encoding": "utf-8"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Pass {
		t.Error("expected fail with exact param match and extra params")
	}
}

func TestToolCallJudge_Fail_WrongOrder(t *testing.T) {
	j := &ToolCallJudge{
		Expected: []ExpectedToolCall{
			{ToolName: "write_file"},
			{ToolName: "read_file"},
		},
	}

	r, err := j.Evaluate(Input{
		ToolCalls: []trace.ToolCallTrace{
			{ToolName: "read_file"},
			{ToolName: "write_file"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// write_file expected first, but read_file comes first.
	// write_file is found at index 1, then read_file is searched from index 2 onward - not found.
	if r.Pass {
		t.Error("expected fail when tools called in wrong order")
	}
}

func TestToolCallJudge_Fail_MissingCall(t *testing.T) {
	j := &ToolCallJudge{
		Expected: []ExpectedToolCall{
			{ToolName: "read_file"},
			{ToolName: "delete_file"},
		},
	}

	r, err := j.Evaluate(Input{
		ToolCalls: []trace.ToolCallTrace{
			{ToolName: "read_file"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Pass {
		t.Error("expected fail when expected tool call is missing")
	}
}

func TestToolCallJudge_NegatePass(t *testing.T) {
	j := &ToolCallJudge{
		Expected: []ExpectedToolCall{
			{ToolName: "delete_file", Negate: true},
		},
	}

	r, err := j.Evaluate(Input{
		ToolCalls: []trace.ToolCallTrace{
			{ToolName: "read_file"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Pass {
		t.Errorf("expected pass for negated assertion, got fail: %s", r.Reason)
	}
}

func TestToolCallJudge_NegateFail(t *testing.T) {
	j := &ToolCallJudge{
		Expected: []ExpectedToolCall{
			{ToolName: "delete_file", Negate: true},
		},
	}

	r, err := j.Evaluate(Input{
		ToolCalls: []trace.ToolCallTrace{
			{ToolName: "delete_file"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Pass {
		t.Error("expected fail when negated tool was actually called")
	}
}

func TestToolCallJudge_InterlevedCalls(t *testing.T) {
	// Expect read_file then write_file, with other calls in between.
	j := &ToolCallJudge{
		Expected: []ExpectedToolCall{
			{ToolName: "read_file"},
			{ToolName: "write_file"},
		},
	}

	r, err := j.Evaluate(Input{
		ToolCalls: []trace.ToolCallTrace{
			{ToolName: "search"},
			{ToolName: "read_file"},
			{ToolName: "search"},
			{ToolName: "write_file"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Pass {
		t.Errorf("expected pass with interleaved calls, got fail: %s", r.Reason)
	}
}

func TestToolCallJudge_Name(t *testing.T) {
	j := &ToolCallJudge{}
	if j.Name() != "toolcall" {
		t.Errorf("name = %q, want %q", j.Name(), "toolcall")
	}
}

func TestToolCallJudge_EmptyExpectations(t *testing.T) {
	j := &ToolCallJudge{}
	r, err := j.Evaluate(Input{
		ToolCalls: []trace.ToolCallTrace{
			{ToolName: "anything"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Pass {
		t.Error("expected pass with no expectations")
	}
}
