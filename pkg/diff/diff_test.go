package diff

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jdgilhuly/go_eval_agent/pkg/result"
)

func runA() *result.RunSummary {
	return &result.RunSummary{
		RunID:     "run-a",
		SuiteName: "test",
		Results: []result.CaseResult{
			{CaseName: "stable", Score: 0.8, Pass: true},
			{CaseName: "improved", Score: 0.5, Pass: false},
			{CaseName: "regressed", Score: 0.9, Pass: true},
			{CaseName: "removed-case", Score: 0.7, Pass: true},
		},
	}
}

func runB() *result.RunSummary {
	return &result.RunSummary{
		RunID:     "run-b",
		SuiteName: "test",
		Results: []result.CaseResult{
			{CaseName: "stable", Score: 0.8, Pass: true},
			{CaseName: "improved", Score: 0.9, Pass: true},
			{CaseName: "regressed", Score: 0.4, Pass: false},
			{CaseName: "new-case", Score: 1.0, Pass: true},
		},
	}
}

func TestCompare(t *testing.T) {
	dr := Compare(runA(), runB(), 0.0)

	if dr.RunA != "run-a" || dr.RunB != "run-b" {
		t.Errorf("RunA=%q RunB=%q", dr.RunA, dr.RunB)
	}

	if len(dr.Cases) != 5 {
		t.Fatalf("len(Cases) = %d, want 5", len(dr.Cases))
	}

	categories := map[string]Category{}
	for _, cd := range dr.Cases {
		categories[cd.CaseName] = cd.Category
	}

	if categories["stable"] != Unchanged {
		t.Errorf("stable = %q, want unchanged", categories["stable"])
	}
	if categories["improved"] != Improved {
		t.Errorf("improved = %q, want improved", categories["improved"])
	}
	if categories["regressed"] != Regressed {
		t.Errorf("regressed = %q, want regressed", categories["regressed"])
	}
	if categories["new-case"] != New {
		t.Errorf("new-case = %q, want new", categories["new-case"])
	}
	if categories["removed-case"] != Removed {
		t.Errorf("removed-case = %q, want removed", categories["removed-case"])
	}

	if dr.Summary.Improved != 1 {
		t.Errorf("Improved = %d, want 1", dr.Summary.Improved)
	}
	if dr.Summary.Regressed != 1 {
		t.Errorf("Regressed = %d, want 1", dr.Summary.Regressed)
	}
	if dr.Summary.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", dr.Summary.Unchanged)
	}
	if dr.Summary.New != 1 {
		t.Errorf("New = %d, want 1", dr.Summary.New)
	}
	if dr.Summary.Removed != 1 {
		t.Errorf("Removed = %d, want 1", dr.Summary.Removed)
	}
}

func TestCompare_WithThreshold(t *testing.T) {
	dr := Compare(runA(), runB(), 0.5)

	categories := map[string]Category{}
	for _, cd := range dr.Cases {
		categories[cd.CaseName] = cd.Category
	}

	// "improved" has delta 0.4, below threshold 0.5 => unchanged.
	if categories["improved"] != Unchanged {
		t.Errorf("improved = %q, want unchanged (below threshold)", categories["improved"])
	}
	// "regressed" has delta -0.5, exactly at threshold => unchanged (|delta| < threshold).
	if categories["regressed"] != Unchanged {
		t.Errorf("regressed = %q, want unchanged (at threshold)", categories["regressed"])
	}
}

func TestCompare_Empty(t *testing.T) {
	a := &result.RunSummary{RunID: "a"}
	b := &result.RunSummary{RunID: "b"}
	dr := Compare(a, b, 0.0)
	if len(dr.Cases) != 0 {
		t.Errorf("len(Cases) = %d, want 0", len(dr.Cases))
	}
}

func TestFilter(t *testing.T) {
	dr := Compare(runA(), runB(), 0.0)

	filtered := dr.Filter([]Category{Improved, Regressed})
	if len(filtered.Cases) != 2 {
		t.Fatalf("filtered len(Cases) = %d, want 2", len(filtered.Cases))
	}

	for _, cd := range filtered.Cases {
		if cd.Category != Improved && cd.Category != Regressed {
			t.Errorf("unexpected category %q in filtered results", cd.Category)
		}
	}
}

func TestFilter_Nil(t *testing.T) {
	dr := Compare(runA(), runB(), 0.0)
	filtered := dr.Filter(nil)
	if len(filtered.Cases) != len(dr.Cases) {
		t.Errorf("nil filter returned %d cases, want %d", len(filtered.Cases), len(dr.Cases))
	}
}

func TestJSON(t *testing.T) {
	dr := Compare(runA(), runB(), 0.0)
	data, err := dr.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("JSON() returned empty")
	}
	if !strings.Contains(string(data), "improved") {
		t.Error("JSON output missing 'improved' category")
	}
}

func TestPrintTable(t *testing.T) {
	dr := Compare(runA(), runB(), 0.0)

	var buf bytes.Buffer
	dr.PrintTable(&buf)
	output := buf.String()

	for _, want := range []string{
		"CASE", "CHANGE", "SCORE A", "SCORE B", "DELTA",
		"stable", "improved", "regressed", "new-case", "removed-case",
		"1 improved", "1 regressed", "1 unchanged", "1 new", "1 removed",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("table output missing %q", want)
		}
	}
}

func TestScoreDelta(t *testing.T) {
	dr := Compare(runA(), runB(), 0.0)

	deltas := map[string]float64{}
	for _, cd := range dr.Cases {
		deltas[cd.CaseName] = cd.ScoreDelta
	}

	if d := deltas["improved"]; d < 0.39 || d > 0.41 {
		t.Errorf("improved delta = %f, want ~0.4", d)
	}
	if d := deltas["regressed"]; d > -0.49 || d < -0.51 {
		t.Errorf("regressed delta = %f, want ~-0.5", d)
	}
	if d := deltas["stable"]; d != 0.0 {
		t.Errorf("stable delta = %f, want 0.0", d)
	}
}
