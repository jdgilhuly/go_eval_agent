package suite

import (
	"os"
	"path/filepath"
	"testing"
)

const basicSuiteYAML = `name: test-suite
description: A test suite
prompt: default
default_judges:
  - type: contains
    value: hello
    weight: 1.0
    comment: default judge
default_mocks:
  - tool_name: search
    default_response:
      content: "mock result"
cases:
  - name: case-one
    id: c1
    input:
      question: "Say hello"
    tags:
      - greeting
      - simple
  - name: case-two
    id: c2
    input:
      question: "Do math"
    tags:
      - math
    judges:
      - type: exact
        value: "42"
        weight: 1.0
        comment: override judge
    mocks:
      - tool_name: calc
        default_response:
          content: "42"
`

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "suite.yaml", basicSuiteYAML)

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if s.Name != "test-suite" {
		t.Errorf("Name = %q, want %q", s.Name, "test-suite")
	}
	if s.Description != "A test suite" {
		t.Errorf("Description = %q, want %q", s.Description, "A test suite")
	}
	if s.Prompt != "default" {
		t.Errorf("Prompt = %q, want %q", s.Prompt, "default")
	}
	if len(s.Cases) != 2 {
		t.Fatalf("len(Cases) = %d, want 2", len(s.Cases))
	}
	if s.Cases[0].Name != "case-one" {
		t.Errorf("Cases[0].Name = %q, want %q", s.Cases[0].Name, "case-one")
	}
	if s.Cases[0].ID != "c1" {
		t.Errorf("Cases[0].ID = %q, want %q", s.Cases[0].ID, "c1")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/suite.yaml")
	if err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "bad.yaml", "name: test\n\t- broken:\n\t\tindent")

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for invalid YAML, got nil")
	}
}

func TestDefaultMerging(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "suite.yaml", basicSuiteYAML)

	s, err := Load(filepath.Join(dir, "suite.yaml"))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// case-one has no judges/mocks, should inherit defaults.
	c1 := s.Cases[0]
	if len(c1.Judges) != 1 {
		t.Fatalf("case-one: len(Judges) = %d, want 1 (from defaults)", len(c1.Judges))
	}
	if c1.Judges[0].Type != "contains" {
		t.Errorf("case-one: Judges[0].Type = %q, want %q", c1.Judges[0].Type, "contains")
	}
	if len(c1.Mocks) != 1 {
		t.Fatalf("case-one: len(Mocks) = %d, want 1 (from defaults)", len(c1.Mocks))
	}
	if c1.Mocks[0].ToolName != "search" {
		t.Errorf("case-one: Mocks[0].ToolName = %q, want %q", c1.Mocks[0].ToolName, "search")
	}

	// case-two specifies its own judges and mocks, should NOT inherit defaults.
	c2 := s.Cases[1]
	if len(c2.Judges) != 1 {
		t.Fatalf("case-two: len(Judges) = %d, want 1", len(c2.Judges))
	}
	if c2.Judges[0].Type != "exact" {
		t.Errorf("case-two: Judges[0].Type = %q, want %q", c2.Judges[0].Type, "exact")
	}
	if len(c2.Mocks) != 1 {
		t.Fatalf("case-two: len(Mocks) = %d, want 1", len(c2.Mocks))
	}
	if c2.Mocks[0].ToolName != "calc" {
		t.Errorf("case-two: Mocks[0].ToolName = %q, want %q", c2.Mocks[0].ToolName, "calc")
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()

	writeTempFile(t, dir, "alpha.yaml", "name: alpha\ncases:\n  - name: a1\n")
	writeTempFile(t, dir, "beta.yml", "name: beta\ncases:\n  - name: b1\n  - name: b2\n")
	writeTempFile(t, dir, "skip.txt", "not yaml")

	// Subdirectory should be skipped.
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	suites, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() error: %v", err)
	}

	if len(suites) != 2 {
		t.Fatalf("LoadDir() returned %d suites, want 2", len(suites))
	}

	names := map[string]int{}
	for _, s := range suites {
		names[s.Name] = len(s.Cases)
	}
	if names["alpha"] != 1 {
		t.Errorf("alpha case count = %d, want 1", names["alpha"])
	}
	if names["beta"] != 2 {
		t.Errorf("beta case count = %d, want 2", names["beta"])
	}
}

func TestLoadDir_NotFound(t *testing.T) {
	_, err := LoadDir("/nonexistent/dir")
	if err == nil {
		t.Fatal("LoadDir() expected error for missing directory, got nil")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		suite   EvalSuite
		wantErr bool
	}{
		{
			name: "valid suite",
			suite: EvalSuite{
				Name:  "test",
				Cases: []EvalCase{{Name: "c1"}},
			},
			wantErr: false,
		},
		{
			name:    "missing name",
			suite:   EvalSuite{Cases: []EvalCase{{Name: "c1"}}},
			wantErr: true,
		},
		{
			name:    "no cases",
			suite:   EvalSuite{Name: "test"},
			wantErr: true,
		},
		{
			name: "case missing name",
			suite: EvalSuite{
				Name:  "test",
				Cases: []EvalCase{{ID: "c1"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.suite.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestFilterByTag(t *testing.T) {
	s := &EvalSuite{
		Name: "filter-test",
		Cases: []EvalCase{
			{Name: "c1", Tags: []string{"greeting", "simple"}},
			{Name: "c2", Tags: []string{"math"}},
			{Name: "c3", Tags: []string{"greeting", "complex"}},
			{Name: "c4", Tags: nil},
		},
	}

	t.Run("filter greeting", func(t *testing.T) {
		filtered := s.FilterByTag([]string{"greeting"})
		if len(filtered.Cases) != 2 {
			t.Fatalf("len(Cases) = %d, want 2", len(filtered.Cases))
		}
		if filtered.Cases[0].Name != "c1" || filtered.Cases[1].Name != "c3" {
			t.Errorf("Cases = [%s, %s], want [c1, c3]", filtered.Cases[0].Name, filtered.Cases[1].Name)
		}
	})

	t.Run("filter math", func(t *testing.T) {
		filtered := s.FilterByTag([]string{"math"})
		if len(filtered.Cases) != 1 {
			t.Fatalf("len(Cases) = %d, want 1", len(filtered.Cases))
		}
		if filtered.Cases[0].Name != "c2" {
			t.Errorf("Cases[0].Name = %q, want %q", filtered.Cases[0].Name, "c2")
		}
	})

	t.Run("filter multiple tags", func(t *testing.T) {
		filtered := s.FilterByTag([]string{"math", "complex"})
		if len(filtered.Cases) != 2 {
			t.Fatalf("len(Cases) = %d, want 2", len(filtered.Cases))
		}
	})

	t.Run("empty tag list returns all", func(t *testing.T) {
		filtered := s.FilterByTag(nil)
		if len(filtered.Cases) != 4 {
			t.Fatalf("len(Cases) = %d, want 4", len(filtered.Cases))
		}
	})

	t.Run("no matching tag", func(t *testing.T) {
		filtered := s.FilterByTag([]string{"nonexistent"})
		if len(filtered.Cases) != 0 {
			t.Fatalf("len(Cases) = %d, want 0", len(filtered.Cases))
		}
	})
}
