package pipelineeditor

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/modal"
)

// HandleKey routes keyboard events to the correct panel/widget.
// Returns (Model, tea.Cmd). When esc is pressed at the top level (not
// inside a sub-mode), it returns (m, nil) and the parent should close us.
func (m Model) HandleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	// ── Global keys ──────────────────────────────────────────────────────────
	switch key {
	case "ctrl+s":
		// Save then run (submit behavior).
		updated, err := m.save()
		if err != nil {
			m.statusMsg = "save error: " + err.Error()
			m.statusErr = true
			return m, nil
		}
		m = updated
		m.focus = FocusRunner
		return m.startRun()

	case "esc":
		// If we're in a sub-mode, handle locally; otherwise bubble up.
		if m.listSearching {
			m.listSearching = false
			m.listSearch = ""
			return m, nil
		}
		if m.clarifyActive {
			m.clarifyActive = false
			m.clarifyInput.SetValue("")
			return m, nil
		}
		if m.confirmDelete {
			m.confirmDelete = false
			return m, nil
		}
		// Signal parent to close the editor.
		return m, func() tea.Msg { return CloseMsg{} }
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
	}
	return m, nil
}

// HandleMsg routes async tea.Msg values from the runner goroutine.
func (m Model) HandleMsg(msg tea.Msg) (Model, tea.Cmd) {
	switch v := msg.(type) {
	case RunLineMsg:
		line := string(v)
		m.runLines = append(m.runLines, line)
		if len(m.runLines) > 200 {
			m.runLines = m.runLines[len(m.runLines)-200:]
		}
		// Schedule reading the next line.
		if m.runOutputCh != nil {
			return m, waitForLine(m.runOutputCh)
		}

	case RunDoneMsg:
		m.runRunning = false
		if v.Err != nil {
			m.statusMsg = "run failed: " + v.Err.Error()
			m.statusErr = true
		} else {
			m.statusMsg = "run complete"
			m.statusErr = false
		}

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

	// Search mode intercepts character input.
	if m.listSearching {
		switch key {
		case "esc", "ctrl+c":
			m.listSearching = false
			m.listSearch = ""
			return m, nil
		case "enter":
			m.listSearching = false
			return m, nil
		case "backspace", "ctrl+h":
			runes := []rune(m.listSearch)
			if len(runes) > 0 {
				m.listSearch = string(runes[:len(runes)-1])
			}
			return m, nil
		default:
			if len(msg.Runes) == 1 {
				m.listSearch += string(msg.Runes[0])
			}
			return m, nil
		}
	}

	// Confirm-delete mode.
	if m.confirmDelete {
		switch key {
		case "y":
			return m.execDelete()
		case "N", "n", "esc":
			m.confirmDelete = false
		}
		return m, nil
	}

	filtered := m.filteredPipelines()

	switch key {
	case "j", "down":
		if m.listSel < len(filtered)-1 {
			m.listSel++
		}
	case "k", "up":
		if m.listSel > 0 {
			m.listSel--
		}
	case "enter":
		if len(filtered) > 0 && m.listSel < len(filtered) {
			m = m.openEdit(filtered[m.listSel])
		}
	case "n":
		m = m.openNew()
	case "d":
		if len(filtered) > 0 {
			m.confirmDelete = true
		}
	case "/":
		m.listSearching = true
		m.listSearch = ""
	case "tab":
		m.focus = FocusEditor
		m.editorFocus = editorFieldPicker
		m.syncEditorFocus()
	}
	return m, nil
}

// execDelete removes the selected pipeline file and refreshes the list.
func (m Model) execDelete() (Model, tea.Cmd) {
	m.confirmDelete = false
	filtered := m.filteredPipelines()
	if m.listSel >= len(filtered) {
		return m, nil
	}
	name := filtered[m.listSel]
	path := filepath.Join(m.pipelinesDir, name+".pipeline.yaml")
	os.Remove(path) //nolint:errcheck
	m.pipelines = m.loadPipelines()
	m.clampListSel()
	m.statusMsg = "deleted " + name
	m.statusErr = false
	return m, nil
}

// ── Editor panel ──────────────────────────────────────────────────────────────

func (m Model) handleEditorKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "tab":
		switch m.editorFocus {
		case editorFieldPicker:
			// If picker's internal focus is on model (1), advance to name.
			if m.picker.Focus() == 1 {
				m.editorFocus = editorFieldName
				m.syncEditorFocus()
			} else {
				// Advance picker's internal focus.
				newPicker, _ := m.picker.Update(msg)
				m.picker = newPicker
			}
		case editorFieldName:
			m.editorFocus = editorFieldPrompt
			m.syncEditorFocus()
		case editorFieldPrompt:
			// Advance outer focus to Runner (YAML preview removed).
			m.focus = FocusRunner
		}
		return m, nil

	case "shift+tab":
		switch m.editorFocus {
		case editorFieldPicker:
			m.focus = FocusList
		case editorFieldName:
			m.editorFocus = editorFieldPicker
			m.picker = m.picker.WithFocus(1) // back to model
			m.syncEditorFocus()
		case editorFieldPrompt:
			m.editorFocus = editorFieldName
			m.syncEditorFocus()
		}
		return m, nil

	case "ctrl+e":
		// Open current pipeline file in $EDITOR from the prompt field.
		if m.editorFocus == editorFieldPrompt {
			if m.currentPath == "" {
				updated, err := m.save()
				if err != nil {
					m.statusMsg = "save error: " + err.Error()
					m.statusErr = true
					return m, nil
				}
				m = updated
			}
			if m.currentPath != "" {
				openEditorInWindow(m.currentPath)
				if data, err := os.ReadFile(m.currentPath); err == nil {
					m.promptArea.SetValue(string(data))
				}
				m.pipelines = m.loadPipelines()
			}
			return m, nil
		}

	case "esc":
		m.focus = FocusList
		return m, nil
	}

	// Route to focused widget.
	switch m.editorFocus {
	case editorFieldPicker:
		newPicker, ev := m.picker.Update(msg)
		m.picker = newPicker
		if ev == modal.AgentPickerConfirmed {
			m.editorFocus = editorFieldName
			m.syncEditorFocus()
		}
	case editorFieldName:
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		if key == "enter" {
			m.editorFocus = editorFieldPrompt
			m.syncEditorFocus()
			return m, nil
		}
		return m, cmd
	case editorFieldPrompt:
		var cmd tea.Cmd
		m.promptArea, cmd = m.promptArea.Update(msg)
		return m, cmd
	}
	return m, nil
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
	case "ctrl+e":
		// Save first if needed, then open in $EDITOR.
		if m.currentPath == "" {
			updated, err := m.save()
			if err != nil {
				m.statusMsg = "save error: " + err.Error()
				m.statusErr = true
				return m, nil
			}
			m = updated
		}
		if m.currentPath != "" {
			openEditorInWindow(m.currentPath)
			// Reload after editor exits.
			if data, err := os.ReadFile(m.currentPath); err == nil {
				m.promptArea.SetValue(string(data))
			}
			m.pipelines = m.loadPipelines()
		}
	case "tab":
		m.focus = FocusRunner
	case "shift+tab":
		m.focus = FocusEditor
	}
	return m, nil
}

