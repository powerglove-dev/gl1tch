package promptmgr

import (
	"context"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"

	"github.com/adam-stokes/orcai/internal/modal"
	"github.com/adam-stokes/orcai/internal/plugin"
	"github.com/adam-stokes/orcai/internal/store"
	"github.com/adam-stokes/orcai/internal/tuikit"
)

// Message types

type promptsLoadedMsg struct{ prompts []store.Prompt }
type modelSlugsLoadedMsg struct{ slugs []string }
type runTokenMsg struct{ token string }
type runDoneMsg struct{ full string }
type runErrMsg struct{ err error }
type statusClearMsg struct{}

// loadPromptsCmd fetches all prompts from the store.
func loadPromptsCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		prompts, err := st.ListPrompts(context.Background())
		if err != nil {
			return runErrMsg{err: err}
		}
		return promptsLoadedMsg{prompts: prompts}
	}
}

// loadModelSlugsCmd fetches available model slugs from the plugin manager.
func loadModelSlugsCmd(mgr *plugin.Manager) tea.Cmd {
	return func() tea.Msg {
		if mgr == nil {
			return modelSlugsLoadedMsg{slugs: []string{}}
		}
		var slugs []string
		for _, p := range mgr.List() {
			slugs = append(slugs, p.Name())
		}
		return modelSlugsLoadedMsg{slugs: slugs}
	}
}

// deletePromptCmd deletes the prompt with the given ID and triggers a reload.
func deletePromptCmd(st *store.Store, id int64) tea.Cmd {
	return func() tea.Msg {
		if err := st.DeletePrompt(context.Background(), id); err != nil {
			return runErrMsg{err: err}
		}
		return promptsLoadedMsg{} // trigger reload path; actual data comes from reloadPromptsCmd
	}
}

// reloadPromptsCmd calls loadPromptsCmd — used after mutations to refresh the list.
func reloadPromptsCmd(st *store.Store) tea.Cmd {
	return loadPromptsCmd(st)
}

// savePromptCmd inserts or updates a prompt then triggers a list reload.
func savePromptCmd(st *store.Store, p store.Prompt) tea.Cmd {
	return func() tea.Msg {
		var err error
		if p.ID == 0 {
			_, err = st.InsertPrompt(context.Background(), p)
		} else {
			err = st.UpdatePrompt(context.Background(), p)
		}
		if err != nil {
			return runErrMsg{err: err}
		}
		return promptsLoadedMsg{} // reload list
	}
}

// applyFilter fuzzy-filters m.prompts into m.filtered based on filterInput.
// Empty query copies all prompts. Resets selectedIdx and scrollOffset.
func (m *Model) applyFilter() {
	query := m.filterInput.Value()
	if query == "" {
		dst := make([]store.Prompt, len(m.prompts))
		copy(dst, m.prompts)
		m.filtered = dst
	} else {
		// Build a slice of strings to match against (title + " " + body).
		sources := make([]string, len(m.prompts))
		for i, p := range m.prompts {
			sources[i] = p.Title + " " + p.Body
		}
		matches := fuzzy.Find(query, sources)
		m.filtered = make([]store.Prompt, 0, len(matches))
		for _, match := range matches {
			m.filtered = append(m.filtered, m.prompts[match.Index])
		}
	}
	m.selectedIdx = 0
	m.scrollOffset = 0
}

// clampList clamps selectedIdx and scrollOffset to valid ranges given the
// number of visible rows in the list panel.
func (m *Model) clampList(visible int) {
	n := len(m.filtered)
	if n == 0 {
		m.selectedIdx = 0
		m.scrollOffset = 0
		return
	}
	if m.selectedIdx >= n {
		m.selectedIdx = n - 1
	}
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
	if m.selectedIdx < m.scrollOffset {
		m.scrollOffset = m.selectedIdx
	}
	if m.selectedIdx >= m.scrollOffset+visible {
		m.scrollOffset = m.selectedIdx - visible + 1
	}
}

// visibleListRows returns the number of rows that fit in the list panel.
func (m *Model) visibleListRows() int {
	v := m.height - 4 // subtract header, filter input, borders
	if v < 1 {
		return 1
	}
	return v
}

