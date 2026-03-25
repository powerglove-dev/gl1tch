## Why

The pipeline builder's current UI relies entirely on left/right arrow cycling for plugin and model selection — a pattern that hides available options and forces the user to cycle blindly through choices. Now that the plugin system and pipeline step schema have been significantly expanded (DAG execution, retry policies, for-each iteration, event publishing, conditional branching, structured args), the builder exposes none of these capabilities and must be overhauled to match what the runtime actually supports, while keeping the ABBS/BBS aesthetic.

## What Changes

- **BREAKING**: Replace plugin and model cycling (`◀ Label ▶`) with inline dropdown overlays using box-drawing characters and the Dracula palette
- Add a **step-type selector** — choose between plugin step, builtin step (`builtin.assert`, `builtin.log`, `builtin.sleep`, `builtin.http_get`, `builtin.set_data`), input step, and output step
- Expose **executor field** alongside the existing plugin/model selectors (replacing deprecated `plugin` field)
- Expose **structured args editor** — key/value pairs for the `args` map, replacing the flat `vars` approach
- Expose **DAG dependency selector** — multi-select dropdown listing sibling step IDs for the `needs` field
- Expose **retry policy form** — `max_attempts`, `interval`, `on` (always / on_failure)
- Expose **for-each field** — text input for template string
- Expose **on-failure field** — dropdown selecting a sibling step ID
- Expose **condition form** — `if` expression input with `then`/`else` step-ID dropdowns
- Expose **publish_to field** — text input for event bus topic
- Introduce a **field group panel** — fields are grouped into collapsible sections (Core, Execution, Advanced) to reduce cognitive load
- Retain ABBS Dracula palette, box-drawing borders, and monospace layout throughout

## Capabilities

### New Capabilities

- `pipeline-builder-dropdown`: Reusable inline dropdown/select component for the BubbleTea TUI, styled with ABBS aesthetics (Dracula palette, box-drawing overlay)
- `pipeline-builder-step-editor`: Full step editor panel exposing all pipeline Step fields, grouped into collapsible sections, replacing the current 3-field cycling layout
- `pipeline-builder-builtin-support`: UI support for configuring builtin step types (`builtin.*`) including their specific arg schemas

### Modified Capabilities

- `pipeline-builtins`: No requirement changes — implementation detail only

## Impact

- `internal/promptbuilder/` — full rewrite of `model.go`, `view.go`, `keys.go`; `save.go` and `run.go` unchanged
- New `internal/promptbuilder/dropdown/` package (or inline component) for the reusable dropdown widget
- No changes to `internal/pipeline/`, `internal/plugin/`, or the YAML schema
- No changes to the CLI entry point (`orcai _promptbuilder`)
