package promptbuilder

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/pipeline"
)

// testProviders is a deterministic provider list used across all BubbleModel tests.
// It mirrors the real picker.Providers structure so index arithmetic is predictable.
var testProviders = []picker.ProviderDef{
	{
		ID: "claude", Label: "Claude",
		Models: []picker.ModelOption{
			{ID: "claude-sonnet-4-6", Label: "Sonnet 4.6"},
			{ID: "claude-opus-4-6", Label: "Opus 4.6"},
			{ID: "claude-haiku-4-5-20251001", Label: "Haiku 4.5"},
		},
	},
	{
		ID: "gemini", Label: "Gemini",
		Models: []picker.ModelOption{
			{ID: "gemini-2.0-flash", Label: "Flash 2.0"},
			{ID: "gemini-1.5-pro", Label: "Pro 1.5"},
		},
	},
	{
		ID: "opencode", Label: "OpenCode",
		Models: []picker.ModelOption{},
	},
	{
		ID: "openclaw", Label: "OpenClaw",
		Models: []picker.ModelOption{},
	},
}

func pressKey(b *BubbleModel, kt tea.KeyType) *BubbleModel {
	m, _ := b.Update(tea.KeyMsg{Type: kt})
	return m.(*BubbleModel)
}

func TestTabCyclesActiveField(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude"})
	b := NewBubble(m, testProviders)

	if b.activeField != 0 {
		t.Fatalf("expected activeField 0 initially, got %d", b.activeField)
	}
	b = pressKey(b, tea.KeyTab)
	if b.activeField != 1 {
		t.Fatalf("expected activeField 1 after tab, got %d", b.activeField)
	}
	b = pressKey(b, tea.KeyTab)
	if b.activeField != 2 {
		t.Fatalf("expected activeField 2 after tab, got %d", b.activeField)
	}
	b = pressKey(b, tea.KeyTab)
	if b.activeField != 0 {
		t.Fatalf("expected activeField 0 after wrap, got %d", b.activeField)
	}
}

func TestShiftTabCyclesBackward(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude"})
	b := NewBubble(m, testProviders)

	b = pressKey(b, tea.KeyShiftTab)
	if b.activeField != 2 {
		t.Fatalf("expected activeField 2 after shift+tab from 0, got %d", b.activeField)
	}
}

func TestRightCyclesPlugin(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude"})
	b := NewBubble(m, testProviders)
	// activeField 0 = Plugin
	b = pressKey(b, tea.KeyRight)
	got := m.Steps()[0].Plugin
	if got != "gemini" {
		t.Fatalf("expected gemini after right from claude, got %s", got)
	}
}

func TestLeftCyclesPluginBackward(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude"})
	b := NewBubble(m, testProviders)
	b = pressKey(b, tea.KeyLeft)
	got := m.Steps()[0].Plugin
	if got != testProviders[len(testProviders)-1].ID {
		t.Fatalf("expected %s after left from claude, got %s", testProviders[len(testProviders)-1].ID, got)
	}
}

func TestRightCyclesModel(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude", Model: "claude-sonnet-4-6"})
	b := NewBubble(m, testProviders)
	b = pressKey(b, tea.KeyTab) // focus Model field (activeField=1)
	b = pressKey(b, tea.KeyRight)
	got := m.Steps()[0].Model
	expected := testProviders[0].Models[1].ID
	if got != expected {
		t.Fatalf("expected %s after right on model, got %s", expected, got)
	}
}

func TestStepNavigationResetsActiveField(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude"})
	m.AddStep(pipeline.Step{ID: "s2", Plugin: "gemini"})
	b := NewBubble(m, testProviders)
	b = pressKey(b, tea.KeyTab) // activeField = 1
	b = pressKey(b, tea.KeyDown)
	if b.activeField != 0 {
		t.Fatalf("expected activeField 0 after step nav, got %d", b.activeField)
	}
	if m.SelectedIndex() != 1 {
		t.Fatalf("expected selectedIndex 1 after down, got %d", m.SelectedIndex())
	}
}

func TestStepNavigationSyncsPluginIndex(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude"})
	m.AddStep(pipeline.Step{ID: "s2", Plugin: "gemini"})
	b := NewBubble(m, testProviders)
	b.syncIndicesFromStep() // sync to s1 (claude = index 0)

	b = pressKey(b, tea.KeyDown) // navigate to s2 (gemini = index 1)
	if b.pluginIndex != 1 {
		t.Fatalf("expected pluginIndex 1 (gemini) after navigating to s2, got %d", b.pluginIndex)
	}
}

func TestTabNoOpWithNoSteps(t *testing.T) {
	m := New(nil)
	b := NewBubble(m, testProviders)
	b = pressKey(b, tea.KeyTab)
	if b.activeField != 0 {
		t.Fatalf("expected activeField 0 (no-op) when no steps, got %d", b.activeField)
	}
}

func TestPromptFieldUpdatesStep(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude"})
	b := NewBubble(m, testProviders)

	// Tab twice to reach Prompt field (0→1→2)
	b = pressKey(b, tea.KeyTab)
	b = pressKey(b, tea.KeyTab)
	if b.activeField != 2 {
		t.Fatalf("expected activeField 2, got %d", b.activeField)
	}

	// Type "hello" — includes no action-bound characters
	for _, r := range "hello" {
		mm, _ := b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		b = mm.(*BubbleModel)
	}
	if m.Steps()[0].Prompt != "hello" {
		t.Fatalf("expected Prompt 'hello', got %q", m.Steps()[0].Prompt)
	}
}

func TestPromptFieldAllowsActionKeys(t *testing.T) {
	// Regression: 's', 'r', 'q', 'k', 'j', '+' were intercepted as action
	// bindings before reaching the textinput when activeField==2.
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude"})
	b := NewBubble(m, testProviders)

	b = pressKey(b, tea.KeyTab)
	b = pressKey(b, tea.KeyTab)

	for _, r := range "save+rqkj" {
		mm, _ := b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		b = mm.(*BubbleModel)
	}
	if m.Steps()[0].Prompt != "save+rqkj" {
		t.Fatalf("expected Prompt 'save+rqkj', got %q", m.Steps()[0].Prompt)
	}
	// activeField must still be 2 — 'k'/'j' must not have navigated steps
	if b.activeField != 2 {
		t.Fatalf("expected activeField 2, got %d", b.activeField)
	}
}
