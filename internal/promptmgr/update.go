package promptmgr

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"

	"github.com/powerglove-dev/gl1tch/internal/executor"
	"github.com/powerglove-dev/gl1tch/internal/modal"
	"github.com/powerglove-dev/gl1tch/internal/store"
	"github.com/powerglove-dev/gl1tch/internal/systemprompts"
	"github.com/powerglove-dev/gl1tch/internal/tuikit"
)

// Message types

type promptsLoadedMsg struct{ prompts []store.Prompt }
type promptSavedMsg struct{ id int64 }
type runTokenMsg struct{ token string }
type runDoneMsg struct{ full string }
type runErrMsg struct{ err error }
type statusClearMsg struct{}

// runnerTurn is one turn in a follow-up conversation thread.
type runnerTurn struct {
	role    string // "user" or "assistant"
	content string
}


type spinnerTickMsg struct{}

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return spinnerTickMsg{} })
}

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

// savePromptCmd inserts or updates a prompt and returns a promptSavedMsg
// carrying the (possibly new) ID so the model can update editingPrompt.ID.
func savePromptCmd(st *store.Store, p store.Prompt) tea.Cmd {
	return func() tea.Msg {
		if p.ID == 0 {
			id, err := st.InsertPrompt(context.Background(), p)
			if err != nil {
				return runErrMsg{err: err}
			}
			return promptSavedMsg{id: id}
		}
		if err := st.UpdatePrompt(context.Background(), p); err != nil {
			return runErrMsg{err: err}
		}
		return promptSavedMsg{id: p.ID}
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
		// Resize body textarea to fill available editor height/width.
		// Mirror the layout math from buildEditorRows:
		// contentH=height-2, editorH=contentH/2
		// reserved: boxTop+titleLabel+titleInput+blank+bodyLabel = 5
		// tail: modelRow+cwdRow+blank+hint+boxBot = 5
		contentH := m.height - 2
		editorH := contentH / 2
		bodyH := editorH - 5 - 5
		if bodyH < 2 {
			bodyH = 2
		}
		m.bodyInput.SetHeight(bodyH)
		rightW := m.width - m.width/3
		bodyW := rightW - 4 // 2 border chars + 2 space indent
		if bodyW < 10 {
			bodyW = 10
		}
		m.bodyInput.SetWidth(bodyW)

	case tea.KeyMsg:
		// When agent picker overlay is active, route all keys through it.
		if m.agentPickerActive {
			newPicker, ev := m.agentPicker.Update(msg)
			m.agentPicker = newPicker
			switch ev {
			case modal.AgentPickerConfirmed:
				m.agentPickerActive = false
				m.editingPrompt.ModelSlug = m.agentPicker.SelectedProviderID() + "/" + m.agentPicker.SelectedModelID()
			case modal.AgentPickerCancelled:
				m.agentPickerActive = false
			}
			return m, nil
		}

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

	case promptSavedMsg:
		m.editingPrompt.ID = msg.id
		return m, reloadPromptsCmd(m.store)

	case promptsLoadedMsg:
		if msg.prompts != nil {
			m.prompts = msg.prompts
		} else {
			// empty msg means we need to reload (e.g. after delete)
			return m, reloadPromptsCmd(m.store)
		}
		m.applyFilter()

	case runDoneMsg:
		m.runnerOutput = msg.full
		m.runnerStreaming = false
		m.runCancel = nil
		m.focusPanel = 2
		m.runnerTurns = append(m.runnerTurns, runnerTurn{role: "assistant", content: msg.full})
		if m.editingPrompt.ID != 0 {
			return m, tea.Batch(saveResponseCmd(m.store, m.editingPrompt.ID, msg.full))
		}

	case runErrMsg:
		m.runnerErrMsg = msg.err.Error()
		m.runnerStreaming = false
		m.runCancel = nil

	case spinnerTickMsg:
		if m.runnerStreaming {
			m.spinnerIdx = (m.spinnerIdx + 1) % len(spinnerFrames)
			return m, spinnerTickCmd()
		}

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
			m.agentPicker = m.agentPicker.SelectBySlug(p.ModelSlug)
			m.dirPicker = modal.NewDirPickerModel()
			m.dirPickerActive = false
			m.editorSubFocus = 0
			m.titleInput.Focus()
			m.focusPanel = 1
		}

	case "tab":
		// cycle 0→1→2→0; sync editor focus when entering editor panel
		m.focusPanel = (m.focusPanel + 1) % 3
		if m.focusPanel == 1 {
			m.syncEditorFocus()
		}

	case "shift+tab":
		// cycle 0→2→1→0
		m.focusPanel = (m.focusPanel + 2) % 3
		if m.focusPanel == 1 {
			m.syncEditorFocus()
		}

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
		// ModelSlug and CWD are already stored in m.editingPrompt via picker handlers.
		// savePromptCmd returns promptSavedMsg which updates editingPrompt.ID and reloads the list.
		return m, savePromptCmd(m.store, p)

	case "ctrl+r":
		return m.startFreshRun()

	case "enter":
		switch m.editorSubFocus {
		case 2:
			m.agentPickerActive = true
			return m, nil
		case 3:
			m.dirPickerActive = true
			return m, modal.DirPickerInit()
		default:
			// title or body: delegate so textarea/textinput handle Enter normally.
			var cmd tea.Cmd
			switch m.editorSubFocus {
			case 0:
				m.titleInput, cmd = m.titleInput.Update(msg)
			case 1:
				m.bodyInput, cmd = m.bodyInput.Update(msg)
			}
			return m, cmd
		}

	case "tab":
		// Advance sub-focus; when wrapping past last slot, move to runner panel.
		if m.editorSubFocus == 3 {
			m.editorSubFocus = 0
			m.syncEditorFocus()
			m.focusPanel = 2
		} else {
			m.editorSubFocus++
			m.syncEditorFocus()
		}

	case "shift+tab":
		// Reverse sub-focus; when at first slot, move to list panel.
		if m.editorSubFocus == 0 {
			m.focusPanel = 0
		} else {
			m.editorSubFocus--
			m.syncEditorFocus()
		}

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
		// case 2: model picker — no text input; enter opens overlay
		// case 3: CWD dir picker — no text input; enter opens overlay
		}
		return m, cmd
	}

	return m, nil
}

