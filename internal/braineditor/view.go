package braineditor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/adam-stokes/orcai/internal/panelrender"
)

// View implements tea.Model — renders the two-column brain editor.
func (m Model) View() string {
	w, h := m.width, m.height
	if w <= 0 {
		w = 120
	}
	if h <= 0 {
		h = 40
	}

	pal := m.pal
	leftW := w / 4
	if leftW < 24 {
		leftW = 24
	}
	rightW := w - leftW
	bodyH := h

	// Left column: sidebar.
	leftLines := m.sidebar.SetFocused(m.focus == focusSidebar).View(leftW, bodyH, pal)

	// Right column: runner + send.
	rightLines := m.buildRight(rightW, bodyH)

	var rows []string
	for i := range bodyH {
		var l, r string
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		lv := lipgloss.Width(l)
		if lv < leftW {
			l = l + strings.Repeat(" ", leftW-lv)
		}
		rows = append(rows, l+r)
	}

	base := strings.Join(rows, "\n")

	// Confirm-delete overlay.
	if m.confirmDelete {
		confirmMsg := fmt.Sprintf(
			"%s\x1b[1m┌─────────────────────────────────────┐\x1b[0m\n"+
				"%s\x1b[1m│  Delete this note?                  │\x1b[0m\n"+
				"%s\x1b[1m│  [y] confirm  ·  any other = cancel │\x1b[0m\n"+
				"%s\x1b[1m└─────────────────────────────────────┘\x1b[0m",
			pal.Error, pal.Error, pal.Error, pal.Error,
		)
		return panelrender.OverlayCenter(base, confirmMsg, w, h)
	}

	return base
}

func (m Model) buildRight(w, h int) []string {
	sendH := 8
	runnerH := h - sendH
	if runnerH < 3 {
		runnerH = 3
	}

	rn := m.runner.SetFocused(m.focus == focusRunner)
	snd := m.send.SetFocused(m.focus == focusSend)

	var rows []string
	rows = append(rows, rn.View(w, runnerH, m.pal)...)
	rows = append(rows, snd.View(w, sendH, m.pal)...)
	return rows
}


// Helpers for stable formatting (used by statusMsg).
var _ = fmt.Sprintf // suppress unused import warning
