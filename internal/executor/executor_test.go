package executor_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/powerglove-dev/gl1tch/internal/executor"
)

func TestManager_Empty(t *testing.T) {
	m := executor.NewManager()
	if len(m.List()) != 0 {
		t.Errorf("expected empty manager, got %d executors", len(m.List()))
	}
}

func TestManager_Register(t *testing.T) {
	m := executor.NewManager()
	e := &executor.StubExecutor{ExecutorName: "test"}
	if err := m.Register(e); err != nil {
		t.Fatalf("Register: %v", err)
	}
	executors := m.List()
	if len(executors) != 1 {
		t.Fatalf("expected 1 executor, got %d", len(executors))
	}
	if executors[0].Name() != "test" {
		t.Errorf("expected name 'test', got %q", executors[0].Name())
	}
}

func TestManager_Get(t *testing.T) {
	m := executor.NewManager()
	if err := m.Register(&executor.StubExecutor{ExecutorName: "alpha"}); err != nil {
		t.Fatalf("Register alpha: %v", err)
	}
	if err := m.Register(&executor.StubExecutor{ExecutorName: "beta"}); err != nil {
		t.Fatalf("Register beta: %v", err)
	}

	e, ok := m.Get("alpha")
	if !ok {
		t.Fatal("expected to find 'alpha'")
	}
	if e.Name() != "alpha" {
		t.Errorf("got wrong executor: %q", e.Name())
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

	m := executor.NewManager()
	errs := m.LoadWrappersFromDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for _, name := range []string{"alpha", "beta"} {
		if _, ok := m.Get(name); !ok {
			t.Errorf("expected executor %q to be registered", name)
		}
	}
}

func TestManager_Register_Duplicate(t *testing.T) {
	m := executor.NewManager()
	if err := m.Register(&executor.StubExecutor{ExecutorName: "dup"}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := m.Register(&executor.StubExecutor{ExecutorName: "dup"})
	if err == nil {
		t.Fatal("expected error on duplicate registration, got nil")
	}
	// First registration must still be intact.
	if _, ok := m.Get("dup"); !ok {
		t.Error("original executor should still be registered after duplicate attempt")
	}
}

func TestManager_LoadWrappersFromDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	m := executor.NewManager()
	errs := m.LoadWrappersFromDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(m.List()) != 0 {
		t.Errorf("expected 0 executors, got %d", len(m.List()))
	}
}

// TestHierarchicalNaming verifies category.action lookup and _action var injection.
func TestHierarchicalNaming(t *testing.T) {
	m := executor.NewManager()

	// Register an executor as the category handler.
	var capturedVars map[string]string
	base := &executor.StubExecutor{
		ExecutorName: "providers.claude",
		ExecuteFn: func(ctx context.Context, input string, vars map[string]string, w io.Writer) error {
			capturedVars = vars
			_, err := w.Write([]byte("ok"))
			return err
		},
	}

	if err := m.Register(base); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Register under category "providers.claude" with action "chat".
	m.RegisterCategory("providers.claude", "chat", base)

	// Direct lookup should still work.
	e, ok := m.Get("providers.claude")
	if !ok {
		t.Fatal("expected direct lookup to succeed")
	}
	if e.Name() != "providers.claude" {
		t.Errorf("expected direct name, got %q", e.Name())
	}

	// Category.action lookup.
	eAction, ok := m.Get("providers.claude.chat")
	if !ok {
		t.Fatal("expected category.action lookup to succeed")
	}

	// Execute the wrapper and verify _action is injected.
	capturedVars = nil
	ctx := context.Background()
	var buf bytes.Buffer
	if err := eAction.Execute(ctx, "test", map[string]string{}, &buf); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if capturedVars["_action"] != "chat" {
		t.Errorf("expected _action='chat', got %q", capturedVars["_action"])
	}

	// Lookup for unknown action still uses category if category exists.
	eOther, ok := m.Get("providers.claude.summarize")
	if !ok {
		t.Fatal("expected category lookup with unknown action to succeed")
	}
	capturedVars = nil
	if err := eOther.Execute(ctx, "test", map[string]string{}, &buf); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if capturedVars["_action"] != "summarize" {
		t.Errorf("expected _action='summarize', got %q", capturedVars["_action"])
	}

	// Unknown top-level name returns not found.
	_, ok = m.Get("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent")
	}
}
