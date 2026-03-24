package pipeline

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Pipeline is the top-level definition loaded from a .pipeline.yaml file.
type Pipeline struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Steps   []Step `yaml:"steps"`
}

// Step is one unit of work in a pipeline.
type Step struct {
	ID        string    `yaml:"id"`
	Type      string    `yaml:"type"`      // "input", "output", or empty (plugin step)
	Plugin    string    `yaml:"plugin"`
	Model     string    `yaml:"model"`
	Prompt    string    `yaml:"prompt"`
	Input     string    `yaml:"input"`
	PublishTo string    `yaml:"publish_to"`
	Condition Condition `yaml:"condition"`
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
