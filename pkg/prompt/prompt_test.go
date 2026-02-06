package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	content := `name: test-prompt
description: A test prompt
system: "You are a helpful assistant."
user: "Hello, world!"
tools:
  - name: search
    description: Search the web
    parameters:
      type: object
      properties:
        query:
          type: string
metadata:
  version: "1.0"
  author: tester
`
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if p.Name != "test-prompt" {
		t.Errorf("Name = %q, want %q", p.Name, "test-prompt")
	}
	if p.Description != "A test prompt" {
		t.Errorf("Description = %q, want %q", p.Description, "A test prompt")
	}
	if p.System != "You are a helpful assistant." {
		t.Errorf("System = %q, want %q", p.System, "You are a helpful assistant.")
	}
	if p.User != "Hello, world!" {
		t.Errorf("User = %q, want %q", p.User, "Hello, world!")
	}
	if len(p.Tools) != 1 {
		t.Fatalf("len(Tools) = %d, want 1", len(p.Tools))
	}
	if p.Tools[0].Name != "search" {
		t.Errorf("Tools[0].Name = %q, want %q", p.Tools[0].Name, "search")
	}
	if p.Metadata["version"] != "1.0" {
		t.Errorf("Metadata[version] = %q, want %q", p.Metadata["version"], "1.0")
	}
	if p.Metadata["author"] != "tester" {
		t.Errorf("Metadata[author] = %q, want %q", p.Metadata["author"], "tester")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/prompt.yaml")
	if err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	// Tabs are not allowed for indentation in YAML, triggering a parse error.
	if err := os.WriteFile(path, []byte("name: test\n\t- broken:\n\t\tindent"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for invalid YAML, got nil")
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"alpha.yaml": "name: alpha\nsystem: Alpha system prompt\n",
		"beta.yml":   "name: beta\nuser: Beta user prompt\n",
		"skip.txt":   "not a yaml file",
	}
	// Create a subdirectory that should be skipped.
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	prompts, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir() error: %v", err)
	}

	if len(prompts) != 2 {
		t.Fatalf("LoadDir() returned %d prompts, want 2", len(prompts))
	}

	names := map[string]bool{}
	for _, p := range prompts {
		names[p.Name] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("LoadDir() names = %v, want alpha and beta", names)
	}
}

func TestLoadDir_NotFound(t *testing.T) {
	_, err := LoadDir("/nonexistent/dir")
	if err == nil {
		t.Fatal("LoadDir() expected error for missing dir, got nil")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		prompt  PromptVariant
		wantErr bool
	}{
		{
			name:    "valid with system",
			prompt:  PromptVariant{Name: "test", System: "hello"},
			wantErr: false,
		},
		{
			name:    "valid with user",
			prompt:  PromptVariant{Name: "test", User: "hello"},
			wantErr: false,
		},
		{
			name:    "valid with both",
			prompt:  PromptVariant{Name: "test", System: "sys", User: "usr"},
			wantErr: false,
		},
		{
			name:    "missing name",
			prompt:  PromptVariant{System: "hello"},
			wantErr: true,
		},
		{
			name:    "missing system and user",
			prompt:  PromptVariant{Name: "test"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.prompt.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestInterpolate(t *testing.T) {
	p := &PromptVariant{
		Name:   "interp-test",
		System: "You are a {{.role}} assistant.",
		User:   "Answer about {{.topic}} in {{.language}}.",
		Metadata: map[string]string{
			"version": "1.0",
		},
	}

	vars := map[string]interface{}{
		"role":     "helpful",
		"topic":    "Go programming",
		"language": "English",
	}

	result, err := p.Interpolate(vars)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}

	if result.System != "You are a helpful assistant." {
		t.Errorf("System = %q, want %q", result.System, "You are a helpful assistant.")
	}
	if result.User != "Answer about Go programming in English." {
		t.Errorf("User = %q, want %q", result.User, "Answer about Go programming in English.")
	}

	// Verify original is not modified.
	if p.System != "You are a {{.role}} assistant." {
		t.Error("Interpolate() modified original System field")
	}
	if p.User != "Answer about {{.topic}} in {{.language}}." {
		t.Error("Interpolate() modified original User field")
	}

	// Verify metadata is preserved.
	if result.Metadata["version"] != "1.0" {
		t.Errorf("Metadata[version] = %q, want %q", result.Metadata["version"], "1.0")
	}
}

func TestInterpolate_UndefinedVariable(t *testing.T) {
	p := &PromptVariant{
		Name:   "undef-test",
		System: "Hello {{.undefined_var}}",
	}

	_, err := p.Interpolate(map[string]interface{}{})
	if err == nil {
		t.Fatal("Interpolate() expected error for undefined variable, got nil")
	}
}

func TestInterpolate_EmptyFields(t *testing.T) {
	p := &PromptVariant{
		Name:   "empty-test",
		System: "",
		User:   "Just user: {{.name}}",
	}

	result, err := p.Interpolate(map[string]interface{}{"name": "Alice"})
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}

	if result.System != "" {
		t.Errorf("System = %q, want empty", result.System)
	}
	if result.User != "Just user: Alice" {
		t.Errorf("User = %q, want %q", result.User, "Just user: Alice")
	}
}

func TestInterpolate_InvalidTemplate(t *testing.T) {
	p := &PromptVariant{
		Name:   "bad-tmpl",
		System: "Hello {{.unclosed",
	}

	_, err := p.Interpolate(map[string]interface{}{})
	if err == nil {
		t.Fatal("Interpolate() expected error for invalid template syntax, got nil")
	}
}
