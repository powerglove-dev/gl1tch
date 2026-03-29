package modal_test

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/modal"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/styles"
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

func key(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func keySpecial(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func TestAgentPickerUpdate_MoveDown(t *testing.T) {
	m := modal.NewAgentPickerModel(testProviders())
	m2, ev := m.Update(key("j"))
	if ev != modal.AgentPickerNone {
		t.Errorf("want None, got %v", ev)
	}
	if m2.SelectedProviderID() != "ollama" {
		t.Errorf("want ollama after j, got %q", m2.SelectedProviderID())
	}
	if m2.SelectedModelID() != "llama3" {
		t.Errorf("want llama3, got %q", m2.SelectedModelID())
	}
}

func TestAgentPickerUpdate_MoveUpAtTop(t *testing.T) {
	m := modal.NewAgentPickerModel(testProviders())
	m2, _ := m.Update(key("k"))
	if m2.SelectedProviderID() != "claude" {
		t.Errorf("want claude (clamped), got %q", m2.SelectedProviderID())
	}
}

func TestAgentPickerUpdate_TabSwitchesFocus(t *testing.T) {
	m := modal.NewAgentPickerModel(testProviders())
	m2, _ := m.Update(keySpecial(tea.KeyTab))
	// now in model focus; j moves model selection
	m3, _ := m2.Update(key("j"))
	if m3.SelectedModelID() != "opus" {
		t.Errorf("want opus, got %q", m3.SelectedModelID())
	}
	if m3.SelectedProviderID() != "claude" {
		t.Errorf("provider should not change in model focus, got %q", m3.SelectedProviderID())
	}
}

func TestAgentPickerUpdate_EnterConfirms(t *testing.T) {
	m := modal.NewAgentPickerModel(testProviders())
	_, ev := m.Update(keySpecial(tea.KeyEnter))
	if ev != modal.AgentPickerConfirmed {
		t.Errorf("want Confirmed, got %v", ev)
	}
}

func TestAgentPickerUpdate_EscCancels(t *testing.T) {
	m := modal.NewAgentPickerModel(testProviders())
	_, ev := m.Update(keySpecial(tea.KeyEsc))
	if ev != modal.AgentPickerCancelled {
		t.Errorf("want Cancelled, got %v", ev)
	}
}

func TestAgentPickerUpdate_ScrollClamps(t *testing.T) {
	providers := make([]picker.ProviderDef, 6)
	for i := range providers {
		providers[i] = picker.ProviderDef{ID: fmt.Sprintf("p%d", i), Label: fmt.Sprintf("P%d", i)}
	}
	m := modal.NewAgentPickerModel(providers)
	for range 5 {
		m, _ = m.Update(key("j"))
	}
	if m.SelectedProviderID() != "p5" {
		t.Errorf("want p5, got %q", m.SelectedProviderID())
	}
}

func TestAgentPickerViewRows_ContainsProviderAndModel(t *testing.T) {
	m := modal.NewAgentPickerModel(testProviders())
	pal := styles.ANSIPalette{Accent: "", Dim: "", FG: "", SelBG: "", Border: ""}
	rows := m.ViewRows(60, pal)
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "PROVIDER") {
		t.Error("ViewRows missing PROVIDER label")
	}
	if !strings.Contains(joined, "MODEL") {
		t.Error("ViewRows missing MODEL label")
	}
	if !strings.Contains(joined, "Claude") {
		t.Error("ViewRows missing provider label Claude")
	}
	if !strings.Contains(joined, "Sonnet") {
		t.Error("ViewRows missing model label Sonnet")
	}
}

func TestAgentPickerViewBox_HasBorders(t *testing.T) {
	m := modal.NewAgentPickerModel(testProviders())
	pal := styles.ANSIPalette{}
	box := m.ViewBox(60, pal)
	if !strings.Contains(box, "AGENT / MODEL") {
		t.Error("ViewBox missing title")
	}
	if !strings.Contains(box, "┌") || !strings.Contains(box, "└") {
		t.Error("ViewBox missing box-drawing borders")
	}
}

func TestAgentPickerModel_FocusAccessor(t *testing.T) {
	m := modal.NewAgentPickerModel(testProviders())
	if m.Focus() != 0 {
		t.Errorf("want focus 0 initially, got %d", m.Focus())
	}
	m2, _ := m.Update(keySpecial(tea.KeyTab))
	if m2.Focus() != 1 {
		t.Errorf("want focus 1 after tab, got %d", m2.Focus())
	}
}
