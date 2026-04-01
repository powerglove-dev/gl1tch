package promptbuilder

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/powerglove-dev/gl1tch/internal/picker"
	"github.com/powerglove-dev/gl1tch/internal/pipeline"
)

// testProviders is a deterministic provider list used across all BubbleModel tests.
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

func pressRune(b *BubbleModel, r rune) *BubbleModel {
	m, _ := b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	return m.(*BubbleModel)
}

// TestTabCyclesActiveField checks that Tab moves through fields in the active group.
func TestTabCyclesActiveField(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
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
	// Core group is the only expanded group initially; wraps back to 0
	if b.activeField != 0 {
		t.Fatalf("expected activeField 0 after wrap, got %d", b.activeField)
	}
}

// TestShiftTabCyclesBackward checks that Shift+Tab goes backward across groups.
func TestShiftTabCyclesBackward(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	b := NewBubble(m, testProviders)
	// activeGroup=0 (expanded), groups 1&2 collapsed; shift+tab from field 0 → group 2 (collapsed)
	b = pressKey(b, tea.KeyShiftTab)
	if b.activeGroup != 2 {
		t.Fatalf("expected activeGroup=2 after shift+tab from group 0 field 0, got %d", b.activeGroup)
	}
	if b.activeField != 0 {
		t.Fatalf("expected activeField=0 (collapsed group), got %d", b.activeField)
	}
}

// TestStepNavigationResetsActiveField checks Up/Down resets group and field.
func TestStepNavigationResetsActiveField(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	m.AddStep(pipeline.Step{ID: "s2", Executor: "gemini"})
	b := NewBubble(m, testProviders)
	b = pressKey(b, tea.KeyTab) // activeField = 1
	b = pressKey(b, tea.KeyDown)
	if b.activeField != 0 {
		t.Fatalf("expected activeField 0 after step nav, got %d", b.activeField)
	}
	if b.activeGroup != 0 {
		t.Fatalf("expected activeGroup 0 after step nav, got %d", b.activeGroup)
	}
	if m.SelectedIndex() != 1 {
		t.Fatalf("expected selectedIndex 1 after down, got %d", m.SelectedIndex())
	}
}

// TestStepNavigationSyncsExecutor checks that navigating steps syncs the executor dropdown.
func TestStepNavigationSyncsExecutor(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	m.AddStep(pipeline.Step{ID: "s2", Executor: "gemini"})
	b := NewBubble(m, testProviders)
	// Initially on s1 (claude)
	if b.executorDD.Value() != "claude" {
		t.Fatalf("expected executor=claude for s1, got %q", b.executorDD.Value())
	}
	// Navigate to s2 (gemini)
	b = pressKey(b, tea.KeyDown)
	if b.executorDD.Value() != "gemini" {
		t.Fatalf("expected executor=gemini for s2, got %q", b.executorDD.Value())
	}
}

// TestTabNoOpWithNoSteps checks that Tab does nothing when there are no steps.
func TestTabNoOpWithNoSteps(t *testing.T) {
	m := New(nil)
	b := NewBubble(m, testProviders)
	b = pressKey(b, tea.KeyTab)
	if b.activeField != 0 {
		t.Fatalf("expected activeField 0 (no-op) when no steps, got %d", b.activeField)
	}
}

// TestPromptFieldUpdatesStep checks that typing in the prompt field updates the step.
func TestPromptFieldUpdatesStep(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	b := NewBubble(m, testProviders)

	// Tab twice to reach Prompt field (0→1→2) in Core group
	b = pressKey(b, tea.KeyTab)
	b = pressKey(b, tea.KeyTab)
	if b.activeField != 2 {
		t.Fatalf("expected activeField 2, got %d", b.activeField)
	}

	// Type "hello"
	for _, r := range "hello" {
		mm, _ := b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		b = mm.(*BubbleModel)
	}
	if m.Steps()[0].Prompt != "hello" {
		t.Fatalf("expected Prompt 'hello', got %q", m.Steps()[0].Prompt)
	}
}

