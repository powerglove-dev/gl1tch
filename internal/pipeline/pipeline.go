package pipeline

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Pipeline is the top-level definition loaded from a .pipeline.yaml file.
type Pipeline struct {
	Name    string         `yaml:"name"`
	Version string         `yaml:"version"`
	Steps   []Step         `yaml:"steps"`
	Vars    map[string]any `yaml:"vars"` // Pipeline-level seed context available to all steps.
}

// Step is one unit of work in a pipeline.
type Step struct {
	ID        string    `yaml:"id"`
	Type      string    `yaml:"type"`      // "input", "output", or empty (plugin step)
	Model     string    `yaml:"model"`
	Prompt    string    `yaml:"prompt"`
	Input     string    `yaml:"input"`
	PublishTo string    `yaml:"publish_to"`
	Condition Condition `yaml:"condition"`

	// Deprecated: use Executor instead. Plugin specifies the plugin name to invoke.
	Plugin string `yaml:"plugin"`
	// Deprecated: use Args instead. Vars is a flat string map passed to the plugin.
	Vars map[string]string `yaml:"vars"`

	// Executor supersedes Plugin when set; use "builtin.*" or "category.action" form.
	Executor string `yaml:"executor"`
	// Args supersedes Vars when set; supports nested values for structured plugin input.
	Args map[string]any `yaml:"args"`
	// Needs lists step IDs that must complete before this step runs (DAG — implemented in pipeline-enhancements).
	Needs []string `yaml:"needs"`
}

// Condition describes a branch: if expression is true go to Then, else go to Else.
type Condition struct {
	If   string `yaml:"if"`
	Then string `yaml:"then"`
	Else string `yaml:"else"`
}

// Load reads and validates a Pipeline from r.
func Load(r io.Reader) (*Pipeline, error) {
	var p Pipeline
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("pipeline yaml: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("pipeline yaml: name is required")
	}
	return &p, nil
}
