package braineditor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	bodyH := h - 1 // reserve one row for the top bar

	// Top bar.
	topBar := pal.Accent + "\x1b[1m BRAIN NOTES\x1b[0m"
	if m.statusMsg != "" {
		sep := pal.Dim + " · \x1b[0m"
		if m.statusErr {
			topBar += sep + pal.Error + m.statusMsg + "\x1b[0m"
		} else {
			topBar += sep + pal.Success + m.statusMsg + "\x1b[0m"
		}
	}
	// Key hint strip.
	hints := pal.Dim + " [n]ew  [e]dit  [d]elete  [q]uit\x1b[0m"
	topBar = padRight(topBar, w-lipgloss.Width(hints)) + hints

	// Confirm-delete overlay row.
	if m.confirmDelete {
		confirmLine := pal.Error + "\x1b[1m  Delete this note? [y] confirm, any other key = cancel\x1b[0m"
		topBar = confirmLine
	}

	// Left column: sidebar.
	leftLines := m.sidebar.SetFocused(m.focus == focusSidebar).View(leftW, bodyH, pal)

	// Right column: runner + send.
	rightLines := m.buildRight(rightW, bodyH)

	var rows []string
	rows = append(rows, topBar)
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

	return strings.Join(rows, "\n")
}

func (m Model) buildRight(w, h int) []string {
	sendH := 6
	if h < 20 {
		sendH = 5
	}
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

// padRight pads s to width n with spaces.
func padRight(s string, n int) string {
	v := lipgloss.Width(s)
	if v >= n {
		return s
	}
	return s + strings.Repeat(" ", n-v)
}

// Helpers for stable formatting (used by statusMsg).
var _ = fmt.Sprintf // suppress unused import warning