// ── Runner panel ──────────────────────────────────────────────────────────────

func (m Model) handleRunnerKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()

	// Clarification input intercepts all keys.
	if m.clarifyActive {
		switch key {
		case "enter":
			answer := m.clarifyInput.Value()
			m.clarifyInput.SetValue("")
			m.clarifyActive = false
			_ = answer
			// TODO: submit clarification answer to store.
			return m, nil
		default:
			var cmd tea.Cmd
			m.clarifyInput, cmd = m.clarifyInput.Update(msg)
			return m, cmd
		}
	}

	switch key {
	case "r", "enter":
		if !m.runRunning {
			return m.startRun()
		}
	case "ctrl+c":
		if m.runRunning && m.runCancel != nil {
			m.runCancel()
		}
	case "ctrl+i":
		// Inject last run output into prompt field.
		if len(m.runLines) > 0 {
			output := strings.Join(m.runLines, "\n")
			existing := m.promptArea.Value()
			if existing != "" {
				m.promptArea.SetValue(existing + "\n\n---\n" + output)
			} else {
				m.promptArea.SetValue(output)
			}
			m.focus = FocusEditor
			m.editorFocus = editorFieldPrompt
			m.syncEditorFocus()
		}
	case "tab":
		m.focus = FocusList
	case "shift+tab":
		m.focus = FocusEditor
		m.editorFocus = editorFieldPrompt
		m.syncEditorFocus()
	}
	return m, nil
}

// openEditorInWindow opens path in $EDITOR via a tmux window (best-effort).
func openEditorInWindow(path string) {
	// Reuse the same approach as the switchboard package.
	// We call tmux directly here to avoid a package import cycle.
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	// Get current tmux session.
	// If not in tmux, skip silently.
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

// newClarifyInput builds a focused textinput for clarification answers.
func newClarifyInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "type your answer…"
	ti.CharLimit = 2000
	return ti
}

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
