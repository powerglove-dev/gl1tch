package buildershared

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/adam-stokes/orcai/internal/modal"
	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/styles"
)

// SendFocus constants for the send panel sub-fields.
const (
	SendFocusName          = 0
	SendFocusAgent         = 1
	SendFocusSavedPrompt   = 2 // opens saved-prompt fuzzy picker
	SendFocusSavedPipeline = 3 // opens saved-pipeline fuzzy picker
	SendFocusMessage       = 4
)

// SendPopupFocus constants for the agent popup sub-fields.
const (
	SendPopupFocusPicker = 0
	SendPopupFocusCWD    = 1
)

// SendSubmitMsg is posted when enter is pressed in the message field.
type SendSubmitMsg struct{ Message string }

// SendTabOutMsg signals that Tab was pressed from the last send panel field.
type SendTabOutMsg struct{}

// SendShiftTabOutMsg signals that Shift+Tab was pressed from the first send panel field.
type SendShiftTabOutMsg struct{}

// SendBrowseCWDMsg is posted when enter is pressed on the CWD row in the agent popup.
type SendBrowseCWDMsg struct{}

// SendPanel holds the send bar: a name input, agent selector, saved prompt picker, pipeline picker, and message input.
type SendPanel struct {
	nameInput            textinput.Model
	msgInput             textinput.Model
	agentPicker          modal.AgentPickerModel
	agentOpen            bool
	agentPopupFocus      int // SendPopupFocusPicker or SendPopupFocusCWD
	focus                int // SendFocusName … SendFocusMessage
	focused              bool
	savedPromptsOpen     bool
	savedPromptPicker    modal.FuzzyPickerModel
	savedPromptIdx       int      // -1 = none selected
	savedPromptTitles    []string
	savedPipelineOpen    bool
	savedPipelinePicker  modal.FuzzyPickerModel
	savedPipelineIdx     int      // -1 = none selected
	savedPipelineTitles  []string
	cwd                  string   // current CWD value (set externally via SetCWD)
}

// NewSendPanel creates a SendPanel with the given providers.
func NewSendPanel(providers []picker.ProviderDef) SendPanel {
	ni := textinput.New()
	ni.Placeholder = "session name"
	ni.Prompt = ""
	ni.CharLimit = 128

	mi := textinput.New()
	mi.Placeholder = "send a message…"
	mi.Prompt = ""
	mi.CharLimit = 4000

	return SendPanel{
		nameInput:           ni,
		msgInput:            mi,
		agentPicker:         modal.NewAgentPickerModel(providers),
		savedPromptPicker:   modal.NewFuzzyPickerModel(8),
		savedPromptIdx:      -1,
		savedPipelinePicker: modal.NewFuzzyPickerModel(8),
		savedPipelineIdx:    -1,
	}
}

// SetFocused sets the outer focus flag without changing inner field focus.
// Used by the view for rendering. Use Enter() when actually moving focus into the panel.
func (s SendPanel) SetFocused(b bool) SendPanel {
	s.focused = b
	if !b {
		s.agentOpen = false
		s.savedPromptsOpen = false
		s.savedPipelineOpen = false
	}
	s.syncFocus()
	return s
}

// Focused returns whether the panel has outer focus.
func (s SendPanel) Focused() bool { return s.focused }

// Enter sets outer focus and resets inner focus to SendFocusName.
// Use this in key handlers when transitioning into the send panel.
func (s SendPanel) Enter() SendPanel {
	s.focused = true
	s.focus = SendFocusName
	s.syncFocus()
	return s
}

// Name returns the current name input value.
func (s SendPanel) Name() string { return s.nameInput.Value() }

// SetName sets the name input value.
func (s SendPanel) SetName(v string) SendPanel {
	s.nameInput.SetValue(v)
	return s
}

// ClearMessage clears the message input.
func (s SendPanel) ClearMessage() SendPanel {
	s.msgInput.SetValue("")
	return s
}

// ProviderID returns the selected provider ID.
func (s SendPanel) ProviderID() string { return s.agentPicker.SelectedProviderID() }

