package switchboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/themes"
)

// viewThemePicker renders the theme picker overlay.
func viewThemePicker(bundles []themes.Bundle, cursor int, activeBundle *themes.Bundle, w int) string {
	var sb strings.Builder

	// Title bar
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#282a36")).
		Background(lipgloss.Color("#bd93f9")).
		Padding(0, 1).
		Render("  SELECT THEME  ")
	sb.WriteString(title + "\n\n")

	for i, b := range bundles {
		// Swatch row: seven colored █ blocks
		pal := b.Palette
		swatch := fmt.Sprintf("%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m",
			fgSeq(pal.BG), fgSeq(pal.FG), fgSeq(pal.Accent),
			fgSeq(pal.Dim), fgSeq(pal.Border), fgSeq(pal.Error), fgSeq(pal.Success),
		)

		name := fmt.Sprintf("%s (%s)", b.DisplayName, b.Name)
		if activeBundle != nil && b.Name == activeBundle.Name {
			name += " \u2713"
		}

		row := fmt.Sprintf(" %s  %s", swatch, name)
		if i == cursor {
			row = lipgloss.NewStyle().
				Background(lipgloss.Color("#44475a")).
				Foreground(lipgloss.Color("#f8f8f2")).
				Render(row)
		}
		sb.WriteString(row + "\n")
	}

	sb.WriteString("\n")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4")).Render("j/k navigate \u00b7 enter apply \u00b7 esc cancel")
	sb.WriteString(" " + help)

	return sb.String()
}

// fgSeq converts a hex color to an ANSI 24-bit foreground escape.
func fgSeq(hex string) string {
	r, g, b := hexToRGBFromStyles(hex)
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

// FgSeqForTest is an exported wrapper for fgSeq — used in tests only.
func FgSeqForTest(hex string) string { return fgSeq(hex) }

// ViewThemePickerForTest renders the theme picker with registry bundles — used in tests.
func ViewThemePickerForTest() string {
	m := New()
	if m.registry == nil {
		return ""
	}
	bundles := m.registry.All()
	return viewThemePicker(bundles, 0, m.registry.Active(), 120)
}

// handleThemePicker routes key events when the theme picker is open.
func (m Model) handleThemePicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	bundles := m.registry.All()
	switch msg.String() {
	case "esc", "q":
		m.themePickerOpen = false
	case "j", "down":
		if m.themePickerCursor < len(bundles)-1 {
			m.themePickerCursor++
		}
	case "k", "up":
		if m.themePickerCursor > 0 {
			m.themePickerCursor--
		}
	case "enter":
		if m.registry != nil && m.themePickerCursor < len(bundles) {
			chosen := bundles[m.themePickerCursor]
			_ = m.registry.SetActive(chosen.Name)
			m.themePickerOpen = false
			return m, tea.ClearScreen
		}
	}
	return m, nil
}
