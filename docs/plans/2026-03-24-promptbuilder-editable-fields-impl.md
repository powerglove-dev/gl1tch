# Prompt Builder Editable Fields Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make Plugin (cycle selector), Model (cycle selector), and Prompt (free-text) fields interactively editable per step, with an empty-canvas default state and helpful onboarding message.

**Architecture:** Add `UpdateStep` to Model for mutating individual steps. Add `activeField`, `pluginIndex`, `modelIndex`, and `promptInput` to BubbleModel. Handle Tab/ShiftTab/Left/Right in Update(); forward remaining keys to textinput when Prompt is focused. Update view to render empty-canvas help and active-field highlights.

**Tech Stack:** Go, BubbleTea (`github.com/charmbracelet/bubbletea`), bubbles key + textinput (`github.com/charmbracelet/bubbles`)

---

### Task 1: Add UpdateStep to Model

**Files:**
- Modify: `internal/promptbuilder/model.go`
- Test: `internal/promptbuilder/model_test.go`

Note: existing tests use package `promptbuilder_test` (external). Continue that pattern.

**Step 1: Write the failing test**

Add to `internal/promptbuilder/model_test.go`:

```go
func TestModel_UpdateStep(t *testing.T) {
	m := promptbuilder.New(nil)
	m.AddStep(pipeline.Step{ID: "a", Plugin: "claude"})
	m.UpdateStep(0, pipeline.Step{ID: "a", Plugin: "gemini", Model: "gemini-2.0-flash"})
	if m.Steps()[0].Plugin != "gemini" {
		t.Fatalf("expected gemini, got %s", m.Steps()[0].Plugin)
	}
	if m.Steps()[0].Model != "gemini-2.0-flash" {
		t.Fatalf("expected gemini-2.0-flash, got %s", m.Steps()[0].Model)
	}
}

func TestModel_UpdateStep_OutOfRange(t *testing.T) {
	m := promptbuilder.New(nil)
	m.AddStep(pipeline.Step{ID: "a", Plugin: "claude"})
	m.UpdateStep(99, pipeline.Step{ID: "x"}) // should be a no-op
	if len(m.Steps()) != 1 {
		t.Fatalf("expected 1 step, got %d", len(m.Steps()))
	}
}
```

**Step 2: Run to verify failure**

```bash
cd /Users/stokes/Projects/orcai
go test ./internal/promptbuilder/ -run TestModel_UpdateStep -v
```

Expected: FAIL — `m.UpdateStep undefined`

**Step 3: Implement UpdateStep**

Add to `internal/promptbuilder/model.go`:

```go
// UpdateStep replaces the step at index i. No-op if i is out of range.
func (m *Model) UpdateStep(i int, s pipeline.Step) {
	if i < 0 || i >= len(m.steps) {
		return
	}
	m.steps[i] = s
}
```

**Step 4: Run to verify pass**

```bash
go test ./internal/promptbuilder/ -run TestModel_UpdateStep -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/promptbuilder/model.go internal/promptbuilder/model_test.go
git commit -m "feat(promptbuilder): add UpdateStep to Model"
```

---

### Task 2: Add Left, Right, ShiftTab key bindings

**Files:**
- Modify: `internal/promptbuilder/keys.go`

**Step 1: Replace the keyMap struct and keys var**

Replace the entire content of `internal/promptbuilder/keys.go` with:

