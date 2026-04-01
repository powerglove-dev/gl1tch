package sidebar_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/sidebar"
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

// Navigation tests use tea.KeyRunes because Update() dispatches on msg.String().
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
	if !strings.Contains(view, "┌") {
		t.Errorf("View() missing outer border '┌':\n%s", view)
	}
	if !strings.Contains(view, "SYSOP MONITOR") {
		t.Errorf("View() missing 'SYSOP MONITOR' header:\n%s", view)
	}
	if !strings.Contains(view, "enter") {
		t.Errorf("View() does not contain footer hint 'enter':\n%s", view)
	}
}

func TestViewContainsActiveAccent(t *testing.T) {
	m := sidebar.NewWithWindows([]sidebar.Window{
		{Index: 1, Name: "claude-1"},
		{Index: 2, Name: "opencode-2"},
	})
	view := m.View()
	// cursor=0 → first node row should have the selection background escape.
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(line, "claude-1") && strings.Contains(line, "[1]") {
			if !strings.Contains(line, "\x1b[48;5;235m") {
				t.Errorf("active node line missing selection background: %q", line)
			}
			return
		}
	}
	t.Errorf("View() does not contain a '[1] claude-1' selected line:\n%s", view)
}

func TestViewActivityLog(t *testing.T) {
	m := sidebar.NewWithWindows([]sidebar.Window{
		{Index: 1, Name: "claude-1"},
	})
	// Send a telemetry event.
	m2, _ := m.Update(sidebar.TelemetryMsg{
		SessionID:    "s1",
		WindowName:   "claude-1",
		Provider:     "claude-sonnet-4",
		Status:       "done",
		InputTokens:  5000,
		OutputTokens: 200,
		CostUSD:      0.018,
	})
	view := m2.(sidebar.Model).View()
	if !strings.Contains(view, "ACTIVITY LOG") {
		t.Errorf("View() missing ACTIVITY LOG section:\n%s", view)
	}
	// Should contain the event entry (done with cost).
	if !strings.Contains(view, "done") && !strings.Contains(view, "0.018") {
		t.Errorf("View() activity log missing event entry:\n%s", view)
	}
}

func TestViewNodeStatus(t *testing.T) {
	m := sidebar.NewWithWindows([]sidebar.Window{
		{Index: 1, Name: "claude-1"},
	})

	// No telemetry → [WAIT]
	if !strings.Contains(m.View(), "[WAIT]") {
		t.Errorf("expected [WAIT] for no-data node:\n%s", m.View())
	}

	// Streaming → [BUSY]
	m2, _ := m.Update(sidebar.TelemetryMsg{
		SessionID:  "s1",
		WindowName: "claude-1",
		Provider:   "claude-sonnet-4",
		Status:     "streaming",
	})
	if !strings.Contains(m2.(sidebar.Model).View(), "[BUSY]") {
		t.Errorf("expected [BUSY] after streaming event:\n%s", m2.(sidebar.Model).View())
	}

	// Done → [IDLE]
	m3, _ := m2.(sidebar.Model).Update(sidebar.TelemetryMsg{
		SessionID:    "s1",
		WindowName:   "claude-1",
		Provider:     "claude-sonnet-4",
		Status:       "done",
		InputTokens:  5000,
		OutputTokens: 200,
		CostUSD:      0.018,
	})
	if !strings.Contains(m3.(sidebar.Model).View(), "[IDLE]") {
		t.Errorf("expected [IDLE] after done event:\n%s", m3.(sidebar.Model).View())
	}
}
