package promptbuilder_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/powerglove-dev/gl1tch/internal/pipeline"
	"github.com/powerglove-dev/gl1tch/internal/promptbuilder"
)

func TestSave_WritesYAML(t *testing.T) {
	dir := t.TempDir()
	m := promptbuilder.New(nil)
	m.SetName("my-test-pipeline")
	m.AddStep(pipeline.Step{ID: "s1", Type: "input", Prompt: "Enter:"})
	m.AddStep(pipeline.Step{ID: "s2", Executor: "claude", Model: "claude-sonnet-4-6"})

	outPath := filepath.Join(dir, "my-test-pipeline.pipeline.yaml")
	if err := promptbuilder.Save(m, outPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "my-test-pipeline") {
		t.Errorf("expected pipeline name in YAML, got:\n%s", content)
	}
	if !strings.Contains(content, "claude-sonnet-4-6") {
		t.Errorf("expected model in YAML, got:\n%s", content)
	}
}

func TestSave_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	m := promptbuilder.New(nil)
	m.SetName("created-pipeline")

	outPath := filepath.Join(dir, "created-pipeline.pipeline.yaml")
	if err := promptbuilder.Save(m, outPath); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Error("expected file to exist after Save")
	}
}
