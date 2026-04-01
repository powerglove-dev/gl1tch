package crontui

import (
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"gopkg.in/yaml.v3"

	"github.com/8op-org/gl1tch/internal/busd"
	"github.com/8op-org/gl1tch/internal/busd/topics"
	"github.com/8op-org/gl1tch/internal/cron"
	"github.com/8op-org/gl1tch/internal/jumpwindow"
	"github.com/8op-org/gl1tch/internal/themes"
	"github.com/8op-org/gl1tch/internal/tuikit"
)


// Update handles all incoming messages and key events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ts, cmd, ok := m.themeState.Handle(msg); ok {
		m.themeState = ts
		m.bundle = ts.Bundle()
		return m, cmd
	}

	switch msg := msg.(type) {
	case jumpwindow.CloseMsg:
		m.jumpOpen = false
		return m, nil

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
	if m.jumpOpen {
		var cmd tea.Cmd
		m.jumpModal, cmd = m.jumpModal.Update(msg)
		return m, cmd
	}
	if m.helpOpen {
		return m.handleHelpKey(msg)
	}
	if m.themePicker.Open {
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
	case "J":
		jm := jumpwindow.NewEmbedded(m.bundle)
		jm.SetSize(m.width, m.height-2)
		m.jumpModal = jm
		m.jumpOpen = true
		return m, nil
	case "?":
		m.helpOpen = true
		m.helpScrollOffset = 0
		return m, nil
	case "T":
		if gr := themes.GlobalRegistry(); gr != nil {
			m.themePicker.Open = true
			m.themePicker.OriginalTheme = gr.Active()
			// Set initial tab based on active theme's mode
			if active := gr.Active(); active != nil && active.Mode == "light" {
				m.themePicker.Tab = 1
			} else {
				m.themePicker.Tab = 0
			}
		}
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
		m.themePicker.Open = false
		return m, nil
	}
	dark := gr.BundlesByMode("dark")
	light := gr.BundlesByMode("light")
	newPicker, close, selected, cmd := tuikit.HandleThemePicker(m.themePicker, dark, light, msg.String())
	m.themePicker = newPicker
	if close {
		m.themePicker.Open = false
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
		// Kill the main GLITCH session so quitting from cron exits everything.
		_ = exec.Command("tmux", "kill-session", "-t", "glitch").Run()
		return m, tea.Quit
	case "n", "N", "esc":
		m.quitConfirm = false
		return m, nil
	}
	return m, nil
}

// handleHelpKey handles key events when the help overlay is open.
func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.helpOpen = false
		return m, nil
	case "j", "down":
		m.helpScrollOffset++
		return m, nil
	case "k", "up":
		if m.helpScrollOffset > 0 {
			m.helpScrollOffset--
		}
		return m, nil
	case "]":
		m.helpScrollOffset += 10
		return m, nil
	case "[":
		if m.helpScrollOffset > 10 {
			m.helpScrollOffset -= 10
		} else {
			m.helpScrollOffset = 0
		}
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

	case "p":
		if len(m.filtered) == 0 {
			return m, nil
		}
		entry := m.filtered[m.selectedIdx]
		if entry.Kind != "pipeline" {
			return m, nil
		}
		path := resolvePipelinePath(entry.Target)
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		return m, tea.ExecProcess(exec.Command(editor, path), nil)
	}
	return m, nil
}

// resolvePipelinePath returns the filesystem path for a pipeline target name.
// It tries ~/.config/glitch/pipelines/<target>.yaml and <target>.pipeline.yaml
// variants, falling back to the raw target string if nothing resolves.
func resolvePipelinePath(target string) string {
	home, err := os.UserHomeDir()
	if err == nil {
		candidates := []string{
			filepath.Join(home, ".config", "glitch", "pipelines", target+".yaml"),
			filepath.Join(home, ".config", "glitch", "pipelines", target+".pipeline.yaml"),
			filepath.Join(home, ".config", "glitch", "pipelines", target),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}
	return target
}

// updatePipelineYAMLName reads the pipeline YAML at the path for target,
// updates the top-level "name" field to newName, and writes the file back.
// Only the name field is changed; all other content is preserved.
func updatePipelineYAMLName(target, newName string) error {
	path := resolvePipelinePath(target)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	// Walk the mapping node to find and update the "name" key.
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		mapping := doc.Content[0]
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == "name" {
				mapping.Content[i+1].Value = newName
				break
			}
		}
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
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

// buildUpdatedEntry constructs the updated Entry from the overlay form,
// preserving fields not exposed as editable inputs (Args, WorkingDir).
func buildUpdatedEntry(ov *EditOverlay) cron.Entry {
	return cron.Entry{
		Name:       ov.fields[0].Value(),
		Schedule:   ov.fields[1].Value(),
		Kind:       ov.fields[2].Value(),
		Target:     ov.fields[3].Value(),
		Timeout:    ov.fields[4].Value(),
		Args:       ov.original.Args,
		WorkingDir: ov.original.WorkingDir,
	}
}

// confirmEdit validates and saves the edited entry.
func (m Model) confirmEdit() (tea.Model, tea.Cmd) {
	ov := m.editOverlay
	updated := buildUpdatedEntry(ov)

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

	// If a pipeline entry was renamed, update the pipeline YAML name field.
	if updated.Kind == "pipeline" && ov.original.Name != updated.Name {
		_ = updatePipelineYAMLName(updated.Target, updated.Name)
	}

	// Publish rename event so switchboard and other consumers can react.
	if ov.original.Name != "" && ov.original.Name != updated.Name {
		if sockPath, err := busd.SocketPath(); err == nil {
			_ = busd.PublishEvent(sockPath, topics.CronEntryUpdated, map[string]string{
				"old_name": ov.original.Name,
				"new_name": updated.Name,
			})
		}
	}

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
