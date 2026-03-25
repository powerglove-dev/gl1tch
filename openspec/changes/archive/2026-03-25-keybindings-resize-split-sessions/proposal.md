## Why

The keybindings action map already supports splitting and navigating panes but has no actions for resizing them, leaving users unable to adjust split sizes via configured keybindings. Adding resize-pane actions closes this gap and rounds out the pane management story.

## What Changes

- Add four new action names to `keybindings.actionMap`: `resize-pane-left`, `resize-pane-right`, `resize-pane-up`, `resize-pane-down`
- Each action maps to `tmux resize-pane` with the appropriate direction flag and a configurable or default cell count
- The `config init` defaults include example resize bindings alongside the existing split/nav defaults

## Capabilities

### New Capabilities

- `pane-resize-keybindings`: Keybinding actions for resizing the active tmux pane in any of the four cardinal directions

### Modified Capabilities

<!-- No existing spec-level requirements change -->

## Impact

- `internal/keybindings/keybindings.go` — add four entries to `actionMap`
- `internal/keybindings/keybindings_test.go` — extend test coverage for new actions
- Default `keybindings.yaml` written by `orcai config init` — add example resize bindings
