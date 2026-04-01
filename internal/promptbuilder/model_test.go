package promptbuilder_test

import (
	"testing"

	"github.com/powerglove-dev/gl1tch/internal/pipeline"
	"github.com/powerglove-dev/gl1tch/internal/promptbuilder"
)

func TestModel_New(t *testing.T) {
	m := promptbuilder.New(nil)
	if m == nil {
		t.Fatal("expected non-nil model")
	}
}

func TestModel_AddStep(t *testing.T) {
	m := promptbuilder.New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Type: "input", Prompt: "Enter:"})
	m.AddStep(pipeline.Step{ID: "s2", Executor: "claude"})
	if len(m.Steps()) != 2 {
		t.Errorf("expected 2 steps, got %d", len(m.Steps()))
	}
}

func TestModel_SelectStep(t *testing.T) {
	m := promptbuilder.New(nil)
	m.AddStep(pipeline.Step{ID: "s1"})
	m.AddStep(pipeline.Step{ID: "s2"})
	m.SelectStep(1)
	if m.SelectedIndex() != 1 {
		t.Errorf("expected selected index 1, got %d", m.SelectedIndex())
	}
}

func TestModel_SelectStep_Clamps(t *testing.T) {
	m := promptbuilder.New(nil)
	m.AddStep(pipeline.Step{ID: "s1"})
	m.SelectStep(99)
	if m.SelectedIndex() != 0 {
		t.Errorf("expected clamped index 0, got %d", m.SelectedIndex())
	}
	m.SelectStep(-5)
	if m.SelectedIndex() != 0 {
		t.Errorf("expected clamped index 0, got %d", m.SelectedIndex())
	}
}

func TestModel_SetName(t *testing.T) {
	m := promptbuilder.New(nil)
	m.SetName("my-pipeline")
	if m.Name() != "my-pipeline" {
		t.Errorf("expected 'my-pipeline', got %q", m.Name())
	}
}

func TestModel_ToPipeline(t *testing.T) {
	m := promptbuilder.New(nil)
	m.SetName("test")
	m.AddStep(pipeline.Step{ID: "s1", Type: "input"})
	m.AddStep(pipeline.Step{ID: "s2", Executor: "claude"})
	p := m.ToPipeline()
	if p.Name != "test" {
		t.Errorf("expected name 'test', got %q", p.Name)
	}
	if len(p.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(p.Steps))
	}
}

func TestModel_UpdateStep(t *testing.T) {
	m := promptbuilder.New(nil)
	m.AddStep(pipeline.Step{ID: "a", Executor: "claude"})
	m.UpdateStep(0, pipeline.Step{ID: "a", Executor: "gemini", Model: "gemini-2.0-flash"})
	if m.Steps()[0].Executor != "gemini" {
		t.Fatalf("expected gemini, got %s", m.Steps()[0].Executor)
	}
	if m.Steps()[0].Model != "gemini-2.0-flash" {
		t.Fatalf("expected gemini-2.0-flash, got %s", m.Steps()[0].Model)
	}
}

func TestModel_UpdateStep_OutOfRange(t *testing.T) {
	m := promptbuilder.New(nil)
	m.AddStep(pipeline.Step{ID: "a", Executor: "claude"})
	m.UpdateStep(99, pipeline.Step{ID: "x"}) // should be a no-op
	if len(m.Steps()) != 1 {
		t.Fatalf("expected 1 step, got %d", len(m.Steps()))
	}
}
