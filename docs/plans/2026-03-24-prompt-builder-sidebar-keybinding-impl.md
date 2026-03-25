# Prompt Builder Sidebar Keybinding Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `p` keybinding to the sidebar that opens the prompt builder TUI in a new tmux window named `prompt-builder`.

**Architecture:** Add a `Run()` function to the promptbuilder package, wire it into main.go's `_`-prefixed dispatch, and add the `p` case + footer update to the sidebar.

**Tech Stack:** Go, BubbleTea, tmux, Cobra

---

### Task 1: Add `Run()` to promptbuilder package

**Files:**
- Create: `internal/promptbuilder/run.go`

**Step 1: Write the file**

```go
package promptbuilder

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/adam-stokes/orcai/internal/pipeline"
	"github.com/adam-stokes/orcai/internal/plugin"
)

// Run launches the prompt builder as a standalone BubbleTea program.
func Run() {
	mgr := plugin.NewManager()
	for _, name := range []string{"claude", "gemini", "openspec", "openclaw"} {
		mgr.Register(plugin.NewCliAdapter(name, name+" CLI adapter", name))
	}

	m := New(mgr)
	m.SetName("new-pipeline")
	m.AddStep(pipeline.Step{ID: "input", Type: "input", Prompt: "Enter your prompt:"})
	m.AddStep(pipeline.Step{ID: "step1", Plugin: "claude", Model: "claude-sonnet-4-6"})
	m.AddStep(pipeline.Step{ID: "output", Type: "output"})

	bubble := NewBubble(m)
	p := tea.NewProgram(bubble, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("prompt builder error: %v\n", err)
	}
}
```

**Step 2: Build to verify it compiles**

Run: `go build ./...`
Expected: no errors

**Step 3: Commit**

```bash
git add internal/promptbuilder/run.go
git commit -m "feat(promptbuilder): add Run() for standalone launch"
```

---

### Task 2: Wire `_promptbuilder` into main.go dispatch

**Files:**
- Modify: `main.go`

**Step 1: Add the import and case**

In `main.go`, add `"github.com/adam-stokes/orcai/internal/promptbuilder"` to imports (already imported via cmd indirectly — add direct import).

Add to the switch in `main()`:

```go
case "_promptbuilder":
    promptbuilder.Run()
    return
```

Place it after the `case "_picker":` block.

Also add `"_promptbuilder"` is handled by the existing fall-through — no change needed to the `"bridge", "git", ...` list since `_promptbuilder` starts with `_` and matches the new explicit case.

**Step 2: Build to verify**

Run: `go build ./...`
Expected: no errors

**Step 3: Commit**

```bash
git add main.go
git commit -m "feat(main): dispatch _promptbuilder to promptbuilder.Run"
```

---

### Task 3: Add `p` keybinding and update footer in sidebar

**Files:**
- Modify: `internal/sidebar/sidebar.go`

**Step 1: Add the `p` case in `Update()`**

In the `tea.KeyMsg` switch inside `Update()`, add after the `case "n":` block:

```go
case "p":
    if m.self != "" {
        exec.Command("tmux", "new-window", "-t", "orcai",
            "-n", "prompt-builder", m.self, "_promptbuilder").Run() //nolint:errcheck
    }
```

**Step 2: Update the footer constant**

Change:
```go
const footerText = "n new  x kill  ↑↓ nav"
```
To:
```go
const footerText = "n new  p build  x kill  ↑↓ nav"
```

**Step 3: Build to verify**

Run: `go build ./...`
Expected: no errors

**Step 4: Commit**

```bash
git add internal/sidebar/sidebar.go
git commit -m "feat(sidebar): add p keybinding to open prompt builder in new window"
```
