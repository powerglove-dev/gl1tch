package crontui

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"

	"github.com/adam-stokes/orcai/internal/cron"
	"github.com/adam-stokes/orcai/internal/themes"
	"github.com/adam-stokes/orcai/internal/tuikit"
)

// Update handles all incoming messages and key events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ts, cmd, ok := m.themeState.Handle(msg); ok {
		m.themeState = ts
		m.bundle = ts.Bundle()
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		entries, _ := cron.LoadConfig()
		m.entries = entries
		m.applyFilter()
		return m, tick()

	case entriesReloadedMsg:
		m.entries = msg.entries
		m.applyFilter()
		return m, nil

	case logLineMsg:
		m.appendLog(msg.line)
		return m, listenLogs(m.logCh)

	case runDoneMsg:
		// Re-render only; the scheduler already logged the result.
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey routes key events based on the current UI state.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Overlays take priority.
	if m.themePickerOpen {
		return m.handleThemePickerKey(msg)
	}
	if m.quitConfirm {
		return m.handleQuitConfirmKey(msg)
	}
	if m.editOverlay != nil {
		return m.handleEditKey(msg)
	}
	if m.deleteConfirm != nil {
		return m.handleDeleteKey(msg)
	}

	switch msg.String() {
	case "T":
		if gr := themes.GlobalRegistry(); gr != nil {
			bundles := gr.All()
			// Set cursor to the currently active theme.
			if active := gr.Active(); active != nil {
				for i, b := range bundles {
					if b.Name == active.Name {
						m.themePickerCursor = i
						break
					}
				}
			}
		}
		m.themePickerOpen = true
		return m, nil
	case "ctrl+q":
		m.quitConfirm = true
		return m, nil
	case "tab":
		m.activePane = 1 - m.activePane
		return m, nil
	}

	if m.activePane == 0 {
		return m.handleJobPaneKey(msg)
	}
	return m.handleLogPaneKey(msg)
}

// handleThemePickerKey handles key events when the theme picker is open.
func (m Model) handleThemePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	gr := themes.GlobalRegistry()
	if gr == nil {
		m.themePickerOpen = false
		return m, nil
	}
	bundles := gr.All()
	newCursor, close, selected, cmd := tuikit.HandleThemePicker(m.themePickerCursor, bundles, msg.String())
	m.themePickerCursor = newCursor
	if close {
		m.themePickerOpen = false
	}
	// Update m.bundle immediately on selection so the view reflects the new
	// theme without waiting for the busd ThemeChangedMsg round-trip.
	if selected != nil {
		m.bundle = selected
	}
	return m, cmd
}

// handleQuitConfirmKey handles key events when the quit confirmation is shown.
func (m Model) handleQuitConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		// Kill the main ORCAI session so quitting from cron exits everything.
		_ = exec.Command("tmux", "kill-session", "-t", "orcai").Run()
		return m, tea.Quit
	case "n", "N", "esc":
		m.quitConfirm = false
		return m, nil
	}
	return m, nil
}

// handleJobPaneKey handles keys for the jobs pane (pane 0).
func (m Model) handleJobPaneKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		return m.handleFilteringKey(msg)
	}

	switch msg.String() {
	case "/":
		m.filtering = true
		m.filterInput.Focus()
		return m, nil

	case "j", "down":
		if m.selectedIdx < len(m.filtered)-1 {
			m.selectedIdx++
		}
		return m, nil

	case "k", "up":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
		return m, nil

	case "e":
		if len(m.filtered) == 0 {
			return m, nil
		}
		entry := m.filtered[m.selectedIdx]
		m.editOverlay = newEditOverlay(entry)
		return m, nil

	case "d":
		if len(m.filtered) == 0 {
			return m, nil
		}
		entry := m.filtered[m.selectedIdx]
		m.deleteConfirm = &DeleteConfirm{entry: entry}
		return m, nil

	case "enter", "r":
		if len(m.filtered) == 0 {
			return m, nil
		}
		entry := m.filtered[m.selectedIdx]
		m.scheduler.RunNow(entry)
		m.appendLog("INFO: triggered run for " + entry.Name)
		return m, nil
	}
	return m, nil
}

