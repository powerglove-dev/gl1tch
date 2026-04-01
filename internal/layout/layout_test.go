package layout

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/8op-org/gl1tch/internal/widgetdispatch"
)

// mockDispatcher records which widgets were dispatched.
type mockDispatcher struct {
	calls []string
}

func (m *mockDispatcher) Dispatch(_ context.Context, name string, _ widgetdispatch.Options) error {
	m.calls = append(m.calls, name)
	return nil
}

// TestLoadConfig_FileAbsent verifies that a missing file returns an empty
// config and nil error.
func TestLoadConfig_FileAbsent(t *testing.T) {
	cfg, err := LoadConfig("/tmp/glitch-layout-does-not-exist-xyz.yaml")
	if err != nil {
		t.Fatalf("LoadConfig absent: unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig absent: expected non-nil config")
	}
	if len(cfg.Panes) != 0 {
		t.Errorf("LoadConfig absent: expected 0 panes, got %d", len(cfg.Panes))
	}
}

// TestLoadConfig_EmptyPanes verifies that a file with an empty panes list
// parses correctly.
func TestLoadConfig_EmptyPanes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "layout.yaml")
	if err := os.WriteFile(path, []byte("panes: []\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig empty: %v", err)
	}
	if len(cfg.Panes) != 0 {
		t.Errorf("expected 0 panes, got %d", len(cfg.Panes))
	}
}

// TestLoadConfig_Valid verifies that a well-formed layout.yaml is parsed.
func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "layout.yaml")
	content := `
panes:
  - name: welcome
    widget: welcome
    position: right
    size: 50%
  - name: sysop
    widget: sysop
    position: right
    size: 40%
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig valid: %v", err)
	}
	if len(cfg.Panes) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(cfg.Panes))
	}
	if cfg.Panes[0].Name != "welcome" {
		t.Errorf("panes[0].Name = %q; want welcome", cfg.Panes[0].Name)
	}
	if cfg.Panes[1].Widget != "sysop" {
		t.Errorf("panes[1].Widget = %q; want sysop", cfg.Panes[1].Widget)
	}
	if cfg.Panes[0].Size != "50%" {
		t.Errorf("panes[0].Size = %q; want 50%%", cfg.Panes[0].Size)
	}
}

// TestApply_InvalidPosition verifies that a pane with an invalid position is
// skipped and Apply returns nil.
func TestApply_InvalidPosition(t *testing.T) {
	cfg := &Config{
		Panes: []Pane{
			{Name: "bad", Widget: "welcome", Position: "diagonal", Size: "50%"},
		},
	}
	d := &mockDispatcher{}
	err := Apply(context.Background(), cfg, d)
	if err != nil {
		t.Fatalf("Apply with invalid position: unexpected error: %v", err)
	}
	if len(d.calls) != 0 {
		t.Errorf("expected 0 dispatcher calls, got %d: %v", len(d.calls), d.calls)
	}
}

// TestApply_NoPanes verifies that Apply with an empty config returns nil and
// does not call the dispatcher.
func TestApply_NoPanes(t *testing.T) {
	cfg := &Config{}
	d := &mockDispatcher{}
	err := Apply(context.Background(), cfg, d)
	if err != nil {
		t.Fatalf("Apply no panes: unexpected error: %v", err)
	}
	if len(d.calls) != 0 {
		t.Errorf("expected 0 dispatcher calls, got %d", len(d.calls))
	}
}
