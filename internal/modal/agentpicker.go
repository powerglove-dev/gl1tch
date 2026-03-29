package modal

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/styles"
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

// ViewRows returns ANSI-rendered rows for embedding into any panel.
// innerW is the available content width (no borders). Each row is a plain string
// that callers wrap in panelrender.BoxRow when embedding inline.
func (m AgentPickerModel) ViewRows(innerW int, pal styles.ANSIPalette) []string {
	rst := panelrender.RST
	bld := panelrender.BLD
	var rows []string

	// ── PROVIDER section ─────────────────────────────────────────────────────
	if m.focus == 0 {
		rows = append(rows, pal.Accent+bld+"  PROVIDER"+rst)
	} else {
		rows = append(rows, pal.Dim+"  PROVIDER"+rst)
	}

	if len(m.providers) == 0 {
		rows = append(rows, pal.Dim+"  no providers"+rst)
	} else {
		start := m.provScrollOffset
		end := start + agentPickerWindow
		if end > len(m.providers) {
			end = len(m.providers)
		}
		for i := start; i < end; i++ {
			p := m.providers[i]
			lbl := p.Label
			if lbl == "" {
				lbl = p.ID
			}
			if i == m.selectedProvider {
				if m.focus == 0 {
					rows = append(rows, pal.SelBG+"\x1b[97m"+"  > "+lbl+rst)
				} else {
					rows = append(rows, pal.Accent+"  > "+lbl+rst)
				}
			} else {
				rows = append(rows, pal.Dim+"    "+pal.FG+lbl+rst)
			}
		}
	}

	rows = append(rows, "")

	// ── MODEL section ─────────────────────────────────────────────────────────
	if m.focus == 1 {
		rows = append(rows, pal.Accent+bld+"  MODEL"+rst)
	} else {
		rows = append(rows, pal.Dim+"  MODEL"+rst)
	}

	models := m.currentModels()
	if len(models) == 0 {
		rows = append(rows, pal.Dim+"  no models"+rst)
	} else {
		start := m.modelScrollOffset
		end := start + agentPickerWindow
		if end > len(models) {
			end = len(models)
		}
		for i := start; i < end; i++ {
			mo := models[i]
			lbl := mo.Label
			if lbl == "" {
				lbl = mo.ID
			}
			if i == m.selectedModel {
				if m.focus == 1 {
					rows = append(rows, pal.SelBG+"\x1b[97m"+"  > "+lbl+rst)
				} else {
					rows = append(rows, pal.Accent+"  > "+lbl+rst)
				}
			} else {
				rows = append(rows, pal.Dim+"    "+pal.FG+lbl+rst)
			}
		}
	}

	rows = append(rows, "")
	rows = append(rows, panelrender.HintBar([]panelrender.Hint{
		{Key: "j/k", Desc: "nav"},
		{Key: "tab", Desc: "switch"},
		{Key: "enter", Desc: "confirm"},
		{Key: "esc", Desc: "cancel"},
	}, innerW, pal))

	return rows
}

// ViewBox renders the picker as a standalone bordered overlay box suitable for
// use with panelrender.OverlayCenter.
func (m AgentPickerModel) ViewBox(boxW int, pal styles.ANSIPalette) string {
	rows := []string{
		panelrender.BoxTop(boxW, "AGENT / MODEL", pal.Border, pal.Accent),
		panelrender.BoxRow("", boxW, pal.Border),
	}
	for _, r := range m.ViewRows(boxW-4, pal) {
		rows = append(rows, panelrender.BoxRow("  "+r, boxW, pal.Border))
	}
	rows = append(rows, panelrender.BoxRow("", boxW, pal.Border))
	rows = append(rows, panelrender.BoxBot(boxW, pal.Border))
	return strings.Join(rows, "\n")
}
