package modal_test

import (
	"testing"

	"github.com/adam-stokes/orcai/internal/modal"
	"github.com/adam-stokes/orcai/internal/picker"
)

func testProviders() []picker.ProviderDef {
	return []picker.ProviderDef{
		{
			ID: "claude", Label: "Claude",
			Models: []picker.ModelOption{
				{ID: "sonnet", Label: "Sonnet"},
				{ID: "opus", Label: "Opus"},
			},
		},
		{
			ID: "ollama", Label: "Ollama",
			Models: []picker.ModelOption{
				{ID: "llama3", Label: "Llama 3"},
			},
		},
		{ID: "shell", Label: "Shell"},
	}
}

func TestAgentPickerModel_NewSelectsFirst(t *testing.T) {
	m := modal.NewAgentPickerModel(testProviders())
	if m.SelectedProviderID() != "claude" {
		t.Errorf("want claude, got %q", m.SelectedProviderID())
	}
	if m.SelectedModelID() != "sonnet" {
		t.Errorf("want sonnet, got %q", m.SelectedModelID())
	}
	if m.SelectedModelLabel() != "Sonnet" {
		t.Errorf("want Sonnet, got %q", m.SelectedModelLabel())
	}
}

func TestAgentPickerModel_EmptyProviders(t *testing.T) {
	m := modal.NewAgentPickerModel(nil)
	if m.SelectedProviderID() != "" {
		t.Errorf("want empty, got %q", m.SelectedProviderID())
	}
	if m.SelectedModelID() != "" {
		t.Errorf("want empty, got %q", m.SelectedModelID())
	}
}

func TestAgentPickerModel_ProviderWithNoModels(t *testing.T) {
	m := modal.NewAgentPickerModel([]picker.ProviderDef{{ID: "shell", Label: "Shell"}})
	if m.SelectedProviderID() != "shell" {
		t.Errorf("want shell, got %q", m.SelectedProviderID())
	}
	if m.SelectedModelID() != "" {
		t.Errorf("want empty model, got %q", m.SelectedModelID())
	}
}