```go
package promptbuilder

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Tab      key.Binding
	ShiftTab key.Binding
	Left     key.Binding
	Right    key.Binding
	AddStep  key.Binding
	Run      key.Binding
	Save     key.Binding
	Quit     key.Binding
	Help     key.Binding
}

var keys = keyMap{
	Up:       key.NewBinding(key.WithKeys("up", "k"),    key.WithHelp("↑/k", "prev step")),
	Down:     key.NewBinding(key.WithKeys("down", "j"),  key.WithHelp("↓/j", "next step")),
	Tab:      key.NewBinding(key.WithKeys("tab"),         key.WithHelp("tab", "next field")),
	ShiftTab: key.NewBinding(key.WithKeys("shift+tab"),  key.WithHelp("shift+tab", "prev field")),
	Left:     key.NewBinding(key.WithKeys("left"),        key.WithHelp("←", "prev value")),
	Right:    key.NewBinding(key.WithKeys("right"),       key.WithHelp("→", "next value")),
	AddStep:  key.NewBinding(key.WithKeys("+"),           key.WithHelp("+", "add step")),
	Run:      key.NewBinding(key.WithKeys("r"),           key.WithHelp("r", "run")),
	Save:     key.NewBinding(key.WithKeys("s"),           key.WithHelp("s", "save")),
	Quit:     key.NewBinding(key.WithKeys("esc", "q"),   key.WithHelp("esc", "quit")),
	Help:     key.NewBinding(key.WithKeys("?"),           key.WithHelp("?", "help")),
}
```

**Step 2: Build**

```bash
go build ./...
```

Expected: no errors

**Step 3: Commit**

```bash
git add internal/promptbuilder/keys.go
git commit -m "feat(promptbuilder): add Left, Right, ShiftTab key bindings"
```

---

### Task 3: Add cycling state + Tab/←→ navigation to BubbleModel

**Files:**
- Modify: `internal/promptbuilder/view.go`
- Create: `internal/promptbuilder/bubble_test.go`

Note: `bubble_test.go` uses `package promptbuilder` (internal) to access private fields `activeField`, `pluginIndex`, `modelIndex`.

**Step 1: Write failing tests**

Create `internal/promptbuilder/bubble_test.go`:

```go
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
	// claude is index 0, left wraps to last
	if got != pluginList[len(pluginList)-1] {
		t.Fatalf("expected %s after left from claude, got %s", pluginList[len(pluginList)-1], got)
	}
}

func TestRightCyclesModel(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude", Model: "claude-sonnet-4-6"})
	b := NewBubble(m)
	b = pressKey(b, tea.KeyTab) // focus Model field
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

func TestTabNoOpWithNoSteps(t *testing.T) {
	m := New(nil)
	b := NewBubble(m)
	b = pressKey(b, tea.KeyTab)
	if b.activeField != 0 {
		t.Fatalf("expected activeField 0 (no-op) when no steps, got %d", b.activeField)
	}
}
```

**Step 2: Run to verify failure**

```bash
go test ./internal/promptbuilder/ -run "TestTab|TestShiftTab|TestRight|TestLeft|TestStep" -v
```

Expected: FAIL — `activeField`, `pluginList`, `modelsByPlugin` undefined

**Step 3: Add plugin/model constants and cycling state to view.go**

At the top of `internal/promptbuilder/view.go`, after the `import` block, add:

```go
var pluginList = []string{"claude", "gemini", "openspec", "openclaw"}

var modelsByPlugin = map[string][]string{
	"claude":   {"claude-sonnet-4-6", "claude-opus-4-6", "claude-haiku-4-5-20251001"},
	"gemini":   {"gemini-2.0-flash", "gemini-1.5-pro"},
	"openspec": {},
	"openclaw": {},
}
```

Update `BubbleModel` struct:

```go
type BubbleModel struct {
	inner       *Model
	width       int
	height      int
	activeField int // 0=Plugin 1=Model 2=Prompt
	pluginIndex int
	modelIndex  int
}
```

Add these helper methods after `NewBubble`:

