package pipeline

import (
	"fmt"
	"io"
	"time"

	"gopkg.in/yaml.v3"
)

// Pipeline is the top-level definition loaded from a .pipeline.yaml file.
type Pipeline struct {
	Name        string         `yaml:"name"`
	Version     string         `yaml:"version"`
	Steps       []Step         `yaml:"steps"`
	Vars        map[string]any `yaml:"vars"` // Pipeline-level seed context available to all steps.
	MaxParallel int            `yaml:"max_parallel"` // Maximum concurrent steps; defaults to 8 when zero.
	WriteBrain  bool           `yaml:"write_brain"`
}

// Step is one unit of work in a pipeline.
type Step struct {
	ID        string    `yaml:"id"`
	Type      string    `yaml:"type"`      // "input", "output", or empty (executor step)
	Model     string    `yaml:"model"`
	Prompt    string    `yaml:"prompt"`
	Input     string    `yaml:"input"`
	PublishTo string    `yaml:"publish_to"`
	Condition Condition `yaml:"condition"`

	// Vars is a flat string map passed to the executor.
	Vars map[string]string `yaml:"vars"`

	// Executor specifies the executor to invoke; use "builtin.*" or "category.action" form.
	Executor string `yaml:"executor"`
	// Args supersedes Vars when set; supports nested values for structured plugin input.
	Args map[string]any `yaml:"args"`
	// Needs lists step IDs that must complete before this step runs (DAG).
	Needs []string `yaml:"needs"`
	// Retry describes the retry policy for this step. Nil means no retry.
	Retry *RetryPolicy `yaml:"retry"`
	// OnFailure names a step ID to run if this step fails after all retry attempts.
	OnFailure string `yaml:"on_failure"`
	// ForEach is a template expression or newline-separated list of items.
	// When set, the step is expanded into N cloned steps, one per item.
	ForEach string `yaml:"for_each"`

	// WriteBrain controls brain write injection for this step.
	// Pointer for tri-state: nil = inherit pipeline setting, true = force on, false = force off.
	WriteBrain *bool `yaml:"write_brain"`
	// PromptID is the title of a saved prompt in the store. When set, the prompt
	// body is prepended (with a blank line separator) to the step's input before
	// execution. Uses case-insensitive title matching.
	PromptID string `yaml:"prompt_id,omitempty"`

	// Outputs declares the output keys produced by this step.
	// After the step completes, its full output string is stored under each declared key.
	Outputs map[string]string `yaml:"outputs,omitempty"`
	// Inputs maps input names to template expressions like "{{ steps.<id>.<key> }}".
	// These are resolved before execution using accumulated step outputs.
	Inputs map[string]string `yaml:"inputs,omitempty"`
}

// RetryPolicy specifies how a step should be retried on failure.
type RetryPolicy struct {
	MaxAttempts int      `yaml:"max_attempts"`
	Interval    Duration `yaml:"interval"`
	// On controls when to retry: "always" (default) or "on_failure".
	On string `yaml:"on"`
}

// Duration wraps time.Duration to support YAML unmarshalling from strings like "2s".
type Duration struct{ time.Duration }

// UnmarshalYAML parses a duration string (e.g. "2s", "500ms") into a Duration.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	dur, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	d.Duration = dur
	return nil
}

// Condition describes a branch: if expression is true go to Then, else go to Else.
type Condition struct {
	If   string `yaml:"if"`
	Then string `yaml:"then"`
	Else string `yaml:"else"`
}

// Load reads and validates a Pipeline from r.
// Returns an error if the YAML is invalid, name is missing, or the step DAG contains a cycle.
func Load(r io.Reader) (*Pipeline, error) {
	var p Pipeline
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("pipeline yaml: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("pipeline yaml: name is required")
	}
	// Reject removed step types.
	for _, s := range p.Steps {
		if s.Type == "db" {
			return nil, fmt.Errorf("pipeline yaml: db step type has been removed (step %q)", s.ID)
		}
	}
	// Validate DAG — detect cycles before execution.
	if _, err := buildDAG(p.Steps); err != nil {
		return nil, fmt.Errorf("pipeline yaml: %w", err)
	}
	return &p, nil
}
