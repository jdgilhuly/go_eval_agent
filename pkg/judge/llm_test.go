package judge

import (
	"context"
	"fmt"
	"testing"

	"github.com/jdgilhuly/go_eval_agent/pkg/provider"
	"github.com/jdgilhuly/go_eval_agent/pkg/trace"
)

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	response *provider.Response
	err      error
	lastReq  *provider.Request
}

func (m *mockProvider) Complete(_ context.Context, req *provider.Request) (*provider.Response, error) {
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *mockProvider) Name() string { return "mock" }

func TestLLMJudge_PassingScore(t *testing.T) {
	mp := &mockProvider{
		response: &provider.Response{
			Content:    `{"score": 5, "pass": true, "reasoning": "Excellent output"}`,
			Usage:      provider.Usage{InputTokens: 100, OutputTokens: 50},
			StopReason: "end_turn",
		},
	}

	j := &LLMJudge{
		Provider: mp,
		Model:    "claude-3-haiku-20240307",
		Rubric:   "Check if the answer is correct.",
		Ctx:      context.Background(),
	}

	r, err := j.Evaluate(Input{
		Output:         "The answer is 42.",
		ExpectedOutput: "42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !r.Pass {
		t.Errorf("expected pass, got fail")
	}
	if r.Score != 1.0 {
		t.Errorf("score = %f, want 1.0", r.Score)
	}
	if r.Reason != "Excellent output" {
		t.Errorf("reason = %q, want %q", r.Reason, "Excellent output")
	}

	// Verify usage tracking.
	usage := j.GetUsage()
	if usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", usage.OutputTokens)
	}
}

