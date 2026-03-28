## Context

The cron TUI runs as a separate tmux session (`orcai-cron`). Two keybinding issues exist:

1. **`^spc m` (theme switcher)**: The tmux chord handler in `bootstrap.go` detects when the active session is `orcai-cron` and sends `T` to the cron pane. However, the cron TUI has no theme picker overlay — it only receives theme changes passively via busd. The `T` keypress is silently ignored.

2. **Quit shortcut**: The cron TUI exposes `q`, `ctrl+c`, and `esc` (on jobs pane) as quit triggers, opening a `quitConfirm` modal. Since the cron TUI lifecycle is managed entirely by ORCAI/tmux, these shortcuts are inappropriate and confusing. The `^spc q` tmux binding also sends `q` directly to the cron pane when in `orcai-cron`, reinforcing this leak.

## Goals / Non-Goals

**Goals:**
- `^spc m` from `orcai-cron` session opens the theme picker in the switchboard (same as from any other context)
- `q`, `ctrl+c`, and `esc`-as-quit are removed from the cron TUI
- All quit-confirm state and UI is removed from the cron TUI
- `^spc q` from `orcai-cron` routes to the switchboard quit flow, consistent with all other sessions

**Non-Goals:**
- Adding a local theme picker overlay to the cron TUI
- Changing how theme changes are applied to the cron TUI (busd subscription stays as-is)
- Modifying the quit modal in the switchboard

## Decisions

### Fix `^spc m`: Route to switchboard instead of sending `T` to cron pane

**Decision**: Change `bootstrap.go` line 135 — when in `orcai-cron`, switch to the `orcai` session and send `T` to `orcai:0` (the switchboard), identical to the non-cron branch.

**Why**: The cron TUI has no theme picker. The switchboard already handles the full theme picker UX and broadcasts the result to all processes (including cron) via busd. Routing to switchboard is the simplest fix with zero new code in crontui.

**Alternative considered**: Add a theme picker overlay to crontui. Rejected — unnecessary duplication; busd already handles cross-process theme sync.

### Fix `^spc q`: Route to switchboard quit flow from cron session too

**Decision**: Change the `^spc q` chord binding — when in `orcai-cron`, switch to orcai session and send `C-q` to `orcai:0`, same as the non-cron branch.

**Why**: Quit from cron should exit ORCAI (or at least the main app), not just the cron process. The switchboard already owns the quit modal.

### Remove all quit-related code from crontui

**Decision**: Remove `quitConfirm bool` from `model.go`, remove `handleQuitConfirmKey`, remove the `q`/`ctrl+c` case and `esc`-as-quit from `handleKey` and `handleJobPaneKey`, remove `viewQuitConfirm` and its call site in `view.go`.

**Why**: These are dead paths once `q` is removed. Keeping dead code is confusing and the `esc` quit trigger is especially surprising.

**`esc` on jobs pane**: Currently opens quit confirm. After this change, `esc` on the jobs pane should simply do nothing (no-op), since there's no "parent" context to escape to.

## Risks / Trade-offs

- **`ctrl+c` removal**: `ctrl+c` is a common "force quit" key. Removing it means users trapped in a broken cron TUI have no keyboard escape. Mitigation: `^spc q` (via tmux) still provides a quit path through the switchboard. This is acceptable — the cron TUI is not directly user-launched.

## Migration Plan

All changes are in-process Go code and tmux config string — no persistent state changes, no migrations needed. Bootstrap config is regenerated on next `orcai` startup.
