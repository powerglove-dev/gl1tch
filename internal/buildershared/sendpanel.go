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
	SendFocusName    = 0
	SendFocusAgent   = 1
	SendFocusMessage = 2
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

// SendPanel holds the send bar: a name input, agent selector, and message input.
type SendPanel struct {
	nameInput       textinput.Model
	msgInput        textinput.Model
	agentPicker     modal.AgentPickerModel
	cwdInput        textinput.Model
	agentOpen       bool
	agentPopupFocus int // SendPopupFocusPicker or SendPopupFocusCWD
	focus           int // SendFocusName, SendFocusAgent, SendFocusMessage
	focused         bool
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

	ci := textinput.New()
	ci.Placeholder = "working directory (default: cwd)"
	ci.Prompt = ""
	ci.CharLimit = 512

	return SendPanel{
		nameInput:   ni,
		msgInput:    mi,
		agentPicker: modal.NewAgentPickerModel(providers),
		cwdInput:    ci,
	}
}

// SetFocused sets whether the send panel has outer focus.
// Entering the panel always resets inner focus to SendFocusName.
func (s SendPanel) SetFocused(b bool) SendPanel {
	s.focused = b
	if b {
		s.focus = SendFocusName
	}
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

// CWD returns the cwd input value.
func (s SendPanel) CWD() string { return s.cwdInput.Value() }

// AgentOpen reports whether the agent picker popup is open.
func (s SendPanel) AgentOpen() bool { return s.agentOpen }

func (s *SendPanel) syncFocus() {
	if !s.focused || s.agentOpen {
		s.nameInput.Blur()
		s.msgInput.Blur()
		s.cwdInput.Blur()
		return
	}
	switch s.focus {
	case SendFocusName:
		s.nameInput.Focus()
		s.msgInput.Blur()
	case SendFocusMessage:
		s.nameInput.Blur()
		s.msgInput.Focus()
	default: // SendFocusAgent
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

	// ── Agent popup open ──────────────────────────────────────────────────────
	if s.agentOpen {
		switch key {
		case "esc":
			s.agentOpen = false
			s.syncFocus()
			return s, nil
		case "enter":
			s.agentOpen = false
			s.syncFocus()
			return s, nil
		case "tab":
			s.agentPopupFocus = 1 - s.agentPopupFocus
			if s.agentPopupFocus == SendPopupFocusCWD {
				s.cwdInput.Focus()
			} else {
				s.cwdInput.Blur()
			}
			return s, nil
		}
		if s.agentPopupFocus == SendPopupFocusCWD {
			var cmd tea.Cmd
			s.cwdInput, cmd = s.cwdInput.Update(keyMsg)
			return s, cmd
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
		case SendFocusMessage:
			s.focus = SendFocusAgent
			s.syncFocus()
		}
		return s, nil

	case "enter":
		switch s.focus {
		case SendFocusAgent:
			s.agentOpen = true
			s.agentPopupFocus = SendPopupFocusPicker
			s.cwdInput.Blur()
			s.syncFocus()
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
		case SendFocusMessage:
			hints = []panelrender.Hint{{Key: "enter", Desc: "send"}, {Key: "ctrl+r", Desc: "re-run"}, {Key: "ctrl+s", Desc: "save"}, {Key: "shift+tab", Desc: "back"}}
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

	// CWD row.
	cwdLabel := sbDim + "CWD" + sbRst
	if s.agentPopupFocus == SendPopupFocusCWD {
		cwdLabel = pal.Accent + sbBld + "CWD" + sbRst
	}
	rows = append(rows, panelrender.BoxRow("  "+cwdLabel, boxW, pal.Border))
	s.cwdInput.Width = innerW - 2
	if s.cwdInput.Width < 10 {
		s.cwdInput.Width = 10
	}
	rows = append(rows, panelrender.BoxRow("  "+s.cwdInput.View(), boxW, pal.Border))
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
