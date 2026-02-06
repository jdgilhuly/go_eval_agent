package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	yaml := `
providers:
  anthropic:
    model: claude-sonnet-4-5-20250929
    base_url: https://api.anthropic.com
    api_key_env: ANTHROPIC_API_KEY
  openai:
    model: gpt-4o
    api_key_env: OPENAI_API_KEY
concurrency: 10
timeout: 30s
output_dir: output/
retry:
  max_retries: 5
  base_delay: 2s
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Concurrency != 10 {
		t.Errorf("Concurrency = %d, want 10", cfg.Concurrency)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %s, want 30s", cfg.Timeout)
	}
	if cfg.OutputDir != "output/" {
		t.Errorf("OutputDir = %q, want %q", cfg.OutputDir, "output/")
	}
	if cfg.RetryConfig.MaxRetries != 5 {
		t.Errorf("RetryConfig.MaxRetries = %d, want 5", cfg.RetryConfig.MaxRetries)
	}
	if cfg.RetryConfig.BaseDelay != 2*time.Second {
		t.Errorf("RetryConfig.BaseDelay = %s, want 2s", cfg.RetryConfig.BaseDelay)
	}

	if len(cfg.Providers) != 2 {
		t.Fatalf("len(Providers) = %d, want 2", len(cfg.Providers))
	}

	anth := cfg.Providers["anthropic"]
	if anth.Model != "claude-sonnet-4-5-20250929" {
		t.Errorf("anthropic.Model = %q, want %q", anth.Model, "claude-sonnet-4-5-20250929")
	}
	if anth.BaseURL != "https://api.anthropic.com" {
		t.Errorf("anthropic.BaseURL = %q, want %q", anth.BaseURL, "https://api.anthropic.com")
	}
	if anth.APIKeyEnv != "ANTHROPIC_API_KEY" {
		t.Errorf("anthropic.APIKeyEnv = %q, want %q", anth.APIKeyEnv, "ANTHROPIC_API_KEY")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTemp(t, "{{invalid yaml")
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for invalid YAML, got nil")
	}
}

func TestLoadOrDefault_FileExists(t *testing.T) {
	yaml := `
