package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdgilhuly/go_eval_agent/pkg/config"
	"github.com/jdgilhuly/go_eval_agent/pkg/prompt"
	"github.com/jdgilhuly/go_eval_agent/pkg/suite"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "eval",
	Short: "Go Agent Eval Framework",
	Long: `A framework for evaluating LLM agent behavior through configurable
test suites, prompt templates, tool mocking, and composable judges.

Use 'eval init' to scaffold a new eval project, then 'eval run' to
execute eval suites against your agent.`,
}

// --- run command ---

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run an eval suite",
	Long: `Execute an eval suite against a configured LLM provider.

Runs all cases in the suite, applies judges, and outputs results.
Results are saved to a JSON file for later comparison with 'eval diff'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath, _ := cmd.Flags().GetString("config")
		cfg, err := config.LoadOrDefault(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}

		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose {
			fmt.Printf("Config loaded: concurrency=%d timeout=%s output=%s\n",
				cfg.Concurrency, cfg.Timeout, cfg.OutputDir)
		}

		fmt.Println("eval run: not yet implemented")
		return nil
	},
}

// --- diff command ---

var diffCmd = &cobra.Command{
	Use:   "diff <run-a.json> <run-b.json>",
	Short: "Compare two run results",
	Long: `Compare results from two eval runs side-by-side.

Shows score regressions, improvements, and unchanged cases.
Useful for evaluating prompt changes or model upgrades.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("eval diff: not yet implemented")
		return nil
	},
}

// --- review command ---

var reviewCmd = &cobra.Command{
	Use:   "review <run.json>",
	Short: "Review flagged cases from a run",
	Long: `Interactively review cases that were flagged during an eval run.

Flagged cases include failures, low-confidence judge scores, and
cases marked for human review.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("eval review: not yet implemented")
		return nil
	},
}

// --- list command ---

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available resources",
	Long:  `List available prompts, suites, or other eval resources.`,
}

var listPromptsCmd = &cobra.Command{
	Use:   "prompts",
	Short: "List available prompt templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _ := cmd.Flags().GetString("dir")
		promptDir := filepath.Join(dir, "prompts")

		prompts, err := prompt.LoadDir(promptDir)
		if err != nil {
			return fmt.Errorf("loading prompts from %s: %w", promptDir, err)
		}

		if len(prompts) == 0 {
			fmt.Println("No prompt templates found.")
			return nil
		}

		for _, p := range prompts {
			desc := p.Description
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Printf("  %-20s %s\n", p.Name, desc)
		}
		return nil
	},
}

var listSuitesCmd = &cobra.Command{
	Use:   "suites",
	Short: "List available eval suites",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _ := cmd.Flags().GetString("dir")
		suiteDir := filepath.Join(dir, "suites")

		suites, err := suite.LoadDir(suiteDir)
		if err != nil {
			return fmt.Errorf("loading suites from %s: %w", suiteDir, err)
		}

		if len(suites) == 0 {
			fmt.Println("No eval suites found.")
			return nil
		}

		for _, s := range suites {
			desc := s.Description
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Printf("  %-20s %-40s (%d cases)\n", s.Name, desc, len(s.Cases))
		}
		return nil
	},
}

// --- validate command ---

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate config and suite files",
	Long: `Check eval configuration and suite files for errors.

Validates YAML syntax, required fields, judge references, and
prompt template variables.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		suitePath, _ := cmd.Flags().GetString("suite")
		if suitePath != "" {
			s, err := suite.Load(suitePath)
			if err != nil {
				return fmt.Errorf("loading suite: %w", err)
			}
			if err := s.Validate(); err != nil {
				return fmt.Errorf("suite validation failed: %w", err)
			}
			fmt.Printf("Suite %q is valid (%d cases).\n", s.Name, len(s.Cases))
		}

		cfgPath, _ := cmd.Flags().GetString("config")
		cfg, err := config.LoadOrDefault(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("config validation failed: %w", err)
		}
		fmt.Printf("Config %q is valid.\n", cfgPath)

		return nil
	},
}

