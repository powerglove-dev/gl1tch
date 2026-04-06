//go:build integration

package pipeline_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPipelineRunIntegration(t *testing.T) {
	// Build the orcai binary
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "glitch")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	// Find module root
	_, file, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Join(filepath.Dir(file), "..", "..")

	buildCmd := exec.Command("go", "build", "-o", binary, ".")
	buildCmd.Dir = moduleRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Find fixture
	fixture, err := filepath.Abs(filepath.Join(filepath.Dir(file), "testdata", "simple.workflow.yaml"))
	if err != nil {
		t.Fatalf("fixture path: %v", err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("fixture not found: %v", err)
	}

	// Run pipeline
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "pipeline", "run", fixture)
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pipeline run failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "integration-ok") {
		t.Fatalf("expected 'integration-ok' in output, got:\n%s", out)
	}
}
