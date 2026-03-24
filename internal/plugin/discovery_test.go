package plugin_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adam-stokes/orcai/internal/plugin"
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

	plugins, errs := plugin.LoadWrappers(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(plugins))
	}
}

func TestLoadWrappers_DirNotExist(t *testing.T) {
	plugins, errs := plugin.LoadWrappers("/nonexistent/wrappers/dir")
	if errs != nil {
		t.Errorf("expected nil errors for missing dir, got %v", errs)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestLoadWrappers_MixedValidity(t *testing.T) {
	dir := t.TempDir()
	writeSidecar(t, dir, "valid.yaml", "name: valid-tool\ncommand: echo\n")
	writeSidecar(t, dir, "invalid.yaml", "name: broken\n") // missing command

	plugins, errs := plugin.LoadWrappers(dir)
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(plugins))
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

	plugins, errs := plugin.LoadWrappers(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(plugins))
	}
}
