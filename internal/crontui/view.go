package crontui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/adam-stokes/orcai/internal/cron"
	"github.com/adam-stokes/orcai/internal/modal"
	"github.com/adam-stokes/orcai/internal/panelrender"
	"github.com/adam-stokes/orcai/internal/styles"
	"github.com/adam-stokes/orcai/internal/themes"
	"github.com/adam-stokes/orcai/internal/tuikit"
)

// ansiPal returns the ANSI-escape palette for the current bundle.
// Falls back to Dracula hardcoded sequences when bundle is nil.
func (m Model) ansiPal() styles.ANSIPalette {
	if m.bundle != nil {
		return styles.BundleANSI(m.bundle)
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


// View renders the full TUI screen.
func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	contentH := m.height

	// Split remaining height: 35% jobs (capped at 14 rows), 65% logs.
	// Hint bars are rendered inside each pane.
	topH, botH := splitHeight(contentH, 0.35, 6)
	if topH > 14 {
		topH = 14
		botH = contentH - topH
	}

	top := m.viewJobList(m.width, topH)
	bot := m.viewLogPane(m.width, botH)

	content := lipgloss.JoinVertical(lipgloss.Left, top, bot)

	// Render overlays on top if open.
	if m.jumpOpen {
		m.jumpModal.SetSize(m.width, m.height-2)
		return renderOverlay(content, m.jumpModal.View(), m.width, m.height, "")
	}
	if m.helpOpen {
		return renderOverlay(content, m.viewHelpModal(), m.width, m.height, "")
	}
	if m.themePicker.Open {
		return renderOverlay(content, m.viewThemePicker(), m.width, m.height, "")
	}
	if m.quitConfirm {
		return renderOverlay(content, m.viewQuitConfirm(), m.width, m.height, "")
	}
	if m.editOverlay != nil {
		return renderOverlay(content, m.viewEditOverlay(), m.width, m.height, "")
	}
	if m.deleteConfirm != nil {
		return renderOverlay(content, m.viewDeleteConfirm(), m.width, m.height, "")
	}
	return content
}

// viewJobList renders the top pane showing the list of cron entries.
func (m Model) viewJobList(width, height int) string {
	pal := m.ansiPal()
	borderColor := pal.Border
	titleColor := pal.Accent
	if m.activePane == 0 {
		borderColor = pal.Accent
	}

	var rows []string

	// Panel header — themed sprite or dynamic header.
	if sprite := panelrender.PanelHeader(m.bundle, "cron", width, borderColor, titleColor); sprite != nil {
		rows = append(rows, sprite...)
		// Filter row appended below header when active.
		if m.filtering {
			prompt := fmt.Sprintf("  %s/%s %s%s%s", pal.Accent, panelrender.RST, pal.FG, m.filterInput.View(), panelrender.RST)
			rows = append(rows, panelrender.BoxRow(prompt, width, borderColor))
		}
	} else {
		title := panelrender.RenderHeader("cron")
		if m.filtering {
			title += " " + m.filterInput.View()
		}
		rows = append(rows, panelrender.BoxTop(width, title, borderColor, titleColor))
	}

	// Available content rows (leave 1 for BoxBot, 1 for always-present hint footer).
	maxRows := height - len(rows) - 2
	if maxRows < 0 {
		maxRows = 0
	}

	if len(m.filtered) == 0 {
		if m.filterInput.Value() != "" {
			rows = append(rows, panelrender.BoxRow(pal.Dim+"  no matches"+panelrender.RST, width, borderColor))
		} else {
			rows = append(rows, panelrender.BoxRow(pal.Dim+"  no scheduled jobs"+panelrender.RST, width, borderColor))
		}
	} else {
		m.clampScrollForList(len(m.filtered), maxRows)
		start := m.scrollOffset
		end := start + maxRows
		if end > len(m.filtered) {
			end = len(m.filtered)
		}
		for i := start; i < end; i++ {
			e := m.filtered[i]
			content := m.formatEntryRowANSI(e, width-4, pal)
			if i == m.selectedIdx {
				indicator := pal.Accent + panelrender.BLD + ">" + panelrender.RST + " "
				rows = append(rows, panelrender.BoxRow(indicator+content, width, borderColor))
			} else {
				rows = append(rows, panelrender.BoxRow("  "+content, width, borderColor))
			}
		}
	}

	// Pad remaining space, leaving room for always-present hint footer.
	for len(rows) < height-2 {
		rows = append(rows, panelrender.BoxRow("", width, borderColor))
	}
	// Hint footer row — always present; shows hints when active, blank when not.
	var jobHints []panelrender.Hint
	if m.activePane == 0 {
		if m.filtering {
			jobHints = []panelrender.Hint{
				{Key: "esc", Desc: "clear filter"},
				{Key: "enter", Desc: "confirm"},
			}
		} else {
			jobHints = []panelrender.Hint{
				{Key: "j/k", Desc: "navigate"},
				{Key: "e", Desc: "edit"},
				{Key: "d", Desc: "delete"},
				{Key: "enter/r", Desc: "run now"},
				{Key: "p", Desc: "pipeline"},
				{Key: "/", Desc: "search"},
				{Key: "J", Desc: "jump"},
				{Key: "?", Desc: "help"},
			}
		}
	}
	rows = append(rows, panelrender.BoxRow(panelrender.HintBar(jobHints, width-2, m.ansiPal()), width, borderColor))
	rows = append(rows, panelrender.BoxBot(width, borderColor))
	if len(rows) > height {
		rows = rows[:height]
	}
	return strings.Join(rows, "\n")
}

// formatEntryRowANSI formats a single cron entry as an ANSI-colored row string.
func (m Model) formatEntryRowANSI(e cron.Entry, width int, pal styles.ANSIPalette) string {
	nextStr := ""
	if t, err := cron.NextRun(e); err == nil {
		nextStr = cron.FormatRelative(t)
	} else {
		nextStr = pal.Error + "invalid" + panelrender.RST
	}

	kindColor := pal.Dim
	if e.Kind == "agent" {
		kindColor = pal.Accent
	}

	nameW := width * 30 / 100
	schedW := width * 25 / 100
	kindW := 10

	name := truncate(e.Name, nameW)
	sched := truncate(e.Schedule, schedW)
	kind := kindColor + truncate(e.Kind, kindW) + panelrender.RST
	next := pal.Dim + nextStr + panelrender.RST

	return fmt.Sprintf("%-*s %-*s %-*s %s",
		nameW, name,
		schedW, sched,
		kindW, kind,
		next,
	)
}

// viewLogPane renders the bottom pane showing recent log output.
func (m Model) viewLogPane(width, height int) string {
	pal := m.ansiPal()
	borderColor := pal.Border
	titleColor := pal.Accent
	if m.activePane == 1 {
		borderColor = pal.Accent
	}

	var rows []string

	// Panel header.
	if sprite := panelrender.PanelHeader(m.bundle, "log_output", width, borderColor, titleColor); sprite != nil {
		rows = append(rows, sprite...)
	} else {
		rows = append(rows, panelrender.BoxTop(width, "LOG OUTPUT", borderColor, titleColor))
	}

	// -1 for BoxBot, -1 for always-present hint footer.
	maxLines := height - len(rows) - 2
	if maxLines < 1 {
		maxLines = 1
	}

	totalLogs := len(m.logBuf)
	maxScroll := totalLogs - maxLines
	if maxScroll < 0 {
		maxScroll = 0
	}
	offset := maxScroll - m.logScrollOffset
	if offset < 0 {
		offset = 0
	}
	end := offset + maxLines
	if end > totalLogs {
		end = totalLogs
	}

	if totalLogs == 0 {
		rows = append(rows, panelrender.BoxRow(pal.Dim+"  waiting for log output..."+panelrender.RST, width, borderColor))
	} else {
		for _, l := range m.logBuf[offset:end] {
			l = strings.TrimRight(l, "\n\r")
			l = truncate(l, width-4)
			rows = append(rows, panelrender.BoxRow(pal.Dim+l+panelrender.RST, width, borderColor))
		}
	}

	// Pad remaining space, leaving room for always-present hint footer.
	for len(rows) < height-2 {
		rows = append(rows, panelrender.BoxRow("", width, borderColor))
	}
	// Hint footer row — always present; shows hints when active, blank when not.
	var logHints []panelrender.Hint
	if m.activePane == 1 {
		logHints = []panelrender.Hint{
			{Key: "j/k", Desc: "scroll"},
		}
	}
	rows = append(rows, panelrender.BoxRow(panelrender.HintBar(logHints, width-2, m.ansiPal()), width, borderColor))
	rows = append(rows, panelrender.BoxBot(width, borderColor))
	if len(rows) > height {
		rows = rows[:height]
	}
	return strings.Join(rows, "\n")
}

// viewEditOverlay renders the edit form overlay using panelrender box drawing
// so that theme sprites and palette are honoured.
func (m Model) viewEditOverlay() string {
	pal := m.ansiPal()
	ov := m.editOverlay
	labels := [5]string{"Name", "Schedule", "Kind", "Target", "Timeout"}

	overlayW := m.width * 2 / 3
	if overlayW < 60 {
		overlayW = 60
	}
	if overlayW > m.width-4 {
		overlayW = m.width - 4
	}

	var rows []string

	// Header — themed sprite or dynamic panel header.
	if sprite := panelrender.PanelHeader(m.bundle, "edit_cron", overlayW, pal.Border, pal.Accent); sprite != nil {
		rows = append(rows, sprite...)
	} else {
		rows = append(rows, panelrender.BoxTop(overlayW, "EDIT CRON JOB", pal.Border, pal.Accent))
	}

	rows = append(rows, panelrender.BoxRow("", overlayW, pal.Border))

	// Field rows — label column (12 visible chars) then the text input.
	const labelVisW = 12
	for i, f := range ov.fields {
		var labelStr string
		if i == ov.focusIdx {
			labelStr = pal.Accent + panelrender.BLD + "> " + panelrender.RST + pal.Success + labels[i] + panelrender.RST
		} else {
			labelStr = pal.Dim + "  " + labels[i] + panelrender.RST
		}
		padCount := labelVisW - (2 + len(labels[i])) // "  "/">" + " " = 2 prefix runes
		if padCount < 0 {
			padCount = 0
		}
		content := " " + labelStr + strings.Repeat(" ", padCount) + pal.Border + ">" + panelrender.RST + " " + f.View()
		rows = append(rows, panelrender.BoxRow(content, overlayW, pal.Border))
	}

	if ov.errMsg != "" {
		rows = append(rows, panelrender.BoxRow("", overlayW, pal.Border))
		rows = append(rows, panelrender.BoxRow(
			" "+pal.Error+panelrender.BLD+"Error: "+panelrender.RST+pal.Error+ov.errMsg+panelrender.RST,
			overlayW, pal.Border))
	}

	rows = append(rows, panelrender.BoxRow("", overlayW, pal.Border))
	hints := panelrender.HintBar([]panelrender.Hint{
		{Key: "tab", Desc: "next field"},
		{Key: "enter", Desc: "save"},
		{Key: "esc", Desc: "cancel"},
	}, overlayW-2, pal)
	rows = append(rows, panelrender.BoxRow(hints, overlayW, pal.Border))
	rows = append(rows, panelrender.BoxBot(overlayW, pal.Border))

	return strings.Join(rows, "\n")
}

// viewThemePicker renders the theme picker overlay using the shared tuikit component.
func (m Model) viewThemePicker() string {
	var dark, light []themes.Bundle
	if gr := themes.GlobalRegistry(); gr != nil {
		dark = gr.BundlesByMode("dark")
		light = gr.BundlesByMode("light")
	}
	// Use m.bundle for color resolution so the picker always reflects the
	// live theme tracked by BubbleTea, not a potentially-stale registry read.
	return tuikit.ViewThemePicker(dark, light, m.themePicker, m.bundle, m.width)
}

// viewQuitConfirm renders the quit confirmation overlay.
func (m Model) viewQuitConfirm() string {
	return panelrender.QuitConfirmBox(m.ansiPal(), "Quit ORCAI", "Quit ORCAI?", m.width)
}

// viewDeleteConfirm renders the delete confirmation overlay.
func (m Model) viewDeleteConfirm() string {
	name := m.deleteConfirm.entry.Name
	cfg := modal.Config{
		Bundle:  m.bundle,
		Title:   "DELETE CRON JOB",
		Message: fmt.Sprintf("Delete %q?", name),
	}
	return modal.RenderConfirm(cfg, m.width, m.height)
}

// viewHelpModal renders the crontui keybinding reference as an 80%-wide ANSI box overlay.
func (m Model) viewHelpModal() string {
	pal := m.ansiPal()
	boxW := m.width * 4 / 5
	if boxW < 48 {
		boxW = 48
	}

	blank := func() string { return panelrender.BoxRow("", boxW, pal.Border) }
	row := func(s string) string { return panelrender.BoxRow(s, boxW, pal.Border) }

	section := func(title string) string {
		dash := pal.Dim + "──" + panelrender.RST
		label := pal.Accent + panelrender.BLD + " " + title + " " + panelrender.RST
		return row("  " + dash + label + dash)
	}

	const keyW = 18
	bind := func(k, desc string) string {
		pad := keyW - lipgloss.Width(k)
		if pad < 1 {
			pad = 1
		}
		return row("    " + pal.Accent + k + panelrender.RST + strings.Repeat(" ", pad) + pal.FG + desc + panelrender.RST)
	}

	note := func(s string) string {
		return row("    " + pal.Dim + s + panelrender.RST)
	}

	rows := []string{
		panelrender.BoxTop(boxW, "help", pal.Border, pal.Accent),
		blank(),
		section("NAVIGATION"),
		bind("j / k", "move up / down in active pane"),
		bind("tab", "switch pane  jobs ↔ logs"),
		bind("/", "search jobs"),
		blank(),
		section("JOB ACTIONS"),
		bind("enter / r", "run selected job immediately"),
		bind("e", "edit selected job"),
		bind("d", "delete selected job  (confirm required)"),
		blank(),
		section("LOG PANE"),
		bind("j / k", "scroll log up / down"),
		blank(),
		section("GLOBAL"),
		bind("T", "theme picker"),
		bind("?", "this help"),
		bind("ctrl+q", "quit  (confirm required)"),
		bind("esc", "close any overlay"),
		blank(),
		section("EDIT OVERLAY"),
		bind("tab / shift+tab", "next / prev field"),
		bind("enter", "save changes"),
		bind("esc", "cancel without saving"),
		note("fields: name · schedule (cron expr) · kind · target · timeout"),
		blank(),
		panelrender.BoxBot(boxW, pal.Border),
	}
	return strings.Join(rows, "\n")
}

// splitHeight divides total rows into top and bottom, applying a ratio and
// enforcing a minimum row count for each pane.
func splitHeight(total int, ratio float64, minRows int) (top, bot int) {
	top = int(float64(total) * ratio)
	bot = total - top
	if top < minRows {
		top = minRows
		bot = total - top
	}
	if bot < minRows {
		bot = minRows
		top = total - bot
	}
	return
}

// renderOverlay places overlayContent centered over background content.
func renderOverlay(background, overlayContent string, width, height int, _ string) string {
	return panelrender.OverlayCenter(background, overlayContent, width, height)
}

// truncate shortens s to at most n runes, appending "…" if truncated.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return string(runes[:n])
	}
	return string(runes[:n-1]) + "…"
}
