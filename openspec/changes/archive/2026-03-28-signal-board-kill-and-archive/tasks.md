## 1. Data Model

- [x] 1.1 Add `archived bool` field to `feedEntry` struct in `switchboard.go`
- [x] 1.2 Change `SignalBoard.activeFilter` default to `"running"` in the `New` / init path
- [x] 1.3 Update `filterCycle` slice in `signal_board.go` to `["running", "all", "done", "failed", "archived"]`

## 2. Filtering Logic

- [x] 2.1 Update `filteredFeed` in `switchboard.go` to exclude `archived == true` entries when filter is not `"archived"`
- [x] 2.2 Add `"archived"` case to `filteredFeed` that returns only `archived == true` entries

## 3. Kill Action

- [x] 3.1 Add key handler for `x` in the signal board focused branch of `switchboard.go Update()`
- [x] 3.2 In the kill handler, look up the selected entry's id in `activeJobs`; if found, call `jh.cancel()` and, if `jh.tmuxWindow != ""`, run `exec.Command("tmux", "kill-window", "-t", jh.tmuxWindow)`
- [x] 3.3 Call `m.setFeedStatus(id, FeedFailed)` (or equivalent) and delete from `activeJobs` after kill

## 4. Archive Action

- [x] 4.1 Add key handler for `d` in the signal board focused branch to set `archived = true` on the selected entry
- [x] 4.2 Advance `selectedIdx` to the next visible entry after archiving (or clamp to list end) so the cursor doesn't point at a now-hidden row

## 5. Hint Bar

- [x] 5.1 Update `sbHints` in `buildSignalBoard` to include `{Key: "d", Desc: "archive"}` when any entry is visible
- [x] 5.2 Conditionally include `{Key: "x", Desc: "kill"}` only when the selected entry is `FeedRunning`

## 6. Tests

- [x] 6.1 Add test: `x` on a running entry transitions status to `FeedFailed` and removes from `activeJobs`
- [x] 6.2 Add test: `x` on a done/failed entry is a no-op
- [x] 6.3 Add test: `d` on any entry sets `archived = true` and entry disappears from non-archived filters
- [x] 6.4 Add test: filter cycle order is `running → all → done → failed → archived → running`
- [x] 6.5 Add test: initial `activeFilter` is `"running"`

## 7. Cron Pipeline Signal Board Presence

- [x] 7.1 Handle `topics.CronJobStarted` in `pipeline_bus.go` — prepend a `FeedRunning` entry for the cron job if not already present
