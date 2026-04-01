package executor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/powerglove-dev/gl1tch/internal/executor"
)

func writeSidecar(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writeSidecar: %v", err)
	}
}

func TestLoadWrappers_Valid(t *testing.T) {
	dir := t.TempDir()
	writeSidecar(t, dir, "tool-a.yaml", "name: tool-a\ncommand: echo\n")
	writeSidecar(t, dir, "tool-b.yaml", "name: tool-b\ncommand: cat\n")

	executors, errs := executor.LoadWrappers(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(executors) != 2 {
		t.Errorf("expected 2 executors, got %d", len(executors))
	}
}

func TestLoadWrappers_DirNotExist(t *testing.T) {
	executors, errs := executor.LoadWrappers("/nonexistent/wrappers/dir")
	if errs != nil {
		t.Errorf("expected nil errors for missing dir, got %v", errs)
	}
	if len(executors) != 0 {
		t.Errorf("expected 0 executors, got %d", len(executors))
	}
}

func TestLoadWrappers_MixedValidity(t *testing.T) {
	dir := t.TempDir()
	writeSidecar(t, dir, "valid.yaml", "name: valid-tool\ncommand: echo\n")
	writeSidecar(t, dir, "invalid.yaml", "name: broken\n") // missing command

	executors, errs := executor.LoadWrappers(dir)
	if len(executors) != 1 {
		t.Errorf("expected 1 executor, got %d", len(executors))
	}
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
}

func TestLoadWrappers_IgnoresNonYAML(t *testing.T) {
	dir := t.TempDir()
	writeSidecar(t, dir, "tool.yaml", "name: real-tool\ncommand: echo\n")
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatalf("write txt: %v", err)
	}

	executors, errs := executor.LoadWrappers(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(executors) != 1 {
		t.Errorf("expected 1 executor, got %d", len(executors))
	}
}