// syncEditorFocus calls Focus/Blur on editor inputs based on editorSubFocus.
// Sub-focus 2 (model picker) and 3 (CWD dir picker) are handled via overlays,
// not text inputs.
func (m *Model) syncEditorFocus() {
	m.titleInput.Blur()
	m.bodyInput.Blur()
	switch m.editorSubFocus {
	case 0:
		m.titleInput.Focus()
	case 1:
		m.bodyInput.Focus()
	// case 2: model picker — no text input to focus.
	// case 3: CWD dir picker — no text input to focus.
	}
}

// updateRunnerPanel handles key events when focusPanel == 2.
func (m *Model) updateRunnerPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// When follow-up input is active, route most keys to it.
	if m.followUpActive {
		switch msg.String() {
		case "esc":
			m.followUpActive = false
			m.followUpInput.Blur()
			return m, nil
		case "ctrl+r", "enter":
			reply := strings.TrimSpace(m.followUpInput.Value())
			if reply == "" {
				return m, nil
			}
			// Append user turn and build full conversation context.
			m.runnerTurns = append(m.runnerTurns, runnerTurn{role: "user", content: reply})
			input := buildConversationContext(m.runnerTurns)
			m.followUpInput.SetValue("")
			m.followUpActive = false
			m.followUpInput.Blur()
			slug := m.editingPrompt.ModelSlug
			if slug == "" {
				m.runnerErrMsg = "no model selected"
				return m, nil
			}
			providerID, modelID, _ := strings.Cut(slug, "/")
			p, ok := m.executorMgr.Get(providerID)
			if !ok {
				m.runnerErrMsg = fmt.Sprintf("executor %q not found", providerID)
				return m, nil
			}
			if m.runCancel != nil {
				m.runCancel()
			}
			ctx, cancel := context.WithCancel(context.Background())
			m.runCancel = cancel
			m.runnerStreaming = true
			m.spinnerIdx = 0
			m.runnerOutput = ""
			m.runnerErrMsg = ""
			m.runnerScrollOffset = 0
			return m, tea.Batch(runExecutorCmd(ctx, p, input, m.editingPrompt.CWD, modelID), spinnerTickCmd())
		default:
			var cmd tea.Cmd
			m.followUpInput, cmd = m.followUpInput.Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "ctrl+r":
		return m.startFreshRun()
	case "ctrl+c", "esc":
		if m.runnerStreaming && m.runCancel != nil {
			m.runCancel()
			m.runnerStreaming = false
			m.runCancel = nil
		} else {
			m.focusPanel = 1
		}
	case "r":
		// Open follow-up input when there's a response to reply to.
		if m.runnerOutput != "" && !m.runnerStreaming {
			m.followUpActive = true
			m.followUpInput.Focus()
		}
	case "p":
		// Promote last response into the body editor for review/editing/saving.
		if m.runnerOutput != "" && !m.runnerStreaming {
			m.bodyInput.SetValue(m.runnerOutput)
			m.focusPanel = 1
			m.editorSubFocus = 1
			m.syncEditorFocus()
			m.statusMsg = "response promoted to body — review and ctrl+s to save"
		}
	case "j", "down":
		m.runnerScrollOffset++
	case "k", "up":
		if m.runnerScrollOffset > 0 {
			m.runnerScrollOffset--
		}
	case "tab":
		m.focusPanel = (m.focusPanel + 1) % 3
		if m.focusPanel == 1 {
			m.syncEditorFocus()
		}
	case "shift+tab":
		m.focusPanel = (m.focusPanel + 2) % 3
		if m.focusPanel == 1 {
			m.syncEditorFocus()
		}
	}
	return m, nil
}