// Update handles all incoming messages for the prompt manager.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Delegate theme messages first.
	if ts, cmd, ok := m.themeState.Handle(msg); ok {
		m.themeState = ts
		return m, cmd
	}

	// When the dir picker overlay is active, route all messages through it
	// first so it can handle its own internal walk results and key events.
	if m.dirPickerActive {
		switch msg := msg.(type) {
		case modal.DirSelectedMsg:
			m.editingPrompt.CWD = msg.Path
			m.dirPickerActive = false
			return m, nil
		case modal.DirCancelledMsg:
			m.dirPickerActive = false
			return m, nil
		default:
			var cmd tea.Cmd
			m.dirPicker, cmd = m.dirPicker.Update(msg)
			return m, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		// When the runner panel is focused, route all keys through it first so
		// ctrl+c can cancel an in-progress run instead of quitting.
		if m.focusPanel == 2 {
			return m.updateRunnerPanel(msg)
		}

		// Global quit keys.
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q", "esc":
			if m.focusPanel != 1 {
				return m, tea.Quit
			}
		}

		switch m.focusPanel {
		case 0: // list panel
			return m.updateListPanel(msg)
		case 1: // editor panel
			return m.updateEditorPanel(msg)
		}

	case promptsLoadedMsg:
		if msg.prompts != nil {
			m.prompts = msg.prompts
		} else {
			// empty msg means we need to reload (e.g. after delete)
			return m, reloadPromptsCmd(m.store)
		}
		m.applyFilter()

	case modelSlugsLoadedMsg:
		m.modelSlugs = msg.slugs

	case runDoneMsg:
		m.runnerOutput = msg.full
		m.runnerStreaming = false
		m.runCancel = nil
		m.focusPanel = 2
		if m.editingPrompt.ID != 0 {
			return m, tea.Batch(saveResponseCmd(m.store, m.editingPrompt.ID, msg.full))
		}

	case runErrMsg:
		m.runnerErrMsg = msg.err.Error()
		m.runnerStreaming = false
		m.runCancel = nil

	case tuikit.ThemeChangedMsg:
		// Already handled above via themeState.Handle.
	}

	return m, nil
}

// updateListPanel handles key events when focusPanel == 0.
func (m *Model) updateListPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	visible := m.visibleListRows()

	// If confirmDelete overlay is active, only handle y/n.
	if m.confirmDelete {
		switch msg.String() {
		case "y", "Y":
			if len(m.filtered) > 0 {
				id := m.filtered[m.selectedIdx].ID
				m.confirmDelete = false
				return m, deletePromptCmd(m.store, id)
			}
			m.confirmDelete = false
		case "n", "N", "esc":
			m.confirmDelete = false
		}
		return m, nil
	}

	// Filter input gets character keys when filter is logically "focused"
	// (panel 0 is always list, filter is the active input within it).
	switch msg.String() {
	case "j", "down":
		m.selectedIdx++
		m.clampList(visible)

	case "k", "up":
		m.selectedIdx--
		m.clampList(visible)

	case "n":
		// New prompt — clear editor and switch to editor panel.
		m.editingPrompt = store.Prompt{}
		m.titleInput.SetValue("")
		m.bodyInput.SetValue("")
		m.dirPicker = modal.NewDirPickerModel()
		m.dirPickerActive = false
		m.editorSubFocus = 0
		m.titleInput.Focus()
		m.focusPanel = 1

	case "d":
		if len(m.filtered) > 0 {
			m.confirmDelete = true
		}

	case "e", "enter":
		if len(m.filtered) > 0 {
			p := m.filtered[m.selectedIdx]
			m.editingPrompt = p
			m.titleInput.SetValue(p.Title)
			m.bodyInput.SetValue(p.Body)
			m.dirPicker = modal.NewDirPickerModel()
			m.dirPickerActive = false
			m.editorSubFocus = 0
			m.titleInput.Focus()
			m.focusPanel = 1
		}

	case "tab":
		// cycle 0→1→2→0
		m.focusPanel = (m.focusPanel + 1) % 3

	case "shift+tab":
		// cycle 0→2→1→0
		m.focusPanel = (m.focusPanel + 2) % 3

	default:
		// Delegate to filter input.
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.applyFilter()
		return m, cmd
	}

	return m, nil
}