```go
// syncIndicesFromStep sets pluginIndex/modelIndex to match the selected step's current values.
func (b *BubbleModel) syncIndicesFromStep() {
	steps := b.inner.Steps()
	if len(steps) == 0 {
		return
	}
	sel := steps[b.inner.SelectedIndex()]
	for i, p := range pluginList {
		if p == sel.Plugin {
			b.pluginIndex = i
			break
		}
	}
	models := modelsByPlugin[pluginList[b.pluginIndex]]
	for i, mo := range models {
		if mo == sel.Model {
			b.modelIndex = i
			break
		}
	}
}

// applyPlugin writes pluginList[pluginIndex] to the selected step and resets model.
func (b *BubbleModel) applyPlugin() {
	steps := b.inner.Steps()
	if len(steps) == 0 {
		return
	}
	idx := b.inner.SelectedIndex()
	s := steps[idx]
	s.Plugin = pluginList[b.pluginIndex]
	b.modelIndex = 0
	models := modelsByPlugin[s.Plugin]
	if len(models) > 0 {
		s.Model = models[0]
	} else {
		s.Model = ""
	}
	b.inner.UpdateStep(idx, s)
}

// applyModel writes modelsByPlugin[plugin][modelIndex] to the selected step.
func (b *BubbleModel) applyModel() {
	steps := b.inner.Steps()
	if len(steps) == 0 {
		return
	}
	idx := b.inner.SelectedIndex()
	s := steps[idx]
	models := modelsByPlugin[pluginList[b.pluginIndex]]
	if len(models) > 0 {
		s.Model = models[b.modelIndex]
	}
	b.inner.UpdateStep(idx, s)
}
```

Replace the `Update()` method body's `tea.KeyMsg` case entirely:

```go
case tea.KeyMsg:
	switch {
	case key.Matches(msg, keys.Quit):
		return b, tea.Quit

	case key.Matches(msg, keys.Tab):
		if len(b.inner.Steps()) > 0 {
			b.activeField = (b.activeField + 1) % 3
		}

	case key.Matches(msg, keys.ShiftTab):
		if len(b.inner.Steps()) > 0 {
			b.activeField = (b.activeField + 2) % 3
		}

	case key.Matches(msg, keys.Up):
		b.inner.SelectStep(b.inner.SelectedIndex() - 1)
		b.activeField = 0
		b.syncIndicesFromStep()

	case key.Matches(msg, keys.Down):
		b.inner.SelectStep(b.inner.SelectedIndex() + 1)
		b.activeField = 0
		b.syncIndicesFromStep()

	case key.Matches(msg, keys.Left):
		if len(b.inner.Steps()) == 0 {
			break
		}
		switch b.activeField {
		case 0:
			b.pluginIndex = (b.pluginIndex - 1 + len(pluginList)) % len(pluginList)
			b.applyPlugin()
		case 1:
			models := modelsByPlugin[pluginList[b.pluginIndex]]
			if len(models) > 0 {
				b.modelIndex = (b.modelIndex - 1 + len(models)) % len(models)
				b.applyModel()
			}
		}

	case key.Matches(msg, keys.Right):
		if len(b.inner.Steps()) == 0 {
			break
		}
		switch b.activeField {
		case 0:
			b.pluginIndex = (b.pluginIndex + 1) % len(pluginList)
			b.applyPlugin()
		case 1:
			models := modelsByPlugin[pluginList[b.pluginIndex]]
			if len(models) > 0 {
				b.modelIndex = (b.modelIndex + 1) % len(models)
				b.applyModel()
			}
		}

	case key.Matches(msg, keys.AddStep):
		id := fmt.Sprintf("step%d", len(b.inner.Steps())+1)
		b.inner.AddStep(pipeline.Step{ID: id, Plugin: pluginList[0]})
		b.inner.SelectStep(len(b.inner.Steps()) - 1)
		b.activeField = 0
		b.pluginIndex = 0
		b.modelIndex = 0

	case key.Matches(msg, keys.Save):
		home, err := os.UserHomeDir()
		if err == nil {
			dir := filepath.Join(home, ".config", "orcai", "pipelines")
			os.MkdirAll(dir, 0o755) //nolint:errcheck
			path := filepath.Join(dir, b.inner.Name()+".pipeline.yaml")
			Save(b.inner, path) //nolint:errcheck
		}
	}
```

**Step 4: Run tests**

```bash
go test ./internal/promptbuilder/ -run "TestTab|TestShiftTab|TestRight|TestLeft|TestStep" -v
```

