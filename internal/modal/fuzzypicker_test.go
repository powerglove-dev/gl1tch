package modal

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/8op-org/gl1tch/internal/styles"
)

func TestFuzzyPickerModel_OpenResetsState(t *testing.T) {
	p := NewFuzzyPickerModel(8)
	if p.IsOpen() {
		t.Fatal("picker should start closed")
	}
	p.Open([]string{"alpha", "beta", "gamma"})
	if !p.IsOpen() {
		t.Fatal("picker should be open after Open()")
	}
	if p.SelectedItem() != "alpha" {
		t.Errorf("cursor should be at first item, got %q", p.SelectedItem())
	}
	if p.SelectedOriginalIdx() != 0 {
		t.Errorf("original idx should be 0, got %d", p.SelectedOriginalIdx())
	}
}

func TestFuzzyPickerModel_OpenEmptyList(t *testing.T) {
	p := NewFuzzyPickerModel(8)
	p.Open([]string{})
	if p.SelectedItem() != "" {
		t.Errorf("empty list should return empty SelectedItem, got %q", p.SelectedItem())
	}
}

func TestFuzzyPickerModel_FilterNarrowsResults(t *testing.T) {
	p := NewFuzzyPickerModel(8)
	p.Open([]string{"apple", "banana", "apricot", "cherry"})
	// Simulate typing "ap"
	p.input.SetValue("ap")
	p.applyFilter()
	for _, item := range p.shown {
		if !strings.Contains(strings.ToLower(item), "ap") && fuzzyScore(item, "ap") == 0 {
			t.Errorf("item %q should not appear in filtered results for 'ap'", item)
		}
	}
	if len(p.shown) == 4 {
		t.Error("filter should have reduced the result set")
	}
}

func TestFuzzyPickerModel_ConfirmSelection(t *testing.T) {
	p := NewFuzzyPickerModel(8)
	p.Open([]string{"(none)", "prompt-one", "prompt-two"})

	// Navigate down once.
	p, _, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	// Press enter.
	p, ev, _ := p.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if ev != FuzzyPickerConfirmed {
		t.Errorf("expected FuzzyPickerConfirmed, got %v", ev)
	}
	if p.IsOpen() {
		t.Error("picker should be closed after confirmation")
	}
	if p.SelectedItem() != "prompt-one" {
		t.Errorf("expected 'prompt-one', got %q", p.SelectedItem())
	}
	if p.SelectedOriginalIdx() != 1 {
		t.Errorf("expected original idx 1, got %d", p.SelectedOriginalIdx())
	}
}

func TestFuzzyPickerModel_CancelWithEscape(t *testing.T) {
	p := NewFuzzyPickerModel(8)
	p.Open([]string{"alpha", "beta"})
	p, ev, _ := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if ev != FuzzyPickerCancelled {
		t.Errorf("expected FuzzyPickerCancelled, got %v", ev)
	}
	if p.IsOpen() {
		t.Error("picker should be closed after cancel")
	}
}

func TestFuzzyPickerModel_EnterOnEmptyListNoConfirm(t *testing.T) {
	p := NewFuzzyPickerModel(8)
	p.Open([]string{})
	_, ev, _ := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if ev != FuzzyPickerNone {
		t.Errorf("enter on empty list should return FuzzyPickerNone, got %v", ev)
	}
}

func TestFuzzyPickerModel_ViewInlineWhenOpen(t *testing.T) {
	p := NewFuzzyPickerModel(8)
	p.Open([]string{"alpha", "beta", "gamma"})
	pal := styles.ANSIPalette{
		Accent: "\x1b[35m",
		Dim:    "\x1b[2m",
		FG:     "\x1b[97m",
		Border: "\x1b[36m",
	}
	view := p.ViewInline(80, pal)
	if view == "" {
		t.Fatal("ViewInline should return non-empty string when open")
	}
	if !strings.Contains(view, "alpha") {
		t.Error("ViewInline should contain first item 'alpha'")
	}
}

func TestFuzzyPickerModel_ViewInlineWhenClosed(t *testing.T) {
	p := NewFuzzyPickerModel(8)
	pal := styles.ANSIPalette{}
	if got := p.ViewInline(80, pal); got != "" {
		t.Errorf("ViewInline should return empty string when closed, got %q", got)
	}
}
