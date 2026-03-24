## Context

The prompt builder's left sidebar currently renders only the steps list. Pipeline-level metadata — name, description, tags — is not editable from within the TUI once the builder opens. The `Model` struct holds `name string`, `steps []pipeline.Step`, and `selectedIndex int`. The `Pipeline` YAML struct has `Name`, `Version`, and `Steps` fields only. The `view.go` renders the left pane as a single-mode steps list; `keys.go` has no settings-related bindings.

## Goals / Non-Goals

**Goals:**
- Expose pipeline `name`, `description`, and `tags` as editable fields in the sidebar
- Add a sidebar navigation mode toggle (steps list ↔ settings panel) via keyboard
- Persist `description` and `tags` to the saved `.pipeline.yaml`
- Keep all interaction keyboard-driven; no mouse required

**Non-Goals:**
- Per-step settings UI (that belongs in the existing right config pane)
- `version` field editing (internal detail, not user-facing)
- Tags autocomplete or multi-value picker (YAGNI)
- Settings persistence across sessions other than via save

## Decisions

**1. `sidebarMode` field on `Model` (not a separate component)**
Options considered: (a) separate `settingsModel` sub-component, (b) mode flag on existing `Model`. Chose (b) — the settings panel shares the same sidebar pane and the state is lightweight (three scalar fields). A sub-component adds indirection with no benefit at this scope.

**2. `,` key to toggle sidebar mode**
`s` is taken (save), `ctrl+,` is idiomatic for settings in many editors but adds a chord. Plain `,` is unused and discoverable. The help bar will show `,` as "settings".

**3. Settings fields as `textinput` bubbles**
`charmbracelet/bubbles/textinput` is already in scope (used elsewhere). Three inputs: `name`, `description`, `tags`. Tags input accepts a comma-separated string — simple, no multi-value widget needed. Validated/split only at save time.

**4. `Pipeline` struct gains `Description` and `Tags` fields**
Adding `description string` and `tags []string` with yaml tags to `pipeline.Pipeline` is non-breaking — existing YAML files without these fields unmarshal with zero values. `ToPipeline()` in `model.go` propagates the new fields.

**5. Tab cycling scoped by sidebar mode**
When `sidebarMode == settings`, Tab advances through the three settings inputs. When `sidebarMode == steps`, Tab behavior is unchanged (next field in right config pane). No global Tab handling change needed — the existing `Update` switch already routes by focus context.

## Risks / Trade-offs

- **Tags string format** — comma-separated input is simple but doesn't enforce tag naming conventions. → Mitigation: split and trim at save time; malformed tags round-trip as-is.
- **Name mutability** — the pipeline name is also the save filename. Editing name mid-session without saving could confuse users. → Mitigation: name field updates `model.name` immediately; filename is derived at save time, consistent with current behavior.

## Migration Plan

No migration required. The two new `Pipeline` fields (`description`, `tags`) are additive with zero-value YAML omitempty defaults. Existing `.pipeline.yaml` files load and save correctly without modification.

## Open Questions

- Should an empty `description` be omitted from the YAML (`omitempty`) or always written? Prefer omitempty for clean files.
- Should tags in YAML be a YAML sequence (`- tag`) or a single comma string? Prefer YAML sequence (`[]string`) for interoperability.