// ModelID returns the selected model ID.
func (s SendPanel) ModelID() string { return s.agentPicker.SelectedModelID() }

// CWD returns the current working directory value.
func (s SendPanel) CWD() string { return s.cwd }

// SetCWD sets the CWD value (called externally after dir picker resolves).
func (s SendPanel) SetCWD(path string) SendPanel {
	s.cwd = path
	return s
}

// AgentOpen reports whether the agent picker popup is open.
func (s SendPanel) AgentOpen() bool { return s.agentOpen }

// AnyModalOpen reports whether any sub-modal (agent, saved prompt, saved pipeline) is open.
func (s SendPanel) AnyModalOpen() bool {
	return s.agentOpen || s.savedPromptsOpen || s.savedPipelineOpen
}

// SavedPromptsOpen reports whether the saved prompts fuzzy picker is open.
func (s SendPanel) SavedPromptsOpen() bool { return s.savedPromptsOpen }

// SavedPromptIdx returns the selected saved prompt index (-1 = none).
func (s SendPanel) SavedPromptIdx() int { return s.savedPromptIdx }

// SetSavedPromptTitles sets the list of saved prompt titles available for selection.
func (s SendPanel) SetSavedPromptTitles(titles []string) SendPanel {
	s.savedPromptTitles = titles
	return s
}

// ClearSavedPrompt resets the saved prompt selection.
func (s SendPanel) ClearSavedPrompt() SendPanel {
	s.savedPromptIdx = -1
	return s
}

// SavedPipelineOpen reports whether the saved pipeline fuzzy picker is open.
func (s SendPanel) SavedPipelineOpen() bool { return s.savedPipelineOpen }

// SavedPipelineIdx returns the selected saved pipeline index (-1 = none).
func (s SendPanel) SavedPipelineIdx() int { return s.savedPipelineIdx }

// SetSavedPipelineTitles sets the list of saved pipeline names available for selection.
func (s SendPanel) SetSavedPipelineTitles(titles []string) SendPanel {
	s.savedPipelineTitles = titles
	return s
}

// ClearSavedPipeline resets the saved pipeline selection.
func (s SendPanel) ClearSavedPipeline() SendPanel {
	s.savedPipelineIdx = -1
	return s
}

func (s *SendPanel) syncFocus() {
	if !s.focused || s.agentOpen || s.savedPromptsOpen || s.savedPipelineOpen {
		s.nameInput.Blur()
		s.msgInput.Blur()
		return
	}
	switch s.focus {
	case SendFocusName:
		s.nameInput.Focus()
		s.msgInput.Blur()
	case SendFocusMessage:
		s.nameInput.Blur()
		s.msgInput.Focus()
	default: // SendFocusAgent or SendFocusSavedPrompt
		s.nameInput.Blur()
		s.msgInput.Blur()
	}
}

