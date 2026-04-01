package buildershared

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/modal"
	"github.com/8op-org/gl1tch/internal/panelrender"
	"github.com/8op-org/gl1tch/internal/picker"
	"github.com/8op-org/gl1tch/internal/styles"
)

// EditorFocus constants for the editor panel sub-fields.
const (
	EditorFocusPicker  = 0
	EditorFocusName    = 1
	EditorFocusContent = 2
)

// EditorTabOutMsg signals that Tab was pressed from the last editor field.
type EditorTabOutMsg struct{}

// EditorShiftTabOutMsg signals that Shift+Tab was pressed from the first editor field.
type EditorShiftTabOutMsg struct{}

// EditorPanel is a sub-model with a provider picker, name input, and content textarea.
type EditorPanel struct {
	picker    modal.AgentPickerModel
	providers []picker.ProviderDef
	nameInput textinput.Model
	content   textarea.Model
	focus     int  // EditorFocusPicker, EditorFocusName, EditorFocusContent
	focused   bool // outer focus (panel is active)
}

// NewEditorPanel creates a new EditorPanel with the given providers.
func NewEditorPanel(providers []picker.ProviderDef) EditorPanel {
	ni := textinput.New()
	ni.Placeholder = "name"
	ni.CharLimit = 128

	ta := textarea.New()
	ta.Placeholder = "Describe what this should do…"
	ta.ShowLineNumbers = false
	ta.SetHeight(6)
	ta.SetWidth(80)

	e := EditorPanel{
		providers: providers,
		picker:    modal.NewAgentPickerModel(providers),
		nameInput: ni,
		content:   ta,
	}
	return e
}

// SetFocused sets whether the editor panel has outer focus.
func (e EditorPanel) SetFocused(b bool) EditorPanel {
	e.focused = b
	e.syncFocus()
	return e
}

// Name returns the current name input value.
func (e EditorPanel) Name() string { return e.nameInput.Value() }

// Content returns the current content textarea value.
func (e EditorPanel) Content() string { return e.content.Value() }

// SetName sets the name input value.
func (e EditorPanel) SetName(s string) EditorPanel {
	e.nameInput.SetValue(s)
	return e
}

// SetContent sets the content textarea value.
func (e EditorPanel) SetContent(s string) EditorPanel {
	e.content.SetValue(s)
	return e
}

// SelectedProviderID returns the selected provider's ID.
func (e EditorPanel) SelectedProviderID() string { return e.picker.SelectedProviderID() }

// SelectedModelID returns the selected model's ID.
func (e EditorPanel) SelectedModelID() string { return e.picker.SelectedModelID() }

// SelectBySlug sets the picker to match "providerID/modelID".
func (e EditorPanel) SelectBySlug(slug string) EditorPanel {
	e.picker = e.picker.SelectBySlug(slug)
	return e
}

// FocusField returns the current inner focus.
func (e EditorPanel) FocusField() int { return e.focus }

// syncFocus updates Focus/Blur on widgets based on e.focus and e.focused.
func (e *EditorPanel) syncFocus() {
	if !e.focused {
		e.nameInput.Blur()
		e.content.Blur()
		return
	}
	switch e.focus {
	case EditorFocusName:
		e.nameInput.Focus()
		e.content.Blur()
	case EditorFocusContent:
		e.nameInput.Blur()
		e.content.Focus()
	default: // EditorFocusPicker
		e.nameInput.Blur()
		e.content.Blur()
	}
}

// Update handles key events for the editor panel.
func (e EditorPanel) Update(msg tea.Msg) (EditorPanel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return e, nil
	}
	key := keyMsg.String()

	switch key {
	case "tab":
		switch e.focus {
		case EditorFocusPicker:
			// If picker's internal focus is on model (1), advance to name.
			if e.picker.Focus() == 1 {
				e.focus = EditorFocusName
				e.syncFocus()
			} else {
				// Advance picker's internal focus.
				newPicker, _ := e.picker.Update(keyMsg)
				e.picker = newPicker
			}
		case EditorFocusName:
			e.focus = EditorFocusContent
			e.syncFocus()
		case EditorFocusContent:
			// Signal parent to advance focus.
			return e, func() tea.Msg { return EditorTabOutMsg{} }
		}
		return e, nil

	case "shift+tab":
		switch e.focus {
		case EditorFocusPicker:
			// Signal parent to go back.
			return e, func() tea.Msg { return EditorShiftTabOutMsg{} }
		case EditorFocusName:
			e.focus = EditorFocusPicker
			e.picker = e.picker.WithFocus(1) // back to model
			e.syncFocus()
		case EditorFocusContent:
			e.focus = EditorFocusName
			e.syncFocus()
		}
		return e, nil
	}

	// Route to focused widget.
	switch e.focus {
	case EditorFocusPicker:
		newPicker, ev := e.picker.Update(keyMsg)
		e.picker = newPicker
		if ev == modal.AgentPickerConfirmed {
			e.focus = EditorFocusName
			e.syncFocus()
		}
	case EditorFocusName:
		var cmd tea.Cmd
		e.nameInput, cmd = e.nameInput.Update(keyMsg)
		if key == "enter" {
			e.focus = EditorFocusContent
			e.syncFocus()
			return e, nil
		}
		return e, cmd
	case EditorFocusContent:
		var cmd tea.Cmd
		e.content, cmd = e.content.Update(keyMsg)
		return e, cmd
	}
	return e, nil
}

