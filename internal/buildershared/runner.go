package buildershared

import (
	"context"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/panelrender"
	"github.com/8op-org/gl1tch/internal/styles"
)

// RunLineMsg carries a single line of output from the runner goroutine.
type RunLineMsg string

// RunDoneMsg signals that the runner goroutine has finished.
type RunDoneMsg struct{ Err error }

// RunnerPanel is a sub-model that displays streaming output from a run.
type RunnerPanel struct {
	lines     []string
	running   bool
	statusMsg string
	statusErr bool
	ch        chan string
	cancel    context.CancelFunc

	// Clarification support.
	clarifyActive bool
	clarifyInput  textinput.Model
	clarifyRunID  string
	clarifyQ      string

	focused bool
}

// NewRunnerPanel creates a new RunnerPanel.
func NewRunnerPanel() RunnerPanel {
	ci := textinput.New()
	ci.Placeholder = "type your answer…"
	ci.CharLimit = 2000
	return RunnerPanel{clarifyInput: ci}
}

// SetFocused sets whether the runner panel has keyboard focus.
func (r RunnerPanel) SetFocused(b bool) RunnerPanel {
	r.focused = b
	return r
}

// Clear resets the runner output and status.
func (r RunnerPanel) Clear() RunnerPanel {
	r.lines = nil
	r.running = false
	r.statusMsg = ""
	r.statusErr = false
	r.clarifyActive = false
	r.clarifyInput.SetValue("")
	return r
}

// SetLines pre-populates the runner panel with static lines and a status message.
// Used to display existing pipeline YAML without running anything.
func (r RunnerPanel) SetLines(lines []string, status string) RunnerPanel {
	r.lines = lines
	r.running = false
	r.statusMsg = status
	r.statusErr = false
	r.clarifyActive = false
	return r
}

// IsRunning returns true if a run is in progress.
func (r RunnerPanel) IsRunning() bool { return r.running }

// Lines returns the current output lines.
func (r RunnerPanel) Lines() []string { return r.lines }

// StartRun begins streaming from ch and returns the model plus the first wait command.
func (r RunnerPanel) StartRun(ch chan string, cancel context.CancelFunc) (RunnerPanel, tea.Cmd) {
	if r.running {
		return r, nil
	}
	if r.cancel != nil {
		r.cancel()
	}
	r.ch = ch
	r.cancel = cancel
	r.lines = nil
	r.running = true
	r.statusMsg = ""
	r.statusErr = false
	r.clarifyActive = false
	return r, WaitForLine(ch)
}

// WaitForLine returns a cmd that waits for the next line from ch.
func WaitForLine(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return RunDoneMsg{}
		}
		return RunLineMsg(line)
	}
}

// Update handles runner-related tea.Msg and key events.
func (r RunnerPanel) Update(msg tea.Msg) (RunnerPanel, tea.Cmd) {
	switch v := msg.(type) {
	case RunLineMsg:
		line := string(v)
		r.lines = append(r.lines, line)
		if len(r.lines) > 200 {
			r.lines = r.lines[len(r.lines)-200:]
		}
		// Schedule reading the next line.
		if r.ch != nil {
			return r, WaitForLine(r.ch)
		}
		return r, nil

	case RunDoneMsg:
		r.running = false
		if v.Err != nil {
			r.statusMsg = "run failed: " + v.Err.Error()
			r.statusErr = true
		} else {
			r.statusMsg = "run complete"
			r.statusErr = false
		}
		return r, nil

	case tea.KeyMsg:
		return r.handleKey(v)
	}
	return r, nil
}

func (r RunnerPanel) handleKey(msg tea.KeyMsg) (RunnerPanel, tea.Cmd) {
	key := msg.String()

	// Clarification input intercepts all keys.
	if r.clarifyActive {
		switch key {
		case "enter":
			r.clarifyInput.SetValue("")
			r.clarifyActive = false
			return r, nil
		default:
			var cmd tea.Cmd
			r.clarifyInput, cmd = r.clarifyInput.Update(msg)
			return r, cmd
		}
	}

	switch key {
	case "ctrl+c":
		if r.running && r.cancel != nil {
			r.cancel()
		}
	}
	return r, nil
}

// View renders the runner panel into box rows of the given dimensions.
func (r RunnerPanel) View(w, h int, pal styles.ANSIPalette) []string {
	borderColor := pal.Border
	if r.focused {
		borderColor = pal.Accent
	}

	var rows []string
	rows = append(rows, panelrender.BoxTop(w, "GENERATOR / TEST RUNNER", borderColor, pal.Accent))

	// Status line.
	var statusLine string
	switch {
	case r.running:
		statusLine = "  " + pal.Warn + "running..." + sbRst
	case r.statusErr:
		statusLine = "  " + pal.Error + r.statusMsg + sbRst
	case r.statusMsg != "":
		statusLine = "  " + pal.Success + r.statusMsg + sbRst
	default:
		statusLine = "  " + pal.Dim + "idle" + sbRst
	}
	rows = append(rows, panelrender.BoxRow(statusLine, w, borderColor))

	// Output lines: h - top(1) - status(1) - hints(1) - clarify(0-2) - bottom(1).
	overhead := 4
	if r.clarifyActive {
		overhead += 2
	}
	visLines := h - overhead
	if visLines < 1 {
		visLines = 1
	}

	// Show last visLines of lines.
	startIdx := len(r.lines) - visLines
	if startIdx < 0 {
		startIdx = 0
	}
	for i := range visLines {
		lineIdx := startIdx + i
		if lineIdx >= len(r.lines) {
			rows = append(rows, panelrender.BoxRow("", w, borderColor))
			continue
		}
		line := r.lines[lineIdx]
		rows = append(rows, panelrender.BoxRow("  "+pal.FG+line+sbRst, w, borderColor))
	}

	// Clarification block.
	if r.clarifyActive {
		qLine := "  " + pal.Warn + "? " + r.clarifyQ + sbRst
		rows = append(rows, panelrender.BoxRow(qLine, w, borderColor))
		r.clarifyInput.Width = w - 14
		if r.clarifyInput.Width < 10 {
			r.clarifyInput.Width = 10
		}
		answerLine := "  " + pal.Dim + "Answer: " + sbRst + r.clarifyInput.View()
		rows = append(rows, panelrender.BoxRow(answerLine, w, borderColor))
	}

	// Hint row — only when focused.
	if r.focused {
		var hints []panelrender.Hint
		switch {
		case r.clarifyActive:
			hints = []panelrender.Hint{{Key: "enter", Desc: "submit"}, {Key: "esc", Desc: "cancel"}}
		case r.running:
			hints = []panelrender.Hint{{Key: "ctrl+c", Desc: "cancel"}}
		case len(r.lines) > 0:
			hints = []panelrender.Hint{{Key: "r", Desc: "re-run"}, {Key: "shift+tab", Desc: "editor"}}
		default:
			hints = []panelrender.Hint{{Key: "r", Desc: "generate"}, {Key: "shift+tab", Desc: "editor"}}
		}
		rows = append(rows, panelrender.BoxRow(panelrender.HintBar(hints, w-2, pal), w, borderColor))
	} else {
		rows = append(rows, panelrender.BoxRow("", w, borderColor))
	}
	rows = append(rows, panelrender.BoxBot(w, borderColor))
	return rows
}
