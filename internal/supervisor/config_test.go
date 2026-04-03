package supervisor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/8op-org/gl1tch/internal/supervisor"
)

func TestLoadConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := supervisor.LoadConfig(dir)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Roles) != 0 {
		t.Errorf("expected empty roles, got: %v", cfg.Roles)
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	yaml := `
roles:
  diagnosis:
    provider: ollama
    model: llama3.2
  routing:
    provider: ollama
    model: mistral
`
	if err := os.WriteFile(filepath.Join(dir, "supervisor.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := supervisor.LoadConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	tests := []struct {
		role     string
		provider string
		model    string
	}{
		{"diagnosis", "ollama", "llama3.2"},
		{"routing", "ollama", "mistral"},
	}
	for _, tt := range tests {
		rc, ok := cfg.Roles[tt.role]
		if !ok {
			t.Errorf("role %q not found", tt.role)
			continue
		}
		if rc.Provider != tt.provider {
			t.Errorf("role %q: got provider %q, want %q", tt.role, rc.Provider, tt.provider)
		}
		if rc.Model != tt.model {
			t.Errorf("role %q: got model %q, want %q", tt.role, rc.Model, tt.model)
		}
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "supervisor.yaml"), []byte("{{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := supervisor.LoadConfig(dir)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	dir := "/some/cfg/dir"
	got := supervisor.DefaultConfigPath(dir)
	want := "/some/cfg/dir/supervisor.yaml"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLoadConfig_EmptyRolesAreNonNil(t *testing.T) {
	dir := t.TempDir()
	// Write a valid YAML with no roles key at all.
	if err := os.WriteFile(filepath.Join(dir, "supervisor.yaml"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := supervisor.LoadConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Roles == nil {
		t.Error("Roles should be non-nil map even when not present in YAML")
	}
}
