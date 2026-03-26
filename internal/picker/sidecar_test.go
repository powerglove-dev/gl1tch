package picker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSidecarMeta_ReturnsModelsFromFixture(t *testing.T) {
	dir := t.TempDir()
	wrappersDir := filepath.Join(dir, "wrappers")
	if err := os.MkdirAll(wrappersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `name: test-provider
command: /bin/true
models:
  - id: model-a
    label: "Model A"
  - id: model-b
    label: "Model B"
`
	if err := os.WriteFile(filepath.Join(wrappersDir, "test-provider.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	got := loadSidecarMeta(dir)
	meta, ok := got["test-provider"]
	if !ok {
		t.Fatal("loadSidecarMeta: expected entry for test-provider")
	}
	if len(meta.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(meta.Models))
	}
	if meta.Models[0].ID != "model-a" || meta.Models[0].Label != "Model A" {
		t.Errorf("models[0] = %+v, want {ID:model-a Label:Model A}", meta.Models[0])
	}
	if meta.Models[1].ID != "model-b" || meta.Models[1].Label != "Model B" {
		t.Errorf("models[1] = %+v, want {ID:model-b Label:Model B}", meta.Models[1])
	}
}

func TestLoadSidecarMeta_DefaultKindIsAgent(t *testing.T) {
	dir := t.TempDir()
	wrappersDir := filepath.Join(dir, "wrappers")
	if err := os.MkdirAll(wrappersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No kind field — should default to "agent".
	yaml := `name: no-kind-provider
command: /bin/true
`
	if err := os.WriteFile(filepath.Join(wrappersDir, "no-kind-provider.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	got := loadSidecarMeta(dir)
	meta, ok := got["no-kind-provider"]
	if !ok {
		t.Fatal("loadSidecarMeta: expected entry for no-kind-provider")
	}
	if meta.Kind != "agent" {
		t.Errorf("expected kind=agent, got %q", meta.Kind)
	}
}

func TestLoadSidecarMeta_KindToolIsPreserved(t *testing.T) {
	dir := t.TempDir()
	wrappersDir := filepath.Join(dir, "wrappers")
	if err := os.MkdirAll(wrappersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `name: tool-plugin
command: /bin/true
kind: tool
`
	if err := os.WriteFile(filepath.Join(wrappersDir, "tool-plugin.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	got := loadSidecarMeta(dir)
	meta, ok := got["tool-plugin"]
	if !ok {
		t.Fatal("loadSidecarMeta: expected entry for tool-plugin")
	}
	if meta.Kind != "tool" {
		t.Errorf("expected kind=tool, got %q", meta.Kind)
	}
}

func TestLoadSidecarMeta_ToolFilteredFromExtras(t *testing.T) {
	dir := t.TempDir()
	wrappersDir := filepath.Join(dir, "wrappers")
	if err := os.MkdirAll(wrappersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	agentYAML := `name: my-agent
command: /bin/true
kind: agent
`
	toolYAML := `name: my-tool
command: /bin/true
kind: tool
`
	if err := os.WriteFile(filepath.Join(wrappersDir, "my-agent.yaml"), []byte(agentYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wrappersDir, "my-tool.yaml"), []byte(toolYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	got := loadSidecarMeta(dir)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}

	agentMeta := got["my-agent"]
	if agentMeta.Kind != "agent" {
		t.Errorf("my-agent: expected kind=agent, got %q", agentMeta.Kind)
	}

	toolMeta := got["my-tool"]
	if toolMeta.Kind != "tool" {
		t.Errorf("my-tool: expected kind=tool, got %q", toolMeta.Kind)
	}
}

func TestLoadSidecarMeta_EmptyWhenNoModels(t *testing.T) {
	dir := t.TempDir()
	wrappersDir := filepath.Join(dir, "wrappers")
	if err := os.MkdirAll(wrappersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `name: no-models-provider
command: /bin/true
`
	if err := os.WriteFile(filepath.Join(wrappersDir, "no-models-provider.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	got := loadSidecarMeta(dir)
	meta := got["no-models-provider"]
	if len(meta.Models) != 0 {
		t.Errorf("expected empty models list, got %d entries", len(meta.Models))
	}
}

func TestLoadSidecarMeta_MissingDirReturnsEmpty(t *testing.T) {
	got := loadSidecarMeta("/tmp/orcai-nonexistent-dir-xyz")
	if len(got) != 0 {
		t.Errorf("expected empty map for missing dir, got %d entries", len(got))
	}
}
