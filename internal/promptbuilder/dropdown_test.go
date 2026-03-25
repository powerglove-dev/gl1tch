package promptbuilder

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newTestDD creates a dropdown with items a, b, c where index 1 is a separator.
func newTestDD() Dropdown {
	items := []string{"alpha", "---", "beta", "gamma"}
	seps := map[int]bool{1: true}
	return NewDropdown(items, seps)
}

func TestDropdown_InitialSkipsSeparator(t *testing.T) {
	items := []string{"---", "alpha", "beta"}
	seps := map[int]bool{0: true}
	d := NewDropdown(items, seps)
	if d.Selected() != 1 {
		t.Fatalf("expected initial selection 1 (skip separator at 0), got %d", d.Selected())
	}
}

func TestDropdown_MoveDownSkipsSeparator(t *testing.T) {
	d := newTestDD()
	// selected=0 (alpha), next non-sep after 0 skipping index 1 (sep) is index 2 (beta)
	d.Open()
	d.moveDown()
	if d.Selected() != 2 {
		t.Fatalf("expected selection 2 (beta) after moveDown, got %d", d.Selected())
	}
}

func TestDropdown_MoveUpSkipsSeparator(t *testing.T) {
	d := newTestDD()
	d.Open()
	// Move down twice: 0 -> 2 -> 3
	d.moveDown()
	d.moveDown()
	if d.Selected() != 3 {
		t.Fatalf("expected selection 3 (gamma), got %d", d.Selected())
	}
	// Now move up: 3 -> 2 (skip sep at 1)
	d.moveUp()
	if d.Selected() != 2 {
		t.Fatalf("expected selection 2 (beta) after moveUp, got %d", d.Selected())
	}
	// Move up again: 2 -> 0 (skip sep at 1)
	d.moveUp()
	if d.Selected() != 0 {
		t.Fatalf("expected selection 0 (alpha) after moveUp, got %d", d.Selected())
	}
}

func TestDropdown_MoveUpAtTop_NoOp(t *testing.T) {
	d := newTestDD()
	d.Open()
	// already at 0
	d.moveUp()
	if d.Selected() != 0 {
		t.Fatalf("expected no-op moveUp at top, got %d", d.Selected())
	}
}

func TestDropdown_MoveDownAtBottom_NoOp(t *testing.T) {
	d := newTestDD()
	d.Open()
	d.moveDown() // -> 2
	d.moveDown() // -> 3
	d.moveDown() // -> no-op (3 is last)
	if d.Selected() != 3 {
		t.Fatalf("expected no-op moveDown at bottom, got %d", d.Selected())
	}
}

func TestDropdown_OpenClose(t *testing.T) {
	d := newTestDD()
	if d.IsOpen() {
		t.Fatal("expected closed initially")
	}
	d.Open()
	if !d.IsOpen() {
		t.Fatal("expected open after Open()")
	}
	d.Close()
	if d.IsOpen() {
		t.Fatal("expected closed after Close()")
	}
}

func TestDropdown_Toggle(t *testing.T) {
	d := newTestDD()
	d.Toggle()
	if !d.IsOpen() {
		t.Fatal("expected open after Toggle")
	}
	d.Toggle()
	if d.IsOpen() {
		t.Fatal("expected closed after second Toggle")
	}
}

func TestDropdown_EnterConfirms(t *testing.T) {
	d := newTestDD()
	d.Open()
	d.moveDown() // -> 2 (beta)
	confirmed, changed := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !confirmed {
		t.Fatal("expected confirmed=true on Enter")
	}
	if !changed {
		t.Fatal("expected changed=true when selection moved from 0 to 2")
	}
	if d.IsOpen() {
		t.Fatal("expected closed after Enter")
	}
	if d.Value() != "beta" {
		t.Fatalf("expected 'beta', got %q", d.Value())
	}
}

func TestDropdown_EnterNoChange(t *testing.T) {
	d := newTestDD()
	d.Open()
	// no move, confirm at same position
	confirmed, changed := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !confirmed {
		t.Fatal("expected confirmed=true on Enter")
	}
	if changed {
		t.Fatal("expected changed=false when selection didn't move")
	}
}