// handleFilteringKey handles keys when the filter input is active.
func (m Model) handleFilteringKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.filterInput.SetValue("")
		m.filterInput.Blur()
		m.applyFilter()
		m.selectedIdx = 0
		return m, nil

	case "enter":
		m.filtering = false
		m.filterInput.Blur()
		return m, nil

	default:
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.applyFilter()
		m.selectedIdx = 0
		return m, cmd
	}
}

// handleLogPaneKey handles keys for the log pane (pane 1).
func (m Model) handleLogPaneKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.logScrollOffset++
		return m, nil
	case "k", "up":
		if m.logScrollOffset > 0 {
			m.logScrollOffset--
		}
		return m, nil
	}
	return m, nil
}

// handleEditKey handles key events when the edit overlay is active.
func (m Model) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ov := m.editOverlay

	switch msg.String() {
	case "esc":
		m.editOverlay = nil
		return m, nil

	case "tab", "shift+tab":
		dir := 1
		if msg.String() == "shift+tab" {
			dir = -1
		}
		ov.fields[ov.focusIdx].Blur()
		ov.focusIdx = (ov.focusIdx + dir + len(ov.fields)) % len(ov.fields)
		ov.fields[ov.focusIdx].Focus()
		m.editOverlay = ov
		return m, nil

	case "enter":
		// On last field, or if focus is on any field and user presses enter, confirm.
		return m.confirmEdit()

	default:
		var cmd tea.Cmd
		ov.fields[ov.focusIdx], cmd = ov.fields[ov.focusIdx].Update(msg)
		m.editOverlay = ov
		return m, cmd
	}
}

// confirmEdit validates and saves the edited entry.
func (m Model) confirmEdit() (tea.Model, tea.Cmd) {
	ov := m.editOverlay
	updated := cron.Entry{
		Name:     ov.fields[0].Value(),
		Schedule: ov.fields[1].Value(),
		Kind:     ov.fields[2].Value(),
		Target:   ov.fields[3].Value(),
		Timeout:  ov.fields[4].Value(),
	}

	if updated.Name == "" {
		ov.errMsg = "name is required"
		m.editOverlay = ov
		return m, nil
	}
	if updated.Schedule == "" {
		ov.errMsg = "schedule is required"
		m.editOverlay = ov
		return m, nil
	}
	if updated.Kind != "pipeline" && updated.Kind != "agent" {
		ov.errMsg = "kind must be 'pipeline' or 'agent'"
		m.editOverlay = ov
		return m, nil
	}

	// If the name changed, remove the old entry first.
	if ov.original.Name != "" && ov.original.Name != updated.Name {
		_ = cron.RemoveEntry(ov.original.Name)
	}

	if err := cron.WriteEntry(updated); err != nil {
		ov.errMsg = "save error: " + err.Error()
		m.editOverlay = ov
		return m, nil
	}

	m.editOverlay = nil
	// Reload entries immediately.
	entries, _ := cron.LoadConfig()
	m.entries = entries
	m.applyFilter()
	return m, nil
}

// handleDeleteKey handles key events when the delete confirmation is active.
func (m Model) handleDeleteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		name := m.deleteConfirm.entry.Name
		_ = cron.RemoveEntry(name)
		m.deleteConfirm = nil
		entries, _ := cron.LoadConfig()
		m.entries = entries
		m.applyFilter()
		if m.selectedIdx >= len(m.filtered) {
			m.selectedIdx = len(m.filtered) - 1
		}
		if m.selectedIdx < 0 {
			m.selectedIdx = 0
		}
		return m, nil

	case "n", "N", "esc":
		m.deleteConfirm = nil
		return m, nil
	}
	return m, nil
}

// newEditOverlay creates an EditOverlay pre-populated with the given entry.
func newEditOverlay(entry cron.Entry) *EditOverlay {
	labels := [5]string{"Name", "Schedule", "Kind", "Target", "Timeout"}
	values := [5]string{entry.Name, entry.Schedule, entry.Kind, entry.Target, entry.Timeout}

	var fields [5]textinput.Model
	for i := range fields {
		ti := textinput.New()
		ti.Placeholder = labels[i]
		ti.CharLimit = 128
		ti.SetValue(values[i])
		fields[i] = ti
	}
	fields[0].Focus()

	return &EditOverlay{
		fields:   fields,
		focusIdx: 0,
		original: entry,
	}
}
