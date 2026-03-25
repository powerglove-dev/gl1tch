## Context

`internal/sidebar/sidebar.go` implements a BubbleTea TUI panel that subscribes to `orcai.telemetry` and renders a window list with token/cost overlays. It is spawned via `RunToggle()` which currently hard-codes `-t orcai:0` in the `tmux split-window` call, locking the panel to window 0.

The `View()` renders a minimal ASCII list — dots row, `═╢ ORCAI ╟═` banner, window list rows with `▎` cursor accent, and a footer. No box-drawing per row, no node numbering, no activity log.

## Goals / Non-Goals

**Goals:**
- Redesign `View()` to render a BBS sysop-style monitor: full `╔═╗ ║ ╚═╝` outer frame, `▒▒▒ ORCAI SYSOP MONITOR ▒▒▒` header block, per-node rows with `NODE XX` labels and `[BUSY]` / `[IDLE]` / `[WAIT]` status badges, metrics line, and a scrolling activity log section.
- Change `RunToggle()` to spawn on the current active window rather than window 0.
- Track panel visibility per-window (marker file named `.panel-visible-<windowIndex>`).
- Add an activity log: a bounded FIFO of the last 12 telemetry events rendered at the bottom of the panel in dim text.

**Non-Goals:**
- Keyboard input to the activity log (no scrollback navigation — that's future work).
- Saving the log across panel restarts.
- Changing the bus subscription or telemetry data model.

## Decisions

### D1: Per-window visibility tracking

Current: single `~/.config/orcai/.sidebar-visible` marker. Proposed: `~/.config/orcai/.panel-<windowIndex>` where `windowIndex` is read from `$TMUX_WINDOW_INDEX` env var at toggle time. This allows each window to independently show or hide its panel.

**Fallback:** if `TMUX_WINDOW_INDEX` is unset, fall back to a single `panel-0` file (safe default).

### D2: Spawn on current window, not window 0

`RunToggle()` change:
```
# Before
tmux split-window -d -h -b -f -l 25% -t orcai:0 <self> _sidebar

# After (no -f, no fixed target — splits current pane)
tmux split-window -d -h -b -l 30% <self> _sidebar
```
The `-f` flag makes the split span the full window height from the outermost frame — removing it gives a normal horizontal split of the current pane. Removing `-t orcai:0` targets the active pane instead. Width changes from 25% to 30% for readability of the wider BBS layout.

### D3: BBS sysop panel layout

```
╔══════════════════════════╗
║ ▒▒▒ ORCAI SYSOP MONITOR ▒▒▒ ║
║ NODES: 2 ACTIVE  17:42   ║
╠══════════════════════════╣
║ NODE 01 [BUSY]           ║
║   claude-sonnet-4        ║
║   12k↑ 347↓  $0.042      ║
╠──────────────────────────╣
║ NODE 02 [IDLE]           ║
║   gemini-2.0             ║
║   8k↑  120↓  $0.000      ║
╠══════════════════════════╣
║ ── ACTIVITY LOG ──       ║
║ 17:41 NODE01 streaming   ║
║ 17:42 NODE01 done $0.042 ║
║ 17:42 NODE02 idle        ║
╚══════════════════════════╝
  enter focus  x kill  ↑↓
```

Node rows use `╠══╣` separator between nodes and `╠──╣` (thin) for the log section divider.

Status badges:
- `[BUSY]` — green `\x1b[38;5;84m`
- `[IDLE]` — dim teal `\x1b[38;5;66m`
- `[WAIT]` — yellow `\x1b[38;5;228m` (streaming start received but no Done yet… same as BUSY actually — use BUSY)

Node numbering: sequential from 1, ordered by window index.

### D4: Activity log as `[]logEntry` ring buffer (cap 12)

```go
type logEntry struct {
    At      time.Time
    Node    int    // 1-based node number
    Event   string // "streaming" | "done"
    CostUSD float64
}
```

Added to `Model` struct. On each `TelemetryMsg`, prepend to the slice and cap at 12 entries. Rendered newest-first (most recent at top of log section).

### D5: No changes to Init/Update logic

The telemetry subscription, tick, and keyboard handling (j/k/enter/x/ctrl+c) remain identical. Only `View()` and `RunToggle()` change significantly.

## Risks / Trade-offs

- **Kill toggle targets current pane `.0`** — the kill path (`tmux kill-pane -t .0`) targets the leftmost pane of the current window. This works if the panel is always the leftmost split (which `-b` ensures). No change needed.
- **Multiple panels on same window** — not guarded; the toggle checks marker file state before spawning, which prevents double-spawning.
- **Node order stability** — nodes are ordered by window index (ascending), which is stable across ticks as long as windows aren't renumbered. Acceptable.
