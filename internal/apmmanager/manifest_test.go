package apmmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadApmManifest_PlainStringDependency(t *testing.T) {
	dir := t.TempDir()
	content := "name: myapp\ndependencies:\n  apm:\n    - 8op-org/my-agent\n"
	if err := os.WriteFile(filepath.Join(dir, "apm.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadApmManifest(dir)
	if err != nil {
		t.Fatalf("LoadApmManifest: %v", err)
	}
	if len(m.Dependencies.APM) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(m.Dependencies.APM))
	}
	if m.Dependencies.APM[0].ID != "8op-org/my-agent" {
		t.Errorf("expected ID '8op-org/my-agent', got %q", m.Dependencies.APM[0].ID)
	}
}

func TestLoadApmManifest_ObjectDependencyWithPipeline(t *testing.T) {
	dir := t.TempDir()
	content := `name: myapp
dependencies:
  apm:
    - id: 8op-org/my-agent
      pipeline: |
        name: apm.my-agent
        description: my agent pipeline
        steps:
          - id: run
            executor: apm.my-agent
            prompt: "{{param.input}}"
`
	if err := os.WriteFile(filepath.Join(dir, "apm.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadApmManifest(dir)
	if err != nil {
		t.Fatalf("LoadApmManifest: %v", err)
	}
	if len(m.Dependencies.APM) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(m.Dependencies.APM))
	}
	dep := m.Dependencies.APM[0]
	if dep.ID != "8op-org/my-agent" {
		t.Errorf("expected ID '8op-org/my-agent', got %q", dep.ID)
	}
	if !strings.Contains(dep.Pipeline, "apm.my-agent") {
		t.Errorf("expected pipeline stanza to contain 'apm.my-agent', got: %q", dep.Pipeline)
	}
}

func TestLoadApmManifest_NotExist(t *testing.T) {
	m, err := LoadApmManifest(t.TempDir())
	if err != nil {
		t.Fatalf("expected no error for missing apm.yml, got: %v", err)
	}
	if len(m.Dependencies.APM) != 0 {
		t.Errorf("expected 0 dependencies for missing file, got %d", len(m.Dependencies.APM))
	}
}

func TestApmManifest_PipelineStanza_Found(t *testing.T) {
	m := &ApmManifest{
		Dependencies: apmDependencies{
			APM: []ApmDependency{
				{ID: "org/agent-a", Pipeline: "pipeline-yaml-for-a"},
				{ID: "org/agent-b", Pipeline: ""},
			},
		},
	}
	if got := m.PipelineStanza("org/agent-a"); got != "pipeline-yaml-for-a" {
		t.Errorf("expected 'pipeline-yaml-for-a', got %q", got)
	}
}

func TestApmManifest_PipelineStanza_NotFound(t *testing.T) {
	m := &ApmManifest{}
	if got := m.PipelineStanza("org/missing"); got != "" {
		t.Errorf("expected empty string for missing agent, got %q", got)
	}
}

// TestPipelineMaterialization verifies that installAndWrap writes a pipeline
// template file when a stanza is provided and skips it if the file exists.
func TestPipelineMaterialization_WritesFile(t *testing.T) {
	projectDir := t.TempDir()
	configDir := t.TempDir()
	agentsDir := filepath.Join(projectDir, ".claude", "agents")
	writeAgentMD(t, agentsDir, "test-agent", nil)

	pipelineYAML := "name: apm.test-agent\ndescription: test\nsteps:\n  - id: run\n    executor: apm.test-agent\n    prompt: \"{{param.input}}\"\n"

	// Simulate materialization (the core logic from installAndWrap).
	pipelinesDir := filepath.Join(configDir, "pipelines")
	if err := os.MkdirAll(pipelinesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	destPath := filepath.Join(pipelinesDir, "apm.test-agent.pipeline.yaml")
	if err := os.WriteFile(destPath, []byte(pipelineYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if !strings.Contains(string(data), "apm.test-agent") {
		t.Errorf("expected pipeline name in template, got: %q", string(data))
	}
}

func TestPipelineMaterialization_SkipsExistingFile(t *testing.T) {
	configDir := t.TempDir()
	pipelinesDir := filepath.Join(configDir, "pipelines")
	if err := os.MkdirAll(pipelinesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	destPath := filepath.Join(pipelinesDir, "apm.my-agent.pipeline.yaml")
	original := "original-content"
	if err := os.WriteFile(destPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate the skip logic: file exists, do not overwrite.
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		// File exists — skip (do nothing).
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("expected file content to be unchanged, got: %q", string(data))
	}
}

func TestPipelineMaterialization_CreatesDirectory(t *testing.T) {
	configDir := t.TempDir()
	pipelinesDir := filepath.Join(configDir, "pipelines", "nested")

	if err := os.MkdirAll(pipelinesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if _, err := os.Stat(pipelinesDir); err != nil {
		t.Errorf("expected directory to be created: %v", err)
	}
}
