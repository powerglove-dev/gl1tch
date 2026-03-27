package crontui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/adam-stokes/orcai/internal/cron"
	"github.com/adam-stokes/orcai/internal/modal"
	"github.com/adam-stokes/orcai/internal/translations"
)

// viewPal caches resolved lipgloss colors for the current render pass.
type viewPal struct {
	accent  lipgloss.Color
	fg      lipgloss.Color
	dim     lipgloss.Color
	selBG   lipgloss.Color
	errCol  lipgloss.Color
	success lipgloss.Color
	border  lipgloss.Color
	pink    lipgloss.Color
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
			border:  lipgloss.Color(b.Palette.Dim),
			pink:    lipgloss.Color(b.Palette.Accent), // fallback — themes don't have pink
		}
	}
	return viewPal{
		accent:  lipgloss.Color(draculaPurple),
		fg:      lipgloss.Color(draculaFg),
		dim:     lipgloss.Color(draculaComment),
		selBG:   lipgloss.Color(draculaCurrent),
		errCol:  lipgloss.Color(draculaRed),
		success: lipgloss.Color(draculaGreen),
		border:  lipgloss.Color(draculaComment),
		pink:    lipgloss.Color(draculaPink),
	}
}

// panelBand renders a full-width accent-background title band, used as the
// header inside each pane. Falls back to a plain bold title if no bundle.
func (m Model) panelBand(title string, innerWidth int) string {
	p := m.pal()
	return lipgloss.NewStyle().
		Background(p.accent).
		Foreground(p.fg).
		Bold(true).
		Width(innerWidth).
		Align(lipgloss.Center).
		Render(title)
}

// View renders the full TUI screen.
func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	// Split height: 35% jobs (capped at 12 rows), 65% logs, minus 1 for hint bar.
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
	if m.editOverlay != nil {
		return renderOverlay(content, m.viewEditOverlay(), m.width, m.height)
	}
	if m.deleteConfirm != nil {
		return renderOverlay(content, m.viewDeleteConfirm(), m.width, m.height)
	}
	return content
}

// viewJobList renders the top pane showing the list of cron entries.
func (m Model) viewJobList(width, height int) string {
	p := m.pal()
	// Inner width accounts for border (2) and padding.
	inner := width - 4
	if inner < 10 {
		inner = 10
	}

	// Panel band header spanning full inner width.
	var headerRight string
	if m.filtering {
		headerRight = " " + m.filterInput.View()
	}
	cronTitle := translations.Safe(translations.GlobalProvider(), translations.KeyCronTitle, "CRON JOBS")
	header := m.panelBand(cronTitle+headerRight, inner)

	// Build rows.
	var rows []string
	if len(m.filtered) == 0 {
		if m.filterInput.Value() != "" {
			rows = append(rows, lipgloss.NewStyle().Foreground(p.dim).Render("  no matches"))
		} else {
			rows = append(rows, lipgloss.NewStyle().Foreground(p.dim).Render("  no scheduled jobs"))
		}
	} else {
		visibleRows := height - 4 // header + border top + border bot + header row
		if visibleRows < 1 {
			visibleRows = 1
		}
		m.clampScrollForList(len(m.filtered), visibleRows)

		start := m.scrollOffset
		end := start + visibleRows
		if end > len(m.filtered) {
			end = len(m.filtered)
		}

		for i := start; i < end; i++ {
			e := m.filtered[i]
			if i == m.selectedIdx {
				// `>` indicator + accent highlight
				indicator := lipgloss.NewStyle().Foreground(p.accent).Bold(true).Render(">")
				rowText := m.formatEntryRow(e, inner-2, p)
				row := lipgloss.NewStyle().Foreground(p.accent).Render(rowText)
				rows = append(rows, indicator+" "+row)
			} else {
				rowText := m.formatEntryRow(e, inner-2, p)
				rows = append(rows, "  "+lipgloss.NewStyle().Foreground(p.fg).Render(rowText))
			}
		}
	}

	// Assemble pane content.
	lines := []string{header}
	lines = append(lines, rows...)

	// Pad to fill height (accounting for borders).
	contentH := height - 2 // 2 for borders
	for len(lines) < contentH {
		lines = append(lines, "")
	}

	body := strings.Join(lines, "\n")

	borderColor := p.border
	if m.activePane == 0 {
		borderColor = p.accent
	}
	paneStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width - 2).
		Height(height - 2)
	return paneStyle.Render(body)
}

