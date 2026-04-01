## Why

The global `^spc t` chord for "go to switchboard" is redundant — the switchboard will be accessible via `orcai-switchboard` in the jump-to-window modal (`^spc j`). Removing it frees `^spc t` for the theme picker, which is currently bound to the less intuitive `^spc m`.

## What Changes

- **Remove** `^spc t` switchboard chord binding from tmux bootstrap
- **Remove** `^spc t` hint from the tmux status bar
- **Remove** `^spc t` entry from the switchboard help modal
- **Remove** `^spc t` entry from any other hint text/modal references
- **Rename** `^spc m` → `^spc t` for the theme picker in tmux bootstrap chord table
- **Rename** `^spc m` → `^spc t` in the tmux status bar hint
- **Rename** `^spc m` → `^spc t` in the switchboard help modal
- **Rename** `^spc m` → `^spc t` in all other hint text/modal references

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `status-bar-session-controls`: `^spc t` hint removed from status bar; `^spc m` renamed to `^spc t`

## Impact

- `internal/bootstrap/bootstrap.go` — chord table: remove `t` binding, rename `m` → `t`
- `internal/themes/tmux.go` — status bar hint strings: remove `^spc t switchboard`, rename `^spc m themes` → `^spc t themes`
- `internal/switchboard/help_modal.go` — help modal bind list: remove `^spc t`, rename `^spc m` → `^spc t`
- `internal/modal/modal.go` — any inline hint text referencing `^spc t` or `^spc m`
