package pipelineeditor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/styles"
)

// ANSI helpers (package-local).
const (
	aRst = "\x1b[0m"
	aBld = "\x1b[1m"
	aDim = "\x1b[2m"
)

// View renders the full-screen two-column pipeline editor.
func (m Model) View(w, h int) string {
	if w <= 0 {
		w = 120
	}
	if h <= 0 {
		h = 40
	}

	pal := m.pal
	leftW := w / 4
	if leftW < 20 {
		leftW = 20
	}
	rightW := w - leftW

	// Top bar.
	topBar := pal.Accent + aBld + " PIPELINE EDITOR" + aRst

	// Footer hint bar.
	footerH := 1
	footer := buildHintBar(pal, w)

	// Body height = total - topbar(1) - footer(1).
	bodyH := h - 2 - footerH

	leftLines := m.buildLeft(leftW, bodyH)
	rightLines := m.buildRight(rightW, bodyH)

	// Merge left and right columns.
	var rows []string
	rows = append(rows, topBar)
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
		// The left panel's right border │ and the right panel's left border │
		// form a natural double-line divider — no extra separator needed.
		rows = append(rows, l+r)
	}
	rows = append(rows, footer)

	return strings.Join(rows, "\n")
}

// buildLeft renders the PIPELINES panel lines (height = h).
func (m Model) buildLeft(w, h int) []string {
	pal := m.pal

	borderColor := pal.Border
	if m.focus == FocusList {
		borderColor = pal.Accent
	}

	var rows []string
	rows = append(rows, peBoxTop(w, "PIPELINES", borderColor, pal.Accent))

	filtered := m.filteredPipelines()
	// Visible body rows = h - top(1) - search(1) - hint(1) - bottom(1) = h-4.
	visibleRows := h - 5
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Scroll clamping.
	scroll := m.listScroll
	if scroll < 0 {
		scroll = 0
	}
	if scroll > len(filtered)-visibleRows && len(filtered) > visibleRows {
		scroll = len(filtered) - visibleRows
	}

	for i := range visibleRows {
		idx := scroll + i
		if idx >= len(filtered) {
			rows = append(rows, peBoxRow("", w, borderColor))
			continue
		}
		name := filtered[idx]
		// Truncate name to fit inner box width: w-2 (borders) - 4 (indent + cursor).
		maxName := w - 6
		if maxName < 4 {
			maxName = 4
		}
		runes := []rune(name)
		if len(runes) > maxName {
			name = string(runes[:maxName-1]) + "…"
		}
		var content string
		if idx == m.listSel && m.focus == FocusList {
			content = pal.Accent + "> " + aRst + pal.FG + name + aRst
		} else if idx == m.listSel {
			content = pal.Dim + "> " + pal.FG + name + aRst
		} else {
			content = pal.Dim + "  " + name + aRst
		}
		rows = append(rows, peBoxRow("  "+content, w, borderColor))
	}

	// Search input row.
	if m.listSearching {
		searchContent := pal.Accent + "/" + aRst + m.listSearch + "█"
		rows = append(rows, peBoxRow("  "+searchContent, w, borderColor))
	} else {
		rows = append(rows, peBoxRow("  "+pal.Dim+"[/] search"+aRst, w, borderColor))
	}

	// Hint row.
	hintRow := "  " + pal.Dim + "[n] new  [d] del" + aRst
	if m.confirmDelete {
		hintRow = "  " + pal.Error + "delete? [y/N]" + aRst
	}
	rows = append(rows, peBoxRow(hintRow, w, borderColor))
	rows = append(rows, peBoxBot(w, borderColor))

	return rows
}

// buildRight renders the right column (editor + runner).
// YAML preview has been removed — the test runner IS the preview.
func (m Model) buildRight(w, h int) []string {
	editorH := h * 55 / 100
	if editorH < 10 {
		editorH = 10
	}
	runnerH := h - editorH
	if runnerH < 6 {
		runnerH = 6
	}

	var rows []string
	rows = append(rows, m.buildEditorBox(w, editorH)...)
	rows = append(rows, m.buildRunnerBox(w, runnerH)...)
	return rows
}