// Update handles key events for the send panel.
func (s SendPanel) Update(msg tea.Msg) (SendPanel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	key := keyMsg.String()

	// ── Saved prompts picker open ─────────────────────────────────────────────
	if s.savedPromptsOpen {
		newPicker, ev, cmd := s.savedPromptPicker.Update(keyMsg)
		s.savedPromptPicker = newPicker
		switch ev {
		case modal.FuzzyPickerConfirmed:
			s.savedPromptIdx = s.savedPromptPicker.SelectedOriginalIdx()
			s.savedPromptsOpen = false
			s.syncFocus()
		case modal.FuzzyPickerCancelled:
			s.savedPromptsOpen = false
			s.syncFocus()
		}
		return s, cmd
	}

	// ── Saved pipeline picker open ────────────────────────────────────────────
	if s.savedPipelineOpen {
		newPicker, ev, cmd := s.savedPipelinePicker.Update(keyMsg)
		s.savedPipelinePicker = newPicker
		switch ev {
		case modal.FuzzyPickerConfirmed:
			s.savedPipelineIdx = s.savedPipelinePicker.SelectedOriginalIdx()
			s.savedPipelineOpen = false
			s.syncFocus()
		case modal.FuzzyPickerCancelled:
			s.savedPipelineOpen = false
			s.syncFocus()
		}
		return s, cmd
	}

	// ── Agent popup open ──────────────────────────────────────────────────────
	if s.agentOpen {
		switch key {
		case "esc":
			s.agentOpen = false
			s.syncFocus()
			return s, nil
		case "enter":
			if s.agentPopupFocus == SendPopupFocusCWD {
				// Emit browse CWD message; keep popup open until caller responds
				return s, func() tea.Msg { return SendBrowseCWDMsg{} }
			}
			s.agentOpen = false
			s.syncFocus()
			return s, nil
		case "tab":
			s.agentPopupFocus = 1 - s.agentPopupFocus
			return s, nil
		}
		if s.agentPopupFocus == SendPopupFocusCWD {
			// CWD row is display-only; no text input to route to
			return s, nil
		}
		newPicker, _ := s.agentPicker.Update(keyMsg)
		s.agentPicker = newPicker
		return s, nil
	}

	// ── Normal mode ───────────────────────────────────────────────────────────
	switch key {
	case "tab":
		switch s.focus {
		case SendFocusName:
			s.focus = SendFocusAgent
			s.syncFocus()
		case SendFocusAgent:
			s.focus = SendFocusSavedPrompt
			s.syncFocus()
		case SendFocusSavedPrompt:
			s.focus = SendFocusSavedPipeline
			s.syncFocus()
		case SendFocusSavedPipeline:
			s.focus = SendFocusMessage
			s.syncFocus()
		case SendFocusMessage:
			return s, func() tea.Msg { return SendTabOutMsg{} }
		}
		return s, nil

	case "shift+tab":
		switch s.focus {
		case SendFocusName:
			return s, func() tea.Msg { return SendShiftTabOutMsg{} }
		case SendFocusAgent:
			s.focus = SendFocusName
			s.syncFocus()
		case SendFocusSavedPrompt:
			s.focus = SendFocusAgent
			s.syncFocus()
		case SendFocusSavedPipeline:
			s.focus = SendFocusSavedPrompt
			s.syncFocus()
		case SendFocusMessage:
			s.focus = SendFocusSavedPipeline
			s.syncFocus()
		}
		return s, nil

	case "enter":
		switch s.focus {
		case SendFocusAgent:
			s.agentOpen = true
			s.agentPopupFocus = SendPopupFocusPicker
			s.syncFocus()
			return s, nil
		case SendFocusSavedPrompt:
			if len(s.savedPromptTitles) > 0 {
				s.savedPromptsOpen = true
				s.savedPromptPicker.Open(s.savedPromptTitles)
				s.syncFocus()
			}
			return s, nil
		case SendFocusSavedPipeline:
			if len(s.savedPipelineTitles) > 0 {
				s.savedPipelineOpen = true
				s.savedPipelinePicker.Open(s.savedPipelineTitles)
				s.syncFocus()
			}
			return s, nil
		case SendFocusMessage:
			val := strings.TrimSpace(s.msgInput.Value())
			s.msgInput.SetValue("")
			return s, func() tea.Msg { return SendSubmitMsg{Message: val} }
		}
	}

	switch s.focus {
	case SendFocusName:
		var cmd tea.Cmd
		s.nameInput, cmd = s.nameInput.Update(keyMsg)
		return s, cmd
	case SendFocusMessage:
		var cmd tea.Cmd
		s.msgInput, cmd = s.msgInput.Update(keyMsg)
		return s, cmd
	}
	return s, nil
}