// buildConversationContext constructs the full conversation string to send to the AI.
// The first user turn is the original prompt body; subsequent turns are appended.
// startFreshRun cancels any active run, injects the prompt-builder prefix, and
// fires a new run from the current body + model. Safe to call from any panel.
func (m *Model) startFreshRun() (tea.Model, tea.Cmd) {
	if m.runCancel != nil {
		m.runCancel()
		m.runCancel = nil
	}
	body := m.bodyInput.Value()
	slug := m.editingPrompt.ModelSlug
	if slug == "" {
		m.runnerErrMsg = "no model selected"
		return m, nil
	}
	providerID, modelID, _ := strings.Cut(slug, "/")
	p, ok := m.executorMgr.Get(providerID)
	if !ok {
		m.runnerErrMsg = fmt.Sprintf("executor %q not found", providerID)
		return m, nil
	}
	input := systemprompts.Load(systemprompts.PromptBuilder) + body
	m.runnerTurns = []runnerTurn{{role: "user", content: input}}
	m.followUpActive = false
	m.followUpInput.SetValue("")
	m.followUpInput.Blur()
	ctx, cancel := context.WithCancel(context.Background())
	m.runCancel = cancel
	m.runnerStreaming = true
	m.spinnerIdx = 0
	m.runnerOutput = ""
	m.runnerErrMsg = ""
	m.runnerScrollOffset = 0
	m.focusPanel = 2
	return m, tea.Batch(runExecutorCmd(ctx, p, input, m.editingPrompt.CWD, modelID), spinnerTickCmd())
}

func buildConversationContext(turns []runnerTurn) string {
	if len(turns) == 0 {
		return ""
	}
	var sb strings.Builder
	// Original prompt body is always the first user turn.
	sb.WriteString(turns[0].content)
	if len(turns) == 1 {
		return sb.String()
	}
	sb.WriteString("\n\n---\n")
	for _, t := range turns[1:] {
		if t.role == "assistant" {
			sb.WriteString("\nAssistant: ")
		} else {
			sb.WriteString("\nUser: ")
		}
		sb.WriteString(t.content)
		sb.WriteString("\n")
	}
	return sb.String()
}

// runExecutorCmd executes the executor with input and returns a runDoneMsg or runErrMsg.
func runExecutorCmd(ctx context.Context, p executor.Executor, input, cwd, modelID string) tea.Cmd {
	return func() tea.Msg {
		pr, pw := io.Pipe()
		vars := map[string]string{}
		if cwd != "" {
			vars["cwd"] = cwd
		}
		if modelID != "" {
			vars["model"] = modelID
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
