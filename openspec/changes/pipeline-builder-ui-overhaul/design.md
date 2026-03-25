## Context

The prompt builder (`internal/promptbuilder/`) is a BubbleTea TUI modal launched via `orcai _promptbuilder`. Its current state:

- `BubbleModel` tracks `activeField int` (0=Plugin, 1=Model, 2=Prompt) and cycles with left/right arrows
- `pluginIndex` / `modelIndex` are raw integers — no visual enumeration of choices
- The Step struct has ~15 fields; the builder exposes 3
- `save.go` and `run.go` are clean and unaffected by this change
- The picker package (`internal/picker`) already enumerates installed providers and their models at startup

The plugin system refactor has introduced `executor` + `args` as the primary mechanism (replacing deprecated `plugin` + `vars`), and the DAG runner unlocks `needs`, `retry`, `for_each`, `on_failure`, and `publish_to`. None of these are accessible in the builder today.

## Goals / Non-Goals

**Goals:**
- Replace all cycling selectors with overlay dropdowns using box-drawing chars and the Dracula/ABBS palette
- Expose every meaningful Step field in the editor, grouped to avoid overwhelming the user
- Introduce a reusable `Dropdown` BubbleTea component usable by other widgets in the future
- Keep `save.go` and `run.go` completely untouched

**Non-Goals:**
- Live pipeline execution from within the builder (already handled by `runner.go`)
- Editing pipeline-level `vars` or `max_parallel`
- Drag-and-drop step reordering
- Any changes to the pipeline YAML schema or plugin system

## Decisions

### Inline overlay dropdown, not a separate screen

**Decision**: Dropdowns open as an in-place overlay (a bordered box rendered below the focused field) rather than pushing to a new view/screen.

**Rationale**: BBS aesthetic is spatially anchored — the screen is a single composed surface. Overlays preserve the split-pane layout while showing choices. Full-screen replacement would lose context.

**Alternative considered**: Replacing the right pane content with a full list when selecting. Rejected — disorienting, breaks the mental model of editing a field.

### Single `Dropdown` struct in `internal/promptbuilder/dropdown.go`

**Decision**: Implement the dropdown as a package-local struct (not a separate sub-package). It holds `items []string`, `selected int`, `open bool`, and renders itself via a `View(x, y, width int) string` method.

**Rationale**: The dropdown is simple enough to not warrant its own package at this stage. Keeping it local avoids premature abstraction while still being extractable later.

**Alternative considered**: Using the `charmbracelet/bubbles/list` component. Rejected — it's designed for full-screen list views and carries significant rendering overhead for a small overlay.

### Field groups with Tab-cycling, not collapsible sections

**Decision**: Fields are organized into three named groups — **Core** (executor/plugin, model, prompt), **Execution** (needs, retry, for-each, on-failure), **Advanced** (condition, publish_to, args) — displayed as a labeled separator. The user Tab-cycles through groups, not individual fields.

**Rationale**: Collapsible sections in a terminal TUI require tracking open/closed state per section and re-rendering logic. Tab-group cycling is simpler, faster, and keyboard-natural for BBS users. Each group renders only its own fields; other groups are shown dimmed.

**Alternative considered**: Accordion-style collapsible sections with `[+]`/`[-]` indicators. Deferred — can be added later without spec changes.

### Executor field replaces Plugin field in UI; save.go writes `executor`

**Decision**: The UI presents an "Executor" dropdown (showing provider names and builtins). `save.go` already writes to `executor` in the YAML. The deprecated `plugin` field is not shown.

**Rationale**: The runtime already prefers `executor`. Surfacing `plugin` would create confusion. Legacy pipelines that use `plugin` continue to work at the runner level.

### Args editor as a simple key/value list

**Decision**: The `args` map is edited as a scrollable list of `key = value` rows, with `+` to add a new row and `d` to delete the focused row. Values are strings only.

**Rationale**: The `args` map in practice contains simple string values (model overrides, flags). Supporting nested maps or typed values in the TUI is disproportionate complexity.

## Risks / Trade-offs

- **Terminal width constraints** → The overlay dropdown is capped at `min(40, paneWidth-4)` columns; long model names truncate with `…`. Mitigation: model labels are already short by convention.
- **Overlay z-ordering in BubbleTea** → BubbleTea renders a single string; overlays are simulated by string surgery or by rendering the overlay last. Mitigation: render overlay as a suffix block absolutely positioned using ANSI cursor moves — well-understood pattern.
- **More fields = more vertical space required** → On short terminals (<30 rows) some field groups may be cut off. Mitigation: scroll within the right pane; add a row-count guard that collapses Advanced group automatically below 24 rows.

## Open Questions

- Should the `needs` multi-select dropdown show step IDs only, or `id: prompt-snippet` label pairs? (Proposal: step IDs — simpler, matches YAML directly)
- Should retry `interval` be a free-text field or a set of preset durations? (Proposal: free-text with format hint `e.g. 2s, 500ms`)
