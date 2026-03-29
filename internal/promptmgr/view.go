package promptmgr

import (
	"fmt"
	"strings"

	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/styles"
)

// ansiPal returns the ANSI palette from the current theme bundle,
// falling back to Dracula hardcoded sequences when no bundle is active.
func (m *Model) ansiPal() styles.ANSIPalette {
	if b := m.themeState.Bundle(); b != nil {
		return styles.BundleANSI(b)
	}
	return styles.ANSIPalette{
		Accent:  "\x1b[35m",
		Dim:     "\x1b[2m",
		Success: "\x1b[32m",
		Error:   "\x1b[31m",
		FG:      "\x1b[97m",
		BG:      "\x1b[40m",
		Border:  "\x1b[36m",
		SelBG:   "\x1b[44m",
	}
}

// View renders the full three-panel prompt manager layout.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	pal := m.ansiPal()
	bundle := m.themeState.Bundle()
	topBar := panelrender.TopBar(bundle, "░▒▓ ORCAI — Prompt Manager ▓▒░", m.width)

	// Reserve 1 row for topBar + 1 blank row.
	contentH := m.height - 2

	leftW := m.width / 3
	rightW := m.width - leftW
	editorH := contentH / 2
	runnerH := contentH - editorH

	leftRows := m.buildListRows(leftW, contentH)
	editorRows := m.buildEditorRows(rightW, editorH)
	runnerRows := m.buildRunnerRows(rightW, runnerH)
	rightRows := append(editorRows, runnerRows...)

	// Zip left and right columns into single lines.
	maxRows := len(leftRows)
	if len(rightRows) > maxRows {
		maxRows = len(rightRows)
	}
	lines := make([]string, maxRows)
	for i := range maxRows {
		left := ""
		if i < len(leftRows) {
			left = leftRows[i]
		} else {
			left = strings.Repeat(" ", leftW)
		}
		right := ""
		if i < len(rightRows) {
			right = rightRows[i]
		}
		lines[i] = left + right
	}

	base := topBar + "\n\n" + strings.Join(lines, "\n")

	// Overlay the dir picker when active.
	if m.dirPickerActive {
		overlay := m.dirPicker.ViewDirPickerBox(m.width, pal)
		return panelrender.OverlayCenter(base, overlay, m.width, m.height)
	}

	// Overlay the agent picker when active.
	if m.agentPickerActive {
		overlay := m.agentPicker.ViewBox(min(m.width-4, 60), pal)
		return panelrender.OverlayCenter(base, overlay, m.width, m.height)
	}

	return base
}

// buildListRows renders the left panel (prompt list + filter) as a slice of
// fixed-width ANSI strings ready to be zipped horizontally with the right panels.
func (m *Model) buildListRows(w, h int) []string {
	pal := m.ansiPal()
	borderColor := pal.Border
	if m.focusPanel == 0 {
		borderColor = pal.Accent
	}

	var rows []string
	rows = append(rows, panelrender.BoxTop(w, "PROMPTS", borderColor, pal.Accent))

	// Filter input row.
	filterLine := "  " + m.filterInput.View()
	rows = append(rows, panelrender.BoxRow(filterLine, w, borderColor))

	// Reserve 2 rows at the end: hint footer + boxBot.
	maxItems := h - len(rows) - 2
	if maxItems < 1 {
		maxItems = 1
	}

	switch {
	case m.confirmDelete && len(m.filtered) > 0:
		title := m.filtered[m.selectedIdx].Title
		delRow := pal.Error + panelrender.BLD + fmt.Sprintf("  delete %q? (y/n)", title) + panelrender.RST
		rows = append(rows, panelrender.BoxRow(delRow, w, borderColor))

	case len(m.filtered) == 0:
		rows = append(rows, panelrender.BoxRow(pal.Dim+"  no prompts — press n to create"+panelrender.RST, w, borderColor))

	default:
		end := m.scrollOffset + maxItems
		if end > len(m.filtered) {
			end = len(m.filtered)
		}
		for i := m.scrollOffset; i < end; i++ {
			p := m.filtered[i]
			label := p.Title
			if p.ModelSlug != "" {
				label = fmt.Sprintf("%s  [%s]", p.Title, p.ModelSlug)
			}
			maxLabelW := w - 6
			if len([]rune(label)) > maxLabelW && maxLabelW > 1 {
				label = string([]rune(label)[:maxLabelW-1]) + "…"
			}
			if i == m.selectedIdx {
				var content string
				if m.focusPanel == 0 {
					content = pal.SelBG + "\x1b[97m" + "  > " + label + panelrender.RST
				} else {
					content = pal.Accent + "  > " + label + panelrender.RST
				}
				rows = append(rows, panelrender.BoxRow(content, w, borderColor))
			} else {
				rows = append(rows, panelrender.BoxRow(pal.FG+"    "+label+panelrender.RST, w, borderColor))
			}
		}
	}

	if m.statusMsg != "" {
		rows = append(rows, panelrender.BoxRow(pal.Dim+"  "+m.statusMsg+panelrender.RST, w, borderColor))
	}

	// Pad to fill available space (leave 2 rows for hint + boxBot).
	for len(rows) < h-2 {
		rows = append(rows, panelrender.BoxRow("", w, borderColor))
	}

	var hints []panelrender.Hint
	if m.focusPanel == 0 {
		hints = []panelrender.Hint{
			{Key: "j/k", Desc: "nav"},
			{Key: "n", Desc: "new"},
			{Key: "enter", Desc: "edit"},
			{Key: "d", Desc: "delete"},
		}
	}
	rows = append(rows, panelrender.BoxRow(panelrender.HintBar(hints, w-2, pal), w, borderColor))
	rows = append(rows, panelrender.BoxBot(w, borderColor))
	if len(rows) > h {
		rows = rows[:h]
	}
	return rows
}