// View renders the send panel into box rows of the given dimensions.
func (s SendPanel) View(w, h int, pal styles.ANSIPalette) []string {
	borderColor := pal.Border
	if s.focused {
		borderColor = pal.Accent
	}

	var rows []string
	rows = append(rows, panelrender.BoxTop(w, "SEND", borderColor, pal.Accent))

	// ── Name > ...... [agent] row ─────────────────────────────────────────────
	agentLabel := s.agentPicker.SelectedProviderID()
	if agentLabel == "" {
		agentLabel = "claude"
	}
	modelLabel := s.agentPicker.SelectedModelID()
	if modelLabel != "" {
		agentLabel += "/" + modelLabel
	}

	nameLabel := sbDim + "Name" + sbRst
	if s.focused && s.focus == SendFocusName {
		nameLabel = pal.Accent + sbBld + "Name" + sbRst
	}

	agentBadge := sbDim + "[" + agentLabel + "]" + sbRst
	if s.focused && s.focus == SendFocusAgent {
		agentBadge = pal.Accent + sbBld + "[" + agentLabel + "]" + sbRst
	}

	// Reserve space for badge: visible badge width + 1 gap minimum.
	badgeVisW := lipgloss.Width(agentBadge)
	s.nameInput.Width = w - 2 - 9 - badgeVisW - 2 // w-2 inner, minus "  Name > " (9), minus badge+gap
	if s.nameInput.Width < 6 {
		s.nameInput.Width = 6
	}
	leftPart := "  " + nameLabel + " > " + s.nameInput.View()
	// Pad to right-align badge within the box inner width (w-2).
	padLen := (w - 2) - lipgloss.Width(leftPart) - badgeVisW
	if padLen < 1 {
		padLen = 1
	}
	nameRow := leftPart + strings.Repeat(" ", padLen) + agentBadge
	rows = append(rows, panelrender.BoxRow(nameRow, w, borderColor))

	// ── Saved Prompt | Saved Pipeline row ────────────────────────────────────
	rows = append(rows, panelrender.BoxRow("", w, borderColor))

	spLabel := sbDim + "Prompt" + sbRst
	if s.focused && s.focus == SendFocusSavedPrompt {
		spLabel = pal.Accent + sbBld + "Prompt" + sbRst
	}
	spValue := sbDim + "(none)" + sbRst
	if s.savedPromptIdx >= 0 && s.savedPromptIdx < len(s.savedPromptTitles) {
		spValue = pal.FG + s.savedPromptTitles[s.savedPromptIdx] + sbRst
	}

	plLabel := sbDim + "Pipeline" + sbRst
	if s.focused && s.focus == SendFocusSavedPipeline {
		plLabel = pal.Accent + sbBld + "Pipeline" + sbRst
	}
	plValue := sbDim + "(none)" + sbRst
	if s.savedPipelineIdx >= 0 && s.savedPipelineIdx < len(s.savedPipelineTitles) {
		plValue = pal.FG + s.savedPipelineTitles[s.savedPipelineIdx] + sbRst
	}

	leftHalf := "  " + spLabel + "  " + spValue
	rightHalf := plLabel + "  " + plValue
	innerW := w - 2
	leftVisW := lipgloss.Width(leftHalf)
	rightVisW := lipgloss.Width(rightHalf)
	gap := innerW - leftVisW - rightVisW
	if gap < 2 {
		gap = 2
	}
	splitRow := leftHalf + strings.Repeat(" ", gap) + rightHalf
	rows = append(rows, panelrender.BoxRow(splitRow, w, borderColor))

	// ── Message input ─────────────────────────────────────────────────────────
	rows = append(rows, panelrender.BoxRow("", w, borderColor))
	s.msgInput.Width = w - 6
	if s.msgInput.Width < 10 {
		s.msgInput.Width = 10
	}
	rows = append(rows, panelrender.BoxRow("  "+s.msgInput.View(), w, borderColor))

	// Pad to h-2, then hint row (only when focused), then bottom.
	for len(rows) < h-2 {
		rows = append(rows, panelrender.BoxRow("", w, borderColor))
	}
	if s.focused {
		var hints []panelrender.Hint
		switch s.focus {
		case SendFocusName:
			hints = []panelrender.Hint{{Key: "tab", Desc: "next"}, {Key: "shift+tab", Desc: "back"}}
		case SendFocusAgent:
			hints = []panelrender.Hint{{Key: "enter", Desc: "pick agent"}, {Key: "tab", Desc: "next"}, {Key: "shift+tab", Desc: "back"}}
		case SendFocusSavedPrompt:
			hints = []panelrender.Hint{{Key: "enter", Desc: "pick prompt"}, {Key: "tab", Desc: "next"}, {Key: "shift+tab", Desc: "back"}}
		case SendFocusSavedPipeline:
			hints = []panelrender.Hint{{Key: "enter", Desc: "pick pipeline"}, {Key: "tab", Desc: "next"}, {Key: "shift+tab", Desc: "back"}}
		case SendFocusMessage:
			hints = []panelrender.Hint{{Key: "enter", Desc: "send"}, {Key: "shift+tab", Desc: "back"}}
		}
		rows = append(rows, panelrender.BoxRow(panelrender.HintBar(hints, w-2, pal), w, borderColor))
	} else {
		rows = append(rows, panelrender.BoxRow("", w, borderColor))
	}
	rows = append(rows, panelrender.BoxBot(w, borderColor))
	return rows
}

