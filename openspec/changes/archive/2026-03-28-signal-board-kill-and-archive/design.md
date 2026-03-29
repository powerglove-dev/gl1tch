## Context

The signal board (`internal/switchboard/signal_board.go` + `switchboard.go`) renders a list of `feedEntry` structs that track pipeline and agent runs. Each entry has a `status` (`FeedRunning`/`FeedDone`/`FeedFailed`), an optional `tmuxWindow` target, and an `id` that keys into `activeJobs map[string]*jobHandle`. The `jobHandle` holds a `context.CancelFunc` and the tmux window name. Running jobs are cancelled via the cancel function; the tmux window is named `session:orcai-<feedID>`.

Currently the board has no kill or dismiss action — users must exit the TUI or wait for jobs to finish. The filter cycle is `["all", "running", "done", "failed"]` with no archived state.

## Goals / Non-Goals

**Goals:**
- `x` kills the selected running entry: cancels the context + kills the tmux window, marks entry `FeedFailed`
- `d` archives the selected entry (any status): sets `archived = true` on the `feedEntry`
- `"all"` filter hides archived entries (as if they don't exist)
- A new `"archived"` filter shows only archived entries
- The hint bar reflects available actions based on the selected entry's state

**Non-Goals:**
- Persistence of archived state across restarts (in-memory only for now)
- Bulk kill/archive operations
- Undo / unarchive
- Confirmation modal before kill or archive

## Decisions

### Decision: `archived` as a bool field on `feedEntry` rather than a separate `FeedArchived` status

`FeedStatus` represents pipeline lifecycle state (running/done/failed). Archived is a display/visibility concern orthogonal to status — a failed run can be archived. Adding a bool keeps the lifecycle enum clean and avoids "what does FeedArchived mean for kill logic?" questions.

*Alternative considered*: `FeedArchived` as a fourth `FeedStatus` value. Rejected because it conflates display state with execution state and would require auditing all switch statements on `FeedStatus`.

### Decision: Kill on `x` only when the selected entry is `FeedRunning`

If the entry is done or failed, `x` is a no-op (no error shown). This matches how most TUI tools handle actions that don't apply — silent no-op rather than modal. The hint bar only shows the kill hint when the selected entry is running, making the constraint visible.

### Decision: Filter cycle becomes `["running", "all", "done", "failed", "archived"]` with `"running"` as default

Starting at `"running"` keeps the board focused on active work immediately on open. `"all"` is still one keypress away. `"archived"` sits at the end as an intentional extra step — it's a rarely-needed view. The default `activeFilter` field on `SignalBoard` is set to `"running"` (previously `""` which fell back to `"all"` in `buildSignalBoard`).

### Decision: `filteredFeed` excludes archived unless `activeFilter == "archived"`

Current `filteredFeed` does a status switch. Add a pre-filter: if `filter != "archived"`, drop all entries where `archived == true`. If `filter == "archived"`, return only archived entries (regardless of status).

## Risks / Trade-offs

- [Feed count drift] The `maxParallelJobs` guard and ring-buffer cap (200 entries) both count entries including archived ones. → Acceptable; archived entries are in-memory and don't affect job scheduling. Kill removes from `activeJobs` immediately.
- [Kill race] If a job completes (context already cancelled by the goroutine) between user pressing `x` and the kill handler executing, the double-cancel is harmless (CancelFunc is idempotent; `tmux kill-window` on a non-existent target is a no-op or returns error that is already ignored in similar paths). → No mitigation needed.
- [tmux kill-window target format] `tmuxWindow` is stored as `"session:orcai-<feedID>"`. Killing requires `exec.Command("tmux", "kill-window", "-t", entry.tmuxWindow)`. If `tmuxWindow` is empty (in-process jobs without tmux), only the cancel is called. → Already the pattern used elsewhere.
