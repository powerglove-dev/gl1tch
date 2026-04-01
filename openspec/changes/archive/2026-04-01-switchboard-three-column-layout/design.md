## Context

The switchboard (`internal/switchboard/switchboard.go`) currently uses a two-column layout:
- **Left** (30%): pipeline launcher + agent runner + inbox + cron
- **Right** (70%): signal board (top) stacked above activity feed (bottom)

Column widths are computed by `leftColWidth()` (30% of terminal width, min 28) and `feedPanelWidth()` (remainder minus 2). `View()` assembles the layout by zip-joining left and right lines with a two-space gutter.

The signal board is rendered by `buildSignalBoard()` and uses the label "signal_board" in the panel header. Steps in the feed are rendered with `├ ` / `└ ` ASCII connectors and 24hr timestamps (`15:04:05`).

## Goals / Non-Goals

**Goals:**
- Introduce a 25% right column exclusively for the activity feed.
- Rename the center panel from "signal board" to "agents" and render active agents as a 2-D grid with `h`/`j`/`k`/`l` cursor navigation.
- Move the agent runner section from the left column into the center column, below the agents grid.
- Center the activity feed content (entries and step timelines) within the right column.
- Replace ASCII step connectors with ANSI box-drawing tree characters (`├─`, `└─`, `│`).
- Change feed timestamps from `15:04:05` to `3:04 pm` (12hr, lowercase am/pm).

**Non-Goals:**
- Changing the left column contents beyond removing the agent runner.
- Altering the inbox detail, agent modal, or any full-screen overlay.
- Changing the signal/agents data model — only the presentation layer changes.
- Responsive breakpoints below the current minimum widths.

## Decisions

### D1 — Column width formula

| Column | Width |
|--------|-------|
| Left   | `w * 30 / 100` (min 28), unchanged |
| Right  | `w * 25 / 100` (min 20) |
| Center | `w - leftW - rightW - 4` (two 2-char gutters) |

**Why**: Keeps the left column at its existing 30% so launcher/inbox/cron are unaffected. 25% for the activity feed matches the user's explicit requirement. Center gets the remainder.

**Alternative considered**: Equal three-way split (33/34/33) — rejected because the left column needs its minimum 28-char hard floor and the activity feed is a secondary panel that doesn't need as much width.

### D2 — Agents grid layout

Agents (previously signal board rows) are rendered as a grid of fixed-width cards inside the center column. Card width = `(midW - 2) / gridCols`; `gridCols` is computed as `max(1, midW / 24)` so cards are at least 24 chars wide. The cursor (`agentsGridRow`, `agentsGridCol`) moves with `h`/`j`/`k`/`l` and is clamped to valid indices on each frame.

**Why**: A grid is more space-efficient than a list for the center column width. The fixed `24`-char minimum prevents cards from becoming illegible at narrow terminals.

**Alternative considered**: Single-column list (old signal board) — rejected because the user asked for a grid.

### D3 — Agent runner placement

`buildAgentSection()` output is appended below the agents grid in the center column render function (`viewCenterColumn`). The left column's `viewLeftColumn()` no longer calls `buildAgentSection()`.

**Why**: Contextual grouping — agent runner controls belong next to the agents grid they launch.

### D4 — Activity feed as social timeline (no raw output)

The feed is redesigned as a Facebook-style event timeline. Each entry is a centered card — **raw step output lines are suppressed entirely**. Showing step `lines` slices was appropriate when the feed was a log tail; now it is a structured status board. Each entry renders:

```
      2:34 pm
  ╭──────────────────────╮
  │ ● agent-name  [done] │
  │   pipeline: foo      │
  │   ├─ ✓ step-one     │
  │   └─ ✓ step-two     │
  ╰──────────────────────╯
```

- Timestamp floats above/beside the card in `pal.Dim` color.
- Card width is fixed at `min(rightW - 4, 36)` characters, centered with `strings.Repeat(" ", indent)`.
- Step connectors use box-drawing: `├─ ` (non-final), `└─ ` (final). No `│` continuation lines because output is suppressed.

**Why**: The user explicitly wants a clean social-style timeline, not a log dump. Raw output belongs in the inbox detail view or a tmux window — not the feed column.

### D5 — ANSI box-drawing step connectors

Replace current step connectors:
| Before | After |
|--------|-------|
| `├ ` | `├─ ` |
| `└ ` | `└─ ` |

`├─` and `└─` are standard Unicode box-drawing characters (U+251C, U+2500, U+2514), treated as single columns by terminal emulators, so existing `stripANSI`/`visWidth` math is unaffected.

### D6 — 12hr timestamp format

Change `entry.ts.Format("15:04:05")` → `strings.ToLower(entry.ts.Format("3:04 PM"))`. This produces `"2:34 pm"` matching the user's requirement. `strings.ToLower` is used because Go's time package only produces uppercase `AM`/`PM`.

## Risks / Trade-offs

- **Tests referencing column widths** → All existing tests that call `View()` or assert on line structure will need updating. Mitigation: identify affected tests before implementation.
- **Narrow terminals** → At very small widths, `midW` could go negative if `leftW + rightW > w`. Mitigation: add `max(midW, 10)` floor and hide right column below a minimum total width (e.g., 80 chars).
- **Centering math with ANSI escapes** → `visWidth()` must be used (not `len()`) when computing indent amounts. Mitigation: `feedRawLines` already strips ANSI; centering uses `stripANSI` lengths.
- **Grid cursor state** → Two new fields (`agentsGridRow`, `agentsGridCol`) are added to the Model struct. They must be clamped whenever the agents list changes length. Mitigation: clamp in the same place the signal board cursor is currently clamped.

## Open Questions

- Should the agents grid show a "selected agent" detail pane on enter/space, or is that out of scope for this change? (Assumed out of scope for now — just navigation.)
- Should the activity feed right column scroll independently with its own `j`/`k` keys when focused, or inherit the existing feed scroll keys? (Assumed: same existing feed focus + scroll model, just narrower width.)
