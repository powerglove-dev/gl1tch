# Prompt Builder Provider List Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the prompt builder's hardcoded plugin/model lists with the canonical picker provider list, giving it runtime-discovered providers (opencode, ollama local models) identical to the session picker.

**Architecture:** Export `picker.BuildProviders()` (wrapper around existing `buildProviders()`, shell-filtered). Update `BubbleModel` to hold `[]picker.ProviderDef` instead of the static `pluginList`/`modelsByPlugin` vars. Update `run.go` to call `BuildProviders()` and pass the result into `NewBubble()`.

**Tech Stack:** Go, BubbleTea, `internal/picker` (ProviderDef, ModelOption), `internal/promptbuilder`

---

### Task 1: Export BuildProviders from picker

**Files:**
- Modify: `internal/picker/picker.go`
- Test: `internal/picker/picker_test.go`

Note: picker tests use `package picker_test` (external). Continue that pattern.

**Step 1: Write the failing test**

Add to `internal/picker/picker_test.go`:

```go
func TestBuildProviders_ExcludesShell(t *testing.T) {
	providers := picker.BuildProviders()
	for _, p := range providers {
		if p.ID == "shell" {
			t.Fatal("BuildProviders must not include the shell provider")
		}
	}
}
```

**Step 2: Run to verify failure**

```bash
cd /Users/stokes/Projects/orcai
go test ./internal/picker/ -run TestBuildProviders_ExcludesShell -v
```

Expected: FAIL — `picker.BuildProviders undefined`

**Step 3: Add BuildProviders to picker.go**

Add after the closing brace of `buildProviders()` (around line 153):

```go
// BuildProviders returns the runtime-filtered, model-enriched provider list,
// excluding the shell provider (not relevant for pipeline steps).
// Behaviour is identical to the session picker: filters by installed CLI,
// injects Ollama models into ollama/opencode, creates ctx32k variants,
// and writes the opencode config when applicable.
func BuildProviders() []ProviderDef {
	all := buildProviders()
	out := make([]ProviderDef, 0, len(all))
	for _, p := range all {
		if p.ID != "shell" {
			out = append(out, p)
		}
	}
	return out
}
```

**Step 4: Run to verify pass**

```bash
go test ./internal/picker/ -run TestBuildProviders_ExcludesShell -v
```

Expected: PASS

**Step 5: Run all picker tests**

```bash
go test ./internal/picker/ -v
```

Expected: all pass

**Step 6: Commit**

```bash
git add internal/picker/picker.go internal/picker/picker_test.go
git commit -m "feat(picker): export BuildProviders for use by prompt builder"
```

---

### Task 2: Update BubbleModel to use []picker.ProviderDef

This is the largest task. Read the full current `internal/promptbuilder/view.go` and `internal/promptbuilder/bubble_test.go` before editing.

**Files:**
- Modify: `internal/promptbuilder/view.go`
- Modify: `internal/promptbuilder/bubble_test.go`

**Step 1: Read current files**

```bash
cat -n internal/promptbuilder/view.go
cat -n internal/promptbuilder/bubble_test.go
```

**Step 2: Write failing test — NewBubble now requires providers**

The existing tests all call `NewBubble(m)` with one argument. After this task they must call `NewBubble(m, testProviders)`. First define `testProviders` at the top of `bubble_test.go` (add after the import block):

```go
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
```

Also add `"github.com/adam-stokes/orcai/internal/picker"` to the imports in `bubble_test.go`.

Update every `NewBubble(m)` call in `bubble_test.go` to `NewBubble(m, testProviders)`.

Update `TestLeftCyclesPluginBackward` — it currently asserts:
```go
if got != pluginList[len(pluginList)-1] {
```
Change to:
```go
if got != testProviders[len(testProviders)-1].ID {
```

**Step 3: Run to verify failure**

```bash
go test ./internal/promptbuilder/ -v
```

Expected: FAIL — `NewBubble` signature mismatch, `pluginList` undefined in test