func TestDropdown_EscapeReverts(t *testing.T) {
	d := newTestDD()
	d.Open()
	prev := d.Selected()
	d.moveDown() // -> 2 (beta)
	d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if d.IsOpen() {
		t.Fatal("expected closed after Esc")
	}
	if d.Selected() != prev {
		t.Fatalf("expected reverted to %d, got %d", prev, d.Selected())
	}
}

func TestDropdown_ScrollClamping(t *testing.T) {
	// Create a dropdown with more items than ddMaxVisible
	items := make([]string, 12)
	for i := range items {
		items[i] = string(rune('a'+i)) + "-item"
	}
	d := NewDropdown(items, nil)
	d.Open()

	// Navigate down past ddMaxVisible
	for i := 0; i < 10; i++ {
		d.moveDown()
	}
	// selected should be 10
	if d.Selected() != 10 {
		t.Fatalf("expected selected=10, got %d", d.Selected())
	}
	// scrollOff should be adjusted: selected(10) - ddMaxVisible(8) + 1 = 3
	if d.scrollOff != 3 {
		t.Fatalf("expected scrollOff=3, got %d", d.scrollOff)
	}

	// Navigate back up
	for i := 0; i < 5; i++ {
		d.moveUp()
	}
	// selected = 5, scrollOff should shrink
	if d.Selected() != 5 {
		t.Fatalf("expected selected=5, got %d", d.Selected())
	}
	if d.scrollOff != 3 {
		// scrollOff should not change when selected is still within visible window
		t.Fatalf("expected scrollOff=3, got %d", d.scrollOff)
	}
}

func TestDropdown_ScrollUpClamp(t *testing.T) {
	items := make([]string, 12)
	for i := range items {
		items[i] = string(rune('a'+i)) + "-item"
	}
	d := NewDropdown(items, nil)
	d.Open()

	// move to item 5
	for i := 0; i < 5; i++ {
		d.moveDown()
	}
	// scrollOff still 0 (5 < 8)
	if d.scrollOff != 0 {
		t.Fatalf("expected scrollOff=0, got %d", d.scrollOff)
	}

	// Go up to item 0 — scrollOff should clamp to selected
	for i := 0; i < 5; i++ {
		d.moveUp()
	}
	if d.Selected() != 0 {
		t.Fatalf("expected selected=0, got %d", d.Selected())
	}
	if d.scrollOff != 0 {
		t.Fatalf("expected scrollOff=0, got %d", d.scrollOff)
	}
}

func TestDropdown_SetValue(t *testing.T) {
	d := newTestDD()
	d.SetValue("gamma")
	if d.Value() != "gamma" {
		t.Fatalf("expected gamma, got %q", d.Value())
	}
}

func TestDropdown_SetValue_NotFound_NoOp(t *testing.T) {
	d := newTestDD()
	orig := d.Value()
	d.SetValue("nonexistent")
	if d.Value() != orig {
		t.Fatalf("expected no change, orig=%q now=%q", orig, d.Value())
	}
}

func TestDropdown_SetValue_SkipsSeparator(t *testing.T) {
	items := []string{"alpha", "---", "beta"}
	seps := map[int]bool{1: true}
	d := NewDropdown(items, seps)
	// Trying to set to "---" should not work since it's a separator
	d.SetValue("---")
	// Should remain at alpha (index 0)
	if d.Value() != "alpha" {
		t.Fatalf("expected alpha (separator not selectable), got %q", d.Value())
	}
}

func TestDropdown_Value_EmptyItems(t *testing.T) {
	d := NewDropdown(nil, nil)
	if d.Value() != "" {
		t.Fatalf("expected empty value for empty items, got %q", d.Value())
	}
}

func TestDropdown_UpdateWhenClosed_NoOp(t *testing.T) {
	d := newTestDD()
	confirmed, changed := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if confirmed || changed {
		t.Fatal("expected no-op when dropdown is closed")
	}
}
