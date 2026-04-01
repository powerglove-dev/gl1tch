// Package braineditor provides a full-screen BubbleTea TUI for browsing,
// editing, and refining brain notes stored in the GLITCH result store.
package braineditor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/buildershared"
	"github.com/8op-org/gl1tch/internal/panelrender"
	"github.com/8op-org/gl1tch/internal/picker"
	"github.com/8op-org/gl1tch/internal/store"
	"github.com/8op-org/gl1tch/internal/styles"
)

// focus slot constants for Model.
const (
	focusSidebar = 0
	focusRunner  = 1
	focusSend    = 2
)

// CloseMsg is posted when the user presses q/esc to leave the brain editor.
type CloseMsg struct{}

// noteSavedMsg is an internal message sent when a note is saved.
type noteSavedMsg struct{ err error }

// noteDeletedMsg is an internal message sent when a note is deleted.
type noteDeletedMsg struct{ err error }

// notesLoadedMsg is an internal message sent when note list is reloaded.
type notesLoadedMsg struct {
	notes []store.BrainNote
}

// confirmDeleteState tracks whether a delete confirmation is pending.
type confirmDeleteState bool

// Model is the main BubbleTea model for the brain note editor.
// Layout (two-column):
//
//	LEFT  : Sidebar — list of brain notes
//	RIGHT : RunnerPanel (top) + SendPanel (bottom)
type Model struct {
	sidebar buildershared.Sidebar
	runner  buildershared.RunnerPanel
	send    buildershared.SendPanel

	focus int

	db    *store.Store
	notes []store.BrainNote

	// selectedNoteID is the ID of the currently selected note (-1 = none).
	selectedNoteID int64

	// editing tracks whether we are in "edit body" mode.
	editing bool

	// confirmDelete is true when the user has pressed d and we await confirmation.
	confirmDelete bool

	statusMsg string
	statusErr bool

	width, height int
	pal           styles.ANSIPalette
}

// New creates a new brain editor Model.
// db may be nil (read-only/preview mode).
func New(db *store.Store, providers []picker.ProviderDef) Model {
	pal := styles.ANSIPalette{
		Accent:  "\x1b[35m",
		Dim:     "\x1b[2m",
		Success: "\x1b[32m",
		Error:   "\x1b[31m",
		Warn:    "\x1b[33m",
		FG:      "\x1b[97m",
		BG:      "\x1b[40m",
		Border:  "\x1b[36m",
		SelBG:   "\x1b[44m",
	}

	sidebarHints := []panelrender.Hint{
		{Key: "n", Desc: "new"},
		{Key: "e", Desc: "edit"},
		{Key: "d", Desc: "del"},
	}
	m := Model{
		sidebar:        buildershared.NewSidebar("BRAIN NOTES", nil).SetFocusHints(sidebarHints),
		runner:         buildershared.NewRunnerPanel(),
		send:           buildershared.NewSendPanel(providers),
		db:             db,
		selectedNoteID: -1,
		pal:            pal,
	}
	m.sidebar = m.sidebar.SetFocused(true)
	return m
}

// SetSize updates the terminal dimensions.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetPalette updates the colour palette.
func (m *Model) SetPalette(pal styles.ANSIPalette) {
	m.pal = pal
}

