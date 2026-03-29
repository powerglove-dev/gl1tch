package modal

import (
	"encoding/json"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/styles"
)

type rerunFocus int

const (
	rerunFocusContext rerunFocus = iota
	rerunFocusPicker
)

// RerunConfirmedMsg is dispatched when the user confirms a re-run.
type RerunConfirmedMsg struct {
	Run               store.Run
	AdditionalContext string // empty if not provided
	ProviderID        string
	ModelID           string
}

// RerunCancelledMsg is dispatched when the user cancels the re-run.
type RerunCancelledMsg struct{}

// RerunModal is a reusable overlay that prompts for optional additional context
// and lets the user pick a different provider/model before re-running a run.
type RerunModal struct {
	run      store.Run
	textarea textarea.Model
	picker   AgentPickerModel
	focus    rerunFocus
}

// NewRerunModal constructs a RerunModal for the given run.
// The picker is pre-seeded from run.Metadata if it contains a "model" slug.
// For agent runs the textarea is pre-populated with the first step's prompt.
func NewRerunModal(run store.Run, providers []picker.ProviderDef) RerunModal {
	ta := textarea.New()
	ta.Placeholder = "Additional context… (optional)"
	ta.CharLimit = 4096
	ta.ShowLineNumbers = false
	ta.SetWidth(60)
	ta.SetHeight(4)
	ta.Focus()

	// For agent runs, pre-fill with the original prompt from the first step.
	if run.Kind == "agent" && len(run.Steps) > 0 && run.Steps[0].Prompt != "" {
		ta.SetValue(run.Steps[0].Prompt)
	}

	pk := NewAgentPickerModel(providers)

	// Seed picker from run metadata "model" field (format: "providerID/modelID").
	var meta struct {
		Model string `json:"model"`
	}
	if json.Unmarshal([]byte(run.Metadata), &meta) == nil && meta.Model != "" {
		pk = pk.SelectBySlug(meta.Model)
	}

	return RerunModal{
		run:      run,
		textarea: ta,
		picker:   pk,
		focus:    rerunFocusContext,
	}
}

// Run returns the run this modal was constructed for.
func (m RerunModal) Run() store.Run { return m.run }

// Update handles key input. Returns the updated model and an optional tea.Cmd.
// Emits RerunConfirmedMsg or RerunCancelledMsg as appropriate.
func (m RerunModal) Update(msg tea.KeyMsg) (RerunModal, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return RerunCancelledMsg{} }

	case "ctrl+r":
		// Shortcut: run immediately from any focus zone.
		return m, m.confirmedCmd()

	case "tab", "shift+tab":
		if m.focus == rerunFocusContext {
			m.focus = rerunFocusPicker
			m.textarea.Blur()
		} else {
			m.focus = rerunFocusContext
			m.textarea.Focus()
		}
		return m, nil
	}

	if m.focus == rerunFocusContext {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}

	// Picker focus.
	var evt AgentPickerEvent
	m.picker, evt = m.picker.Update(msg)
	switch evt {
	case AgentPickerConfirmed:
		return m, m.confirmedCmd()
	case AgentPickerCancelled:
		return m, func() tea.Msg { return RerunCancelledMsg{} }
	}
	return m, nil
}

func (m RerunModal) confirmedCmd() tea.Cmd {
	return func() tea.Msg {
		return RerunConfirmedMsg{
			Run:               m.run,
			AdditionalContext: strings.TrimSpace(m.textarea.Value()),
			ProviderID:        m.picker.SelectedProviderID(),
			ModelID:           m.picker.SelectedModelID(),
		}
	}
}

// ViewBox renders the modal as a bordered box for use with panelrender.OverlayCenter.
func (m RerunModal) ViewBox(w int, pal styles.ANSIPalette) string {
	innerW := w - 4
	rst := panelrender.RST
	bld := panelrender.BLD

	title := "RE-RUN: " + m.run.Name

	rows := []string{
		panelrender.BoxTop(w, title, pal.Border, pal.Accent),
		panelrender.BoxRow("", w, pal.Border),
	}

	// Context section label.
	ctxLabel := pal.Dim + "  additional context" + rst
	if m.focus == rerunFocusContext {
		ctxLabel = pal.Accent + bld + "  ADDITIONAL CONTEXT" + rst
	}
	rows = append(rows, panelrender.BoxRow(ctxLabel, w, pal.Border))

	// Textarea lines.
	for line := range strings.SplitSeq(m.textarea.View(), "\n") {
		rows = append(rows, panelrender.BoxRow("  "+line, w, pal.Border))
	}

	rows = append(rows, panelrender.BoxRow("", w, pal.Border))

	// Picker section.
	for _, r := range m.picker.ViewRows(innerW, pal) {
		rows = append(rows, panelrender.BoxRow("  "+r, w, pal.Border))
	}

	hint := panelrender.HintBar([]panelrender.Hint{
		{Key: "tab", Desc: "focus"},
		{Key: "ctrl+r", Desc: "run"},
		{Key: "enter", Desc: "run (picker)"},
		{Key: "esc", Desc: "cancel"},
	}, innerW, pal)
	rows = append(rows, panelrender.BoxRow("  "+hint, w, pal.Border))
	rows = append(rows, panelrender.BoxRow("", w, pal.Border))
	rows = append(rows, panelrender.BoxBot(w, pal.Border))

	return strings.Join(rows, "\n")
}