**Step 4: Update view.go — imports**

Add `"github.com/adam-stokes/orcai/internal/picker"` to the import block in `view.go`.

**Step 5: Update view.go — remove package-level vars, add providers field**

Remove these two package-level vars entirely:

```go
var pluginList = []string{"claude", "gemini", "openspec", "openclaw"}

var modelsByPlugin = map[string][]string{
    "claude":   {"claude-sonnet-4-6", "claude-opus-4-6", "claude-haiku-4-5-20251001"},
    "gemini":   {"gemini-2.0-flash", "gemini-1.5-pro"},
    "openspec": {},
    "openclaw": {},
}
```

Update `BubbleModel` struct to add `providers`:

```go
type BubbleModel struct {
	inner       *Model
	width       int
	height      int
	providers   []picker.ProviderDef
	activeField int // 0=Plugin 1=Model 2=Prompt
	pluginIndex int
	modelIndex  int
	promptInput textinput.Model
}
```

Update `NewBubble` signature and body:

```go
func NewBubble(m *Model, providers []picker.ProviderDef) *BubbleModel {
	ti := textinput.New()
	ti.Placeholder = "enter prompt..."
	return &BubbleModel{inner: m, providers: providers, promptInput: ti}
}
```

**Step 6: Update syncIndicesFromStep**

Replace the current implementation with:

```go
func (b *BubbleModel) syncIndicesFromStep() {
	b.pluginIndex = 0
	b.modelIndex = 0
	steps := b.inner.Steps()
	if len(steps) == 0 || len(b.providers) == 0 {
		return
	}
	sel := steps[b.inner.SelectedIndex()]
	for i, p := range b.providers {
		if p.ID == sel.Plugin {
			b.pluginIndex = i
			break
		}
	}
	for i, mo := range b.providers[b.pluginIndex].Models {
		if !mo.Separator && mo.ID == sel.Model {
			b.modelIndex = i
			break
		}
	}
}
```

**Step 7: Update applyPlugin**

Replace the current implementation with:

```go
func (b *BubbleModel) applyPlugin() {
	steps := b.inner.Steps()
	if len(steps) == 0 || len(b.providers) == 0 {
		return
	}
	idx := b.inner.SelectedIndex()
	s := steps[idx]
	s.Plugin = b.providers[b.pluginIndex].ID
	b.modelIndex = 0
	s.Model = ""
	for _, mo := range b.providers[b.pluginIndex].Models {
		if !mo.Separator {
			s.Model = mo.ID
			break
		}
	}
	b.inner.UpdateStep(idx, s)
}
```

**Step 8: Update applyModel**

Replace the current implementation with:

```go
func (b *BubbleModel) applyModel() {
	steps := b.inner.Steps()
	if len(steps) == 0 || len(b.providers) == 0 {
		return
	}
	idx := b.inner.SelectedIndex()
	s := steps[idx]
	models := b.providers[b.pluginIndex].Models
	if b.modelIndex < len(models) && !models[b.modelIndex].Separator {
		s.Model = models[b.modelIndex].ID
	}
	b.inner.UpdateStep(idx, s)
}
```

**Step 9: Update model cycling in Update() — Left/Right cases**

Replace the `case 0:` and `case 1:` blocks inside `keys.Left` and `keys.Right` with provider-aware cycling that skips separators:

For `keys.Left`:
```go
case key.Matches(msg, keys.Left):
	if len(b.inner.Steps()) == 0 || len(b.providers) == 0 {
		break
	}
	switch b.activeField {
	case 0:
		b.pluginIndex = (b.pluginIndex - 1 + len(b.providers)) % len(b.providers)
		b.applyPlugin()
	case 1:
		models := b.providers[b.pluginIndex].Models
		if len(models) == 0 {
			break
		}
		next := (b.modelIndex - 1 + len(models)) % len(models)
		for models[next].Separator {
			next = (next - 1 + len(models)) % len(models)
		}
		b.modelIndex = next
		b.applyModel()
	}
```