// formatEntryRow formats a single entry as a fixed-width row string.
func (m Model) formatEntryRow(e cron.Entry, width int, p viewPal) string {
	nextStr := ""
	if t, err := cron.NextRun(e); err == nil {
		nextStr = cron.FormatRelative(t)
	} else {
		nextStr = lipgloss.NewStyle().Foreground(p.errCol).Render("invalid")
	}

	// Kind badge color: accent for agent, dim for pipeline.
	kindStyle := lipgloss.NewStyle().Foreground(p.dim)
	if e.Kind == "agent" {
		kindStyle = lipgloss.NewStyle().Foreground(p.accent)
	}

	// Columns: name (30%), schedule (25%), kind (10%), next (rest)
	nameW := width * 30 / 100
	schedW := width * 25 / 100
	kindW := 10

	name := truncate(e.Name, nameW)
	sched := truncate(e.Schedule, schedW)
	kind := kindStyle.Render(truncate(e.Kind, kindW))
	next := lipgloss.NewStyle().Foreground(p.dim).Render(nextStr)

	return fmt.Sprintf("%-*s %-*s %-*s %s",
		nameW, name,
		schedW, sched,
		kindW, kind,
		next,
	)
}

// viewLogPane renders the bottom pane showing recent log output.
func (m Model) viewLogPane(width, height int) string {
	p := m.pal()
	inner := width - 4
	if inner < 10 {
		inner = 10
	}

	header := m.panelBand("LOG OUTPUT", inner)

	// Available lines for log content.
	contentLines := height - 4 // header + 2 borders + header row
	if contentLines < 1 {
		contentLines = 1
	}

	// Calculate scroll: auto-scroll to bottom unless user scrolled up.
	totalLogs := len(m.logBuf)
	maxScroll := totalLogs - contentLines
	if maxScroll < 0 {
		maxScroll = 0
	}

	offset := maxScroll - m.logScrollOffset
	if offset < 0 {
		offset = 0
	}

	end := offset + contentLines
	if end > totalLogs {
		end = totalLogs
	}

	var logLines []string
	dimStyle := lipgloss.NewStyle().Foreground(p.dim)
	if totalLogs == 0 {
		logLines = append(logLines, dimStyle.Render("  waiting for log output..."))
	} else {
		slice := m.logBuf[offset:end]
		for _, l := range slice {
			l = strings.TrimRight(l, "\n\r")
			l = truncate(l, inner)
			logLines = append(logLines, dimStyle.Render(l))
		}
	}

	lines := []string{header}
	lines = append(lines, logLines...)

	// Pad to fill content height.
	paneH := height - 2
	for len(lines) < paneH {
		lines = append(lines, "")
	}

	body := strings.Join(lines, "\n")

	borderColor := p.border
	if m.activePane == 1 {
		borderColor = p.accent
	}
	paneStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width - 2).
		Height(height - 2)
	return paneStyle.Render(body)
}

// viewHintBar renders the single-line hint strip at the bottom.
func (m Model) viewHintBar(_ int) string {
	p := m.pal()
	dimStyle := lipgloss.NewStyle().Foreground(p.dim)
	var hints string
	if m.activePane == 0 {
		if m.filtering {
			hints = dimStyle.Render("[esc] clear filter  [enter] confirm  [tab] logs pane")
		} else {
			hints = dimStyle.Render("[j/k] navigate  [e] edit  [d] delete  [enter/r] run now  [/] filter  [tab] logs  [q] quit")
		}
	} else {
		hints = dimStyle.Render("[j/k] scroll  [tab] jobs pane  [q] quit")
	}
	return hints
}

// viewEditOverlay renders the edit form overlay.
func (m Model) viewEditOverlay() string {
	p := m.pal()
	ov := m.editOverlay
	labels := [5]string{"Name", "Schedule", "Kind", "Target", "Timeout"}

	overlayStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.pink).
		Background(lipgloss.Color(draculaBg)).
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

// renderOverlay places overlayContent centered over background using lipgloss.Place.
func renderOverlay(background, overlayContent string, width, height int) string {
	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		overlayContent,
		lipgloss.WithWhitespaceBackground(lipgloss.Color(draculaBg)),
	)
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