// TestPromptFieldAllowsActionKeys checks that action-bound runes ('s', 'q', etc.)
// are forwarded to the text input, not intercepted as actions.
func TestPromptFieldAllowsActionKeys(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	b := NewBubble(m, testProviders)

	// Tab twice to reach Prompt field
	b = pressKey(b, tea.KeyTab)
	b = pressKey(b, tea.KeyTab)

	for _, r := range "save+rqkj" {
		mm, _ := b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		b = mm.(*BubbleModel)
	}
	if m.Steps()[0].Prompt != "save+rqkj" {
		t.Fatalf("expected Prompt 'save+rqkj', got %q", m.Steps()[0].Prompt)
	}
	// activeField must still be 2
	if b.activeField != 2 {
		t.Fatalf("expected activeField 2, got %d", b.activeField)
	}
}

// TestAddStepAddsStep checks that pressing '+' adds a new step.
func TestAddStepAddsStep(t *testing.T) {
	m := New(nil)
	b := NewBubble(m, testProviders)
	// activeGroup=0, activeField=0 (executor), no steps: '+' should add a step
	b = pressRune(b, '+')
	if len(m.Steps()) != 1 {
		t.Fatalf("expected 1 step after '+', got %d", len(m.Steps()))
	}
}

// TestExecutorDropdownOpen checks that Enter opens the executor dropdown.
func TestExecutorDropdownOpen(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	b := NewBubble(m, testProviders)
	// activeGroup=0, activeField=0 (executorDD) — press Enter
	b = pressKey(b, tea.KeyEnter)
	if !b.executorDD.IsOpen() {
		t.Fatal("expected executorDD to be open after Enter on field 0")
	}
}

// TestExecutorDropdownClose checks that Esc closes and reverts the dropdown.
func TestExecutorDropdownClose(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	b := NewBubble(m, testProviders)
	b = pressKey(b, tea.KeyEnter) // open
	b = pressKey(b, tea.KeyEsc)   // close (but Esc also quits — need to check order)
	// Esc when a dropdown is open should close the dropdown, not quit
	// (because anyDropdownOpen() is true, routeToOpenDropdown handles it)
	if b.executorDD.IsOpen() {
		t.Fatal("expected executorDD closed after Esc")
	}
}

// TestModelEnabledForProvider checks that modelEnabled is true for a provider executor.
func TestModelEnabledForProvider(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	b := NewBubble(m, testProviders)
	if !b.modelEnabled {
		t.Fatal("expected modelEnabled=true for provider executor")
	}
}

// TestModelDisabledForBuiltin checks that modelEnabled is false for a builtin executor.
func TestModelDisabledForBuiltin(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "builtin.log"})
	b := NewBubble(m, testProviders)
	if b.modelEnabled {
		t.Fatal("expected modelEnabled=false for builtin executor")
	}
}

// TestBuiltinPrePopulatesArgs checks that builtin args are pre-populated.
func TestBuiltinPrePopulatesArgs(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "builtin.log"})
	b := NewBubble(m, testProviders)
	if len(b.argsRows) == 0 {
		t.Fatal("expected argsRows pre-populated for builtin.log")
	}
	if b.argsRows[0].key != "message" {
		t.Fatalf("expected first arg key 'message', got %q", b.argsRows[0].key)
	}
}

// TestGroupExpandedInitialState checks that groupExpanded starts as {true, false, false}.
func TestGroupExpandedInitialState(t *testing.T) {
	m := New(nil)
	b := NewBubble(m, testProviders)
	if !b.groupExpanded[0] {
		t.Fatal("expected groupExpanded[0]=true initially")
	}
	if b.groupExpanded[1] {
		t.Fatal("expected groupExpanded[1]=false initially")
	}
	if b.groupExpanded[2] {
		t.Fatal("expected groupExpanded[2]=false initially")
	}
}

// TestAccordionExpandWithRightArrow checks that pressing → expands the active group.
func TestAccordionExpandWithRightArrow(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	b := NewBubble(m, testProviders)

	// activeGroup=0 is already expanded; collapse it first with ←
	b = pressKey(b, tea.KeyLeft)
	if b.groupExpanded[0] {
		t.Fatal("expected groupExpanded[0]=false after ← on group 0")
	}

	// now expand with →
	b = pressKey(b, tea.KeyRight)
	if !b.groupExpanded[0] {
		t.Fatal("expected groupExpanded[0]=true after → on group 0")
	}
}