// updateEditorPanel handles key events when focusPanel == 1.
func (m *Model) updateEditorPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+s":
		// Build prompt from current editor state.
		p := m.editingPrompt
		p.Title = m.titleInput.Value()
		p.Body = m.bodyInput.Value()
		// CWD is already stored in m.editingPrompt.CWD via DirSelectedMsg handler.
		if len(m.modelSlugs) > 0 && m.modelIdx < len(m.modelSlugs) {
			p.ModelSlug = m.modelSlugs[m.modelIdx]
		}
		return m, tea.Batch(savePromptCmd(m.store, p), reloadPromptsCmd(m.store))

	case "ctrl+r":
		// Cancel any existing run.
		if m.runCancel != nil {
			m.runCancel()
			m.runCancel = nil
		}
		body := m.bodyInput.Value()
		var slug string
		if len(m.modelSlugs) > 0 && m.modelIdx < len(m.modelSlugs) {
			slug = m.modelSlugs[m.modelIdx]
		}
		if slug == "" {
			m.runnerErrMsg = "no model selected"
			return m, nil
		}
		p, ok := m.pluginMgr.Get(slug)
		if !ok {
			m.runnerErrMsg = fmt.Sprintf("plugin %q not found", slug)
			return m, nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		m.runCancel = cancel
		m.runnerStreaming = true
		m.runnerOutput = ""
		m.runnerErrMsg = ""
		m.runnerScrollOffset = 0
		return m, runPluginCmd(ctx, p, body, m.editingPrompt.CWD)

	case "[":
		if len(m.modelSlugs) > 0 {
			m.modelIdx = (m.modelIdx - 1 + len(m.modelSlugs)) % len(m.modelSlugs)
		}

	case "]":
		if len(m.modelSlugs) > 0 {
			m.modelIdx = (m.modelIdx + 1) % len(m.modelSlugs)
		}

	case "tab":
		// Cycle editor sub-focus: title→body→cwd→title
		m.editorSubFocus = (m.editorSubFocus + 1) % 3
		m.syncEditorFocus()
		// When sub-focus lands on cwd (2), open the dir picker immediately.
		if m.editorSubFocus == 2 {
			m.dirPickerActive = true
			return m, modal.DirPickerInit()
		}

	case "shift+tab":
		// Cycle panel: 0→2→1→0
		m.focusPanel = (m.focusPanel + 2) % 3

	case "esc":
		m.focusPanel = 0

	default:
		// Delegate to active sub-input.
		var cmd tea.Cmd
		switch m.editorSubFocus {
		case 0:
			m.titleInput, cmd = m.titleInput.Update(msg)
		case 1:
			m.bodyInput, cmd = m.bodyInput.Update(msg)
		case 2:
			// CWD sub-focus: open dir picker on any key.
			m.dirPickerActive = true
			return m, modal.DirPickerInit()
		}
		return m, cmd
	}

	return m, nil
}

// syncEditorFocus calls Focus/Blur on editor inputs based on editorSubFocus.
// Sub-focus 2 (CWD) is handled via the dir picker overlay, not a text input.
func (m *Model) syncEditorFocus() {
	m.titleInput.Blur()
	m.bodyInput.Blur()
	switch m.editorSubFocus {
	case 0:
		m.titleInput.Focus()
	case 1:
		m.bodyInput.Focus()
	// case 2: CWD is managed via dirPickerActive overlay — no text input to focus.
	}
}

// updateRunnerPanel handles key events when focusPanel == 2.
func (m *Model) updateRunnerPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		if m.runnerStreaming && m.runCancel != nil {
			m.runCancel()
			m.runnerStreaming = false
			m.runCancel = nil
		} else {
			m.focusPanel = 1
		}
	case "j", "down":
		m.runnerScrollOffset++
	case "k", "up":
		if m.runnerScrollOffset > 0 {
			m.runnerScrollOffset--
		}
	case "tab":
		m.focusPanel = (m.focusPanel + 1) % 3
	case "shift+tab":
		m.focusPanel = (m.focusPanel + 2) % 3
	}
	return m, nil
}

// runPluginCmd executes the plugin with input and returns a runDoneMsg or runErrMsg.
func runPluginCmd(ctx context.Context, p plugin.Plugin, input, cwd string) tea.Cmd {
	return func() tea.Msg {
		pr, pw := io.Pipe()
		var vars map[string]string
		if cwd != "" {
			vars = map[string]string{"cwd": cwd}
		}
		go func() {
			err := p.Execute(ctx, input, vars, pw)
			pw.CloseWithError(err)
		}()
		data, err := io.ReadAll(pr)
		if err != nil && ctx.Err() == nil {
			return runErrMsg{err: fmt.Errorf("run: %w", err)}
		}
		return runDoneMsg{full: string(data)}
	}
}

// saveResponseCmd persists the runner output to the store.
func saveResponseCmd(st *store.Store, id int64, response string) tea.Cmd {
	return func() tea.Msg {
		_ = st.SavePromptResponse(context.Background(), id, response)
		return nil
	}
}
