package pipeline_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/adam-stokes/orcai/internal/pipeline"
	"github.com/adam-stokes/orcai/internal/plugin"
)

// TestStepOutputs_HappyPath verifies that step A's declared output is accessible
// via {{ steps.A.body }} in step B's prompt.
func TestStepOutputs_HappyPath(t *testing.T) {
	mgr := plugin.NewManager()
	var capturedB string

	_ = mgr.Register(&plugin.StubPlugin{
		PluginName: "step-a-plugin",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte("output from step A"))
			return err
		},
	})
	_ = mgr.Register(&plugin.StubPlugin{
		PluginName: "step-b-plugin",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			capturedB = input
			_, err := w.Write([]byte("step B got: " + input))
			return err
		},
	})

	p := &pipeline.Pipeline{
		Name: "step-outputs-happy",
		Steps: []pipeline.Step{
			{
				ID:      "step-a",
				Plugin:  "step-a-plugin",
				Outputs: map[string]string{"body": "string"},
			},
			{
				ID:     "step-b",
				Plugin: "step-b-plugin",
				Prompt: "Data: {{ steps.step-a.body }}",
				Needs:  []string{"step-a"},
			},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(capturedB, "output from step A") {
		t.Errorf("expected step A output in step B prompt, got: %q", capturedB)
	}
}

// TestStepOutputs_MissingKey verifies that referencing a nonexistent step output
// returns a descriptive error.
func TestStepOutputs_MissingKey(t *testing.T) {
	mgr := plugin.NewManager()

	_ = mgr.Register(&plugin.StubPlugin{
		PluginName: "step-a-plugin",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte("some output"))
			return err
		},
	})
	_ = mgr.Register(&plugin.StubPlugin{
		PluginName: "step-b-plugin",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte(input))
			return err
		},
	})

	p := &pipeline.Pipeline{
		Name: "step-outputs-missing",
		Steps: []pipeline.Step{
			{
				ID:     "step-a",
				Plugin: "step-a-plugin",
				// No Outputs declared — so "result" key doesn't exist.
			},
			{
				ID:     "step-b",
				Plugin: "step-b-plugin",
				Prompt: "Data: {{ steps.step-a.result }}",
				Needs:  []string{"step-a"},
			},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "")
	if err == nil {
		t.Fatal("expected error for missing step output, got nil")
	}
	if !strings.Contains(err.Error(), "step-a") || !strings.Contains(err.Error(), "result") {
		t.Errorf("expected error mentioning step-a and result key, got: %v", err)
	}
}

// TestStepOutputs_ThreeStepChain verifies A → B → C data flow.
func TestStepOutputs_ThreeStepChain(t *testing.T) {
	mgr := plugin.NewManager()
	var capturedC string

	_ = mgr.Register(&plugin.StubPlugin{
		PluginName: "a-plugin",
		ExecuteFn: func(_ context.Context, _ string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte("value-from-A"))
			return err
		},
	})
	_ = mgr.Register(&plugin.StubPlugin{
		PluginName: "b-plugin",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			_, err := w.Write([]byte("B-received: " + input))
			return err
		},
	})
	_ = mgr.Register(&plugin.StubPlugin{
		PluginName: "c-plugin",
		ExecuteFn: func(_ context.Context, input string, _ map[string]string, w io.Writer) error {
			capturedC = input
			_, err := w.Write([]byte("C-received: " + input))
			return err
		},
	})

	p := &pipeline.Pipeline{
		Name: "three-step-chain",
		Steps: []pipeline.Step{
			{
				ID:      "step-a",
				Plugin:  "a-plugin",
				Outputs: map[string]string{"data": "string"},
			},
			{
				ID:      "step-b",
				Plugin:  "b-plugin",
				Prompt:  "A said: {{ steps.step-a.data }}",
				Needs:   []string{"step-a"},
				Outputs: map[string]string{"result": "string"},
			},
			{
				ID:     "step-c",
				Plugin: "c-plugin",
				Prompt: "B result: {{ steps.step-b.result }}",
				Needs:  []string{"step-b"},
			},
		},
	}

	_, err := pipeline.Run(context.Background(), p, mgr, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(capturedC, "value-from-A") {
		t.Errorf("expected A's value to flow through to C via B, got: %q", capturedC)
	}
}
