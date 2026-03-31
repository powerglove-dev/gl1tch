package modal

import (
	"encoding/json"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/styles"
	"github.com/adam-stokes/orcai/internal/translations"
)

type rerunFocus int

const (
	rerunFocusContext  rerunFocus = iota // additional context textarea
	rerunFocusCWD                        // working directory text input
	rerunFocusProvider                   // provider list in agent picker
	rerunFocusModel                      // model list in agent picker
)

// RerunConfirmedMsg is dispatched when the user confirms a re-run.
type RerunConfirmedMsg struct {
	Run               store.Run
	AdditionalContext string // empty if not provided
	ProviderID        string
	ModelID           string
	CWD               string
}

// RerunCancelledMsg is dispatched when the user cancels the re-run.
type RerunCancelledMsg struct{}

// RerunModal is a full-screen overlay that prompts for optional additional
// context, a working directory, and an optional provider/model change before
// re-running a pipeline or agent run.
type RerunModal struct {
	run      store.Run
	textarea textarea.Model
	cwdInput textinput.Model
	picker   AgentPickerModel
	focus    rerunFocus
}

// NewRerunModal constructs a RerunModal for the given run.
// cwdFallback is used when run.Metadata has no cwd field.
// The picker is pre-seeded from run.Metadata "model" slug if present.
// For agent runs the textarea is pre-populated with the first step's prompt.
func NewRerunModal(run store.Run, providers []picker.ProviderDef, cwdFallback string) RerunModal {
	ta := textarea.New()
	ta.Placeholder = "Additional context… (optional)"
	ta.CharLimit = 4096
	ta.ShowLineNumbers = false
	ta.SetWidth(60)
	ta.SetHeight(4)
	ta.Focus()

	// Textarea is for additional context only; leave it blank so the user can
	// type new instructions without the original prompt being re-submitted twice.

	// CWD input — default from metadata, fall back to cwdFallback.
	var meta struct {
		Model string `json:"model"`
		CWD   string `json:"cwd"`
	}
	_ = json.Unmarshal([]byte(run.Metadata), &meta)
	defaultCWD := meta.CWD
	if defaultCWD == "" {
		defaultCWD = cwdFallback
	}

	ci := textinput.New()
	ci.Placeholder = "working directory"
	ci.CharLimit = 1024
	ci.SetValue(defaultCWD)

	pk := NewAgentPickerModel(providers)
	if meta.Model != "" {
		pk = pk.SelectBySlug(meta.Model)
	}

	return RerunModal{
		run:      run,
		textarea: ta,
		cwdInput: ci,
		picker:   pk,
		focus:    rerunFocusContext,
	}
}

// Run returns the run this modal was constructed for.
func (m RerunModal) Run() store.Run { return m.run }

// CWD returns the current working directory value from the input field.
func (m RerunModal) CWD() string { return strings.TrimSpace(m.cwdInput.Value()) }

// WithModelSlug pre-selects the provider/model from a "providerID/modelID" slug.
func (m RerunModal) WithModelSlug(slug string) RerunModal {
	m.picker = m.picker.SelectBySlug(slug)
	return m
}

// Update handles key input. Cycles focus with tab/shift+tab across all four
// zones (context → cwd → provider → model → context). enter or ctrl+r
// confirms from any zone. esc cancels.
func (m RerunModal) Update(msg tea.KeyMsg) (RerunModal, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return RerunCancelledMsg{} }

	case "ctrl+r", "enter":
		return m, m.confirmedCmd()

	case "tab":
		return m.advanceFocus(+1), nil

	case "shift+tab":
		return m.advanceFocus(-1), nil
	}

	// Delegate to the focused zone.
	switch m.focus {
	case rerunFocusContext:
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd

	case rerunFocusCWD:
		var cmd tea.Cmd
		m.cwdInput, cmd = m.cwdInput.Update(msg)
		return m, cmd

	case rerunFocusProvider:
		m.picker = m.picker.WithFocus(0)
		var evt AgentPickerEvent
		m.picker, evt = m.picker.Update(msg)
		if evt == AgentPickerConfirmed {
			return m, m.confirmedCmd()
		}

	case rerunFocusModel:
		m.picker = m.picker.WithFocus(1)
		var evt AgentPickerEvent
		m.picker, evt = m.picker.Update(msg)
		if evt == AgentPickerConfirmed {
			return m, m.confirmedCmd()
		}
	}
	return m, nil
}

