package result

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jdgilhuly/go_eval_agent/pkg/runner"
)

// RunSummary is the top-level structure persisted to JSON for each eval run.
type RunSummary struct {
	RunID     string    `json:"run_id"`
	SuiteName string    `json:"suite_name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Stats     Stats         `json:"stats"`
	Results   []CaseResult  `json:"results"`
}

// Stats holds aggregate statistics for the run.
type Stats struct {
	TotalCases   int     `json:"total_cases"`
	PassedCases  int     `json:"passed_cases"`
	FailedCases  int     `json:"failed_cases"`
	ErroredCases int     `json:"errored_cases"`
	PassRate     float64 `json:"pass_rate"`
	AvgScore     float64 `json:"avg_score"`
	LatencyP50   time.Duration `json:"latency_p50"`
	LatencyP95   time.Duration `json:"latency_p95"`
	TotalInputTokens  int `json:"total_input_tokens"`
	TotalOutputTokens int `json:"total_output_tokens"`
}

// CaseResult is the per-case result stored in the JSON output.
type CaseResult struct {
	CaseID        string        `json:"case_id"`
	CaseName      string        `json:"case_name"`
	Prompt        string        `json:"prompt"`
	Model         string        `json:"model"`
	FinalResponse string        `json:"final_response"`
	Score         float64       `json:"score"`
	Pass          bool          `json:"pass"`
	Error         string        `json:"error,omitempty"`
	Duration      time.Duration `json:"duration"`
	InputTokens   int           `json:"input_tokens"`
	OutputTokens  int           `json:"output_tokens"`
}

// FromRunResult converts a runner.RunResult into a RunSummary, generating
// a run ID and computing summary statistics. Scores and pass/fail are left
// at zero values since judging is performed separately.
func FromRunResult(rr *runner.RunResult) *RunSummary {
	runID := fmt.Sprintf("%s-%s", rr.StartTime.Format("20060102-150405"), rr.SuiteName)

	summary := &RunSummary{
		RunID:     runID,
		SuiteName: rr.SuiteName,
		StartTime: rr.StartTime,
		EndTime:   rr.EndTime,
		Duration:  rr.Duration,
	}

	for _, cr := range rr.Cases {
		caseResult := CaseResult{
			CaseID:        cr.CaseID,
			CaseName:      cr.CaseName,
			Prompt:        cr.Prompt,
			Model:         cr.Model,
			FinalResponse: cr.FinalResponse,
			Error:         cr.Error,
			Duration:      cr.Duration,
		}
		if cr.Trace != nil {
			usage := cr.Trace.GetUsage()
			caseResult.InputTokens = usage.InputTokens
			caseResult.OutputTokens = usage.OutputTokens
		}
		summary.Results = append(summary.Results, caseResult)
	}

	summary.Stats = ComputeStats(summary.Results)
	return summary
}

// ComputeStats calculates aggregate statistics from a slice of CaseResults.
func ComputeStats(results []CaseResult) Stats {
	s := Stats{TotalCases: len(results)}
	if len(results) == 0 {
		return s
	}

	var totalScore float64
	var durations []time.Duration

	for _, r := range results {
		if r.Error != "" {
			s.ErroredCases++
		} else if r.Pass {
			s.PassedCases++
		} else {
			s.FailedCases++
		}
		totalScore += r.Score
		durations = append(durations, r.Duration)
		s.TotalInputTokens += r.InputTokens
		s.TotalOutputTokens += r.OutputTokens
	}

	nonErrored := s.TotalCases - s.ErroredCases
	if nonErrored > 0 {
		s.PassRate = float64(s.PassedCases) / float64(nonErrored)
	}
	s.AvgScore = totalScore / float64(s.TotalCases)

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	s.LatencyP50 = percentile(durations, 0.5)
	s.LatencyP95 = percentile(durations, 0.95)

	return s
}

// percentile returns the value at the given percentile (0.0-1.0) from a
// sorted slice of durations.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return time.Duration(float64(sorted[lower])*(1-frac) + float64(sorted[upper])*frac)
}

// DefaultPath returns the default output file path for a run result.
func DefaultPath(outputDir, suiteName string, startTime time.Time) string {
	filename := fmt.Sprintf("%s-%s.json", startTime.Format("20060102-150405"), suiteName)
	return filepath.Join(outputDir, filename)
}

// Save writes the RunSummary as pretty-printed JSON to the given path.
// Parent directories are created automatically.
func (s *RunSummary) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating result directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling result: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing result to %s: %w", path, err)
	}

	return nil
}

// LoadSummary reads a RunSummary from a JSON file.
func LoadSummary(path string) (*RunSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading result file %s: %w", path, err)
	}

	var s RunSummary
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing result file %s: %w", path, err)
	}

	return &s, nil
}