// OverlayView renders the agent picker popup as a string to overlay on the full view.
// Call this when AgentOpen() is true, using panelrender.OverlayCenter.
// The CWD row now shows the current path (set via SetCWD) with a hint to browse.
func (s SendPanel) OverlayView(w int, pal styles.ANSIPalette) string {
	boxW := w / 2
	if boxW < 40 {
		boxW = 40
	}
	if boxW > 70 {
		boxW = 70
	}
	innerW := boxW - 4

	var rows []string
	rows = append(rows, panelrender.BoxTop(boxW, "AGENT RUNNER", pal.Border, pal.Accent))
	rows = append(rows, panelrender.BoxRow("", boxW, pal.Border))

	for _, r := range s.agentPicker.ViewRows(innerW, pal) {
		rows = append(rows, panelrender.BoxRow("  "+r, boxW, pal.Border))
	}

	// CWD row — display only, enter triggers browse.
	cwdLabel := sbDim + "CWD" + sbRst
	if s.agentPopupFocus == SendPopupFocusCWD {
		cwdLabel = pal.Accent + sbBld + "CWD" + sbRst
	}
	cwdDisplay := s.cwd
	if cwdDisplay == "" {
		cwdDisplay = "(current directory)"
	}
	cwdColor := sbDim
	if s.agentPopupFocus == SendPopupFocusCWD {
		cwdColor = pal.Accent
	}
	rows = append(rows, panelrender.BoxRow("  "+cwdLabel, boxW, pal.Border))
	rows = append(rows, panelrender.BoxRow("  "+cwdColor+cwdDisplay+sbRst, boxW, pal.Border))
	if s.agentPopupFocus == SendPopupFocusCWD {
		rows = append(rows, panelrender.BoxRow(sbDim+"  enter to browse"+sbRst, boxW, pal.Border))
	}
	rows = append(rows, panelrender.BoxRow("", boxW, pal.Border))

	hint := panelrender.HintBar([]panelrender.Hint{
		{Key: "j/k", Desc: "nav"},
		{Key: "tab", Desc: "cwd"},
		{Key: "enter", Desc: "confirm"},
		{Key: "esc", Desc: "cancel"},
	}, innerW, pal)
	rows = append(rows, panelrender.BoxRow("  "+hint, boxW, pal.Border))
	rows = append(rows, panelrender.BoxBot(boxW, pal.Border))

	return strings.Join(rows, "\n")
}

// SavedPromptOverlayView renders the saved prompts fuzzy picker as an overlay.
// Call this when SavedPromptsOpen() is true, using panelrender.OverlayCenter.
func (s SendPanel) SavedPromptOverlayView(w int, pal styles.ANSIPalette) string {
	return s.savedPromptPicker.ViewBox(w, pal)
}

// SavedPipelineOverlayView renders the saved pipeline fuzzy picker as an overlay.
// Call this when SavedPipelineOpen() is true, using panelrender.OverlayCenter.
func (s SendPanel) SavedPipelineOverlayView(w int, pal styles.ANSIPalette) string {
	return s.savedPipelinePicker.ViewBox(w, pal)
}
