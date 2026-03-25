## Context

The `internal/keybindings` package maps user-configured action names to tmux command argument slices. The `actionMap` already contains entries for pane splitting (`split-pane-right`, `split-pane-down`) and pane navigation (`select-pane-{left,right,up,down}`), but has no entries for resizing. Users who configure keybindings for pane management cannot adjust pane sizes without dropping out to raw tmux keybindings.

## Goals / Non-Goals

**Goals:**
- Add `resize-pane-left`, `resize-pane-right`, `resize-pane-up`, `resize-pane-down` to `actionMap`
- Map each to `tmux resize-pane -<DIR> <cells>` using a sensible default cell count (5)
- Include example resize bindings in the `config init` defaults

**Non-Goals:**
- Variable resize amounts (no per-binding `amount` parameter — keep the action model simple)
- Mouse resize support
- Any changes to the YAML config schema

## Decisions

### Fixed cell count of 5

**Decision**: Hard-code 5 cells per resize step in `actionMap`.

**Rationale**: The current action model maps action names to a static `[]string` of tmux args; there is no per-binding parameter support. Adding a parameter field would require a schema change and is out of scope. Five cells is a conventional tmux default and matches what most users expect from a single keypress.

**Alternative considered**: Allow an optional `amount` field in `Binding` and pass it as a flag. Rejected because it requires changing the YAML schema, the `Binding` struct, and `Apply` — complexity disproportionate to a minor UX preference.

### No new config schema fields

Adding resize actions requires only four new entries in `actionMap`. No changes to `Config`, `Binding`, `LoadConfig`, or `Apply` are needed.

## Risks / Trade-offs

- **Fixed step size may feel too coarse or too fine** → Users can work around it by pressing the key repeatedly; a future change can introduce parameterized actions if demand arises.
- **`config init` defaults are opinionated** → Example bindings use `M-<arrow>` keys, which may conflict with user shell bindings. The defaults are just examples; users can remove them.

## Migration Plan

No migration required. New action names are additive. Existing `keybindings.yaml` files that do not reference resize actions are unaffected.
