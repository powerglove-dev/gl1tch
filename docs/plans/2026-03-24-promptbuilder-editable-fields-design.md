# Prompt Builder Editable Fields — Design

## Goal

Make the pipeline builder TUI interactive: editable Plugin (cycle selector), Model (cycle selector), and Prompt (free-text input) fields per step, with an empty-canvas default and helpful onboarding message.

## Default State

Builder opens with zero steps. Right pane shows a centered help message:

> Press [+] to add your first step.
> Each step needs a provider — use ←→ to cycle plugins, Tab to move between fields.

`run.go` seeds no steps — only `SetName("new-pipeline")`.

## Field Editing

When a step is selected, Tab/Shift+Tab cycles focus through 3 fields:

| Field  | Type           | Interaction                                      |
|--------|----------------|--------------------------------------------------|
| Plugin | cycle selector | ←/→ cycles: claude → gemini → openspec → openclaw |
| Model  | cycle selector | ←/→ cycles models for the current plugin         |
| Prompt | text input     | typing goes directly into a bubbles textinput    |

Active selectors render as `◀ value ▶`. Prompt field shows a blinking cursor.

Model lists per plugin:
- **claude**: claude-sonnet-4-6, claude-opus-4-6, claude-haiku-4-5-20251001
- **gemini**: gemini-2.0-flash, gemini-1.5-pro
- **openspec** / **openclaw**: no predefined models (blank)

`↑↓` navigates steps regardless of focused field (no conflict with text input).
`+` adds an empty step and auto-focuses the Plugin field.
Changing Plugin resets Model to index 0 for the new plugin.

## State Changes

**`BubbleModel` new fields:**
- `activeField int` — 0=Plugin, 1=Model, 2=Prompt; resets to 0 on step change
- `promptInput textinput.Model` — synced to step.Prompt on every keystroke

**`Update()` additions:**
- Tab → `activeField = (activeField+1) % 3`
- Shift+Tab → `activeField = (activeField+2) % 3`
- ←/→ when Plugin focused → cycle plugin list, reset Model index
- ←/→ when Model focused → cycle model list for current plugin
- Keys when Prompt focused → forward to textinput, sync to step.Prompt
- ↑↓ → navigate steps, reset activeField to 0

**`view.go` changes:**
- Empty canvas message when no steps
- Active field highlighted: selectors show `◀ value ▶`, prompt shows cursor
- Footer: `[+] add  [←→] cycle  [tab] next field  [↑↓] steps  [s] save  [esc] quit`

**`run.go` change:**
- Remove all `AddStep` calls
