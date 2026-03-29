package modal

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/picker"
)

const agentPickerWindow = 4

// AgentPickerEvent signals the result of a key event in the picker.
type AgentPickerEvent int

const (
	AgentPickerNone      AgentPickerEvent = iota
	AgentPickerConfirmed                  // enter was pressed
	AgentPickerCancelled                  // esc was pressed
)

// AgentPickerModel owns all provider/model selection state.
// Use NewAgentPickerModel to construct; pass providers from picker.BuildProviders().
type AgentPickerModel struct {
	providers         []picker.ProviderDef
	selectedProvider  int
	selectedModel     int
	provScrollOffset  int
	modelScrollOffset int
	focus             int // 0 = provider list, 1 = model list
}

// NewAgentPickerModel creates a picker pre-selecting the first provider and model.
func NewAgentPickerModel(providers []picker.ProviderDef) AgentPickerModel {
	return AgentPickerModel{providers: providers}
}

// SelectedProviderID returns the ID of the currently highlighted provider, or "".
func (m AgentPickerModel) SelectedProviderID() string {
	if m.selectedProvider >= len(m.providers) {
		return ""
	}
	return m.providers[m.selectedProvider].ID
}

// SelectedModelID returns the ID of the currently highlighted model, or "".
func (m AgentPickerModel) SelectedModelID() string {
	models := m.currentModels()
	if m.selectedModel >= len(models) {
		return ""
	}
	return models[m.selectedModel].ID
}

// SelectedModelLabel returns the display label of the highlighted model, or "".
func (m AgentPickerModel) SelectedModelLabel() string {
	models := m.currentModels()
	if m.selectedModel >= len(models) {
		return ""
	}
	lbl := models[m.selectedModel].Label
	if lbl == "" {
		lbl = models[m.selectedModel].ID
	}
	return lbl
}

// Focus returns the current internal focus: 0 = provider list, 1 = model list.
func (m AgentPickerModel) Focus() int { return m.focus }

// currentModels returns non-separator models for the selected provider.
func (m AgentPickerModel) currentModels() []picker.ModelOption {
	if m.selectedProvider >= len(m.providers) {
		return nil
	}
	return agentPickerFilterModels(m.providers[m.selectedProvider].Models)
}

// agentPickerFilterModels strips separator entries from a model list.
func agentPickerFilterModels(models []picker.ModelOption) []picker.ModelOption {
	out := make([]picker.ModelOption, 0, len(models))
	for _, mo := range models {
		if !mo.Separator {
			out = append(out, mo)
		}
	}
	return out
}

// Update handles key input and returns the updated model plus an event signal.
func (m AgentPickerModel) Update(msg tea.KeyMsg) (AgentPickerModel, AgentPickerEvent) {
	switch msg.String() {
	case "enter":
		return m, AgentPickerConfirmed
	case "esc":
		return m, AgentPickerCancelled
	case "tab":
		m.focus = 1 - m.focus
	case "j", "down":
		if m.focus == 0 {
			if m.selectedProvider < len(m.providers)-1 {
				m.selectedProvider++
				m.selectedModel = 0
				m.modelScrollOffset = 0
			}
			m = m.clampProvScroll()
		} else {
			models := m.currentModels()
			if m.selectedModel < len(models)-1 {
				m.selectedModel++
			}
			m = m.clampModelScroll()
		}
	case "k", "up":
		if m.focus == 0 {
			if m.selectedProvider > 0 {
				m.selectedProvider--
				m.selectedModel = 0
				m.modelScrollOffset = 0
			}
			m = m.clampProvScroll()
		} else {
			if m.selectedModel > 0 {
				m.selectedModel--
			}
			m = m.clampModelScroll()
		}
	}
	return m, AgentPickerNone
}

// clampProvScroll adjusts provScrollOffset so selectedProvider stays in the window.
func (m AgentPickerModel) clampProvScroll() AgentPickerModel {
	if m.selectedProvider < m.provScrollOffset {
		m.provScrollOffset = m.selectedProvider
	}
	if m.selectedProvider >= m.provScrollOffset+agentPickerWindow {
		m.provScrollOffset = m.selectedProvider - agentPickerWindow + 1
	}
	return m
}

// clampModelScroll adjusts modelScrollOffset so selectedModel stays in the window.
func (m AgentPickerModel) clampModelScroll() AgentPickerModel {
	if m.selectedModel < m.modelScrollOffset {
		m.modelScrollOffset = m.selectedModel
	}
	if m.selectedModel >= m.modelScrollOffset+agentPickerWindow {
		m.modelScrollOffset = m.selectedModel - agentPickerWindow + 1
	}
	return m
}
