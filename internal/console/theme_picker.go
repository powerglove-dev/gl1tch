package console

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/styles"
	"github.com/8op-org/gl1tch/internal/tuikit"
)

// viewThemePicker renders the theme picker overlay.
func viewThemePicker(m Model) string {
	if m.registry == nil {
		return ""
	}
	dark := m.registry.BundlesByMode("dark")
	light := m.registry.BundlesByMode("light")
	return tuikit.ViewThemePicker(dark, light, m.themePicker, m.registry.Active(), m.width)
}

// handleThemePicker routes key events when the theme picker is open.
func (m Model) handleThemePicker(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.registry == nil {
		m.themePicker.Open = false
		m.previewBundle = nil
		return m, nil
	}
	dark := m.registry.BundlesByMode("dark")
	light := m.registry.BundlesByMode("light")
	newPicker, close, _, cmd := tuikit.HandleThemePicker(m.themePicker, dark, light, msg.String())
	m.themePicker = newPicker
	if close {
		m.themePicker.Open = false
		m.previewBundle = nil // restore registry-backed active bundle
		m.refreshTDFHeader()
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
	dark := m.registry.BundlesByMode("dark")
	light := m.registry.BundlesByMode("light")
	return tuikit.ViewThemePicker(dark, light, m.themePicker, m.registry.Active(), 120)
}
