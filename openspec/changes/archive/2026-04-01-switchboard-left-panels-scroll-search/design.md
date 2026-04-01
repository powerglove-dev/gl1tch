## Context

The ABBS Switchboard left column contains four stacked panels: Pipelines, Agent Runner, Inbox, and Cron Jobs. `viewLeftColumn` already attempts to distribute remaining height between Inbox and Cron — but the Cron panel (`buildCronSection`) renders every entry unconditionally, and neither panel supports scroll-offset navigation or text filtering. The Inbox panel (`buildInboxSection`) has a `maxRows` guard but no scroll position; items past the visible window are simply dropped. Neither panel shows a search input or supports fuzzy filtering of its list content.

Additionally, the Inbox has no concept of "read" state — every completed run appears on every session start, producing noise in the panel (see screenshot: 50+ identical `agent-opencode-*` entries).

Key files:
- `internal/switchboard/switchboard.go` — `viewLeftColumn`, `buildInboxSection`, `Model`
- `internal/switchboard/cron_panel.go` — `buildCronSection`, `CronPanel` (already has `scrollOffset`/`selectedIdx` fields)
- `internal/inbox/model.go` — `inbox.Model` wrapping a `bubbles/list`
- `internal/store/` — `store.Run` persistence layer

## Goals / Non-Goals

**Goals:**
- Left column panels never overflow the terminal height; height is distributed proportionally among all four panels
- Inbox and Cron Jobs panels support scroll-offset navigation (up/down/`j`/`k`) to browse entries beyond the visible window
- Both panels support in-panel fuzzy text search activated with `/`, dismissed with `Esc`
- Inbox items can be marked as read with `x` (or `Enter` while focused), removing them from the unread list
- Read state persists across sessions (survives TUI restart)

**Non-Goals:**
- Scrolling for the Pipelines or Agent Runner panels (they are short fixed-height lists)
- Full-text search across feed entries or the signal board
- Pagination UI (page numbers, scroll bars) — cursor-based scroll is sufficient for now
- Archiving or deleting runs from the store (mark-as-read hides from the list view only)

## Decisions

### 1. Scroll state lives in the panel structs, not inbox.Model

`CronPanel` already has `scrollOffset` and `selectedIdx`. We extend this pattern: add `scrollOffset`, `selectedIdx`, and `filterQuery`/`filterActive` fields to a new `InboxPanel` struct (mirroring `CronPanel`), and move Inbox focus/selection state there. This keeps the switchboard `Model` consistent and avoids threading extra state through `inbox.Model`.

**Alternatives considered:** Using `bubbles/list`'s built-in filter (already present in `inbox.Model.list`) — rejected because `buildInboxSection` renders ANSI directly and does not use the list component's `View()`. Adding a scroll viewport to `inbox.Model` itself was also considered but would require duplicating ANSI rendering logic inside the inbox package.

### 2. Fuzzy search implemented inline with `strings.Contains` (case-insensitive) on the item name

A full fuzzy library (e.g. `sahilm/fuzzy`) is overkill for short lists. Case-folded `strings.Contains` on the run/job name gives the expected "filter as you type" UX. The search query is displayed as a one-line input row inside the panel box (between the header border and the first item row).

**Alternatives considered:** Regex match — adds user complexity without clear benefit for this use case.

### 3. Read state stored as a flat JSON/text file under `~/.config/orcai/inbox-read.json`

A simple `map[string]bool` keyed on run ID (stringified `store.Run.ID`) is written on each mark-as-read action. On startup the Switchboard loads this set and filters `inboxModel.Runs()` before rendering. This avoids a DB schema migration.

**Alternatives considered:** Adding a `ReadAt` column to the SQLite `runs` table — cleaner long term but requires a schema migration and changes to the store package. Deferred to a future cleanup.

### 4. Height budget uses integer division with floor allocation

`viewLeftColumn` receives the total column height. Banner + blank line is fixed (2 rows). Launcher and Agent Runner have known fixed heights (measured at render time by capturing their output line counts). The remaining rows are split: Inbox gets 60%, Cron gets 40% (minimum 4 rows each). If the remaining budget cannot satisfy both minimums, Cron is dropped entirely.

**Alternatives considered:** Equal split — produces an awkwardly tall Cron panel when the inbox is busy. User-configurable ratio — deferred.

## Risks / Trade-offs

- **Read-state file diverges from store** — If a run ID is recycled (SQLite auto-increment), a stale read entry could accidentally hide a new run. Mitigation: run IDs are append-only integers; recycling cannot occur without manual DB manipulation.
- **Scroll + filter interaction** — When a filter is applied, `scrollOffset` must be clamped to the filtered list length. Mitigation: clamp `scrollOffset` to `max(0, len(filteredItems)-visibleRows)` on every filter update.
- **ANSI rendering width of the search row** — The inline search prompt consumes one row from the panel's `maxRows` budget. Mitigation: subtract 1 from `maxRows` when `filterActive` is true.

## Open Questions

- Should Cron Jobs also support mark-as-read / dismiss? Deferred — cron entries are config-driven, not run-history-driven, so "read" semantics are less clear.
- Long-term: migrate read state to a `read_at` column in SQLite? Yes, but out of scope for this change.