// advanceFocus cycles focus by delta (+1 forward, -1 backward) through the
// four zones and updates blur/focus state on the embedded inputs.
func (m RerunModal) advanceFocus(delta int) RerunModal {
	const n = 4
	next := rerunFocus((int(m.focus) + delta + n) % n)

	// Blur outgoing zone.
	switch m.focus {
	case rerunFocusContext:
		m.textarea.Blur()
	case rerunFocusCWD:
		m.cwdInput.Blur()
	}

	m.focus = next

	// Focus incoming zone.
	switch m.focus {
	case rerunFocusContext:
		m.textarea.Focus()
	case rerunFocusCWD:
		m.cwdInput.Focus()
	case rerunFocusProvider:
		m.picker = m.picker.WithFocus(0)
	case rerunFocusModel:
		m.picker = m.picker.WithFocus(1)
	}

	return m
}

func (m RerunModal) confirmedCmd() tea.Cmd {
	return func() tea.Msg {
		return RerunConfirmedMsg{
			Run:               m.run,
			AdditionalContext: strings.TrimSpace(m.textarea.Value()),
			ProviderID:        m.picker.SelectedProviderID(),
			ModelID:           m.picker.SelectedModelID(),
			CWD:               strings.TrimSpace(m.cwdInput.Value()),
		}
	}
}

// ViewBox renders the modal as a full-screen bordered box.
// w and h are the total terminal dimensions.
func (m RerunModal) ViewBox(w, h int, pal styles.ANSIPalette) string {
	innerW := w - 4
	rst := panelrender.RST
	bld := panelrender.BLD

	active := func(label string, f rerunFocus) string {
		if m.focus == f {
			return pal.Accent + bld + "  " + label + rst
		}
		return pal.Dim + "  " + label + rst
	}

	rows := []string{
		panelrender.BoxTop(w, "RE-RUN: "+m.run.Name, pal.Border, pal.Accent),
		panelrender.BoxRow("", w, pal.Border),
	}

	// ── Additional context ───────────────────────────────────────────────────
	contextLabel := "ADDITIONAL CONTEXT"
	if p := translations.GlobalProvider(); p != nil {
		contextLabel = p.T(translations.KeyRerunContextLabel, contextLabel)
	}
	rows = append(rows, panelrender.BoxRow(active(contextLabel, rerunFocusContext), w, pal.Border))
	for line := range strings.SplitSeq(m.textarea.View(), "\n") {
		rows = append(rows, panelrender.BoxRow("  "+line, w, pal.Border))
	}
	rows = append(rows, panelrender.BoxRow("", w, pal.Border))

	// ── Working directory ────────────────────────────────────────────────────
	cwdLabel := "WORKING DIRECTORY"
	if p := translations.GlobalProvider(); p != nil {
		cwdLabel = p.T(translations.KeyRerunCwdLabel, cwdLabel)
	}
	rows = append(rows, panelrender.BoxRow(active(cwdLabel, rerunFocusCWD), w, pal.Border))
	rows = append(rows, panelrender.BoxRow("  "+m.cwdInput.View(), w, pal.Border))
	rows = append(rows, panelrender.BoxRow("", w, pal.Border))

	// ── Provider / model picker ──────────────────────────────────────────────
	// Set picker's visual focus to match our focus state; -1 dims both columns.
	var pickerView AgentPickerModel
	switch m.focus {
	case rerunFocusProvider:
		pickerView = m.picker.WithFocus(0)
	case rerunFocusModel:
		pickerView = m.picker.WithFocus(1)
	default:
		pickerView = m.picker.WithFocus(-1)
	}
	for _, r := range pickerView.ViewRows(innerW, pal) {
		rows = append(rows, panelrender.BoxRow("  "+r, w, pal.Border))
	}

	// ── Hint bar + bottom ────────────────────────────────────────────────────
	hint := panelrender.HintBar([]panelrender.Hint{
		{Key: "tab", Desc: "next"},
		{Key: "shift+tab", Desc: "prev"},
		{Key: "enter", Desc: "run"},
		{Key: "esc", Desc: "cancel"},
	}, innerW, pal)

	// Pad remaining height with empty rows so the box fills the screen.
	contentRows := len(rows) + 3 // +3 for hint, blank, bot
	for range max(h-contentRows, 0) {
		rows = append(rows, panelrender.BoxRow("", w, pal.Border))
	}

	rows = append(rows, panelrender.BoxRow("  "+hint, w, pal.Border))
	rows = append(rows, panelrender.BoxRow("", w, pal.Border))
	rows = append(rows, panelrender.BoxBot(w, pal.Border))

	return strings.Join(rows, "\n")
}
