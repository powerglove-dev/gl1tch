package pipeline_test

import (
	"strings"
	"testing"

	"github.com/powerglove-dev/gl1tch/internal/pipeline"
)

const sampleYAML = `
name: test-pipeline
version: "1.0"
steps:
  - id: step1
    type: input
    prompt: "Enter topic:"
  - id: step2
    executor: claude
    model: claude-sonnet-4-6
    prompt: "Summarize: {{step1.out}}"
    condition:
      if: "contains:spec"
      then: step3a
      else: step3b
  - id: step3a
    executor: openspec
    input: "{{step2.out}}"
  - id: step3b
    executor: openclaw
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
	vars := map[string]any{"step1": map[string]any{"out": "golang plugins"}}
	result := pipeline.Interpolate("Summarize: {{.step1.out}}", vars)
	if result != "Summarize: golang plugins" {
		t.Errorf("got %q", result)
	}
}

func TestInterpolate_Multiple(t *testing.T) {
	vars := map[string]any{
		"a": map[string]any{"out": "foo"},
		"b": map[string]any{"out": "bar"},
	}
	result := pipeline.Interpolate("{{.a.out}} and {{.b.out}}", vars)
	if result != "foo and bar" {
		t.Errorf("got %q", result)
	}
}

func TestInterpolate_Missing(t *testing.T) {
	vars := map[string]any{}
	// Chained access through a missing key causes a template execution error;
	// the original string is returned unchanged (useful for debugging).
	result := pipeline.Interpolate("hello {{.missing.out}}", vars)
	if result != "hello {{.missing.out}}" {
		t.Errorf("got %q", result)
	}
}

func TestInterpolate_NonStringValue(t *testing.T) {
	vars := map[string]any{"count": 42}
	result := pipeline.Interpolate("count={{.count}}", vars)
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
		result := pipeline.Interpolate("url={{.step.fetch.data.url}}", vars)
		if result != "url=https://example.com" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("missing nested path returns original placeholder", func(t *testing.T) {
		vars := map[string]any{
			"step": map[string]any{},
		}
		// Chained access through a nil/missing intermediate value causes a template
		// execution error; the original placeholder is preserved for debugging.
		result := pipeline.Interpolate("{{.step.missing.data.key}}", vars)
		if result != "{{.step.missing.data.key}}" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("multi-segment param key", func(t *testing.T) {
		vars := map[string]any{
			"param": map[string]any{
				"key": "myvalue",
			},
		}
		result := pipeline.Interpolate("value={{.param.key}}", vars)
		if result != "value=myvalue" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("get function for hyphenated step IDs", func(t *testing.T) {
		vars := map[string]any{
			"step": map[string]any{
				"ask-llama": map[string]any{
					"data": map[string]any{"value": "hello from llama"},
				},
			},
		}
		result := pipeline.Interpolate(`{{get "step.ask-llama.data.value" .}}`, vars)
		if result != "hello from llama" {
			t.Errorf("got %q", result)
		}
	})
}

// TestInterpolateTemplateFunctions tests the text/template second pass with FuncMap.
func TestInterpolateTemplateFunctions(t *testing.T) {
	t.Run("upper function via pipe", func(t *testing.T) {
		vars := map[string]any{"param": map[string]any{"query": "hello world"}}
		result := pipeline.Interpolate("{{.param.query | upper}}", vars)
		if result != "HELLO WORLD" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("default function for missing key", func(t *testing.T) {
		vars := map[string]any{"param": map[string]any{}}
		result := pipeline.Interpolate(`{{.param.missing | default "fallback"}}`, vars)
		if result != "fallback" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("default function for empty string", func(t *testing.T) {
		vars := map[string]any{"param": map[string]any{"mode": ""}}
		result := pipeline.Interpolate(`{{.param.mode | default "concise"}}`, vars)
		if result != "concise" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("env function", func(t *testing.T) {
		t.Setenv("GLITCH_TEST_VAR", "testvalue")
		vars := map[string]any{}
		result := pipeline.Interpolate(`{{env "GLITCH_TEST_VAR"}}`, vars)
		if result != "testvalue" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("trim function via pipe", func(t *testing.T) {
		vars := map[string]any{"param": map[string]any{"value": "  hello  "}}
		result := pipeline.Interpolate("{{.param.value | trim}}", vars)
		if result != "hello" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("lower function via pipe", func(t *testing.T) {
		vars := map[string]any{"param": map[string]any{"name": "WORLD"}}
		result := pipeline.Interpolate("{{.param.name | lower}}", vars)
		if result != "world" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("replace function via pipe", func(t *testing.T) {
		vars := map[string]any{"param": map[string]any{"text": "foo bar foo"}}
		result := pipeline.Interpolate(`{{.param.text | replace "foo" "baz"}}`, vars)
		if result != "baz bar baz" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("conditional if/else", func(t *testing.T) {
		vars := map[string]any{"param": map[string]any{"verbose": "true"}}
		result := pipeline.Interpolate(`{{if .param.verbose}}detailed{{else}}concise{{end}}`, vars)
		if result != "detailed" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("if/else with falsy value", func(t *testing.T) {
		vars := map[string]any{"param": map[string]any{"verbose": ""}}
		result := pipeline.Interpolate(`{{if .param.verbose}}detailed{{else}}concise{{end}}`, vars)
		if result != "concise" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("catLines function via pipe", func(t *testing.T) {
		vars := map[string]any{"param": map[string]any{"text": "line1\nline2\nline3"}}
		result := pipeline.Interpolate("{{.param.text | catLines}}", vars)
		if result != "line1 line2 line3" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("cwd and param combined", func(t *testing.T) {
		vars := map[string]any{
			"cwd":   "/home/user/project",
			"param": map[string]any{"mode": ""},
		}
		result := pipeline.Interpolate(`Context: {{.cwd}}. Mode: {{.param.mode | default "auto"}}`, vars)
		if result != `Context: /home/user/project. Mode: auto` {
			t.Errorf("got %q", result)
		}
	})

	t.Run("steps pattern preserved for ResolveStepInputs", func(t *testing.T) {
		vars := map[string]any{}
		input := "Result: {{ steps.fetch.output }}"
		result := pipeline.Interpolate(input, vars)
		if result != input {
			t.Errorf("steps pattern should be preserved, got %q", result)
		}
	})
}