// View renders the editor panel into box rows of the given dimensions.
func (e EditorPanel) View(w, h int, pal styles.ANSIPalette) []string {
	borderColor := pal.Border
	if e.focused {
		borderColor = pal.Accent
	}

	var rows []string
	rows = append(rows, panelrender.BoxTop(w, "EDITOR", borderColor, pal.Accent))

	// Provider/model section.
	if e.focus == EditorFocusPicker && e.focused {
		// Show full picker rows.
		innerW := w - 4
		if innerW < 10 {
			innerW = 10
		}
		for _, r := range e.picker.ViewRows(innerW, pal) {
			rows = append(rows, panelrender.BoxRow("  "+r, w, borderColor))
		}
	} else {
		// Two summary rows: PROVIDER and MODEL as labeled fields.
		provID := e.picker.SelectedProviderID()
		if provID == "" {
			provID = "claude"
		}
		modelID := e.picker.SelectedModelID()
		if modelID == "" {
			modelID = "(default)"
		}
		provLabel := edSectionLabel("PROVIDER", e.focus == EditorFocusPicker && e.focused, pal)
		rows = append(rows, panelrender.BoxRow("  "+provLabel, w, borderColor))
		rows = append(rows, panelrender.BoxRow("  "+pal.FG+provID+sbRst, w, borderColor))
		modelLabel := edSectionLabel("MODEL", e.focus == EditorFocusPicker && e.focused, pal)
		rows = append(rows, panelrender.BoxRow("  "+modelLabel, w, borderColor))
		rows = append(rows, panelrender.BoxRow("  "+pal.FG+modelID+sbRst, w, borderColor))
	}

	rows = append(rows, panelrender.BoxRow("", w, borderColor))

	// Name field.
	nameLabel := edSectionLabel("NAME", e.focus == EditorFocusName && e.focused, pal)
	rows = append(rows, panelrender.BoxRow("  "+nameLabel, w, borderColor))
	e.nameInput.Width = w - 6
	if e.nameInput.Width < 10 {
		e.nameInput.Width = 10
	}
	rows = append(rows, panelrender.BoxRow("  "+e.nameInput.View(), w, borderColor))
	rows = append(rows, panelrender.BoxRow("", w, borderColor))

	// Content field.
	contentLabel := edSectionLabel("PROMPT", e.focus == EditorFocusContent && e.focused, pal)
	rows = append(rows, panelrender.BoxRow("  "+contentLabel, w, borderColor))

	contentInnerW := w - 6
	if contentInnerW < 10 {
		contentInnerW = 10
	}
	// Available rows for content = h - used rows so far.
	used := len(rows) + 1 // +1 for box bottom
	contentRows := h - used
	if contentRows < 2 {
		contentRows = 2
	}
	if contentRows > 10 {
		contentRows = 10
	}
	e.content.SetWidth(contentInnerW)
	e.content.SetHeight(contentRows)
	for _, pLine := range strings.Split(e.content.View(), "\n") {
		pLine = strings.TrimRight(pLine, "\r")
		rows = append(rows, panelrender.BoxRow("  "+pLine, w, borderColor))
	}

	// Pad to h-2 then add hint row (only when focused), then bottom border.
	for len(rows) < h-2 {
		rows = append(rows, panelrender.BoxRow("", w, borderColor))
	}
	if e.focused {
		var editorHints []panelrender.Hint
		switch e.focus {
		case EditorFocusPicker:
			editorHints = []panelrender.Hint{{Key: "↑↓", Desc: "select"}, {Key: "tab", Desc: "next"}}
		case EditorFocusName:
			editorHints = []panelrender.Hint{{Key: "enter", Desc: "next"}, {Key: "tab", Desc: "next"}, {Key: "shift+tab", Desc: "back"}}
		case EditorFocusContent:
			editorHints = []panelrender.Hint{{Key: "ctrl+s", Desc: "save"}, {Key: "tab", Desc: "runner"}, {Key: "shift+tab", Desc: "back"}}
		}
		rows = append(rows, panelrender.BoxRow(panelrender.HintBar(editorHints, w-2, pal), w, borderColor))
	} else {
		rows = append(rows, panelrender.BoxRow("", w, borderColor))
	}
	rows = append(rows, panelrender.BoxBot(w, borderColor))
	return rows
}

func edSectionLabel(title string, focused bool, pal styles.ANSIPalette) string {
	if focused {
		return pal.Accent + sbBld + title + sbRst
	}
	return sbDim + title + sbRst
}