// buildEditorBox renders the EDITOR section.
func (m Model) buildEditorBox(w, h int) []string {
	pal := m.pal
	borderColor := pal.Border
	if m.focus == FocusEditor {
		borderColor = pal.Accent
	}

	var rows []string
	rows = append(rows, peBoxTop(w, "EDITOR", borderColor, pal.Accent))

	// Provider/model section.
	if m.editorFocus == editorFieldPicker && m.focus == FocusEditor {
		// Show full picker rows.
		innerW := w - 4
		if innerW < 10 {
			innerW = 10
		}
		for _, r := range m.picker.ViewRows(innerW, pal) {
			rows = append(rows, peBoxRow("  "+r, w, borderColor))
		}
	} else {
		// Single summary line.
		provID := m.picker.SelectedProviderID()
		if provID == "" {
			provID = "claude"
		}
		modelID := m.picker.SelectedModelID()
		if modelID == "" {
			modelID = "(default)"
		}
		summary := fmt.Sprintf("  %sProvider:%s %s  %sModel:%s %s",
			pal.Dim, aRst, pal.FG+provID+aRst,
			pal.Dim, aRst, pal.FG+modelID+aRst,
		)
		rows = append(rows, peBoxRow(summary, w, borderColor))
	}

	rows = append(rows, peBoxRow("", w, borderColor))

	// Name field.
	nameLabel := peSectionLabel("NAME", m.editorFocus == editorFieldName && m.focus == FocusEditor, pal)
	rows = append(rows, peBoxRow("  "+nameLabel, w, borderColor))
	m.nameInput.Width = w - 6
	if m.nameInput.Width < 10 {
		m.nameInput.Width = 10
	}
	rows = append(rows, peBoxRow("  "+m.nameInput.View(), w, borderColor))
	rows = append(rows, peBoxRow("", w, borderColor))

	// Prompt field.
	promptLabel := peSectionLabel("PROMPT", m.editorFocus == editorFieldPrompt && m.focus == FocusEditor, pal)
	rows = append(rows, peBoxRow("  "+promptLabel, w, borderColor))

	promptInnerW := w - 6
	if promptInnerW < 10 {
		promptInnerW = 10
	}
	// Available rows for prompt = h - used rows so far.
	used := len(rows) + 1 // +1 for box bottom
	promptRows := h - used
	if promptRows < 2 {
		promptRows = 2
	}
	if promptRows > 10 {
		promptRows = 10
	}
	m.promptArea.SetWidth(promptInnerW)
	m.promptArea.SetHeight(promptRows)
	for _, pLine := range strings.Split(m.promptArea.View(), "\n") {
		pLine = strings.TrimRight(pLine, "\r")
		rows = append(rows, peBoxRow("  "+pLine, w, borderColor))
	}

	// Hint row (shown when editor is focused).
	if m.focus == FocusEditor {
		var hint string
		switch m.editorFocus {
		case editorFieldPicker:
			hint = "  " + pal.Dim + "[tab] next  [↑↓] select" + aRst
		case editorFieldName:
			hint = "  " + pal.Dim + "[enter/tab] next  [shift+tab] back" + aRst
		case editorFieldPrompt:
			hint = "  " + pal.Dim + "[ctrl+s] save+run  [tab] focus runner  [ctrl+e] $EDITOR" + aRst
		}
		// Pad to h-2 first, then inject hint, then bottom border.
		for len(rows) < h-2 {
			rows = append(rows, peBoxRow("", w, borderColor))
		}
		rows = append(rows, peBoxRow(hint, w, borderColor))
	} else {
		// Pad to h-1.
		for len(rows) < h-1 {
			rows = append(rows, peBoxRow("", w, borderColor))
		}
	}
	rows = append(rows, peBoxBot(w, borderColor))
	return rows
}

// buildYAMLBox is intentionally unused — YAML preview was removed;
// the test runner serves as the live preview.
// nolint: unused
func (m Model) buildYAMLBox(w, h int) []string {
	pal := m.pal
	borderColor := pal.Border
	if m.focus == FocusYAML {
		borderColor = pal.Accent
	}

	headerTitle := "YAML PREVIEW"
	if m.focus == FocusYAML {
		headerTitle = "YAML PREVIEW  " + pal.Dim + "[ctrl+e] $EDITOR" + aRst + pal.Accent
	}
	var rows []string
	rows = append(rows, peBoxTop(w, headerTitle, borderColor, pal.Accent))

	yamlStr := buildYAML(
		strings.TrimSpace(m.nameInput.Value()),
		m.picker.SelectedProviderID(),
		m.picker.SelectedModelID(),
		m.promptArea.Value(),
	)

	yamlLines := strings.Split(strings.TrimRight(yamlStr, "\n"), "\n")
	innerW := w - 4
	if innerW < 10 {
		innerW = 10
	}
	textColor := pal.Dim
	if m.focus == FocusYAML {
		textColor = pal.FG
	}

	visH := h - 2 // subtract top and bottom borders
	scroll := m.yamlScroll
	if scroll < 0 {
		scroll = 0
	}

	for i := range visH {
		idx := scroll + i
		if idx >= len(yamlLines) {
			rows = append(rows, peBoxRow("", w, borderColor))
			continue
		}
		line := yamlLines[idx]
		if panelrender.VisibleWidth(line) > innerW {
			// Truncate to innerW visible chars (simple rune truncation).
			runes := []rune(line)
			line = string(runes[:innerW])
		}
		rows = append(rows, peBoxRow("  "+textColor+line+aRst, w, borderColor))
	}

	rows = append(rows, peBoxBot(w, borderColor))
	return rows
}

