package suite

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdgilhuly/go_eval_agent/pkg/mock"
	"gopkg.in/yaml.v3"
)

// EvalSuite defines a collection of test cases to run against an LLM agent.
type EvalSuite struct {
	Name          string        `yaml:"name"`
	Description   string        `yaml:"description"`
	Prompt        string        `yaml:"prompt"`
	DefaultJudges []JudgeConfig `yaml:"default_judges"`
	DefaultMocks  []mock.MockConfig `yaml:"default_mocks"`
	Cases         []EvalCase    `yaml:"cases"`
}

// JudgeConfig describes a judge to apply to a case result.
type JudgeConfig struct {
	Type    string  `yaml:"type"`
	Value   string  `yaml:"value"`
	Weight  float64 `yaml:"weight"`
	Comment string  `yaml:"comment"`
}

// EvalCase is a single test case within a suite.
type EvalCase struct {
	ID             string                 `yaml:"id"`
	Name           string                 `yaml:"name"`
	Input          map[string]interface{} `yaml:"input"`
	Context        string                 `yaml:"context"`
	Mocks          []mock.MockConfig      `yaml:"mocks"`
	Judges         []JudgeConfig          `yaml:"judges"`
	ExpectedOutput string                 `yaml:"expected_output"`
	ExpectedTools  []string               `yaml:"expected_tools"`
	Tags           []string               `yaml:"tags"`
	Timeout        time.Duration          `yaml:"timeout"`
}

// Load reads a single EvalSuite from a YAML file. Suite-level defaults are
// merged into cases that don't specify their own judges or mocks.
func Load(path string) (*EvalSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading suite file %s: %w", path, err)
	}

	var s EvalSuite
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing suite file %s: %w", path, err)
	}

	s.applyDefaults()
	return &s, nil
}

// LoadDir loads all .yaml and .yml files from dir as EvalSuites.
func LoadDir(dir string) ([]*EvalSuite, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading suite directory %s: %w", dir, err)
	}

	var suites []*EvalSuite
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		s, err := Load(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		suites = append(suites, s)
	}

	return suites, nil
}

// Validate checks that the EvalSuite has the minimum required fields.
func (s *EvalSuite) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("suite name is required")
	}
	if len(s.Cases) == 0 {
		return fmt.Errorf("suite %q must have at least one case", s.Name)
	}
	for i, c := range s.Cases {
		if c.Name == "" {
			return fmt.Errorf("suite %q: case %d has no name", s.Name, i)
		}
	}
	return nil
}

// FilterByTag returns a new suite containing only cases that have at least one
// of the specified tags. An empty tag list returns all cases.
func (s *EvalSuite) FilterByTag(tags []string) *EvalSuite {
	if len(tags) == 0 {
		return s
	}

	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[t] = true
	}

	filtered := &EvalSuite{
		Name:          s.Name,
		Description:   s.Description,
		Prompt:        s.Prompt,
		DefaultJudges: s.DefaultJudges,
		DefaultMocks:  s.DefaultMocks,
	}

	for _, c := range s.Cases {
		for _, t := range c.Tags {
			if tagSet[t] {
				filtered.Cases = append(filtered.Cases, c)
				break
			}
		}
	}

	return filtered
}

// applyDefaults merges suite-level default judges and mocks into cases that
// don't specify their own.
func (s *EvalSuite) applyDefaults() {
	for i := range s.Cases {
		if len(s.Cases[i].Judges) == 0 && len(s.DefaultJudges) > 0 {
			s.Cases[i].Judges = s.DefaultJudges
		}
		if len(s.Cases[i].Mocks) == 0 && len(s.DefaultMocks) > 0 {
			s.Cases[i].Mocks = s.DefaultMocks
		}
	}
}
