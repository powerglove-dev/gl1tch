package pipelineeditor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/buildershared"
)

// HandleKey routes keyboard events to the correct panel/widget.
// Returns (Model, tea.Cmd). When esc is pressed at the top level (not
// inside a sub-mode), it returns (m, nil) and the parent should close us.
func (m Model) HandleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	// ── Global keys (any focus) ───────────────────────────────────────────────
	switch key {
	case "ctrl+c":
		return m, func() tea.Msg { return CloseMsg{} }

	case "J":
		// Open jump window as a tmux popup.
		if os.Getenv("TMUX") != "" {
			return m, func() tea.Msg {
				self, _ := os.Executable()
				exec.Command("tmux", "display-popup", "-E", "-w", "80%", "-h", "70%",
					filepath.Clean(self)+" widget jump-window").Run() //nolint:errcheck
				return nil
			}
		}
		return m, nil

	case "ctrl+s":
		// Save the current pipeline and refresh the sidebar.
		updated, err := m.save()
		if err != nil {
			m.statusMsg = "save error: " + err.Error()
			m.statusErr = true
			return m, nil
		}
		m = updated
		m.pipelines = m.loadPipelines()
		m.sidebar = m.sidebar.SetItems(m.pipelines)
		m.statusMsg = "saved"
		m.statusErr = false
		return m, nil

	case "ctrl+r":
		// Re-run with the first prompt sent this session.
		if m.firstPrompt != "" {
			m.runner = m.runner.Clear()
			return m.startRunWithPrompt(m.firstPrompt)
		}
		// Fall through to regular run with current editor content.
		m.runner = m.runner.Clear()
		return m.startRun()

	case "esc":
		if m.clarifyActive {
			m.clarifyActive = false
			m.clarifyInput.SetValue("")
			return m, nil
		}
		// Navigate back through focus chain; never close the TUI on esc.
		switch m.focus {
		case FocusChat:
			m.send = m.send.SetFocused(false)
			m.runner = m.runner.SetFocused(true)
			m.focus = FocusRunner
		case FocusRunner:
			m.runner = m.runner.SetFocused(false)
			m.editor = m.editor.SetFocused(true)
			m.focus = FocusEditor
		case FocusYAML:
			m.editor = m.editor.SetFocused(true)
			m.focus = FocusEditor
		case FocusEditor:
			// Editor is removed; fall through to sidebar.
			m.editor = m.editor.SetFocused(false)
			m.sidebar = m.sidebar.SetFocused(true)
			m.focus = FocusList
		case FocusList:
			m.sidebar, _ = m.sidebar.Update(msg)
		}
		return m, nil
	}

	// ── Focus-specific routing ────────────────────────────────────────────────
	switch m.focus {
	case FocusList:
		return m.handleListKey(msg)
	case FocusEditor:
		return m.handleEditorKey(msg)
	case FocusYAML:
		return m.handleYAMLKey(msg)
	case FocusRunner:
		return m.handleRunnerKey(msg)
	case FocusChat:
		return m.handleChatKey(msg)
	}
	return m, nil
}

// HandleMsg routes async tea.Msg values from the runner goroutine.
func (m Model) HandleMsg(msg tea.Msg) (Model, tea.Cmd) {
	switch v := msg.(type) {
	case buildershared.RunLineMsg, buildershared.RunDoneMsg:
		var cmd tea.Cmd
		m.runner, cmd = m.runner.Update(v)
		return m, cmd

	case ClarifyPollMsg:
		m.clarifyActive = true
		m.clarifyRunID = v.RunID
		m.clarifyQ = v.Question
		m.clarifyInput.Focus()
	}
	return m, nil
}

// ── List panel ────────────────────────────────────────────────────────────────

func (m Model) handleListKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	var cmd tea.Cmd
	m.sidebar, cmd = m.sidebar.Update(msg)

	// Handle sidebar messages emitted by the Update call.
	if cmd != nil {
		innerMsg := cmd()
		switch v := innerMsg.(type) {
		case buildershared.SidebarSelectMsg:
			m = m.openEdit(v.Name)
			m.send = m.send.SetName(v.Name)
			m.sidebar = m.sidebar.SetFocused(false)
			m.send = m.send.SetFocused(true)
			m.focus = FocusChat
			return m, nil
		case buildershared.SidebarDeleteMsg:
			path := m.pipelinesDir + "/" + v.Name + ".pipeline.yaml"
			os.Remove(path) //nolint:errcheck
			m.pipelines = m.loadPipelines()
			m.sidebar = m.sidebar.SetItems(m.pipelines)
			return m, nil
		}
		// 'n' key — new pipeline
		if key == "n" {
			m = m.openNew()
			m.sidebar = m.sidebar.SetFocused(false)
			m.send = m.send.SetFocused(true)
			m.focus = FocusChat
			return m, nil
		}
	}

	// 'n' key when sidebar doesn't emit a cmd
	if key == "n" {
		m = m.openNew()
		m.sidebar = m.sidebar.SetFocused(false)
		m.send = m.send.SetFocused(true)
		m.focus = FocusChat
		return m, nil
	}

	// Tab: move focus to send panel (editor panel removed from view).
	if key == "tab" {
		m.sidebar = m.sidebar.SetFocused(false)
		m.send = m.send.SetFocused(true)
		m.focus = FocusChat
		return m, nil
	}

	return m, nil
}