// buildRunnerBox renders the TEST RUNNER section.
func (m Model) buildRunnerBox(w, h int) []string {
	pal := m.pal
	borderColor := pal.Border
	if m.focus == FocusRunner {
		borderColor = pal.Accent
	}

	var rows []string
	rows = append(rows, peBoxTop(w, "TEST RUNNER", borderColor, pal.Accent))

	// Status line.
	var statusLine string
	switch {
	case m.runRunning:
		statusLine = "  " + pal.Warn + "running..." + aRst
	case m.statusErr:
		statusLine = "  " + pal.Error + m.statusMsg + aRst
	case m.statusMsg != "":
		statusLine = "  " + pal.Success + m.statusMsg + aRst
	default:
		statusLine = "  " + pal.Dim + "idle" + aRst
	}
	rows = append(rows, peBoxRow(statusLine, w, borderColor))

	// Output lines: h - top(1) - status(1) - hints(1) - clarify(0-2) - bottom(1).
	overhead := 4
	if m.clarifyActive {
		overhead += 2
	}
	visLines := h - overhead
	if visLines < 1 {
		visLines = 1
	}

	// Show last visLines of runLines.
	startIdx := len(m.runLines) - visLines
	if startIdx < 0 {
		startIdx = 0
	}
	for i := range visLines {
		lineIdx := startIdx + i
		if lineIdx >= len(m.runLines) {
			rows = append(rows, peBoxRow("", w, borderColor))
			continue
		}
		line := m.runLines[lineIdx]
		rows = append(rows, peBoxRow("  "+pal.FG+line+aRst, w, borderColor))
	}

	// Clarification block.
	if m.clarifyActive {
		qLine := "  " + pal.Warn + "? " + m.clarifyQ + aRst
		rows = append(rows, peBoxRow(qLine, w, borderColor))
		m.clarifyInput.Width = w - 14
		if m.clarifyInput.Width < 10 {
			m.clarifyInput.Width = 10
		}
		answerLine := "  " + pal.Dim + "Answer: " + aRst + m.clarifyInput.View()
		rows = append(rows, peBoxRow(answerLine, w, borderColor))
	}

	// Hint row — contextual based on state.
	var hintLine string
	switch {
	case m.clarifyActive:
		hintLine = "  " + pal.Dim + "[enter] submit answer  [esc] cancel" + aRst
	case m.runRunning:
		hintLine = "  " + pal.Dim + "[ctrl+c] cancel run" + aRst
	case len(m.runLines) > 0 && m.focus == FocusRunner:
		hintLine = "  " + pal.Dim + "[r] re-run  [ctrl+i] inject output into prompt  [ctrl+c] cancel" + aRst
	default:
		hintLine = "  " + pal.Dim + "[r] run  [shift+tab] editor" + aRst
	}
	rows = append(rows, peBoxRow(hintLine, w, borderColor))
	rows = append(rows, peBoxBot(w, borderColor))
	return rows
}

// buildHintBar renders the bottom hint bar across the full width.
func buildHintBar(pal styles.ANSIPalette, w int) string {
	hints := []struct{ key, desc string }{
		{"tab", "focus"},
		{"ctrl+s", "save+run"},
		{"esc", "back"},
	}
	var parts []string
	sep := pal.Dim + " · " + aRst
	for _, h := range hints {
		parts = append(parts, pal.Dim+"["+aRst+pal.Accent+aBld+h.key+aRst+pal.Dim+"] "+h.desc+aRst)
	}
	bar := " " + strings.Join(parts, sep)
	// Pad to w.
	vl := lipgloss.Width(bar)
	if vl < w {
		bar += strings.Repeat(" ", w-vl)
	}
	return bar
}

// Box drawing helpers delegate to panelrender.
func peBoxTop(w int, title, borderColor, labelColor string) string { return panelrender.BoxTop(w, title, borderColor, labelColor) }
func peBoxBot(w int, borderColor string) string                    { return panelrender.BoxBot(w, borderColor) }
func peBoxRow(content string, w int, borderColor string) string    { return panelrender.BoxRow(content, w, borderColor) }

func peSectionLabel(title string, focused bool, pal styles.ANSIPalette) string {
	if focused {
		return pal.Accent + aBld + title + aRst
	}
	return aDim + title + aRst
}
