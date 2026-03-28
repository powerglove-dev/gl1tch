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

// viewPal caches resolved lipgloss colors for overlay rendering.
type viewPal struct {
	accent  lipgloss.Color
	fg      lipgloss.Color
	dim     lipgloss.Color
	selBG   lipgloss.Color
	errCol  lipgloss.Color
	success lipgloss.Color
	pink    lipgloss.Color
	bg      lipgloss.Color
}

// pal resolves the active palette from the bundle, falling back to Dracula.
func (m Model) pal() viewPal {
	b := m.bundle
	if b != nil {
		return viewPal{
			accent:  lipgloss.Color(b.Palette.Accent),
			fg:      lipgloss.Color(b.Palette.FG),
			dim:     lipgloss.Color(b.Palette.Dim),
			selBG:   lipgloss.Color(b.Palette.Border),
			errCol:  lipgloss.Color(b.Palette.Error),
			success: lipgloss.Color(b.Palette.Success),
			pink:    lipgloss.Color(b.Palette.Accent),
			bg:      lipgloss.Color(b.Palette.BG),
		}
	}
	return viewPal{
		accent:  lipgloss.Color(draculaPurple),
		fg:      lipgloss.Color(draculaFg),
		dim:     lipgloss.Color(draculaComment),
		selBG:   lipgloss.Color(draculaCurrent),
		errCol:  lipgloss.Color(draculaRed),
		success: lipgloss.Color(draculaGreen),
		pink:    lipgloss.Color(draculaPink),
		bg:      lipgloss.Color(draculaBg),
	}
}

// View renders the full TUI screen.
func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	// Split height: 35% jobs (capped at 14 rows), 65% logs, minus 1 for hint bar.
	topH, botH := splitHeight(m.height-1, 0.35, 6)
	if topH > 14 {
		topH = 14
		botH = m.height - 1 - topH
	}

	top := m.viewJobList(m.width, topH)
	bot := m.viewLogPane(m.width, botH)
	bar := m.viewHintBar(m.width)

	content := lipgloss.JoinVertical(lipgloss.Left, top, bot, bar)

	// Render overlays on top if open.
	bgColor := string(m.pal().bg)
	if m.themePickerOpen {
		return renderOverlay(content, m.viewThemePicker(), m.width, m.height, bgColor)
	}
	if m.quitConfirm {
		return renderOverlay(content, m.viewQuitConfirm(), m.width, m.height, bgColor)
	}
	if m.editOverlay != nil {
		return renderOverlay(content, m.viewEditOverlay(), m.width, m.height, bgColor)
	}
	if m.deleteConfirm != nil {
		return renderOverlay(content, m.viewDeleteConfirm(), m.width, m.height, bgColor)
	}
	return content
}

// viewJobList renders the top pane showing the list of cron entries.
func (m Model) viewJobList(width, height int) string {
	pal := m.ansiPal()
	borderColor := pal.Border
	if m.activePane == 0 {
		borderColor = pal.Accent
	}

	var rows []string

	// Panel header — themed sprite or dynamic header.
	if sprite := panelrender.PanelHeader(m.bundle, "cron", width, borderColor); sprite != nil {
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
		rows = append(rows, panelrender.BoxTop(width, title, borderColor, pal.Accent))
	}

	// Available content rows (leave 1 for BoxBot).
	maxRows := height - len(rows) - 1
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

	// Pad remaining space.
	for len(rows) < height-1 {
		rows = append(rows, panelrender.BoxRow("", width, borderColor))
	}
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
	if m.activePane == 1 {
		borderColor = pal.Accent
	}

	var rows []string

	// Panel header.
	if sprite := panelrender.PanelHeader(m.bundle, "log_output", width, borderColor); sprite != nil {
		rows = append(rows, sprite...)
	} else {
		rows = append(rows, panelrender.BoxTop(width, "LOG OUTPUT", borderColor, pal.Accent))
	}

	maxLines := height - len(rows) - 1 // -1 for BoxBot
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

	for len(rows) < height-1 {
		rows = append(rows, panelrender.BoxRow("", width, borderColor))
	}
	rows = append(rows, panelrender.BoxBot(width, borderColor))
	if len(rows) > height {
		rows = rows[:height]
	}
	return strings.Join(rows, "\n")
}

// viewHintBar renders the single-line keybinding hint strip at the bottom.
// Keys are colored with the theme's accent; descriptions with dim.
func (m Model) viewHintBar(width int) string {
	pal := m.ansiPal()
	hint := func(key, desc string) string {
		return pal.Accent + key + pal.Dim + " " + desc + panelrender.RST
	}
	sep := pal.Dim + " · " + panelrender.RST

	var parts []string
	if m.activePane == 0 {
		if m.filtering {
			parts = []string{
				hint("esc", "clear filter"),
				hint("enter", "confirm"),
				hint("tab", "logs pane"),
			}
		} else {
			parts = []string{
				hint("j/k", "navigate"),
				hint("e", "edit"),
				hint("d", "delete"),
				hint("enter/r", "run now"),
				hint("/", "filter"),
				hint("tab", "logs"),
			}
		}
	} else {
		parts = []string{
			hint("j/k", "scroll"),
			hint("tab", "jobs pane"),
		}
	}

	bar := strings.Join(parts, sep)
	if lipgloss.Width(bar) < width {
		bar += strings.Repeat(" ", width-lipgloss.Width(bar))
	}
	return bar + panelrender.RST
}

// viewEditOverlay renders the edit form overlay.
func (m Model) viewEditOverlay() string {
	p := m.pal()
	ov := m.editOverlay
	labels := [5]string{"Name", "Schedule", "Kind", "Target", "Timeout"}

	overlayStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.pink).
		Background(p.bg).
		Padding(1, 2)

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Foreground(p.accent).Bold(true).Render("EDIT CRON JOB") + "\n\n")

	for i, f := range ov.fields {
		label := labels[i]
		if i == ov.focusIdx {
			label = lipgloss.NewStyle().Foreground(p.success).Render("> " + label)
		} else {
			label = lipgloss.NewStyle().Foreground(p.dim).Render("  " + label)
		}
		sb.WriteString(fmt.Sprintf("%-20s %s\n", label, f.View()))
	}

	if ov.errMsg != "" {
		sb.WriteString("\n" + lipgloss.NewStyle().Foreground(p.errCol).Render("Error: "+ov.errMsg))
	}

	sb.WriteString("\n" + lipgloss.NewStyle().Foreground(p.dim).Render("[tab] next field  [enter] save  [esc] cancel"))

	return overlayStyle.Render(sb.String())
}

// viewThemePicker renders the theme picker overlay using the shared tuikit component.
func (m Model) viewThemePicker() string {
	var bundles []themes.Bundle
	if gr := themes.GlobalRegistry(); gr != nil {
		bundles = gr.All()
	}
	// Use m.bundle for color resolution so the picker always reflects the
	// live theme tracked by BubbleTea, not a potentially-stale registry read.
	return tuikit.ViewThemePicker(bundles, m.themePickerCursor, m.bundle, m.width)
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
