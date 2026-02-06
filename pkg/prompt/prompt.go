package prompt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// PromptVariant represents a single prompt template that can be loaded from
// YAML and rendered with variable interpolation.
type PromptVariant struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	System      string            `yaml:"system"`
	User        string            `yaml:"user"`
	Tools       []ToolDefinition  `yaml:"tools"`
	Metadata    map[string]string `yaml:"metadata"`
}

// ToolDefinition describes a tool that the LLM can invoke during evaluation.
type ToolDefinition struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Parameters  map[string]interface{} `yaml:"parameters"` // JSON Schema
}

// Load reads a single PromptVariant from a YAML file at path.
func Load(path string) (*PromptVariant, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading prompt file %s: %w", path, err)
	}

	var p PromptVariant
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing prompt file %s: %w", path, err)
	}

	return &p, nil
}

// LoadDir loads all .yaml and .yml files from dir as PromptVariants.
func LoadDir(dir string) ([]*PromptVariant, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading prompt directory %s: %w", dir, err)
	}

	var prompts []*PromptVariant
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		p, err := Load(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		prompts = append(prompts, p)
	}

	return prompts, nil
}

// Validate checks that the PromptVariant has the minimum required fields.
func (p *PromptVariant) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("prompt name is required")
	}
	if p.System == "" && p.User == "" {
		return fmt.Errorf("prompt %q must have at least a system or user prompt", p.Name)
	}
	return nil
}

// Interpolate applies Go text/template rendering to the System and User fields
// using the provided variables. It returns a new PromptVariant with the
// rendered strings; the original is not modified.
//
// Template variables use {{.VarName}} syntax. An error is returned if a
// template references a variable not present in vars.
func (p *PromptVariant) Interpolate(vars map[string]interface{}) (*PromptVariant, error) {
	rendered := &PromptVariant{
		Name:        p.Name,
		Description: p.Description,
		Tools:       p.Tools,
		Metadata:    p.Metadata,
	}

	var err error
	rendered.System, err = renderTemplate(p.Name+".system", p.System, vars)
	if err != nil {
		return nil, fmt.Errorf("interpolating system prompt for %q: %w", p.Name, err)
	}

	rendered.User, err = renderTemplate(p.Name+".user", p.User, vars)
	if err != nil {
		return nil, fmt.Errorf("interpolating user prompt for %q: %w", p.Name, err)
	}

	return rendered, nil
}

// renderTemplate parses and executes a Go text/template with "missingkey=error"
// so that undefined variables produce an error instead of empty strings.
func renderTemplate(name, text string, vars map[string]interface{}) (string, error) {
	if text == "" {
		return "", nil
	}

	tmpl, err := template.New(name).Option("missingkey=error").Parse(text)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}
