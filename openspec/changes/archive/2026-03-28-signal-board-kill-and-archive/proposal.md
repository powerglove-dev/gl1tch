## Why

The signal board has no way to stop a running pipeline without leaving the TUI, and completed/failed jobs accumulate with no way to dismiss them — cluttering the board and making the filter useless for dismissal. Adding kill and archive actions closes the most common operational gaps users hit during active pipeline work.

## What Changes

- Add a **kill** action (`x` key) to the signal board that terminates the selected running pipeline (cancels the context, kills the tmux window) and marks the entry `failed`
- Add an **archive** action (`d` key) that removes the selected entry from the board permanently (any status)
- Show contextual hint text in the hint bar for both new actions when signal board is focused
- Add `"archived"` to the filter cycle so users can hide/show archived entries (archived entries are hidden from the default filter view)
- Change the default filter from `"all"` to `"running"` so the board opens focused on active work

## Capabilities

### New Capabilities

- `signal-board-kill-pipeline`: Kill action on the signal board — `x` on a running entry cancels the job and marks it failed
- `signal-board-archive`: Archive action on the signal board — `d` dismisses the selected entry; archived entries are excluded from all non-archived filter views and accessible via a dedicated `archived` filter
- `signal-board-default-filter`: The signal board initialises with `"running"` as its active filter instead of `"all"`, keeping the board focused on active work by default

### Modified Capabilities

- `pipeline-step-lifecycle`: No requirement changes; kill terminates via existing cancel context mechanism

## Impact

- `internal/switchboard/signal_board.go` — filter cycle, hint bar hints, archive display logic
- `internal/switchboard/switchboard.go` — `feedEntry` gets an `archived` bool field; `filteredFeed` excludes archived entries under "all"; key handler for `x` (kill) and `d` (archive)
- `internal/switchboard/switchboard.go` — kill dispatches cancel + tmux kill-window for the selected entry's job handle