// ── Editor panel ──────────────────────────────────────────────────────────────

func (m Model) handleEditorKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)

	// Sync legacy fields from shared editor (for save/run).
	m.nameInput.SetValue(m.editor.Name())
	m.promptArea.SetValue(m.editor.Content())

	// Handle tab-out / shift-tab-out signals.
	if cmd != nil {
		innerMsg := cmd()
		switch innerMsg.(type) {
		case buildershared.EditorTabOutMsg:
			m.editor = m.editor.SetFocused(false)
			m.send = m.send.SetFocused(true)
			m.focus = FocusChat
			return m, nil
		case buildershared.EditorShiftTabOutMsg:
			m.editor = m.editor.SetFocused(false)
			m.sidebar = m.sidebar.SetFocused(true)
			m.focus = FocusList
			return m, nil
		}
	}

	return m, cmd
}

// ── YAML panel ────────────────────────────────────────────────────────────────

func (m Model) handleYAMLKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "j", "down":
		m.yamlScroll++
	case "k", "up":
		if m.yamlScroll > 0 {
			m.yamlScroll--
		}
	case "tab":
		m.send = m.send.SetFocused(true)
		m.focus = FocusChat
	case "shift+tab":
		m.focus = FocusEditor
		m.editor = m.editor.SetFocused(true)
	}
	return m, nil
}

// ── Runner panel ──────────────────────────────────────────────────────────────

func (m Model) handleRunnerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	// Delegate runner key handling to shared runner.
	var cmd tea.Cmd
	m.runner, cmd = m.runner.Update(msg)

	switch key {
	case "r", "enter":
		if !m.runner.IsRunning() {
			m.runner = m.runner.Clear()
			return m.startRun()
		}
	case "ctrl+i":
		// Inject last run output into prompt field.
		if lines := m.runner.Lines(); len(lines) > 0 {
			output := strings.Join(lines, "\n")
			existing := m.editor.Content()
			if existing != "" {
				m.editor = m.editor.SetContent(existing + "\n\n---\n" + output)
			} else {
				m.editor = m.editor.SetContent(output)
			}
			m.runner = m.runner.SetFocused(false)
			m.editor = m.editor.SetFocused(true)
			m.focus = FocusEditor
		}
	case "tab":
		m.runner = m.runner.SetFocused(false)
		m.focus = FocusList
		m.sidebar = m.sidebar.SetFocused(true)
	case "shift+tab":
		m.runner = m.runner.SetFocused(false)
		m.send = m.send.SetFocused(true)
		m.focus = FocusChat
	}
	return m, cmd
}

// ── Chat / Send panel ─────────────────────────────────────────────────────────

func (m Model) handleChatKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.send, cmd = m.send.Update(msg)

	if cmd != nil {
		innerMsg := cmd()
		switch v := innerMsg.(type) {
		case buildershared.SendSubmitMsg:
			if v.Message == "" {
				return m, nil
			}
			if !m.sentOnce {
				m.firstPrompt = v.Message
				m.sentOnce = true
			}
			m.runner = m.runner.Clear()
			m.focus = FocusRunner
			m.runner = m.runner.SetFocused(true)
			return m.startRunWithPrompt(v.Message)
		case buildershared.SendTabOutMsg:
			m.send = m.send.SetFocused(false)
			m.sidebar = m.sidebar.SetFocused(true)
			m.focus = FocusList
			return m, nil
		case buildershared.SendShiftTabOutMsg:
			m.send = m.send.SetFocused(false)
			m.runner = m.runner.SetFocused(true)
			m.focus = FocusRunner
			return m, nil
		}
	}

	return m, cmd
}

// openEditorInWindow opens path in $EDITOR via a tmux window (best-effort).
func openEditorInWindow(path string) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	sessionBytes, err := runCmd("tmux", "display-message", "-p", "#{session_name}")
	if err != nil || len(sessionBytes) == 0 {
		return
	}
	session := trimNL(string(sessionBytes))
	if session == "" {
		return
	}
	cmdStr := editor + " " + path
	runCmdIgnore("tmux", "new-window", "-d", "-t", session+":", "-n", "orcai-edit", cmdStr)
}