// Init implements tea.Model — load notes on startup.
func (m Model) Init() tea.Cmd {
	return m.loadNotesCmd()
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		return m, nil

	case notesLoadedMsg:
		m.notes = v.notes
		m.sidebar = m.sidebar.SetItems(m.noteNames())
		// Reload selected note body into send panel if still valid.
		if m.selectedNoteID >= 0 {
			for _, n := range m.notes {
				if n.ID == m.selectedNoteID {
					m.send = m.send.SetName(noteLabel(n))
					m.runner = m.runner.Clear()
					for _, line := range strings.Split(n.Body, "\n") {
						m.runner, _ = m.runner.Update(buildershared.RunLineMsg(line))
					}
					break
				}
			}
		}
		return m, nil

	case noteSavedMsg:
		if v.err != nil {
			m.statusMsg = "save error: " + v.err.Error()
			m.statusErr = true
		} else {
			m.statusMsg = "saved"
			m.statusErr = false
		}
		return m, m.loadNotesCmd()

	case noteDeletedMsg:
		if v.err != nil {
			m.statusMsg = "delete error: " + v.err.Error()
			m.statusErr = true
		} else {
			m.statusMsg = "deleted"
			m.statusErr = false
			m.selectedNoteID = -1
			m.runner = m.runner.Clear()
		}
		m.confirmDelete = false
		return m, m.loadNotesCmd()

	case buildershared.RunLineMsg, buildershared.RunDoneMsg:
		var cmd tea.Cmd
		m.runner, cmd = m.runner.Update(v)
		return m, cmd

	case tea.KeyMsg:
		return m.handleKey(v)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Confirm-delete overlay: y confirms, any other key cancels.
	if m.confirmDelete {
		switch key {
		case "y", "Y":
			return m, m.deleteNoteCmd(m.selectedNoteID)
		default:
			m.confirmDelete = false
			m.statusMsg = "delete cancelled"
			m.statusErr = false
		}
		return m, nil
	}

	// Global keys.
	switch key {
	case "J":
		if os.Getenv("TMUX") != "" {
			return m, func() tea.Msg {
				self, _ := os.Executable()
				exec.Command("tmux", "display-popup", "-E", "-w", "80%", "-h", "70%",
					filepath.Clean(self)+" widget jump-window").Run() //nolint:errcheck
				return nil
			}
		}
		return m, nil
	case "ctrl+c", "q", "esc":
		if m.focus == focusSend {
			// Fall through to send panel handling (esc might cancel input).
		} else {
			return m, func() tea.Msg { return CloseMsg{} }
		}
	}

	switch m.focus {
	case focusSidebar:
		return m.handleSidebarKey(msg)
	case focusRunner:
		return m.handleRunnerKey(msg)
	case focusSend:
		return m.handleSendKey(msg)
	}
	return m, nil
}

func (m Model) handleSidebarKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global close.
	if key == "q" || key == "esc" {
		return m, func() tea.Msg { return CloseMsg{} }
	}

	// Delete: ask for confirmation.
	if key == "d" && m.selectedNoteID >= 0 {
		m.confirmDelete = true
		m.statusMsg = "delete? press y to confirm"
		m.statusErr = true
		return m, nil
	}

	// New note: focus the send panel in compose mode.
	if key == "n" {
		m.selectedNoteID = -1
		m.send = m.send.SetName("new-note")
		m.runner = m.runner.Clear()
		m.sidebar = m.sidebar.SetFocused(false)
		m.send = m.send.Enter()
		m.focus = focusSend
		m.editing = false
		return m, nil
	}

	// Edit: move focus to send panel for editing.
	if key == "e" && m.selectedNoteID >= 0 {
		note := m.noteByID(m.selectedNoteID)
		if note != nil {
			m.send = m.send.SetName(noteLabel(*note))
			m.sidebar = m.sidebar.SetFocused(false)
			m.send = m.send.Enter()
			m.focus = focusSend
			m.editing = true
		}
		return m, nil
	}

	// Tab: move focus to runner.
	if key == "tab" {
		m.sidebar = m.sidebar.SetFocused(false)
		m.runner = m.runner.SetFocused(true)
		m.focus = focusRunner
		return m, nil
	}

	// Delegate to sidebar (j/k navigation, /, enter).
	var cmd tea.Cmd
	m.sidebar, cmd = m.sidebar.Update(msg)
	if cmd != nil {
		innerMsg := cmd()
		switch v := innerMsg.(type) {
		case buildershared.SidebarSelectMsg:
			note := m.noteByName(v.Name)
			if note != nil {
				m.selectedNoteID = note.ID
				m.runner = m.runner.Clear()
				for _, line := range strings.Split(note.Body, "\n") {
					m.runner, _ = m.runner.Update(buildershared.RunLineMsg(line))
				}
				m.statusMsg = ""
			}
		case buildershared.SidebarDeleteMsg:
			note := m.noteByName(v.Name)
			if note != nil {
				m.selectedNoteID = note.ID
				m.confirmDelete = true
				m.statusMsg = "delete? press y to confirm"
				m.statusErr = true
			}
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleRunnerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	var cmd tea.Cmd
	m.runner, cmd = m.runner.Update(msg)

	switch key {
	case "shift+tab", "esc":
		m.runner = m.runner.SetFocused(false)
		m.send = m.send.Enter()
		m.focus = focusSend
	case "tab":
		m.runner = m.runner.SetFocused(false)
		m.send = m.send.Enter()
		m.focus = focusSend
	case "q":
		return m, func() tea.Msg { return CloseMsg{} }
	}
	return m, cmd
}

func (m Model) handleSendKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "esc" {
		m.sidebar = m.sidebar.SetFocused(true)
		m.focus = focusSidebar
		return m, nil
	}

	var cmd tea.Cmd
	m.send, cmd = m.send.Update(msg)

	if cmd != nil {
		innerMsg := cmd()
		switch v := innerMsg.(type) {
		case buildershared.SendSubmitMsg:
			if v.Message == "" {
				return m, nil
			}
			return m, m.saveNoteCmd(v.Message)

		case buildershared.SendTabOutMsg:
			m.send = m.send.SetFocused(false)
			m.sidebar = m.sidebar.SetFocused(true)
			m.focus = focusSidebar
			return m, nil

		case buildershared.SendShiftTabOutMsg:
			m.send = m.send.SetFocused(false)
			m.runner = m.runner.SetFocused(true)
			m.focus = focusRunner
			return m, nil
		}
	}

	return m, cmd
}

