package promptbuilder

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adam-stokes/orcai/internal/pipeline"
)

func pressKey(b *BubbleModel, kt tea.KeyType) *BubbleModel {
	m, _ := b.Update(tea.KeyMsg{Type: kt})
	return m.(*BubbleModel)
}

func TestTabCyclesActiveField(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude"})
	b := NewBubble(m)

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
	b := NewBubble(m)

	b = pressKey(b, tea.KeyShiftTab)
	if b.activeField != 2 {
		t.Fatalf("expected activeField 2 after shift+tab from 0, got %d", b.activeField)
	}
}

func TestRightCyclesPlugin(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude"})
	b := NewBubble(m)
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
	b := NewBubble(m)
	b = pressKey(b, tea.KeyLeft)
	got := m.Steps()[0].Plugin
	if got != pluginList[len(pluginList)-1] {
		t.Fatalf("expected %s after left from claude, got %s", pluginList[len(pluginList)-1], got)
	}
}

func TestRightCyclesModel(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude", Model: "claude-sonnet-4-6"})
	b := NewBubble(m)
	b = pressKey(b, tea.KeyTab) // focus Model field (activeField=1)
	b = pressKey(b, tea.KeyRight)
	got := m.Steps()[0].Model
	expected := modelsByPlugin["claude"][1]
	if got != expected {
		t.Fatalf("expected %s after right on model, got %s", expected, got)
	}
}

func TestStepNavigationResetsActiveField(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude"})
	m.AddStep(pipeline.Step{ID: "s2", Plugin: "gemini"})
	b := NewBubble(m)
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
	b := NewBubble(m)
	b.syncIndicesFromStep() // sync to s1 (claude = index 0)

	b = pressKey(b, tea.KeyDown) // navigate to s2 (gemini = index 1)
	if b.pluginIndex != 1 {
		t.Fatalf("expected pluginIndex 1 (gemini) after navigating to s2, got %d", b.pluginIndex)
	}
}

func TestTabNoOpWithNoSteps(t *testing.T) {
	m := New(nil)
	b := NewBubble(m)
	b = pressKey(b, tea.KeyTab)
	if b.activeField != 0 {
		t.Fatalf("expected activeField 0 (no-op) when no steps, got %d", b.activeField)
	}
}
