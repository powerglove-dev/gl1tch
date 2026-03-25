package pipeline_test

import (
	"strings"
	"testing"

	"github.com/adam-stokes/orcai/internal/pipeline"
)

const sampleYAML = `
name: test-pipeline
version: "1.0"
steps:
  - id: step1
    type: input
    prompt: "Enter topic:"
  - id: step2
    plugin: claude
    model: claude-sonnet-4-6
    prompt: "Summarize: {{step1.out}}"
    condition:
      if: "contains:spec"
      then: step3a
      else: step3b
  - id: step3a
    plugin: openspec
    input: "{{step2.out}}"
  - id: step3b
    plugin: openclaw
    input: "{{step2.out}}"
  - id: output
    type: output
    publish_to: "pipeline.test-pipeline.done"
`

func TestLoad_Valid(t *testing.T) {
	p, err := pipeline.Load(strings.NewReader(sampleYAML))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.Name != "test-pipeline" {
		t.Errorf("expected name 'test-pipeline', got %q", p.Name)
	}
	if len(p.Steps) != 5 {
		t.Errorf("expected 5 steps, got %d", len(p.Steps))
	}
	if p.Steps[0].ID != "step1" {
		t.Errorf("expected first step id 'step1', got %q", p.Steps[0].ID)
	}
	if p.Steps[1].Condition.If != "contains:spec" {
		t.Errorf("expected condition 'contains:spec', got %q", p.Steps[1].Condition.If)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	_, err := pipeline.Load(strings.NewReader(":::bad yaml:::"))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoad_MissingName(t *testing.T) {
	_, err := pipeline.Load(strings.NewReader("version: '1.0'\nsteps: []"))
	if err == nil {
		t.Error("expected error when name is missing")
	}
}

func TestInterpolate_Simple(t *testing.T) {
	vars := map[string]any{"step1.out": "golang plugins"}
	result := pipeline.Interpolate("Summarize: {{step1.out}}", vars)
	if result != "Summarize: golang plugins" {
		t.Errorf("got %q", result)
	}
}

func TestInterpolate_Multiple(t *testing.T) {
	vars := map[string]any{"a.out": "foo", "b.out": "bar"}
	result := pipeline.Interpolate("{{a.out}} and {{b.out}}", vars)
	if result != "foo and bar" {
		t.Errorf("got %q", result)
	}
}

func TestInterpolate_Missing(t *testing.T) {
	vars := map[string]any{}
	result := pipeline.Interpolate("hello {{missing.out}}", vars)
	// Missing vars are left as-is.
	if result != "hello {{missing.out}}" {
		t.Errorf("got %q", result)
	}
}

func TestInterpolate_NonStringValue(t *testing.T) {
	vars := map[string]any{"count": 42}
	result := pipeline.Interpolate("count={{count}}", vars)
	if result != "count=42" {
		t.Errorf("got %q", result)
	}
}
