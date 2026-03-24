package sidebar_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/sidebar"
)

func TestParseWindows(t *testing.T) {
	input := "0 ORCAI 0\n1 claude-1 1\n2 opencode-2 0\n"
	windows := sidebar.ParseWindows(input)
	if len(windows) != 2 {
		t.Fatalf("expected 2 non-home windows, got %d", len(windows))
	}
	if windows[0].Index != 1 || windows[0].Name != "claude-1" || !windows[0].Active {
		t.Errorf("windows[0] wrong: %+v", windows[0])
	}
	if windows[1].Index != 2 || windows[1].Name != "opencode-2" || windows[1].Active {
		t.Errorf("windows[1] wrong: %+v", windows[1])
	}
}

func TestParseWindows_Empty(t *testing.T) {
	windows := sidebar.ParseWindows("")
	if len(windows) != 0 {
		t.Errorf("expected 0 windows for empty input, got %d", len(windows))
	}
}

func TestParseWindows_OnlyHome(t *testing.T) {
	input := "0 ORCAI 1\n"
	windows := sidebar.ParseWindows(input)
	if len(windows) != 0 {
		t.Errorf("expected 0 windows (home only), got %d", len(windows))
	}
}

// Navigation tests use tea.KeyRunes because Task 2 implements a custom
// Update() that dispatches on msg.String(), matching "j", "k", "down", "up".
func TestCursorDown(t *testing.T) {
	m := sidebar.NewWithWindows([]sidebar.Window{
		{Index: 1, Name: "claude-1"},
		{Index: 2, Name: "opencode-2"},
	})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := m2.(sidebar.Model).Cursor(); got != 1 {
		t.Errorf("cursor after j: got %d, want 1", got)
	}
}

func TestCursorUp(t *testing.T) {
	m := sidebar.NewWithWindows([]sidebar.Window{
		{Index: 1, Name: "claude-1"},
		{Index: 2, Name: "opencode-2"},
	})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m3, _ := m2.(sidebar.Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if got := m3.(sidebar.Model).Cursor(); got != 0 {
		t.Errorf("cursor after j then k: got %d, want 0", got)
	}
}

func TestCursorDoesNotGoNegative(t *testing.T) {
	m := sidebar.NewWithWindows([]sidebar.Window{
		{Index: 1, Name: "claude-1"},
	})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if got := m2.(sidebar.Model).Cursor(); got != 0 {
		t.Errorf("cursor should stay at 0: got %d", got)
	}
}

func TestCursorDoesNotExceedLength(t *testing.T) {
	m := sidebar.NewWithWindows([]sidebar.Window{
		{Index: 1, Name: "claude-1"},
	})
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := m2.(sidebar.Model).Cursor(); got != 0 {
		t.Errorf("cursor should stay at 0 with one window: got %d", got)
	}
}

func TestViewContainsWindowName(t *testing.T) {
	m := sidebar.NewWithWindows([]sidebar.Window{
		{Index: 1, Name: "claude-1"},
		{Index: 2, Name: "opencode-2"},
	})
	view := m.View()
	if !strings.Contains(view, "claude-1") {
		t.Errorf("View() does not contain 'claude-1':\n%s", view)
	}
	if !strings.Contains(view, "opencode-2") {
		t.Errorf("View() does not contain 'opencode-2':\n%s", view)
	}
}

func TestViewContainsHeader(t *testing.T) {
	m := sidebar.NewWithWindows([]sidebar.Window{})
	view := m.View()
	// Footer should show nav hints but NOT new-session/prompt-builder (moved to status bar).
	if !strings.Contains(view, "enter focus") {
		t.Errorf("View() does not contain footer hint 'enter focus':\n%s", view)
	}
	if strings.Contains(view, "n new") {
		t.Errorf("View() still contains removed 'n new' footer hint:\n%s", view)
	}
}

func TestViewContainsActiveAccent(t *testing.T) {
	m := sidebar.NewWithWindows([]sidebar.Window{
		{Index: 1, Name: "claude-1"},
		{Index: 2, Name: "opencode-2"},
	})
	view := m.View()
	// The first window (cursor=0) should have the active accent.
	// Find the line containing claude-1 and verify it has the accent.
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(line, "claude-1") {
			if !strings.Contains(line, "▎") {
				t.Errorf("active window line missing accent '▎': %q", line)
			}
			return
		}
	}
	t.Errorf("View() does not contain a line with 'claude-1':\n%s", view)
}
