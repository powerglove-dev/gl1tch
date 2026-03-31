package pipelineeditor

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/adam-stokes/orcai/internal/panelrender"
)


// View renders the full-screen two-column pipeline editor.
func (m Model) View(w, h int) string {
	if w <= 0 {
		w = 120
	}
	if h <= 0 {
		h = 40
	}

	leftW := w / 4
	if leftW < 20 {
		leftW = 20
	}
	rightW := w - leftW

	bodyH := h

	leftLines := m.buildLeft(leftW, bodyH)
	rightLines := m.buildRight(rightW, bodyH)

	// Merge left and right columns.
	var rows []string
	maxRows := bodyH
	for i := range maxRows {
		var l, r string
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		// Pad left to leftW visible chars.
		lv := lipgloss.Width(l)
		if lv < leftW {
			l = l + strings.Repeat(" ", leftW-lv)
		}
		rows = append(rows, l+r)
	}

	base := strings.Join(rows, "\n")

	// Overlay agent picker popup if open.
	if m.send.AgentOpen() {
		overlay := m.send.OverlayView(w, m.pal)
		return panelrender.OverlayCenter(base, overlay, w, h)
	}
	return base
}

// buildLeft delegates to the shared Sidebar sub-model.
func (m Model) buildLeft(w, h int) []string {
	sb := m.sidebar.SetFocused(m.focus == FocusList)
	return sb.View(w, h, m.pal)
}

// buildRight renders: runner (top) + send panel (bottom).
func (m Model) buildRight(w, h int) []string {
	sendH := 8
	runnerH := h - sendH
	if runnerH < 5 {
		runnerH = 5
	}

	var rows []string
	rows = append(rows, m.buildRunnerBox(w, runnerH)...)
	rows = append(rows, m.buildSendBox(w, sendH)...)
	return rows
}

// buildRunnerBox delegates to the shared RunnerPanel sub-model.
func (m Model) buildRunnerBox(w, h int) []string {
	rn := m.runner.SetFocused(m.focus == FocusRunner)
	return rn.View(w, h, m.pal)
}

// buildSendBox delegates to the shared SendPanel sub-model.
func (m Model) buildSendBox(w, h int) []string {
	snd := m.send.SetFocused(m.focus == FocusChat)
	return snd.View(w, h, m.pal)
}