For `keys.Right`:
```go
case key.Matches(msg, keys.Right):
	if len(b.inner.Steps()) == 0 || len(b.providers) == 0 {
		break
	}
	switch b.activeField {
	case 0:
		b.pluginIndex = (b.pluginIndex + 1) % len(b.providers)
		b.applyPlugin()
	case 1:
		models := b.providers[b.pluginIndex].Models
		if len(models) == 0 {
			break
		}
		next := (b.modelIndex + 1) % len(models)
		for models[next].Separator {
			next = (next + 1) % len(models)
		}
		b.modelIndex = next
		b.applyModel()
	}
```

**Step 10: Update AddStep in Update() — use providers[0] as default plugin**

```go
case key.Matches(msg, keys.AddStep):
	id := fmt.Sprintf("step%d", len(b.inner.Steps())+1)
	plugin := ""
	if len(b.providers) > 0 {
		plugin = b.providers[0].ID
	}
	b.inner.AddStep(pipeline.Step{ID: id, Plugin: plugin})
	b.inner.SelectStep(len(b.inner.Steps()) - 1)
	b.activeField = 0
	b.pluginIndex = 0
	b.modelIndex = 0
```

**Step 11: Update View() — Plugin and Model display**

In `View()`, replace:
```go
pluginVal := pluginList[b.pluginIndex]
rightContent += b.renderSelector("Plugin:  ", pluginVal, 0)

modelVal := sel.Model
if modelVal == "" {
	modelVal = "(none)"
}
rightContent += b.renderSelector("Model:   ", modelVal, 1)
```

With:
```go
pluginLabel := ""
if len(b.providers) > 0 && b.pluginIndex < len(b.providers) {
	pluginLabel = b.providers[b.pluginIndex].Label
}
rightContent += b.renderSelector("Plugin:  ", pluginLabel, 0)

modelLabel := "(none)"
if len(b.providers) > 0 && b.pluginIndex < len(b.providers) {
	models := b.providers[b.pluginIndex].Models
	if b.modelIndex < len(models) && !models[b.modelIndex].Separator {
		modelLabel = models[b.modelIndex].Label
	} else if sel.Model != "" {
		modelLabel = sel.Model
	}
}
rightContent += b.renderSelector("Model:   ", modelLabel, 1)
```

**Step 12: Run all tests**

```bash
go test ./internal/promptbuilder/ -v
```

Expected: all pass

**Step 13: Build**

```bash
go build ./...
```

Expected: no errors

**Step 14: Commit**

```bash
git add internal/promptbuilder/view.go internal/promptbuilder/bubble_test.go
git commit -m "feat(promptbuilder): replace static plugin list with picker.ProviderDef"
```

---

### Task 3: Update run.go to use BuildProviders

**Files:**
- Modify: `internal/promptbuilder/run.go`

**Step 1: Replace run.go contents**

Read the current file, then replace with:

```go
package promptbuilder

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/adam-stokes/orcai/internal/picker"
	"github.com/adam-stokes/orcai/internal/plugin"
)

// Run launches the prompt builder as a standalone BubbleTea program.
func Run() {
	providers := picker.BuildProviders()

	mgr := plugin.NewManager()
	for _, p := range providers {
		mgr.Register(plugin.NewCliAdapter(p.ID, p.Label+" CLI adapter", p.ID))
	}

	m := New(mgr)
	m.SetName("new-pipeline")

	bubble := NewBubble(m, providers)
	prog := tea.NewProgram(bubble, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		fmt.Printf("prompt builder error: %v\n", err)
	}
}
```

Note: renamed `p` to `prog` to avoid shadowing the loop variable `p` in the range.

**Step 2: Build**

```bash
go build ./...
```

Expected: no errors

**Step 3: Run all tests**

```bash
go test ./...
```

Expected: all pass

**Step 4: Commit**

```bash
git add internal/promptbuilder/run.go
git commit -m "feat(promptbuilder): drive provider list and adapters from picker.BuildProviders"
```