// --- init command ---

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new eval project",
	Long: `Scaffold a new eval project with example configuration, prompts,
suites, and a results directory.

Creates the following structure:
  eval.yaml          - Main configuration file
  prompts/           - Prompt template directory
  suites/            - Eval suite directory
  results/           - Run result output directory`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	dirs := []string{"prompts", "suites", "results"}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
		fmt.Printf("  created %s/\n", d)
	}

	if err := writeExampleConfig("eval.yaml"); err != nil {
		return err
	}
	if err := writeExamplePrompt(filepath.Join("prompts", "default.yaml")); err != nil {
		return err
	}
	if err := writeExampleSuite(filepath.Join("suites", "example.yaml")); err != nil {
		return err
	}

	fmt.Println("\nEval project initialized. Run 'eval validate' to check your config.")
	return nil
}

func writeYAML(path string, data any) error {
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("  skipped %s (already exists)\n", path)
		return nil
	}

	out, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Printf("  created %s\n", path)
	return nil
}

func writeExampleConfig(path string) error {
	data := map[string]any{
		"version": 1,
		"defaults": map[string]any{
			"provider":    "anthropic",
			"model":       "claude-sonnet-4-5-20250929",
			"concurrency": 5,
			"timeout":     "30s",
		},
		"providers": map[string]any{
			"anthropic": map[string]any{
				"type":    "anthropic",
				"api_key": "${ANTHROPIC_API_KEY}",
			},
		},
	}
	return writeYAML(path, data)
}

func writeExamplePrompt(path string) error {
	data := map[string]any{
		"name":    "default",
		"version": "1.0.0",
		"system":  "You are a helpful assistant.",
		"template": `Answer the user's question concisely.

Question: {{.question}}`,
		"variables": []string{"question"},
	}
	return writeYAML(path, data)
}

func writeExampleSuite(path string) error {
	data := map[string]any{
		"name":        "example",
		"description": "An example eval suite to get started",
		"prompt":      "default",
		"cases": []map[string]any{
			{
				"name": "simple-greeting",
				"vars": map[string]any{
					"question": "Say hello.",
				},
				"judges": []map[string]any{
					{
						"type":    "contains",
						"value":   "hello",
						"weight":  1.0,
						"comment": "Response should contain a greeting",
					},
				},
			},
		},
	}
	return writeYAML(path, data)
}

func init() {
	// run command flags
	runCmd.Flags().StringP("suite", "s", "", "Path to eval suite YAML file")
	runCmd.Flags().StringP("prompt", "p", "", "Override prompt template")
	runCmd.Flags().StringP("model", "m", "", "Override model name")
	runCmd.Flags().StringP("config", "c", "eval.yaml", "Path to config file")
	runCmd.Flags().IntP("concurrency", "j", 0, "Max concurrent eval cases (0 = use config default)")
	runCmd.Flags().StringP("tag", "t", "", "Tag this run for identification")
	runCmd.Flags().StringP("output", "o", "", "Output file path (default: results/<timestamp>.json)")
	runCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")

	// diff command flags
	diffCmd.Flags().Float64("threshold", 0.0, "Minimum score change to highlight")
	diffCmd.Flags().String("format", "table", "Output format: table, json, markdown")

	// review command flags
	reviewCmd.Flags().String("filter", "", "Filter cases: failed, flagged, all")

	// list command flags
	listCmd.PersistentFlags().String("dir", ".", "Base directory to search")
	listCmd.AddCommand(listPromptsCmd)
	listCmd.AddCommand(listSuitesCmd)

	// validate command flags
	validateCmd.Flags().String("suite", "", "Path to suite file to validate")
	validateCmd.Flags().String("config", "eval.yaml", "Path to config file to validate")

	// register all subcommands
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(reviewCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(initCmd)
}
