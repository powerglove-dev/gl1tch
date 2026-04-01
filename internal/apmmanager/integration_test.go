//go:build integration

package apmmanager

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	executorpkg "github.com/8op-org/gl1tch/internal/executor"
	"github.com/8op-org/gl1tch/internal/store"
)

// TestIntegration_BrainSeeding verifies that after installAndWrap, APM agent
// capabilities appear in the brain store with the correct tags.
// Skips if the `apm` CLI is unavailable.
func TestIntegration_BrainSeeding(t *testing.T) {
	if _, err := exec.LookPath("apm"); err != nil {
		t.Skip("apm CLI not found — skipping integration test")
	}

	projectDir := t.TempDir()
	s, err := store.OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	mgr := executorpkg.NewManager()
	a := Agent{
		ID:           "8op-org/gl1tch",
		Name:         "gl1tch",
		Capabilities: []string{"test-capability"},
	}

	_, _, installErr := installAndWrap(context.Background(), projectDir, a, mgr, s, "", "")
	if installErr != nil {
		t.Fatalf("installAndWrap: %v", installErr)
	}

	ctx := context.Background()
	notes, err := s.CapabilityNotes(ctx)
	if err != nil {
		t.Fatalf("CapabilityNotes: %v", err)
	}

	found := false
	for _, n := range notes {
		if strings.Contains(n.Tags, "source:apm") && strings.Contains(n.Body, "test-capability") {
			found = true
		}
	}
	if !found {
		t.Error("expected APM capability brain note to be seeded after install")
	}
}

// TestIntegration_PipelineMaterialization verifies that after installAndWrap
// with a pipeline stanza, the template file is written to the config dir.
// Skips if the `apm` CLI is unavailable.
func TestIntegration_PipelineMaterialization(t *testing.T) {
	if _, err := os.Stat("apm"); err != nil {
		t.Skip("apm CLI not found — skipping integration test")
	}

	projectDir := t.TempDir()
	configDir := t.TempDir()
	mgr := executorpkg.NewManager()

	pipelineYAML := "name: apm.gl1tch\ndescription: test\nsteps:\n  - id: run\n    executor: apm.gl1tch\n    prompt: \"{{param.input}}\"\n"
	a := Agent{
		ID:   "8op-org/gl1tch",
		Name: "gl1tch",
	}

	_, _, err := installAndWrap(context.Background(), projectDir, a, mgr, nil, pipelineYAML, configDir)
	if err != nil {
		t.Fatalf("installAndWrap: %v", err)
	}

	destPath := filepath.Join(configDir, "pipelines", "apm.gl1tch.pipeline.yaml")
	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("expected pipeline template at %s: %v", destPath, err)
	}
}
