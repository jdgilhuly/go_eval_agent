package result

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdgilhuly/go_eval_agent/pkg/runner"
	"github.com/jdgilhuly/go_eval_agent/pkg/trace"
)

func TestFromRunResult(t *testing.T) {
	tr := trace.New()
	tr.AddUsage(100, 50)
	tr.Finish()

	rr := &runner.RunResult{
		SuiteName: "test-suite",
		StartTime: time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2025, 6, 15, 10, 0, 5, 0, time.UTC),
		Duration:  5 * time.Second,
		Cases: []runner.CaseResult{
			{
				CaseID:        "c1",
				CaseName:      "case-one",
				Prompt:        "default",
				Model:         "test-model",
				FinalResponse: "hello",
				Trace:         tr,
				Duration:      2 * time.Second,
			},
		},
	}

	summary := FromRunResult(rr)

	if summary.RunID == "" {
		t.Error("RunID is empty")
	}
	if summary.SuiteName != "test-suite" {
		t.Errorf("SuiteName = %q, want %q", summary.SuiteName, "test-suite")
	}
	if len(summary.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(summary.Results))
	}

	cr := summary.Results[0]
	if cr.CaseName != "case-one" {
		t.Errorf("CaseName = %q, want %q", cr.CaseName, "case-one")
	}
	if cr.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", cr.InputTokens)
	}
	if cr.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", cr.OutputTokens)
	}
}

func TestComputeStats(t *testing.T) {
	results := []CaseResult{
		{CaseName: "c1", Pass: true, Score: 1.0, Duration: 100 * time.Millisecond, InputTokens: 10, OutputTokens: 5},
		{CaseName: "c2", Pass: true, Score: 0.8, Duration: 200 * time.Millisecond, InputTokens: 20, OutputTokens: 10},
		{CaseName: "c3", Pass: false, Score: 0.3, Duration: 300 * time.Millisecond, InputTokens: 15, OutputTokens: 8},
		{CaseName: "c4", Error: "timeout", Score: 0.0, Duration: 500 * time.Millisecond, InputTokens: 5, OutputTokens: 2},
	}

	s := ComputeStats(results)

	if s.TotalCases != 4 {
		t.Errorf("TotalCases = %d, want 4", s.TotalCases)
	}
	if s.PassedCases != 2 {
		t.Errorf("PassedCases = %d, want 2", s.PassedCases)
	}
	if s.FailedCases != 1 {
		t.Errorf("FailedCases = %d, want 1", s.FailedCases)
	}
	if s.ErroredCases != 1 {
		t.Errorf("ErroredCases = %d, want 1", s.ErroredCases)
	}

	// Pass rate = 2 / (4 - 1 errored) = 2/3
	expectedPassRate := 2.0 / 3.0
	if diff := s.PassRate - expectedPassRate; diff > 0.001 || diff < -0.001 {
		t.Errorf("PassRate = %f, want %f", s.PassRate, expectedPassRate)
	}

	// Avg score = (1.0 + 0.8 + 0.3 + 0.0) / 4 = 0.525
	expectedAvg := 0.525
	if diff := s.AvgScore - expectedAvg; diff > 0.001 || diff < -0.001 {
		t.Errorf("AvgScore = %f, want %f", s.AvgScore, expectedAvg)
	}

	if s.TotalInputTokens != 50 {
		t.Errorf("TotalInputTokens = %d, want 50", s.TotalInputTokens)
	}
	if s.TotalOutputTokens != 25 {
		t.Errorf("TotalOutputTokens = %d, want 25", s.TotalOutputTokens)
	}

	// P50 of sorted [100ms, 200ms, 300ms, 500ms] = interpolated at index 1.5 = 250ms
	if s.LatencyP50 != 250*time.Millisecond {
		t.Errorf("LatencyP50 = %v, want 250ms", s.LatencyP50)
	}
}

func TestComputeStats_Empty(t *testing.T) {
	s := ComputeStats(nil)
	if s.TotalCases != 0 {
		t.Errorf("TotalCases = %d, want 0", s.TotalCases)
	}
	if s.PassRate != 0 {
		t.Errorf("PassRate = %f, want 0", s.PassRate)
	}
}

func TestDefaultPath(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 30, 45, 0, time.UTC)
	path := DefaultPath("results", "my-suite", ts)
	expected := filepath.Join("results", "20250615-103045-my-suite.json")
	if path != expected {
		t.Errorf("DefaultPath = %q, want %q", path, expected)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output", "test-result.json")

	summary := &RunSummary{
		RunID:     "test-run-id",
		SuiteName: "save-test",
		StartTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2025, 1, 1, 0, 0, 5, 0, time.UTC),
		Duration:  5 * time.Second,
		Stats: Stats{
			TotalCases:  2,
			PassedCases: 1,
			FailedCases: 1,
			PassRate:    0.5,
		},
		Results: []CaseResult{
			{CaseID: "c1", CaseName: "pass-case", Pass: true, Score: 1.0, Duration: time.Second},
			{CaseID: "c2", CaseName: "fail-case", Pass: false, Score: 0.0, Duration: 2 * time.Second},
		},
	}

	if err := summary.Save(path); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify the output directory was created.
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("output directory not created: %v", err)
	}

	loaded, err := LoadSummary(path)
	if err != nil {
		t.Fatalf("LoadSummary() error: %v", err)
	}

	if loaded.RunID != summary.RunID {
		t.Errorf("RunID = %q, want %q", loaded.RunID, summary.RunID)
	}
	if loaded.SuiteName != summary.SuiteName {
		t.Errorf("SuiteName = %q, want %q", loaded.SuiteName, summary.SuiteName)
	}
	if len(loaded.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(loaded.Results))
	}
	if loaded.Stats.PassRate != 0.5 {
		t.Errorf("Stats.PassRate = %f, want 0.5", loaded.Stats.PassRate)
	}
}

func TestLoadSummary_NotFound(t *testing.T) {
	_, err := LoadSummary("/nonexistent/result.json")
	if err == nil {
		t.Fatal("LoadSummary() expected error for missing file, got nil")
	}
}

func TestLoadSummary_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadSummary(path)
	if err == nil {
		t.Fatal("LoadSummary() expected error for invalid JSON, got nil")
	}
}