// buildEditorRows renders the right-top editor panel as a slice of fixed-width
// ANSI strings.
func (m *Model) buildEditorRows(w, h int) []string {
	pal := m.ansiPal()
	borderColor := pal.Border
	if m.focusPanel == 1 {
		borderColor = pal.Accent
	}

	sectionLabel := func(title string, active bool) string {
		if active {
			return pal.Accent + panelrender.BLD + title + panelrender.RST
		}
		return pal.Dim + title + panelrender.RST
	}

	var rows []string
	rows = append(rows, panelrender.BoxTop(w, "EDITOR", borderColor, pal.Accent))

	// TITLE section.
	titleActive := m.focusPanel == 1 && m.editorSubFocus == 0
	rows = append(rows, panelrender.BoxRow("  "+sectionLabel("TITLE", titleActive), w, borderColor))
	rows = append(rows, panelrender.BoxRow("  "+m.titleInput.View(), w, borderColor))
	rows = append(rows, panelrender.BoxRow("", w, borderColor))

	// BODY section — reserve rows for model/cwd/hints/bot.
	bodyActive := m.focusPanel == 1 && m.editorSubFocus == 1
	rows = append(rows, panelrender.BoxRow("  "+sectionLabel("BODY", bodyActive), w, borderColor))
	const reservedTail = 5 // model row + cwd row + blank + hint + boxBot
	bodyMaxLines := h - len(rows) - reservedTail
	if bodyMaxLines < 1 {
		bodyMaxLines = 1
	}
	count := 0
	for _, bLine := range strings.Split(m.bodyInput.View(), "\n") {
		if count >= bodyMaxLines {
			break
		}
		bLine = strings.TrimRight(bLine, "\r")
		rows = append(rows, panelrender.BoxRow("  "+bLine, w, borderColor))
		count++
	}
	rows = append(rows, panelrender.BoxRow("", w, borderColor))

	// MODEL row (collapsed — enter opens overlay).
	modelActive := m.focusPanel == 1 && m.editorSubFocus == 2
	provID := m.agentPicker.SelectedProviderID()
	modelLabel := m.agentPicker.SelectedModelLabel()
	modelSummary := "(none)"
	if provID != "" {
		modelSummary = provID
		if modelLabel != "" {
			modelSummary += " / " + modelLabel
		}
	}
	pickHint := pal.Dim + "  [enter to pick]" + panelrender.RST
	modelRow := "  " + sectionLabel("MODEL", modelActive) +
		"  " + pal.FG + modelSummary + panelrender.RST + pickHint
	rows = append(rows, panelrender.BoxRow(modelRow, w, borderColor))

	// CWD field.
	cwdActive := m.focusPanel == 1 && m.editorSubFocus == 3
	cwdValue := m.editingPrompt.CWD
	if cwdValue == "" {
		cwdValue = "(none)"
	}
	cwdRow := "  " + sectionLabel("CWD", cwdActive) +
		"  " + pal.FG + cwdValue + panelrender.RST +
		pal.Dim + "  [enter to pick dir]" + panelrender.RST
	rows = append(rows, panelrender.BoxRow(cwdRow, w, borderColor))

	// Pad remaining space.
	for len(rows) < h-2 {
		rows = append(rows, panelrender.BoxRow("", w, borderColor))
	}

	var hints []panelrender.Hint
	if m.focusPanel == 1 {
		hints = []panelrender.Hint{
			{Key: "ctrl+s", Desc: "save"},
			{Key: "ctrl+r", Desc: "run"},
			{Key: "tab", Desc: "field/panel"},
		}
	}
	rows = append(rows, panelrender.BoxRow(panelrender.HintBar(hints, w-2, pal), w, borderColor))
	rows = append(rows, panelrender.BoxBot(w, borderColor))
	if len(rows) > h {
		rows = rows[:h]
	}
	return rows
}

