package console

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ── glitchFilterSuggestions ───────────────────────────────────────────────────

func TestGlitchFilterSuggestions_BareSlash(t *testing.T) {
	results := glitchFilterSuggestions("/")
	if len(results) != len(glitchSlashCommands) {
		t.Errorf("bare '/' should return all %d commands, got %d", len(glitchSlashCommands), len(results))
	}
}

func TestGlitchFilterSuggestions_ExactPrefix(t *testing.T) {
	results := glitchFilterSuggestions("/model")
	if len(results) == 0 {
		t.Fatal("expected matches for /model, got none")
	}
	// /model and /models should both appear; /model should rank first (higher score).
	if results[0].cmd != "/model" {
		t.Errorf("expected /model to rank first, got %s", results[0].cmd)
	}
	found := false
	for _, r := range results {
		if r.cmd == "/models" {
			found = true
		}
	}
	if !found {
		t.Error("expected /models to appear in results for query /model")
	}
}

func TestGlitchFilterSuggestions_PartialMatch(t *testing.T) {
	results := glitchFilterSuggestions("/th")
	for _, r := range results {
		if !strings.Contains(r.cmd, "th") {
			t.Errorf("unexpected command %s in results for /th query", r.cmd)
		}
	}
	found := false
	for _, r := range results {
		if r.cmd == "/themes" {
			found = true
		}
	}
	if !found {
		t.Error("expected /themes in results for /th query")
	}
}

func TestGlitchFilterSuggestions_NoMatch(t *testing.T) {
	results := glitchFilterSuggestions("/zzz")
	if len(results) != 0 {
		t.Errorf("expected no results for /zzz, got %d", len(results))
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// newTestPanel returns a focused glitchChatPanel with no backend.
func newTestPanel() glitchChatPanel {
	return newGlitchPanel("", nil, nil, "", nil)
}

// sendKeys sends a series of key strings to the panel and returns the final state.
func sendKeys(p glitchChatPanel, keys ...string) glitchChatPanel {
	for _, k := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		// For special keys use the named mapping.
		switch k {
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		}
		p, _ = p.update(msg)
	}
	return p
}

// typeString types a string character by character into the panel.
func typeString(p glitchChatPanel, s string) glitchChatPanel {
	for _, r := range s {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		p, _ = p.update(msg)
	}
	return p
}

// ── BubbleTea model tests ─────────────────────────────────────────────────────

func TestGlitchPanel_SlashActivatesOverlay(t *testing.T) {
	p := newTestPanel()
	p = typeString(p, "/")
	if !p.acActive {
		t.Error("expected acActive == true after typing '/'")
	}
	if len(p.acSuggestions) != len(glitchSlashCommands) {
		t.Errorf("expected %d suggestions, got %d", len(glitchSlashCommands), len(p.acSuggestions))
	}
}

func TestGlitchPanel_TabAdvancesCursor(t *testing.T) {
	p := newTestPanel()
	p = typeString(p, "/")
	if !p.acActive {
		t.Fatal("overlay not active after '/'")
	}
	initial := p.acCursor
	p = sendKeys(p, "tab")
	if p.acCursor != (initial+1)%len(p.acSuggestions) {
		t.Errorf("expected cursor %d after tab, got %d", (initial+1)%len(p.acSuggestions), p.acCursor)
	}
}

func TestGlitchPanel_TabWrapsAround(t *testing.T) {
	p := newTestPanel()
	p = typeString(p, "/")
	n := len(p.acSuggestions)
	for i := 0; i < n; i++ {
		p = sendKeys(p, "tab")
	}
	if p.acCursor != 0 {
		t.Errorf("expected cursor to wrap to 0 after %d tabs, got %d", n, p.acCursor)
	}
}

func TestGlitchPanel_EnterInsertsSuggestion(t *testing.T) {
	p := newTestPanel()
	p = typeString(p, "/")
	chosen := p.acSuggestions[p.acCursor].cmd
	p = sendKeys(p, "enter")
	if p.acActive {
		t.Error("expected acActive == false after enter")
	}
	want := chosen + " "
	if p.input.Value() != want {
		t.Errorf("expected input %q, got %q", want, p.input.Value())
	}
}

func TestGlitchPanel_EscDismissesOverlay(t *testing.T) {
	p := newTestPanel()
	p = typeString(p, "/")
	if !p.acActive {
		t.Fatal("overlay not active")
	}
	origVal := p.input.Value()
	p = sendKeys(p, "esc")
	if p.acActive {
		t.Error("expected acActive == false after esc")
	}
	if p.input.Value() != origVal {
		t.Errorf("expected input unchanged after esc: got %q, want %q", p.input.Value(), origVal)
	}
}
