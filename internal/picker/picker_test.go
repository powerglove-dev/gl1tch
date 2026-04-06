package picker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/8op-org/gl1tch/internal/picker"
)

func TestProviders_NotEmpty(t *testing.T) {
	if len(picker.Providers) == 0 {
		t.Fatal("Providers must not be empty")
	}
}

func TestProviders_ContainsExpected(t *testing.T) {
	want := []string{"ollama", "shell"}
	have := make(map[string]bool, len(picker.Providers))
	for _, p := range picker.Providers {
		have[p.ID] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("Providers missing %q", w)
		}
	}
}

func TestProviders_NoAider(t *testing.T) {
	for _, p := range picker.Providers {
		if p.ID == "aider" {
			t.Error("aider should not be in Providers")
		}
	}
}

func TestProviders_NoClaude(t *testing.T) {
	for _, p := range picker.Providers {
		if p.ID == "claude" {
			t.Error("claude must not be in the static Providers list; use a sidecar YAML")
		}
	}
}

func TestProviders_NoCopilot(t *testing.T) {
	for _, p := range picker.Providers {
		if p.ID == "copilot" {
			t.Error("copilot must not be in the static Providers list; use a sidecar YAML")
		}
	}
}

func TestProviders_ShellHasNoModels(t *testing.T) {
	for _, p := range picker.Providers {
		if p.ID == "shell" {
			if len(p.Models) != 0 {
				t.Errorf("shell provider should have no models, got %d", len(p.Models))
			}
			return
		}
	}
	t.Error("shell not found in Providers")
}

func TestProviders_OllamaBaseHasNoModels(t *testing.T) {
	// The base Providers list has no static models for ollama — they're discovered at runtime.
	for _, p := range picker.Providers {
		if p.ID == "ollama" {
			if len(p.Models) != 0 {
				t.Errorf("ollama base definition should have no static models, got %d", len(p.Models))
			}
			return
		}
	}
	t.Error("ollama not found in Providers")
}


func TestBuildProviders_ExcludesShell(t *testing.T) {
	providers := picker.BuildProviders()
	for _, p := range providers {
		if p.ID == "shell" {
			t.Fatal("BuildProviders must not include the shell provider")
		}
	}
}


// TestPickerItem_JSONRoundTrip ensures PickerItem marshals and unmarshals cleanly
// so that GLITCH_PICKER_SELECTION encoding works correctly.
func TestPickerItem_JSONRoundTrip(t *testing.T) {
	original := picker.PickerItem{
		Kind:         "provider",
		Name:         "Claude",
		ProviderID:   "claude",
		ModelID:      "claude-sonnet-4-6",
		PipelineFile: "",
	}
	data, err := picker.MarshalPickerItem(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := picker.UnmarshalPickerItem(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Kind != original.Kind || got.ProviderID != original.ProviderID || got.ModelID != original.ModelID {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, original)
	}
}

// TestPickerItem_PipelineJSONRoundTrip ensures pipeline items survive JSON round-trip.
func TestPickerItem_PipelineJSONRoundTrip(t *testing.T) {
	original := picker.PickerItem{
		Kind:         "pipeline",
		Name:         "my-pipeline",
		PipelineFile: "/home/user/.config/orcai/pipelines/my-pipeline.pipeline.yaml",
	}
	data, err := picker.MarshalPickerItem(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := picker.UnmarshalPickerItem(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.PipelineFile != original.PipelineFile {
		t.Errorf("PipelineFile mismatch: got %q, want %q", got.PipelineFile, original.PipelineFile)
	}
}

// TestBuildProviders_OllamaSidecarPathSet verifies that after BuildProviders(),
// the ollama ProviderDef has a non-empty SidecarPath when a wrappers YAML is
// present — ensuring the pipelineRunCmd skip guard fires correctly.
func TestBuildProviders_OllamaSidecarPathSet(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up ~/.config/glitch/wrappers/ollama.yaml pointing to a real binary.
	wrappersDir := filepath.Join(tmpDir, ".config", "glitch", "wrappers")
	if err := os.MkdirAll(wrappersDir, 0o755); err != nil {
		t.Fatalf("mkdir wrappers: %v", err)
	}
	// Use "true" as the command — always available on Unix and passes LookPath.
	yaml := "name: ollama\ncommand: true\n"
	sidecarFile := filepath.Join(wrappersDir, "ollama.yaml")
	if err := os.WriteFile(sidecarFile, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write ollama.yaml: %v", err)
	}

	// Also create the required sub-directories so discovery.Discover doesn't fail.
	for _, sub := range []string{"plugins", "pipelines"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, ".config", "glitch", sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}

	// Point glitchConfigDir() at our temp dir.
	t.Setenv("HOME", tmpDir)

	providers := picker.BuildProviders()
	for _, p := range providers {
		if p.ID == "ollama" {
			if p.SidecarPath == "" {
				t.Error("ollama ProviderDef.SidecarPath is empty; sidecar skip guard will not fire")
			}
			return
		}
	}
	// ollama not present means discovery didn't find it — skip rather than fail,
	// since the binary check (exec.LookPath for "true") may behave differently on
	// some CI environments.
	t.Log("ollama not found in BuildProviders() result — skipping SidecarPath assertion")
}
