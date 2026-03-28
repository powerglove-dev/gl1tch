package switchboard

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/styles"
	"github.com/adam-stokes/orcai/internal/themes"
	"github.com/adam-stokes/orcai/internal/tuikit"
)

// viewThemePicker renders the theme picker overlay.
func viewThemePicker(bundles []themes.Bundle, cursor int, activeBundle *themes.Bundle, w int) string {
	return tuikit.ViewThemePicker(bundles, cursor, activeBundle, w)
}

// handleThemePicker routes key events when the theme picker is open.
func (m Model) handleThemePicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	bundles := m.registry.All()
	newCursor, close, _, cmd := tuikit.HandleThemePicker(m.themePickerCursor, bundles, msg.String())
	m.themePickerCursor = newCursor
	if close {
		m.themePickerOpen = false
	}
	return m, cmd
}

// FgSeqForTest is an exported wrapper for styles.FgSeq — used in tests only.
func FgSeqForTest(hex string) string {
	return styles.FgSeq(hex)
}

// ViewThemePickerForTest renders the theme picker with registry bundles — used in tests.
func ViewThemePickerForTest() string {
	m := New()
	if m.registry == nil {
		return ""
	}
	bundles := m.registry.All()
	return tuikit.ViewThemePicker(bundles, 0, m.registry.Active(), 120)
}
