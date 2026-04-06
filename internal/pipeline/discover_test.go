package pipeline_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/8op-org/gl1tch/internal/pipeline"
)

func TestDiscoverPipelines(t *testing.T) {
	dir := t.TempDir()

	// Pipeline with explicit description field.
	writeFile(t, filepath.Join(dir, "with-desc.workflow.yaml"), `
name: with-desc
description: "Does the thing with the stuff"
version: "1"
steps: []
`)

	// Pipeline with leading comment, no description field.
	writeFile(t, filepath.Join(dir, "with-comment.workflow.yaml"), `# Runs the nightly digest job
name: with-comment
version: "1"
steps: []
`)

	// Pipeline with neither description nor comment — falls back to name.
	writeFile(t, filepath.Join(dir, "bare.workflow.yaml"), `
name: bare
version: "1"
steps: []
`)

	// Malformed YAML — should be skipped silently.
	writeFile(t, filepath.Join(dir, "broken.workflow.yaml"), `{{{not yaml`)

	// Non-pipeline file — should be ignored.
	writeFile(t, filepath.Join(dir, "other.yaml"), `name: ignored`)

	refs, err := pipeline.DiscoverPipelines(dir)
	if err != nil {
		t.Fatalf("DiscoverPipelines: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d: %v", len(refs), refs)
	}

	byName := make(map[string]pipeline.PipelineRef, len(refs))
	for _, r := range refs {
		byName[r.Name] = r
	}

	if got := byName["with-desc"].Description; got != "Does the thing with the stuff" {
		t.Errorf("with-desc description = %q, want explicit description", got)
	}
	if got := byName["with-comment"].Description; got != "Runs the nightly digest job" {
		t.Errorf("with-comment description = %q, want comment text", got)
	}
	if got := byName["bare"].Description; got != "bare" {
		t.Errorf("bare description = %q, want name fallback", got)
	}
}

func TestDiscoverPipelines_MissingDir(t *testing.T) {
	refs, err := pipeline.DiscoverPipelines("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected empty refs for missing dir, got %d", len(refs))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile %s: %v", path, err)
	}
}