func TestLLMJudge_FailingScore(t *testing.T) {
	mp := &mockProvider{
		response: &provider.Response{
			Content:    `{"score": 2, "pass": false, "reasoning": "Missing key details"}`,
			Usage:      provider.Usage{InputTokens: 80, OutputTokens: 40},
			StopReason: "end_turn",
		},
	}

	j := &LLMJudge{
		Provider: mp,
		Model:    "claude-3-haiku-20240307",
		Rubric:   "Evaluate completeness.",
		Ctx:      context.Background(),
	}

	r, err := j.Evaluate(Input{Output: "partial answer"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r.Pass {
		t.Error("expected fail, got pass")
	}
	if r.Score != 0.4 {
		t.Errorf("score = %f, want 0.4", r.Score)
	}
}

func TestLLMJudge_ProviderError(t *testing.T) {
	mp := &mockProvider{
		err: fmt.Errorf("API rate limit"),
	}

	j := &LLMJudge{
		Provider: mp,
		Model:    "claude-3-haiku-20240307",
		Rubric:   "Check correctness.",
		Ctx:      context.Background(),
	}

	_, err := j.Evaluate(Input{Output: "anything"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "llm judge call failed: API rate limit" {
		t.Errorf("error = %q, want wrapped API error", got)
	}
}

func TestLLMJudge_MalformedJSON_FallbackParsing(t *testing.T) {
	mp := &mockProvider{
		response: &provider.Response{
			Content: "I'd give this a score of 4 out of 5. Good work.",
			Usage:   provider.Usage{InputTokens: 50, OutputTokens: 20},
		},
	}

	j := &LLMJudge{
		Provider: mp,
		Model:    "claude-3-haiku-20240307",
		Rubric:   "Check quality.",
		Ctx:      context.Background(),
	}

	r, err := j.Evaluate(Input{Output: "decent answer"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !r.Pass {
		t.Error("expected pass for score 4")
	}
	if r.Score != 0.8 {
		t.Errorf("score = %f, want 0.8", r.Score)
	}
}

func TestLLMJudge_JSONInCodeFence(t *testing.T) {
	mp := &mockProvider{
		response: &provider.Response{
			Content: "```json\n{\"score\": 3, \"pass\": false, \"reasoning\": \"Mediocre\"}\n```",
			Usage:   provider.Usage{InputTokens: 60, OutputTokens: 25},
		},
	}

	j := &LLMJudge{
		Provider: mp,
		Model:    "claude-3-haiku-20240307",
		Rubric:   "Check quality.",
		Ctx:      context.Background(),
	}

	r, err := j.Evaluate(Input{Output: "some answer"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if r.Pass {
		t.Error("expected fail for score 3")
	}
	if r.Score != 0.6 {
		t.Errorf("score = %f, want 0.6", r.Score)
	}
	if r.Reason != "Mediocre" {
		t.Errorf("reason = %q, want %q", r.Reason, "Mediocre")
	}
}

func TestLLMJudge_UnparseableResponse(t *testing.T) {
	mp := &mockProvider{
		response: &provider.Response{
			Content: "I cannot evaluate this without more context.",
			Usage:   provider.Usage{InputTokens: 30, OutputTokens: 10},
		},
	}

	j := &LLMJudge{
		Provider: mp,
		Model:    "claude-3-haiku-20240307",
		Rubric:   "Check quality.",
		Ctx:      context.Background(),
	}

	_, err := j.Evaluate(Input{Output: "anything"})
	if err == nil {
		t.Fatal("expected error for unparseable response")
	}
}

func TestLLMJudge_UsageAccumulation(t *testing.T) {
	callCount := 0
	mp := &mockProvider{
		response: &provider.Response{
			Content: `{"score": 4, "pass": true, "reasoning": "Good"}`,
			Usage:   provider.Usage{InputTokens: 100, OutputTokens: 50},
		},
	}

	j := &LLMJudge{
		Provider: mp,
		Model:    "claude-3-haiku-20240307",
		Rubric:   "Check quality.",
		Ctx:      context.Background(),
	}

	for i := 0; i < 3; i++ {
		callCount++
		_, err := j.Evaluate(Input{Output: fmt.Sprintf("answer %d", i)})
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}

	usage := j.GetUsage()
	if usage.InputTokens != 300 {
		t.Errorf("accumulated InputTokens = %d, want 300", usage.InputTokens)
	}
	if usage.OutputTokens != 150 {
		t.Errorf("accumulated OutputTokens = %d, want 150", usage.OutputTokens)
	}
}

func TestLLMJudge_RequestStructure(t *testing.T) {
	mp := &mockProvider{
		response: &provider.Response{
			Content: `{"score": 5, "pass": true, "reasoning": "Perfect"}`,
			Usage:   provider.Usage{InputTokens: 100, OutputTokens: 50},
		},
	}

	j := &LLMJudge{
		Provider: mp,
		Model:    "claude-3-haiku-20240307",
		Rubric:   "Check if correct.",
		Ctx:      context.Background(),
	}

	_, err := j.Evaluate(Input{
		Output:         "The sky is blue.",
		ExpectedOutput: "blue",
		ToolCalls: []trace.ToolCallTrace{
			{ToolName: "search", Parameters: map[string]interface{}{"query": "sky color"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify system prompt is the judge system prompt.
	if mp.lastReq.System != judgeSystemPrompt {
		t.Errorf("system prompt not set correctly")
	}
	if mp.lastReq.Model != "claude-3-haiku-20240307" {
		t.Errorf("model = %q, want %q", mp.lastReq.Model, "claude-3-haiku-20240307")
	}
	if len(mp.lastReq.Messages) != 1 {
		t.Fatalf("messages length = %d, want 1", len(mp.lastReq.Messages))
	}

	msg := mp.lastReq.Messages[0].Content
	// Verify the user message contains expected output, agent output, tool calls, and rubric.
	for _, want := range []string{"Expected Output", "blue", "Agent Output", "The sky is blue.", "Tool Calls Made", "search", "Rubric", "Check if correct."} {
		if !containsStr(msg, want) {
			t.Errorf("user message missing %q", want)
		}
	}
}

func TestLLMJudge_Name(t *testing.T) {
	j := &LLMJudge{}
	if got := j.Name(); got != "llm" {
		t.Errorf("Name() = %q, want %q", got, "llm")
	}
}

func TestLLMJudge_NilContext(t *testing.T) {
	mp := &mockProvider{
		response: &provider.Response{
			Content: `{"score": 4, "pass": true, "reasoning": "Good"}`,
			Usage:   provider.Usage{InputTokens: 50, OutputTokens: 25},
		},
	}

	j := &LLMJudge{
		Provider: mp,
		Model:    "claude-3-haiku-20240307",
		Rubric:   "Check quality.",
		// Ctx intentionally nil.
	}

	r, err := j.Evaluate(Input{Output: "answer"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Pass {
		t.Error("expected pass")
	}
}

func TestParseJudgeResponse(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    Result
		wantErr bool
	}{
		{
			name:    "valid JSON score 5",
			content: `{"score": 5, "pass": true, "reasoning": "Perfect"}`,
			want:    Result{Pass: true, Score: 1.0, Reason: "Perfect"},
		},
		{
			name:    "valid JSON score 1",
			content: `{"score": 1, "pass": false, "reasoning": "Wrong"}`,
			want:    Result{Pass: false, Score: 0.2, Reason: "Wrong"},
		},
		{
			name:    "JSON with surrounding text",
			content: `Here is my evaluation: {"score": 4, "pass": true, "reasoning": "Good"} That's it.`,
			want:    Result{Pass: true, Score: 0.8, Reason: "Good"},
		},
		{
			name:    "fallback text with score",
			content: "Overall I'd rate this a 3 out of 5.",
			want:    Result{Pass: false, Score: 0.6},
		},
		{
			name:    "no parseable score",
			content: "This is a completely irrelevant response.",
			wantErr: true,
		},
		{
			name:    "invalid score range in JSON",
			content: `{"score": 10, "pass": true, "reasoning": "Too high"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseJudgeResponse(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got result: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Pass != tt.want.Pass {
				t.Errorf("Pass = %v, want %v", got.Pass, tt.want.Pass)
			}
			if got.Score != tt.want.Score {
				t.Errorf("Score = %f, want %f", got.Score, tt.want.Score)
			}
			if tt.want.Reason != "" && got.Reason != tt.want.Reason {
				t.Errorf("Reason = %q, want %q", got.Reason, tt.want.Reason)
			}
		})
	}
}

func TestLLMJudge_CompositeIntegration(t *testing.T) {
	// Verify LLMJudge works with CompositeScorer via the Judge interface.
	mp := &mockProvider{
		response: &provider.Response{
			Content: `{"score": 5, "pass": true, "reasoning": "Great"}`,
			Usage:   provider.Usage{InputTokens: 100, OutputTokens: 50},
		},
	}

	llmJudge := &LLMJudge{
		Provider: mp,
		Model:    "claude-3-haiku-20240307",
		Rubric:   "Is it correct?",
		Ctx:      context.Background(),
	}

	exactJudge := &ExactJudge{}

	scorer := NewCompositeScorer(0.5)
	result := scorer.Score(
		Input{Output: "hello", ExpectedOutput: "hello"},
		[]JudgeConfig{
			{Judge: exactJudge, Weight: 1.0},
			{Judge: llmJudge, Weight: 2.0},
		},
	)

	if !result.Pass {
		t.Errorf("expected composite pass, got fail: %s", result.Reason)
	}
	// exact = 1.0 * 1.0, llm = 1.0 * 2.0 => (1 + 2) / 3 = 1.0
	if result.CompositeScore != 1.0 {
		t.Errorf("composite score = %f, want 1.0", result.CompositeScore)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
