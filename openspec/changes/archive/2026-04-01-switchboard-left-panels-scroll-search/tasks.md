## 1. Read State Persistence

- [x] 1.1 Create `internal/switchboard/readstate.go` with `LoadReadSet(path string) (map[int64]bool, error)` and `SaveReadSet(path string, ids map[int64]bool) error` using JSON encoding
- [x] 1.2 Add `readStateFile() string` helper to `Model` that returns `~/.config/orcai/inbox-read.json`
- [x] 1.3 Load the read set during `switchboard.New(...)` and store it in `Model.inboxReadIDs map[int64]bool`
- [x] 1.4 Handle missing or corrupt file gracefully (treat as empty set, log warning)

## 2. Inbox Panel State

- [x] 2.1 Add `InboxPanel` struct to `switchboard.go` with fields: `selectedIdx int`, `scrollOffset int`, `filterQuery string`, `filterActive bool`
- [x] 2.2 Replace bare `inboxFocused bool` field in `Model` with `inboxPanel InboxPanel` (keeping focused state inside it)
- [x] 2.3 Update all existing references to `m.inboxFocused` and `m.inboxModel.SelectedIndex()` to use `m.inboxPanel`

## 3. Height Budget

- [x] 3.1 Refactor `viewLeftColumn` to measure the line count of `buildLauncherSection` and `buildAgentSection` output before allocating remaining rows
- [x] 3.2 Implement 60/40 split: `inboxRows = remaining * 6 / 10`, `cronRows = remaining - inboxRows - 1`, each with minimum 4
- [x] 3.3 Drop Cron panel from rendering when `cronRows < 4` after the split

## 4. Inbox Scroll + Filter Rendering

- [x] 4.1 Add `filteredInboxRuns(runs []store.Run) []store.Run` helper that applies `m.inboxReadIDs` exclusion and `m.inboxPanel.filterQuery` substring match (case-insensitive)
- [x] 4.2 Update `buildInboxSection` to use `filteredInboxRuns` instead of raw `m.inboxModel.Runs()`
- [x] 4.3 When `filterActive` is true, render a search prompt row (`/ <query>█`) as the first content row inside the box (subtract 1 from `maxRows`)
- [x] 4.4 Apply `scrollOffset` so only items in `[scrollOffset, scrollOffset+maxRows)` are rendered

## 5. Cron Panel Scroll + Filter

- [x] 5.1 Add `filterQuery string` and `filterActive bool` fields to `CronPanel` struct in `cron_panel.go`
- [x] 5.2 Add `filteredCronEntries` helper that applies the cron panel's `filterQuery` to entry names
- [x] 5.3 Update `buildCronSection` to accept a `height int` parameter and use it as `maxRows` (mirror `buildInboxSection` pattern)
- [x] 5.4 Apply `scrollOffset` in `buildCronSection` analogously to the inbox
- [x] 5.5 Render search prompt row inside the Cron box when `filterActive` is true

## 6. Key Handling — Inbox Panel

- [x] 6.1 Handle up/`k` and down/`j` in the Inbox panel focus path: advance `inboxPanel.selectedIdx`, clamp to filtered list length, update `scrollOffset` to keep cursor in view
- [x] 6.2 Handle `/` to set `inboxPanel.filterActive = true` and route subsequent printable keystrokes to `filterQuery`
- [x] 6.3 Handle `Backspace` in search mode to remove last rune from `filterQuery`
- [x] 6.4 Handle `Esc` in search mode to clear `filterQuery` and set `filterActive = false`, reset `scrollOffset` to 0
- [x] 6.5 Handle `x` (mark as read): add selected run's ID to `m.inboxReadIDs`, persist to disk via `SaveReadSet`, advance cursor

## 7. Key Handling — Cron Panel

- [x] 7.1 Handle up/`k` and down/`j` in the Cron panel focus path with the same scroll-follows-cursor logic
- [x] 7.2 Handle `/` to activate cron search, route keystrokes to `cronPanel.filterQuery`
- [x] 7.3 Handle `Backspace` and `Esc` in cron search mode (same as inbox)

## 8. Help Bar

- [x] 8.1 Add `x mark read` to the help bar tokens rendered when `m.inboxPanel.focused` is true
- [x] 8.2 Add `/ search` to the help bar tokens rendered when either inbox or cron panel is focused and search is inactive
- [x] 8.3 Add `esc cancel` to the help bar tokens when search is active in either panel

## 9. Tests

- [x] 9.1 Unit-test `LoadReadSet` / `SaveReadSet`: missing file returns empty map, corrupt file returns empty map + no panic, round-trip preserves IDs
- [x] 9.2 Unit-test `filteredInboxRuns`: exclusion by read ID, case-insensitive substring match, empty query returns all
- [x] 9.3 Unit-test height budget logic: verify 60/40 split, minimum enforcement, cron omission when budget is too small
- [x] 9.4 Unit-test `buildInboxSection` with scroll offset: only expected items rendered, search prompt row present when active
