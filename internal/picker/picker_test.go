package picker_test

import (
	"testing"

	"github.com/adam-stokes/orcai/internal/picker"
)

func TestProviders_NotEmpty(t *testing.T) {
	if len(picker.Providers) == 0 {
		t.Fatal("Providers must not be empty")
	}
}

func TestProviders_ContainsExpected(t *testing.T) {
	want := []string{"claude", "copilot", "ollama", "shell"}
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

func TestProviders_ClaudeHasModels(t *testing.T) {
	for _, p := range picker.Providers {
		if p.ID == "claude" {
			if len(p.Models) == 0 {
				t.Error("claude provider has no models")
			}
			return
		}
	}
	t.Error("claude not found in Providers")
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

func TestProviders_GeminiNoSeparatorSelectableModels(t *testing.T) {
	for _, p := range picker.Providers {
		if p.ID == "gemini" {
			sel := 0
			for _, m := range p.Models {
				if !m.Separator {
					sel++
				}
			}
			if sel == 0 {
				t.Error("gemini should have selectable (non-separator) models")
			}
			return
		}
	}
}

func TestGetOrCreateWorktreeFrom_EmptyPathReturnsEmpty(t *testing.T) {
	worktree, root := picker.GetOrCreateWorktreeFrom("", "test-session")
	if worktree != "" || root != "" {
		t.Errorf("expected empty strings, got worktree=%q root=%q", worktree, root)
	}
}

func TestParseWindowList_Basic(t *testing.T) {
	input := "0 ORCAI\n1 claude-1\n2 gemini-2\n"
	got := picker.ParseWindowList(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 windows, got %d: %v", len(got), got)
	}
	if got[0].Name != "claude-1" {
		t.Errorf("got[0].Name = %q, want %q", got[0].Name, "claude-1")
	}
	if got[1].Index != "2" {
		t.Errorf("got[1].Index = %q, want %q", got[1].Index, "2")
	}
}

func TestParseWindowList_FiltersSystemWindows(t *testing.T) {
	input := "0 ORCAI\n1 _sidebar\n2 claude-1\n"
	got := picker.ParseWindowList(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 window, got %d: %v", len(got), got)
	}
	if got[0].Name != "claude-1" {
		t.Errorf("expected claude-1, got %q", got[0].Name)
	}
}

func TestParseWindowList_Empty(t *testing.T) {
	got := picker.ParseWindowList("")
	if got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

func TestParseWindowList_MalformedLines(t *testing.T) {
	// Lines with no space should be silently skipped.
	input := "0 claude-1\nno-space-line\n2 gemini-1\n"
	got := picker.ParseWindowList(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(got), got)
	}
}

func TestParseWindowList_FiltersWelcome(t *testing.T) {
	input := "0 _welcome\n1 claude-1\n"
	got := picker.ParseWindowList(input)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(got), got)
	}
	if got[0].Name != "claude-1" {
		t.Errorf("expected claude-1, got %q", got[0].Name)
	}
}

func TestBuildProviders_ExcludesShell(t *testing.T) {
	providers := picker.BuildProviders()
	for _, p := range providers {
		if p.ID == "shell" {
			t.Fatal("BuildProviders must not include the shell provider")
		}
	}
}

// TestBuildPickerItems_PipelineCarriesFileField verifies that pipeline PickerItems
// built by BuildPickerItems always have their PipelineFile field set.
func TestBuildPickerItems_PipelineCarriesFileField(t *testing.T) {
	items := picker.BuildPickerItems(nil, nil, "", "")
	for _, item := range items {
		if item.Kind == "pipeline" && item.PipelineFile == "" {
			t.Errorf("pipeline item %q has empty PipelineFile", item.Name)
		}
	}
}

// TestPickerItem_JSONRoundTrip ensures PickerItem marshals and unmarshals cleanly
// so that ORCAI_PICKER_SELECTION encoding works correctly.
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

func TestPickerStates_AllDistinct(t *testing.T) {
	states := []picker.PickerState{
		picker.StateSearch,
		picker.StateProvider,
		picker.StateModel,
		picker.StateWorkdirPick,
		picker.StateWorkdir,
		picker.StateWorkflow,
		picker.StateOpenSpecName,
	}
	seen := map[picker.PickerState]bool{}
	for _, s := range states {
		if seen[s] {
			t.Errorf("duplicate picker state value: %v", s)
		}
		seen[s] = true
	}
}