Expected: PASS

**Step 5: Run all tests**

```bash
go test ./internal/promptbuilder/ -v
```

Expected: all PASS

**Step 6: Commit**

```bash
git add internal/promptbuilder/view.go internal/promptbuilder/bubble_test.go
git commit -m "feat(promptbuilder): add cycling state and field navigation"
```

---

### Task 4: Add Prompt textinput

**Files:**
- Modify: `internal/promptbuilder/view.go`
- Modify: `internal/promptbuilder/bubble_test.go`

**Step 1: Write failing test**

Add to `internal/promptbuilder/bubble_test.go`:

```go
func TestPromptFieldUpdatesStep(t *testing.T) {
	m := New(nil)
	m.AddStep(pipeline.Step{ID: "s1", Plugin: "claude"})
	b := NewBubble(m)

	// Tab twice to reach Prompt field (0→1→2)
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
```

**Step 2: Run to verify failure**

```bash
go test ./internal/promptbuilder/ -run TestPromptField -v
```

Expected: FAIL — typing does nothing, Prompt stays empty

**Step 3: Add promptInput to BubbleModel**

Add `"github.com/charmbracelet/bubbles/textinput"` to the import block in `view.go`.

Add field to `BubbleModel`:

```go
type BubbleModel struct {
	inner       *Model
	width       int
	height      int
	activeField int
	pluginIndex int
	modelIndex  int
	promptInput textinput.Model
}
```

Update `NewBubble` to initialize it:

```go
func NewBubble(m *Model) *BubbleModel {
	ti := textinput.New()
	ti.Placeholder = "enter prompt..."
	return &BubbleModel{inner: m, promptInput: ti}
}
```

In `Update()`, add a `default` case at the end of the `tea.KeyMsg` switch to forward keys to promptInput when the Prompt field is active:

```go
default:
	if b.activeField == 2 && len(b.inner.Steps()) > 0 {
		var cmd tea.Cmd
		b.promptInput, cmd = b.promptInput.Update(msg)
		idx := b.inner.SelectedIndex()
		s := b.inner.Steps()[idx]
		s.Prompt = b.promptInput.Value()
		b.inner.UpdateStep(idx, s)
		return b, cmd
	}
```

Also, sync the textinput value whenever the active field changes to 2. In the `keys.Tab` and `keys.ShiftTab` cases, after updating `activeField`, add:

```go
if b.activeField == 2 {
	b.promptInput.SetValue(b.inner.Steps()[b.inner.SelectedIndex()].Prompt)
	b.promptInput.Focus()
} else {
	b.promptInput.Blur()
}
```

Apply the same `promptInput.Blur()` call in the `keys.Up` and `keys.Down` cases (activeField resets to 0, so always blur):

```go
case key.Matches(msg, keys.Up):
	b.inner.SelectStep(b.inner.SelectedIndex() - 1)
	b.activeField = 0
	b.promptInput.Blur()
	b.syncIndicesFromStep()

case key.Matches(msg, keys.Down):
	b.inner.SelectStep(b.inner.SelectedIndex() + 1)
	b.activeField = 0
	b.promptInput.Blur()
	b.syncIndicesFromStep()
```

**Step 4: Run test**

```bash
go test ./internal/promptbuilder/ -run TestPromptField -v
```

Expected: PASS

**Step 5: Run all tests**

```bash
go test ./internal/promptbuilder/ -v
```

Expected: all PASS

**Step 6: Commit**

```bash
git add internal/promptbuilder/view.go internal/promptbuilder/bubble_test.go
git commit -m "feat(promptbuilder): add Prompt textinput with step sync"
```

---

### Task 5: Update view rendering

**Files:**
- Modify: `internal/promptbuilder/view.go`

No new tests — view rendering is visual; verified by building and running.

**Step 1: Add renderSelector helper**

Add after the style vars at the top of `view.go`:

```go
// renderSelector renders a cycle-selector field. Active fields show ◀ value ▶.
func (b *BubbleModel) renderSelector(label, value string, fieldIdx int) string {
	l := labelStyle.Render(label)
	if b.activeField == fieldIdx && len(b.inner.Steps()) > 0 {
		return l + selectedStep.Render("◀ "+value+" ▶") + "\n"
	}
	return l + value + "\n"
}
```

**Step 2: Update right pane in View()**

Replace the static right-pane field rendering block:

```go
// OLD — remove this:
rightContent += labelStyle.Render("ID:      ") + sel.ID + "\n"
rightContent += labelStyle.Render("Plugin:  ") + sel.Plugin + "\n"
rightContent += labelStyle.Render("Model:   ") + sel.Model + "\n"
rightContent += labelStyle.Render("Prompt:  ") + sel.Prompt + "\n"
```

With:

```go
// NEW
rightContent += labelStyle.Render("ID:      ") + sel.ID + "\n"

pluginVal := pluginList[b.pluginIndex]
rightContent += b.renderSelector("Plugin:  ", pluginVal, 0)

modelVal := sel.Model
if modelVal == "" {
	modelVal = "(none)"
}
rightContent += b.renderSelector("Model:   ", modelVal, 1)

if b.activeField == 2 {
	rightContent += labelStyle.Render("Prompt:  ") + b.promptInput.View() + "\n"
} else {
	rightContent += labelStyle.Render("Prompt:  ") + sel.Prompt + "\n"
}
```

**Step 3: Add empty-canvas message**

In `View()`, find the block that sets `rightContent` when `len(steps) > 0`. Before that block, add an else branch:

```go
if len(steps) > 0 {
	// ... existing code ...
} else {
	rightContent = "\n\n" +
		dimStep.Render("  No steps yet.") + "\n\n" +
		dimStep.Render("  Press [+] to add your first step.") + "\n" +
		dimStep.Render("  Each step requires a provider (Plugin).") + "\n\n" +
		dimStep.Render("  Once a step is selected:") + "\n" +
		dimStep.Render("    [←→]  cycle Plugin or Model") + "\n" +
		dimStep.Render("    [tab] move between fields") + "\n" +
		dimStep.Render("    type  enter a Prompt")
}
```

**Step 4: Update footer**

Replace:
```go
footer := statusBar.Render("[r] run  [s] save  [tab] next field  [↑↓] steps  [esc] quit")
```

With:
```go
footer := statusBar.Render("[+] add  [←→] cycle  [tab] next field  [↑↓] steps  [s] save  [esc] quit")
```

**Step 5: Build**

```bash
go build ./...
```

Expected: no errors

**Step 6: Run all tests**

```bash
go test ./internal/promptbuilder/ -v
```

Expected: all PASS

**Step 7: Commit**

```bash
git add internal/promptbuilder/view.go
git commit -m "feat(promptbuilder): add field highlighting, cycle rendering, empty canvas"
```

---

### Task 6: Simplify run.go — empty canvas default

**Files:**
- Modify: `internal/promptbuilder/run.go`

**Step 1: Remove AddStep calls**

Replace the body of `Run()` with (keep plugin registration, remove all AddStep calls):

```go
func Run() {
	mgr := plugin.NewManager()
	for _, name := range []string{"claude", "gemini", "openspec", "openclaw"} {
		mgr.Register(plugin.NewCliAdapter(name, name+" CLI adapter", name))
	}

	m := New(mgr)
	m.SetName("new-pipeline")

	bubble := NewBubble(m)
	p := tea.NewProgram(bubble, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("prompt builder error: %v\n", err)
	}
}
```

**Step 2: Build**

```bash
go build ./...
```

Expected: no errors

**Step 3: Run all tests**

```bash
go test ./...
```

Expected: all PASS

**Step 4: Commit**

```bash
git add internal/promptbuilder/run.go
git commit -m "feat(promptbuilder): start with empty canvas, no default steps"
```