concurrency: 20
timeout: 45s
`
	path := writeTemp(t, yaml)
	cfg, err := LoadOrDefault(path)
	if err != nil {
		t.Fatalf("LoadOrDefault() error: %v", err)
	}
	if cfg.Concurrency != 20 {
		t.Errorf("Concurrency = %d, want 20", cfg.Concurrency)
	}
	if cfg.Timeout != 45*time.Second {
		t.Errorf("Timeout = %s, want 45s", cfg.Timeout)
	}
	// Defaults should still be populated for unset fields.
	if cfg.OutputDir != "results/" {
		t.Errorf("OutputDir = %q, want default %q", cfg.OutputDir, "results/")
	}
}

func TestLoadOrDefault_FileMissing(t *testing.T) {
	cfg, err := LoadOrDefault("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("LoadOrDefault() error: %v", err)
	}

	def := Default()
	if cfg.Concurrency != def.Concurrency {
		t.Errorf("Concurrency = %d, want default %d", cfg.Concurrency, def.Concurrency)
	}
	if cfg.Timeout != def.Timeout {
		t.Errorf("Timeout = %s, want default %s", cfg.Timeout, def.Timeout)
	}
	if cfg.OutputDir != def.OutputDir {
		t.Errorf("OutputDir = %q, want default %q", cfg.OutputDir, def.OutputDir)
	}
	if cfg.RetryConfig.MaxRetries != def.RetryConfig.MaxRetries {
		t.Errorf("RetryConfig.MaxRetries = %d, want default %d", cfg.RetryConfig.MaxRetries, def.RetryConfig.MaxRetries)
	}
}

func TestLoadOrDefault_InvalidYAML(t *testing.T) {
	path := writeTemp(t, "{{bad yaml")
	_, err := LoadOrDefault(path)
	if err == nil {
		t.Fatal("LoadOrDefault() expected error for invalid YAML, got nil")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := Default()
	cfg.Providers["test"] = ProviderConfig{
		Model:     "test-model",
		APIKeyEnv: "TEST_API_KEY",
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

func TestValidate_NoProviders(t *testing.T) {
	cfg := Default()
	// No providers is valid -- there's nothing invalid about the providers map.
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() unexpected error with no providers: %v", err)
	}
}

func TestValidate_MissingModel(t *testing.T) {
	cfg := Default()
	cfg.Providers["bad"] = ProviderConfig{
		APIKeyEnv: "SOME_KEY",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for missing model")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("error = %q, want it to mention 'model is required'", err)
	}
}

func TestValidate_MissingAPIKeyEnv(t *testing.T) {
	cfg := Default()
	cfg.Providers["bad"] = ProviderConfig{
		Model: "some-model",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for missing api_key_env")
	}
	if !strings.Contains(err.Error(), "api_key_env is required") {
		t.Errorf("error = %q, want it to mention 'api_key_env is required'", err)
	}
}

func TestValidate_BadConcurrency(t *testing.T) {
	cfg := Default()
	cfg.Concurrency = 0
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for concurrency=0")
	}
	if !strings.Contains(err.Error(), "concurrency") {
		t.Errorf("error = %q, want it to mention 'concurrency'", err)
	}
}

func TestValidate_BadTimeout(t *testing.T) {
	cfg := Default()
	cfg.Timeout = 0
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for timeout=0")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error = %q, want it to mention 'timeout'", err)
	}
}

func TestValidate_EmptyOutputDir(t *testing.T) {
	cfg := Default()
	cfg.OutputDir = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for empty output_dir")
	}
	if !strings.Contains(err.Error(), "output_dir") {
		t.Errorf("error = %q, want it to mention 'output_dir'", err)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := &Config{
		Concurrency: 0,
		Timeout:     0,
		OutputDir:   "",
		Providers: map[string]ProviderConfig{
			"bad": {},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() expected multiple errors")
	}
	msg := err.Error()
	for _, want := range []string{"concurrency", "timeout", "output_dir", "model is required", "api_key_env is required"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing mention of %q: %s", want, msg)
		}
	}
}

func TestResolveAPIKey(t *testing.T) {
	cfg := Default()
	cfg.Providers["anthropic"] = ProviderConfig{
		Model:     "claude-sonnet-4-5-20250929",
		APIKeyEnv: "TEST_EVAL_ANTHROPIC_KEY",
	}

	t.Setenv("TEST_EVAL_ANTHROPIC_KEY", "sk-test-12345")

	key, err := cfg.ResolveAPIKey("anthropic")
	if err != nil {
		t.Fatalf("ResolveAPIKey() error: %v", err)
	}
	if key != "sk-test-12345" {
		t.Errorf("ResolveAPIKey() = %q, want %q", key, "sk-test-12345")
	}
}

func TestResolveAPIKey_UnknownProvider(t *testing.T) {
	cfg := Default()
	_, err := cfg.ResolveAPIKey("unknown")
	if err == nil {
		t.Fatal("ResolveAPIKey() expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to mention 'not found'", err)
	}
}

func TestResolveAPIKey_NoEnvVar(t *testing.T) {
	cfg := Default()
	cfg.Providers["test"] = ProviderConfig{
		Model:     "test-model",
		APIKeyEnv: "COMPLETELY_NONEXISTENT_ENV_VAR_FOR_TEST",
	}
	_, err := cfg.ResolveAPIKey("test")
	if err == nil {
		t.Fatal("ResolveAPIKey() expected error for unset env var")
	}
	if !strings.Contains(err.Error(), "not set") {
		t.Errorf("error = %q, want it to mention 'not set'", err)
	}
}

func TestResolveAPIKey_NoAPIKeyEnv(t *testing.T) {
	cfg := Default()
	cfg.Providers["test"] = ProviderConfig{
		Model: "test-model",
	}
	_, err := cfg.ResolveAPIKey("test")
	if err == nil {
		t.Fatal("ResolveAPIKey() expected error for empty api_key_env")
	}
	if !strings.Contains(err.Error(), "no api_key_env configured") {
		t.Errorf("error = %q, want it to mention 'no api_key_env configured'", err)
	}
}

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Concurrency != 5 {
		t.Errorf("Default Concurrency = %d, want 5", cfg.Concurrency)
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Default Timeout = %s, want 60s", cfg.Timeout)
	}
	if cfg.OutputDir != "results/" {
		t.Errorf("Default OutputDir = %q, want %q", cfg.OutputDir, "results/")
	}
	if cfg.RetryConfig.MaxRetries != 3 {
		t.Errorf("Default RetryConfig.MaxRetries = %d, want 3", cfg.RetryConfig.MaxRetries)
	}
	if cfg.RetryConfig.BaseDelay != 1*time.Second {
		t.Errorf("Default RetryConfig.BaseDelay = %s, want 1s", cfg.RetryConfig.BaseDelay)
	}
	if cfg.Providers == nil {
		t.Error("Default Providers is nil, want initialized map")
	}
}

// writeTemp writes content to a temp YAML file and returns the path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}
