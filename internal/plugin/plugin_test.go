package plugin_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adam-stokes/orcai/internal/plugin"
)

func TestManager_Empty(t *testing.T) {
	m := plugin.NewManager()
	if len(m.List()) != 0 {
		t.Errorf("expected empty manager, got %d plugins", len(m.List()))
	}
}

func TestManager_Register(t *testing.T) {
	m := plugin.NewManager()
	p := &plugin.StubPlugin{PluginName: "test"}
	m.Register(p)
	plugins := m.List()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name() != "test" {
		t.Errorf("expected name 'test', got %q", plugins[0].Name())
	}
}

func TestManager_Get(t *testing.T) {
	m := plugin.NewManager()
	m.Register(&plugin.StubPlugin{PluginName: "alpha"})
	m.Register(&plugin.StubPlugin{PluginName: "beta"})

	p, ok := m.Get("alpha")
	if !ok {
		t.Fatal("expected to find 'alpha'")
	}
	if p.Name() != "alpha" {
		t.Errorf("got wrong plugin: %q", p.Name())
	}

	_, ok = m.Get("missing")
	if ok {
		t.Error("expected not found for 'missing'")
	}
}

func TestManager_LoadWrappersFromDir_Valid(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		content := "name: " + name + "\ncommand: echo\n"
		if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(content), 0o644); err != nil {
			t.Fatalf("write sidecar: %v", err)
		}
	}

	m := plugin.NewManager()
	errs := m.LoadWrappersFromDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for _, name := range []string{"alpha", "beta"} {
		if _, ok := m.Get(name); !ok {
			t.Errorf("expected plugin %q to be registered", name)
		}
	}
}

func TestManager_LoadWrappersFromDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	m := plugin.NewManager()
	errs := m.LoadWrappersFromDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(m.List()) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(m.List()))
	}
}
