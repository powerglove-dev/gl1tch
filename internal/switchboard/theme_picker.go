package switchboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/themes"
)

// viewThemePicker renders the theme picker overlay using a lipgloss modal style.
func viewThemePicker(bundles []themes.Bundle, cursor int, activeBundle *themes.Bundle, w int) string {
	// Resolve modal colors from active bundle (same pattern as viewQuitModalBox).
	borderColor := lipgloss.Color("#bd93f9") // Dracula purple fallback
	titleBG := lipgloss.Color("#bd93f9")
	titleFG := lipgloss.Color("#282a36")
	selBG := lipgloss.Color("#44475a")
	selFG := lipgloss.Color("#f8f8f2")
	helpFG := lipgloss.Color("#6272a4")

	if activeBundle != nil {
		if border := activeBundle.ResolveRef(activeBundle.Modal.Border); border != "" {
			borderColor = lipgloss.Color(border)
		}
		if tBG := activeBundle.ResolveRef(activeBundle.Modal.TitleBG); tBG != "" {
			titleBG = lipgloss.Color(tBG)
		}
		if tFG := activeBundle.ResolveRef(activeBundle.Modal.TitleFG); tFG != "" {
			titleFG = lipgloss.Color(tFG)
		}
		if dim := activeBundle.Palette.Dim; dim != "" {
			selBG = lipgloss.Color(dim)
		}
		if fg := activeBundle.Palette.FG; fg != "" {
			selFG = lipgloss.Color(fg)
		}
		if dim := activeBundle.Palette.Dim; dim != "" {
			helpFG = lipgloss.Color(dim)
		}
	}

	// innerW = content width; rows use Padding(0,1) adding 2, then border adds 2 more.
	innerW := 56
	if innerW+4 > w {
		innerW = max(w-4, 20)
	}

	headerStyle := lipgloss.NewStyle().
		Background(titleBG).
		Foreground(titleFG).
		Bold(true).
		Width(innerW).
		Padding(0, 1)

	rowStyle := lipgloss.NewStyle().
		Width(innerW).
		Padding(0, 1)

	selStyle := lipgloss.NewStyle().
		Background(selBG).
		Foreground(selFG).
		Width(innerW).
		Padding(0, 1)

	helpStyle := lipgloss.NewStyle().
		Foreground(helpFG).
		Width(innerW).
		Padding(0, 1)

	var rows []string
	rows = append(rows, headerStyle.Render("SELECT THEME"))

	for i, b := range bundles {
		// Swatch: seven colored █ blocks.
		pal := b.Palette
		swatch := fmt.Sprintf("%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m%s█\x1b[0m",
			fgSeq(pal.BG), fgSeq(pal.FG), fgSeq(pal.Accent),
			fgSeq(pal.Dim), fgSeq(pal.Border), fgSeq(pal.Error), fgSeq(pal.Success),
		)

		name := fmt.Sprintf("%s (%s)", b.DisplayName, b.Name)
		if activeBundle != nil && b.Name == activeBundle.Name {
			name += " \u2713"
		}

		content := swatch + "  " + name
		if i == cursor {
			rows = append(rows, selStyle.Render(content))
		} else {
			rows = append(rows, rowStyle.Render(content))
		}
	}

	rows = append(rows, helpStyle.Render("j/k navigate \u00b7 enter apply \u00b7 esc cancel"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(innerW + 2).
		Render(strings.Join(rows, "\n"))
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
