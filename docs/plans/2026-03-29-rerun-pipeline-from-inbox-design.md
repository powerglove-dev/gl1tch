# Re-run Pipeline/Agent from Inbox

**Date:** 2026-03-29
**Status:** Approved

## Overview

Add a re-run action to the inbox that opens a reusable modal overlay. The modal lets the user supply optional additional context and optionally change the provider/model before re-executing the selected run.

## Component & Data Model

New file `internal/modal/rerun.go` exports `RerunModal`:

```go
type RerunModal struct {
    run         store.Run
    textarea    textarea.Model   // bubbletea/bubbles textarea
    picker      AgentPickerModel
    focus       rerunFocus       // focusContext | focusPicker
    width, height int
}

type RerunConfirmedMsg struct {
    Run               store.Run
    AdditionalContext string  // empty if not provided
    ProviderID        string
    ModelID           string
}

type RerunCancelledMsg struct{}
```

On init, `picker` is pre-seeded with the provider/model from `run.Metadata` (best-effort fallback to default). The textarea starts empty.

## Layout

Centered overlay via `panelrender.OverlayCenter` with three vertical zones:

```
┌─────────────────────────────────┐
│  RE-RUN: <run name>             │  title bar
├─────────────────────────────────┤
│  Additional context (optional)  │  textarea — focused first
│  > _                            │
├─────────────────────────────────┤
│  PROVIDER          MODEL        │  embedded AgentPickerModel
│  > Claude Code     sonnet-4.6   │
│    Ollama          ...          │
├─────────────────────────────────┤
│  tab focus · enter run · esc cancel │  hint bar
└─────────────────────────────────┘
```

## Navigation

| Key | Context | Action |
|-----|---------|--------|
| `tab` / `shift+tab` | anywhere | cycle focus between textarea and picker |
| normal keys | textarea focused | text input; `enter` inserts newline |
| `j/k`, `tab` | picker focused | existing AgentPickerModel navigation |
| `enter` | picker focused | confirm and submit |
| `esc` | anywhere | cancel |

## Integration & Data Flow

**Switchboard changes** (`internal/switchboard/switchboard.go`):
- Add `rerunModal *modal.RerunModal` and `showRerun bool` to model
- On `r` keypress in inbox with selected item: construct `RerunModal` from `store.Run`, set `showRerun = true`
- Forward all msgs to `rerunModal.Update()` when `showRerun`
- Handle `RerunConfirmedMsg` and `RerunCancelledMsg`
- Render `rerunModal.ViewBox()` as overlay when `showRerun`

**On `RerunConfirmedMsg`**:
1. Build final prompt: `original + "\n\n---\nAdditional context:\n" + additionalContext` (skip suffix if context empty)
2. Call existing pipeline/agent runner with resolved provider, model, and prompt
3. Set `showRerun = false`

**Inbox hint bar**: add `· r re-run` alongside existing hints.

## Scope

- Works for both `pipeline.run.*` and `agent.run.*` inbox items
- Keybinding: `r` from inbox list
- `RerunModal` lives in `internal/modal/` for reuse by other panels