// buildRunnerRows renders the right-bottom test runner panel as a slice of
// fixed-width ANSI strings.
func (m *Model) buildRunnerRows(w, h int) []string {
	pal := m.ansiPal()
	borderColor := pal.Border
	if m.focusPanel == 2 {
		borderColor = pal.Accent
	}

	headerTitle := "TEST RUNNER"
	if m.runnerStreaming {
		headerTitle += " " + spinnerFrames[m.spinnerIdx%len(spinnerFrames)]
	}

	var rows []string
	rows = append(rows, panelrender.BoxTop(w, headerTitle, borderColor, pal.Accent))

	// When follow-up is active reserve 3 extra rows: blank + input + blank.
	followUpRows := 0
	if m.followUpActive || (m.runnerOutput != "" && !m.runnerStreaming && m.focusPanel == 2) {
		followUpRows = 3
	}

	// Reserve 2 rows for hint + boxBot, plus follow-up rows.
	maxLines := h - len(rows) - 2 - followUpRows
	if maxLines < 1 {
		maxLines = 1
	}

	switch {
	case m.runnerErrMsg != "":
		rows = append(rows, panelrender.BoxRow("  "+pal.Error+m.runnerErrMsg+panelrender.RST, w, borderColor))

	case m.runnerOutput == "" && !m.runnerStreaming:
		rows = append(rows, panelrender.BoxRow(pal.Dim+"  ctrl+r to run"+panelrender.RST, w, borderColor))

	default:
		lines := splitLines(m.runnerOutput)
		if m.runnerScrollOffset < len(lines) {
			lines = lines[m.runnerScrollOffset:]
		} else if len(lines) > 0 {
			lines = lines[len(lines)-1:]
		}
		if len(lines) > maxLines {
			lines = lines[:maxLines]
		}
		for _, l := range lines {
			rows = append(rows, panelrender.BoxRow("  "+pal.FG+l+panelrender.RST, w, borderColor))
		}
	}

	// Follow-up input row — shown when there's a response to reply to.
	if followUpRows > 0 {
		rows = append(rows, panelrender.BoxRow("", w, borderColor))
		replyLabel := pal.Dim + "  REPLY  " + panelrender.RST
		if m.followUpActive {
			replyLabel = pal.Accent + panelrender.BLD + "  REPLY  " + panelrender.RST
		}
		rows = append(rows, panelrender.BoxRow(replyLabel+m.followUpInput.View(), w, borderColor))
		rows = append(rows, panelrender.BoxRow("", w, borderColor))
	}

	// Pad remaining space.
	for len(rows) < h-2 {
		rows = append(rows, panelrender.BoxRow("", w, borderColor))
	}

	var hints []panelrender.Hint
	if m.focusPanel == 2 {
		if m.followUpActive {
			hints = []panelrender.Hint{
				{Key: "enter", Desc: "send reply"},
				{Key: "esc", Desc: "cancel reply"},
			}
		} else {
			hints = []panelrender.Hint{
				{Key: "ctrl+r", Desc: "run"},
				{Key: "r", Desc: "reply"},
				{Key: "p", Desc: "promote to body"},
				{Key: "j/k", Desc: "scroll"},
			}
		}
	}
	rows = append(rows, panelrender.BoxRow(panelrender.HintBar(hints, w-2, pal), w, borderColor))
	rows = append(rows, panelrender.BoxBot(w, borderColor))
	if len(rows) > h {
		rows = rows[:h]
	}
	return rows
}

// splitLines splits s into lines without a trailing empty element.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := range len(s) {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