// TestAccordionCollapseWithLeftArrow checks that pressing ← collapses the active group.
func TestAccordionCollapseWithLeftArrow(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	b := NewBubble(m, testProviders)

	// group 0 starts expanded; collapse with ←
	b = pressKey(b, tea.KeyLeft)
	if b.groupExpanded[0] {
		t.Fatal("expected groupExpanded[0]=false after ← on group 0")
	}
}

// TestTabCyclesIntoCollapsedGroups checks that Tab advances activeGroup even into collapsed groups.
func TestTabCyclesIntoCollapsedGroups(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	b := NewBubble(m, testProviders)

	// Only group 0 is expanded. Tab past last field should move to group 1 (collapsed).
	count := b.groupFieldCount(0) // 3 (executor, model, prompt) for claude
	for i := 0; i < count-1; i++ {
		b = pressKey(b, tea.KeyTab)
	}
	// Now at last field of group 0
	if b.activeField != count-1 {
		t.Fatalf("expected activeField=%d, got %d", count-1, b.activeField)
	}
	// Tab once more — should advance to group 1 (collapsed)
	b = pressKey(b, tea.KeyTab)
	if b.activeGroup != 1 {
		t.Fatalf("expected activeGroup=1 after tab past last field of group 0, got %d", b.activeGroup)
	}
	if b.activeField != 0 {
		t.Fatalf("expected activeField=0, got %d", b.activeField)
	}
}

// TestTabMovesIntoExpandedGroup checks that Tab moves into a newly expanded group.
func TestTabMovesIntoExpandedGroup(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	b := NewBubble(m, testProviders)

	// Expand group 1 as well
	b.groupExpanded[1] = true

	// Tab through all of group 0's fields
	count0 := b.groupFieldCount(0)
	for i := 0; i < count0; i++ {
		b = pressKey(b, tea.KeyTab)
	}
	// Should now be in group 1, field 0
	if b.activeGroup != 1 {
		t.Fatalf("expected activeGroup=1 after tabbing through group 0, got %d", b.activeGroup)
	}
	if b.activeField != 0 {
		t.Fatalf("expected activeField=0 at start of group 1, got %d", b.activeField)
	}
}

// TestNeedsMultiSelectSync checks that needs are synced from step.
func TestNeedsMultiSelectSync(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	m.AddStep(pipeline.Step{ID: "s2", Executor: "claude", Needs: []string{"s1"}})
	m.SelectStep(1)
	b := NewBubble(m, testProviders)
	// Synced from s2: needs should include "s1"
	needs := b.needsDD.Selected()
	if len(needs) != 1 || needs[0] != "s1" {
		t.Fatalf("expected needs=[s1], got %v", needs)
	}
}

// TestFocusPaneInitial checks that focusPane starts at 0.
func TestFocusPaneInitial(t *testing.T) {
	m := New(nil)
	b := NewBubble(m, testProviders)
	if b.focusPane != 0 {
		t.Fatalf("expected focusPane=0 initially, got %d", b.focusPane)
	}
}

// TestRightPaneScrollAutoAdvance checks that Tab triggers rightScroll update when
// the terminal is sized to force scrolling.
func TestRightPaneScrollAutoAdvance(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Executor: "claude"})
	b := NewBubble(m, testProviders)
	// Set a small terminal to force scrolling
	b.width = 80
	b.height = 10 // paneH = 10-5 = 5

	// Expand all groups so there are many fields
	b.groupExpanded[0] = true
	b.groupExpanded[1] = true
	b.groupExpanded[2] = true

	initialScroll := b.rightScroll
	// Tab through enough fields to push scroll
	for i := 0; i < 20; i++ {
		b = pressKey(b, tea.KeyTab)
	}
	// rightScroll should have advanced at some point
	if b.rightScroll == initialScroll && b.activeGroup > 0 {
		t.Fatal("expected rightScroll to advance when tabbing through many fields")
	}
}
