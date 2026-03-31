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

func TestLoadRejectsDbStepType(t *testing.T) {
	input := `
name: db-pipeline
version: "1.0"
steps:
  - id: my_query
    type: db
    args:
      op: query
      sql: "SELECT 1"
`
	_, err := pipeline.Load(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for db step type, got nil")
	}
	if !strings.Contains(err.Error(), "db step type has been removed") {
		t.Errorf("expected error to contain %q, got: %v", "db step type has been removed", err)
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

// TestLoadAcceptsBrainFields verifies that a pipeline with write_brain
// fields at both the pipeline and step level loads without error and the values are
// correctly parsed. use_brain is no longer a field (brain is always on).
func TestLoadAcceptsBrainFields(t *testing.T) {
	input := `
name: test-brain-fields
version: "1"
write_brain: true
steps:
  - id: s1
    executor: claude
    model: claude-sonnet-4-6
    write_brain: false
    prompt: "hello"
`
	p, err := pipeline.Load(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if !p.WriteBrain {
		t.Error("expected pipeline.WriteBrain to be true")
	}
	if len(p.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(p.Steps))
	}
	s := p.Steps[0]
	if s.WriteBrain == nil || *s.WriteBrain {
		t.Error("expected step.WriteBrain to be *false")
	}
}

// TestTemplateInterpolation tests for the new nested path and legacy compat features.
func TestTemplateInterpolation(t *testing.T) {
	t.Run("nested step.id.data.key path", func(t *testing.T) {
		vars := map[string]any{
			"step": map[string]any{
				"fetch": map[string]any{
					"data": map[string]any{
						"url": "https://example.com",
					},
				},
			},
		}
		result := pipeline.Interpolate("url={{step.fetch.data.url}}", vars)
		if result != "url=https://example.com" {
			t.Errorf("expected nested path resolution, got %q", result)
		}
	})

	t.Run("missing nested path left unchanged", func(t *testing.T) {
		vars := map[string]any{
			"step": map[string]any{},
		}
		result := pipeline.Interpolate("{{step.missing.data.key}}", vars)
		if result != "{{step.missing.data.key}}" {
			t.Errorf("expected unchanged placeholder, got %q", result)
		}
	})

	t.Run("legacy {{ID.out}} falls back to step.ID.data.value", func(t *testing.T) {
		vars := map[string]any{
			"step": map[string]any{
				"s1": map[string]any{
					"data": map[string]any{
						"value": "legacy output",
					},
				},
			},
		}
		result := pipeline.Interpolate("{{s1.out}}", vars)
		if result != "legacy output" {
			t.Errorf("expected legacy compat, got %q", result)
		}
	})

	t.Run("flat key takes precedence over nested path", func(t *testing.T) {
		vars := map[string]any{
			"s1.out": "flat value",
			"step": map[string]any{
				"s1": map[string]any{
					"data": map[string]any{"value": "nested value"},
				},
			},
		}
		result := pipeline.Interpolate("{{s1.out}}", vars)
		// Flat key takes precedence.
		if result != "flat value" {
			t.Errorf("expected flat key precedence, got %q", result)
		}
	})

	t.Run("multi-segment keys resolve via dot traversal", func(t *testing.T) {
		vars := map[string]any{
			"param": map[string]any{
				"key": "myvalue",
			},
		}
		result := pipeline.Interpolate("value={{param.key}}", vars)
		if result != "value=myvalue" {
			t.Errorf("expected dot traversal, got %q", result)
		}
	})
}
