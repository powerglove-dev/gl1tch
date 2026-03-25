package pipeline_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/adam-stokes/orcai/internal/pipeline"
	"github.com/adam-stokes/orcai/internal/plugin"
)

func makeWritePlugin(name, output string) *plugin.StubPlugin {
	return &plugin.StubPlugin{
		PluginName: name,
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte(output))
			return err
		},
	}
}

func TestRunner_LinearPipeline(t *testing.T) {
	p := &pipeline.Pipeline{
		Name:    "linear-test",
		Version: "1.0",
		Steps: []pipeline.Step{
			{ID: "s1", Type: "input"},
			{ID: "s2", Plugin: "echo"},
			{ID: "s3", Type: "output"},
		},
	}

	mgr := plugin.NewManager()
	if err := mgr.Register(&plugin.StubPlugin{
		PluginName: "echo",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte("echoed: " + input))
			return err
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := pipeline.Run(context.Background(), p, mgr, "hello world", pipeline.NoopPublisher{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(result, "echoed: hello world") {
		t.Errorf("expected 'echoed: hello world' in output, got %q", result)
	}
}

func TestRunner_ConditionalBranch_Then(t *testing.T) {
	p := &pipeline.Pipeline{
		Name: "branch-test",
		Steps: []pipeline.Step{
			{ID: "s1", Type: "input"},
			{
				ID:     "s2",
				Plugin: "classifier",
				Condition: pipeline.Condition{
					If:   "contains:go",
					Then: "golang-step",
					Else: "other-step",
				},
			},
			{ID: "golang-step", Plugin: "go-handler"},
			{ID: "other-step", Plugin: "other-handler"},
			{ID: "out", Type: "output"},
		},
	}

	mgr := plugin.NewManager()
	for _, p := range []plugin.Plugin{
		makeWritePlugin("classifier", "golang rocks"),
		makeWritePlugin("go-handler", "handled by go"),
		makeWritePlugin("other-handler", "handled by other"),
	} {
		if err := mgr.Register(p); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}

	result, err := pipeline.Run(context.Background(), p, mgr, "golang rocks", pipeline.NoopPublisher{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(result, "handled by go") {
		t.Errorf("expected 'handled by go', got %q", result)
	}
}

func TestRunner_ConditionalBranch_Else(t *testing.T) {
	p := &pipeline.Pipeline{
		Name: "branch-else-test",
		Steps: []pipeline.Step{
			{ID: "s1", Type: "input"},
			{
				ID:     "s2",
				Plugin: "classifier",
				Condition: pipeline.Condition{
					If:   "contains:python",
					Then: "python-step",
					Else: "default-step",
				},
			},
			{ID: "python-step", Plugin: "py-handler"},
			{ID: "default-step", Plugin: "default-handler"},
			{ID: "out", Type: "output"},
		},
	}

	mgr := plugin.NewManager()
	for _, p := range []plugin.Plugin{
		makeWritePlugin("classifier", "golang rocks"),
		makeWritePlugin("py-handler", "python handler"),
		makeWritePlugin("default-handler", "default handler"),
	} {
		if err := mgr.Register(p); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}

	result, err := pipeline.Run(context.Background(), p, mgr, "golang rocks", pipeline.NoopPublisher{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(result, "default handler") {
		t.Errorf("expected 'default handler', got %q", result)
	}
}

func TestRunner_TemplateInterpolation(t *testing.T) {
	p := &pipeline.Pipeline{
		Name: "interp-test",
		Steps: []pipeline.Step{
			{ID: "s1", Type: "input"},
			{ID: "s2", Plugin: "upper", Prompt: "input was: {{s1.out}}"},
			{ID: "out", Type: "output"},
		},
	}

	mgr := plugin.NewManager()
	var capturedInput string
	if err := mgr.Register(&plugin.StubPlugin{
		PluginName: "upper",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			capturedInput = input
			_, err := w.Write([]byte("done"))
			return err
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "hello", pipeline.NoopPublisher{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(capturedInput, "input was: hello") {
		t.Errorf("expected interpolated input, got %q", capturedInput)
	}
}

func TestRunner_MissingPlugin(t *testing.T) {
	p := &pipeline.Pipeline{
		Name: "missing-test",
		Steps: []pipeline.Step{
			{ID: "s1", Type: "input"},
			{ID: "s2", Plugin: "nonexistent"},
			{ID: "out", Type: "output"},
		},
	}
	mgr := plugin.NewManager() // empty — no plugins registered intentionally
	_, err := pipeline.Run(context.Background(), p, mgr, "hello", pipeline.NoopPublisher{})
	if err == nil {
		t.Error("expected error for missing plugin")
	}
}
