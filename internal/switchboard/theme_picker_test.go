package switchboard_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/switchboard"
)

// TestThemePicker_OpenOnT verifies that pressing 'T' sets themePickerOpen.
func TestThemePicker_OpenOnT(t *testing.T) {
	m := switchboard.NewWithPipelines([]string{"alpha"})
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	m2, ok := result.(switchboard.Model)
	if !ok {
		t.Fatalf("Update returned %T, want switchboard.Model", result)
	}
	if !m2.ThemePickerOpen() {
		t.Error("pressing T should open the theme picker")
	}
}

// TestThemePicker_CloseOnEsc verifies that pressing 'esc' in picker closes it.
func TestThemePicker_CloseOnEsc(t *testing.T) {
	m := switchboard.NewWithPipelines([]string{"alpha"})
	// Open picker.
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")})
	m2, ok := result.(switchboard.Model)
	if !ok {
		t.Fatalf("Update after T returned %T", result)
	}
	if !m2.ThemePickerOpen() {
		t.Skip("theme picker did not open (registry may be nil)")
	}
	// Close picker.
	result2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m3, ok := result2.(switchboard.Model)
	if !ok {
		t.Fatalf("Update after esc returned %T", result2)
	}
	if m3.ThemePickerOpen() {
		t.Error("pressing esc should close the theme picker")
	}
}

// TestThemePicker_ViewContainsBundleNames verifies viewThemePicker renders bundle names.
func TestThemePicker_ViewContainsBundleNames(t *testing.T) {
	view := switchboard.ViewThemePickerForTest()
	// The ABS theme should be listed.
	if !strings.Contains(view, "abs") && !strings.Contains(view, "ABS") {
		t.Errorf("theme picker view should contain 'abs' or 'ABS', got:\n%s", view)
	}
}

// TestFgSeq_RedColor verifies fgSeq produces the correct ANSI sequence for #ff0000.
func TestFgSeq_RedColor(t *testing.T) {
	got := switchboard.FgSeqForTest("#ff0000")
	want := "\x1b[38;2;255;0;0m"
	if got != want {
		t.Errorf("fgSeq(#ff0000) = %q, want %q", got, want)
	}
}
