package bootstrap_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/8op-org/gl1tch/internal/bootstrap"
)

func TestWriteReloadMarker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgDir := filepath.Join(home, ".config", "glitch")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := bootstrap.WriteReloadMarker(); err != nil {
		t.Fatalf("WriteReloadMarker: %v", err)
	}

	marker := filepath.Join(cfgDir, ".reload")
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("reload marker not created: %v", err)
	}
}
