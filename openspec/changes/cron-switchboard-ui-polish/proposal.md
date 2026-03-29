## Why

Several small UX inconsistencies have accumulated between the Switchboard and Cron TUI: live theme preview is broken in Switchboard, the Cron TUI is missing the top header bar present in Switchboard, both TUIs lack a padding row below that header, renaming a cron job doesn't immediately propagate to the Switchboard's inbox/cron panel, and there's no quick shortcut to open a job's pipeline file from the cron jobs panel.

## What Changes

- **Fix Switchboard live theme preview**: `handleThemePicker` discards the `selected` bundle (uses `_`); capture it and assign `m.bundle` immediately so the UI updates in real time, matching how crontui works.
- **Add top header bar to Cron TUI**: Port `viewTopBar` (or a shared variant) from Switchboard into crontui's `View()`, rendering the full-width ORCAI title bar above the two panes.
- **Add one padding row below the top header bar** in both Switchboard and Cron TUI (a single blank lipgloss row inserted between the header bar and the panel area).
- **Propagate cron entry renames to Switchboard**: After a successful rename in crontui's `confirmEdit`, publish a `cron.entry.updated` busd event carrying the old and new entry names. Switchboard subscribes and immediately re-renders, so the cron panel and inbox reflect the new name without waiting for the next poll tick.
- **Add "p pipeline" hint + handler in Cron TUI**: When the cron jobs pane is focused, pressing `p` opens the scheduled pipeline's YAML file in `$EDITOR` (falling back to `vi`) using `tea.ExecProcess`. The hint bar shows `p pipeline`.

## Capabilities

### New Capabilities

_(none — all changes are within existing components)_

### Modified Capabilities

- `status-bar-session-controls`: The Switchboard top bar and panel layout changes (padding row) affect the overall height budget.

## Impact

- `internal/crontui/view.go` — add `viewTopBar`, insert it in `View()`, add padding row, update height budget for panes
- `internal/crontui/update.go` — add `p` key handler, publish `cron.entry.updated` after rename
- `internal/crontui/model.go` — no structural changes needed
- `internal/switchboard/theme_picker.go` — capture `selected` bundle in `handleThemePicker`
- `internal/switchboard/switchboard.go` — add padding row after topBar in `View()`, subscribe to `cron.entry.updated` and re-render on receipt
- `internal/busd/topics/` (or equivalent) — add `CronEntryUpdated` topic constant if one doesn't exist
