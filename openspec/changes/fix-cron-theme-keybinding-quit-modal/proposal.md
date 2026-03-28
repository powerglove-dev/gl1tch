## Why

The cron TUI runs as a separate tmux session inside ORCAI, but `^spc m` (theme switcher) does not open the theme picker from within it. Additionally, the cron TUI exposes a `q` quit shortcut and a quit confirmation modal — both inappropriate for a component that is fully managed by ORCAI/tmux and should not be user-quittable. The quit modal copy also references "cron job manager" rather than ORCAI, which is misleading since quitting exits ORCAI entirely.

## What Changes

- **Fix `^spc m` from cron TUI**: The tmux chord binding for `m` in `orcai-cron` session should open the theme picker. Currently the cron TUI receives a `T` key but has no theme picker of its own — the binding needs to route to the switchboard session instead, or the cron TUI needs a theme picker overlay wired up.
- **Remove `q` quit shortcut from cron TUI**: The cron TUI should not be directly quittable by the user; ORCAI/tmux manages its lifecycle.
- **Remove quit confirmation modal from cron TUI**: Since `q` is removed, the quit confirmation flow (`viewQuitConfirm`, `handleQuitConfirmKey`, `confirmQuit` state) is dead code and should be removed.
- **Update quit modal copy** (if any quit path remains): Change message from `"Exit the cron job manager?"` to `"Quit ORCAI"` to reflect that quitting exits ORCAI entirely, not just the panel.

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `cron-tui-keybindings`: Remove `q` quit shortcut and quit confirmation modal from cron TUI; fix `^spc m` theme switcher routing from cron session.

## Impact

- `internal/crontui/update.go` — remove `q` key handler and quit confirm key handler
- `internal/crontui/view.go` — remove `viewQuitConfirm()` and its call site
- `internal/crontui/model.go` — remove `confirmQuit` field
- `internal/crontui/helpers.go` — remove any quit-confirm helpers if present
- `internal/bootstrap/bootstrap.go` — fix tmux `^spc m` chord binding for `orcai-cron` session to route theme picker correctly