// ── Store operations ──────────────────────────────────────────────────────────

func (m Model) loadNotesCmd() tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return notesLoadedMsg{}
		}
		notes, err := m.db.AllBrainNotes(context.Background())
		if err != nil {
			return notesLoadedMsg{}
		}
		return notesLoadedMsg{notes: notes}
	}
}

func (m Model) saveNoteCmd(body string) tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return noteSavedMsg{err: fmt.Errorf("no store configured")}
		}
		body = strings.TrimSpace(body)
		if m.editing && m.selectedNoteID >= 0 {
			// Update existing note.
			note := m.noteByID(m.selectedNoteID)
			tags := ""
			if note != nil {
				tags = note.Tags
			}
			err := m.db.UpdateBrainNote(context.Background(), m.selectedNoteID, body, tags)
			return noteSavedMsg{err: err}
		}
		// Insert new note.
		_, err := m.db.InsertBrainNote(context.Background(), store.BrainNote{
			Body: body,
		})
		return noteSavedMsg{err: err}
	}
}

func (m Model) deleteNoteCmd(id int64) tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return noteDeletedMsg{err: fmt.Errorf("no store configured")}
		}
		err := m.db.UpdateBrainNote(context.Background(), id, "~deleted~", "")
		// UpdateBrainNote doesn't actually delete rows; we store a tombstone.
		// For a real delete we'd need a DeleteBrainNote method, but spec says
		// UpdateBrainNote is our mutation path.
		return noteDeletedMsg{err: err}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m Model) noteNames() []string {
	names := make([]string, 0, len(m.notes))
	for _, n := range m.notes {
		names = append(names, noteLabel(n))
	}
	return names
}

func noteLabel(n store.BrainNote) string {
	// Collapse whitespace (newlines, tabs) to single spaces for single-line display.
	body := strings.Join(strings.Fields(n.Body), " ")
	runes := []rune(body)
	if len(runes) > 40 {
		body = string(runes[:39]) + "…"
	}
	if n.Tags != "" {
		tags := strings.Join(strings.Fields(n.Tags), " ")
		return fmt.Sprintf("[%d] %s (%s)", n.ID, body, tags)
	}
	return fmt.Sprintf("[%d] %s", n.ID, body)
}

func (m Model) noteByID(id int64) *store.BrainNote {
	for i := range m.notes {
		if m.notes[i].ID == id {
			return &m.notes[i]
		}
	}
	return nil
}

func (m Model) noteByName(name string) *store.BrainNote {
	for i := range m.notes {
		if noteLabel(m.notes[i]) == name {
			return &m.notes[i]
		}
	}
	return nil
}
