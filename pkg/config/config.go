package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the top-level eval framework configuration.
type Config struct {
	Providers   map[string]ProviderConfig `yaml:"providers"`
	Concurrency int                       `yaml:"concurrency"`
	Timeout     time.Duration             `yaml:"timeout"`
	OutputDir   string                    `yaml:"output_dir"`
	RetryConfig RetryConfig               `yaml:"retry"`
}

// ProviderConfig holds configuration for a single LLM provider.
type ProviderConfig struct {
	Model     string `yaml:"model"`
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// RetryConfig holds retry behavior settings.
type RetryConfig struct {
	MaxRetries int           `yaml:"max_retries"`
	BaseDelay  time.Duration `yaml:"base_delay"`
}

// Default returns a Config populated with sensible defaults.
func Default() *Config {
	return &Config{
		Providers:   make(map[string]ProviderConfig),
		Concurrency: 5,
		Timeout:     60 * time.Second,
		OutputDir:   "results/",
		RetryConfig: RetryConfig{
			MaxRetries: 3,
			BaseDelay:  1 * time.Second,
		},
	}
}

// Load reads and parses a YAML config file at the given path.
// It returns an error if the file cannot be read or parsed.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	return cfg, nil
}

// LoadOrDefault loads config from the given path. If the file does not exist,
// it returns the default configuration. Other errors (e.g. parse failures)
// are still returned.
func LoadOrDefault(path string) (*Config, error) {
	cfg, err := Load(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return nil, err
	}
	return cfg, nil
}

// ResolveAPIKey reads the API key for the named provider from the environment
// variable specified in that provider's APIKeyEnv field.
func (c *Config) ResolveAPIKey(providerName string) (string, error) {
	p, ok := c.Providers[providerName]
	if !ok {
		return "", fmt.Errorf("provider %q not found in config", providerName)
	}
	if p.APIKeyEnv == "" {
		return "", fmt.Errorf("provider %q has no api_key_env configured", providerName)
	}
	key := os.Getenv(p.APIKeyEnv)
	if key == "" {
		return "", fmt.Errorf("environment variable %s for provider %q is not set", p.APIKeyEnv, providerName)
	}
	return key, nil
}

// Validate checks the config for required fields and returns a descriptive
// error if any are missing or invalid.
func (c *Config) Validate() error {
	var errs []error

	if c.Concurrency < 1 {
		errs = append(errs, fmt.Errorf("concurrency must be >= 1, got %d", c.Concurrency))
	}
	if c.Timeout <= 0 {
		errs = append(errs, fmt.Errorf("timeout must be > 0, got %s", c.Timeout))
	}
	if c.OutputDir == "" {
		errs = append(errs, errors.New("output_dir must not be empty"))
	}
	if c.RetryConfig.MaxRetries < 0 {
		errs = append(errs, fmt.Errorf("retry.max_retries must be >= 0, got %d", c.RetryConfig.MaxRetries))
	}
	if c.RetryConfig.BaseDelay < 0 {
		errs = append(errs, fmt.Errorf("retry.base_delay must be >= 0, got %s", c.RetryConfig.BaseDelay))
	}

	for name, p := range c.Providers {
		if p.Model == "" {
			errs = append(errs, fmt.Errorf("provider %q: model is required", name))
		}
		if p.APIKeyEnv == "" {
			errs = append(errs, fmt.Errorf("provider %q: api_key_env is required", name))
		}
	}

	return errors.Join(errs...)
}
