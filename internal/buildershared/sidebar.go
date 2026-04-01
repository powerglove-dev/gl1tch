// Package buildershared provides reusable sub-model components shared between
// the pipelineeditor and promptbuilder TUI packages.
package buildershared

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/panelrender"
	"github.com/8op-org/gl1tch/internal/styles"
)

// SidebarSelectMsg is posted when the user presses enter on a sidebar item.
type SidebarSelectMsg struct{ Name string }

// SidebarDeleteMsg is posted when the user confirms deletion of a sidebar item.
type SidebarDeleteMsg struct{ Name string }

// ANSI helpers (package-local).
const (
	sbRst = "\x1b[0m"
	sbBld = "\x1b[1m"
	sbDim = "\x1b[2m"
)

// Sidebar is a searchable, scrollable list sub-model.
// The parent composes this as a field and delegates key/msg handling.
type Sidebar struct {
	items         []string
	sel           int
	scroll        int
	searching     bool
	query         string
	focused       bool
	confirmDelete bool
	title         string
	focusHints    []panelrender.Hint // overrides default hints when focused (nil = use defaults)
}

// NewSidebar creates a new Sidebar with the given title and items.
func NewSidebar(title string, items []string) Sidebar {
	return Sidebar{
		title: title,
		items: items,
	}
}

// SetFocusHints overrides the hint bar shown when the sidebar is focused.
// Pass nil to restore the default hints.
func (s Sidebar) SetFocusHints(hints []panelrender.Hint) Sidebar {
	s.focusHints = hints
	return s
}

// SetItems replaces the item list.
func (s Sidebar) SetItems(items []string) Sidebar {
	s.items = items
	// Clamp selection.
	if len(s.items) == 0 {
		s.sel = 0
		s.scroll = 0
	} else if s.sel >= len(s.items) {
		s.sel = len(s.items) - 1
	}
	return s
}

// SetFocused sets whether the sidebar has keyboard focus.
func (s Sidebar) SetFocused(b bool) Sidebar {
	s.focused = b
	return s
}

// Focus returns whether the sidebar has keyboard focus.
func (s Sidebar) Focus() bool { return s.focused }

// Filtered returns the items matching the current search query.
func (s Sidebar) Filtered() []string {
	if s.query == "" {
		return s.items
	}
	q := strings.ToLower(s.query)
	var out []string
	for _, item := range s.items {
		if strings.Contains(strings.ToLower(item), q) {
			out = append(out, item)
		}
	}
	return out
}

// SelectedName returns the name of the currently selected item, or "".
func (s Sidebar) SelectedName() string {
	filtered := s.Filtered()
	if s.sel < 0 || s.sel >= len(filtered) {
		return ""
	}
	return filtered[s.sel]
}

// Update handles key events and returns the updated Sidebar plus any command.
func (s Sidebar) Update(msg tea.Msg) (Sidebar, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	key := keyMsg.String()

	// Search mode intercepts character input.
	if s.searching {
		switch key {
		case "esc", "ctrl+c":
			s.searching = false
			s.query = ""
			return s, nil
		case "enter":
			s.searching = false
			return s, nil
		case "backspace", "ctrl+h":
			runes := []rune(s.query)
			if len(runes) > 0 {
				s.query = string(runes[:len(runes)-1])
			}
			return s, nil
		default:
			if len(keyMsg.Runes) == 1 {
				s.query += string(keyMsg.Runes[0])
			}
			return s, nil
		}
	}

	// Confirm-delete mode.
	if s.confirmDelete {
		switch key {
		case "y":
			name := s.SelectedName()
			s.confirmDelete = false
			if name != "" {
				return s, func() tea.Msg { return SidebarDeleteMsg{Name: name} }
			}
		case "N", "n", "esc":
			s.confirmDelete = false
		}
		return s, nil
	}

	filtered := s.Filtered()

	switch key {
	case "j", "down":
		if s.sel < len(filtered)-1 {
			s.sel++
		}
	case "k", "up":
		if s.sel > 0 {
			s.sel--
		}
	case "enter":
		if len(filtered) > 0 && s.sel < len(filtered) {
			name := filtered[s.sel]
			return s, func() tea.Msg { return SidebarSelectMsg{Name: name} }
		}
	case "d":
		if len(filtered) > 0 {
			s.confirmDelete = true
		}
	case "/":
		s.searching = true
		s.query = ""
	}
	return s, nil
}

// View renders the sidebar into box rows of the given dimensions.
func (s Sidebar) View(w, h int, pal styles.ANSIPalette) []string {
	borderColor := pal.Border
	if s.focused {
		borderColor = pal.Accent
	}

	var rows []string
	rows = append(rows, panelrender.BoxTop(w, s.title, borderColor, pal.Accent))

	filtered := s.Filtered()
	// Visible body rows = h - top(1) - hint(1) - bottom(1) = h-3.
	// -1 if search row is active.
	visibleRows := h - 3
	if s.searching {
		visibleRows--
	}
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Scroll clamping.
	scroll := s.scroll
	if scroll < 0 {
		scroll = 0
	}
	if scroll > len(filtered)-visibleRows && len(filtered) > visibleRows {
		scroll = len(filtered) - visibleRows
	}

	for i := range visibleRows {
		idx := scroll + i
		if idx >= len(filtered) {
			rows = append(rows, panelrender.BoxRow("", w, borderColor))
			continue
		}
		name := filtered[idx]
		// Truncate name to fit inner box width: w-2 (borders) - 4 (indent+cursor).
		maxName := w - 6
		if maxName < 4 {
			maxName = 4
		}
		runes := []rune(name)
		if len(runes) > maxName {
			name = string(runes[:maxName-1]) + "…"
		}
		var content string
		if idx == s.sel && s.focused {
			content = pal.Accent + "> " + sbRst + pal.FG + name + sbRst
		} else if idx == s.sel {
			content = sbDim + "> " + pal.FG + name + sbRst
		} else {
			content = sbDim + "  " + name + sbRst
		}
		rows = append(rows, panelrender.BoxRow("  "+content, w, borderColor))
	}

	// Search input row — only shown when actively searching.
	if s.searching {
		searchContent := pal.Accent + "/" + sbRst + s.query + "█"
		rows = append(rows, panelrender.BoxRow("  "+searchContent, w, borderColor))
	}

	// Hint row — only when focused.
	if s.focused {
		var hints []panelrender.Hint
		if s.confirmDelete {
			hints = []panelrender.Hint{{Key: "y", Desc: "confirm"}, {Key: "N", Desc: "cancel"}}
		} else if s.focusHints != nil {
			hints = s.focusHints
		} else {
			hints = []panelrender.Hint{
				{Key: "d", Desc: "del"},
				{Key: "/", Desc: "search"},
				{Key: "esc", Desc: "back"},
			}
		}
		rows = append(rows, panelrender.BoxRow(panelrender.HintBar(hints, w-2, pal), w, borderColor))
	} else {
		rows = append(rows, panelrender.BoxRow("", w, borderColor))
	}
	rows = append(rows, panelrender.BoxBot(w, borderColor))

	return rows
}
