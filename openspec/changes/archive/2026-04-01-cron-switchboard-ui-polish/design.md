## Context

Five independent UI polish items span two TUI components (Switchboard and Cron TUI) and one shared subsystem (busd event bus):

1. **Switchboard theme picker** — `handleThemePicker` in `theme_picker.go` discards the `selected` bundle from `tuikit.HandleThemePicker` (assigned to `_`). Crontui's equivalent already captures it and assigns `m.bundle` immediately, giving real-time preview. Switchboard needs the same fix.

2. **Cron TUI header bar** — Switchboard has `viewTopBar` rendering a full-width ORCAI title row. Crontui has no equivalent; its `View()` goes directly to the two panes.

3. **Padding row** — Both TUIs should have a single blank row between the top header bar and the first panel, matching each other visually.

4. **Inbox/cron panel rename propagation** — When `confirmEdit` in crontui saves a renamed entry it only calls `cron.WriteEntry` and reloads locally. It does not publish a busd event, so Switchboard's cron panel (which reads `LoadConfig()` on every render) stays stale until the next `WindowResizeMsg` or other forced re-render. More importantly, there is no mechanism for the Switchboard to know a rename occurred so it can show a refresh badge or re-read the config proactively.

5. **"p pipeline" shortcut** — No way to jump from a cron job to its pipeline file. Pressing `p` on a selected job should open the pipeline YAML in `$EDITOR`.

## Goals / Non-Goals

**Goals:**
- Real-time theme preview in Switchboard (parity with Cron TUI)
- Cron TUI gets the same ORCAI header bar as Switchboard
- One blank padding row below header bar in both TUIs
- Renaming a cron entry publishes a busd event so all consumers can react
- `p` key in crontui cron jobs pane opens the pipeline YAML in `$EDITOR`

**Non-Goals:**
- Retroactively updating `store.Run` names for historical runs
- Refactoring the shared header component (keep each TUI self-contained for now)
- Adding pipeline-open shortcut to Switchboard's cron panel (future work)

## Decisions

### D1: Fix live theme preview by capturing `selected` in Switchboard's `handleThemePicker`

`tuikit.HandleThemePicker` already returns the selected bundle as its third return value. Crontui captures it; Switchboard throws it away. The fix is one line: replace `_` with a named variable and assign `m.bundle`. No new architecture needed.

**Alternative considered**: propagate via busd `ThemeChangedMsg`. Rejected — adds unnecessary round-trip latency; direct assignment is already the pattern in crontui.

### D2: Add `viewTopBar` to crontui — copy rather than share

Switchboard's `viewTopBar` is a method on `switchboard.Model` and uses `m.activeBundle()` / `translations.GlobalProvider()`. Crontui uses `m.bundle` directly. Rather than extracting a shared helper (which would require a new `tuikit` or `panelrender` function and touching multiple files), copy the implementation into `crontui/view.go` adapted for `m.bundle`. The two bars are visually identical; divergence risk is low.

**Alternative considered**: extract to `panelrender.TopBar(bundle, title, width)`. Reasonable but scope-creep for this change; can be done in a follow-up.

### D3: Insert padding row by adjusting height budget

In crontui `View()`, compute `contentH := m.height - 1 - 1` (1 for topBar, 1 for padding row) before calling `splitHeight`. Render the padding row as a plain `strings.Repeat(" ", m.width)` line joined between topBar and panels.

In Switchboard, insert a blank line between `topBar` and `body` in the view function using a single `"\n"` join.

### D4: Publish `cron.entry.updated` busd event after rename

Add topic constant `CronEntryUpdated = "cron.entry.updated"` (or reuse existing pattern). In `crontui/update.go:confirmEdit`, after `cron.WriteEntry` succeeds and the name changed, call `publisher.Publish(ctx, topics.CronEntryUpdated, payload)` with `{"old_name": ..., "new_name": ...}`.

Switchboard's `Update` already handles many busd messages. Add a case for `CronEntryUpdated` that triggers a `tea.Cmd` to reload — since `filteredCronEntries()` already calls `LoadConfig()` on every render, the Switchboard just needs a no-op message to force a re-render cycle (similar to how other busd events work).

**Alternative considered**: file-system watcher on the cron YAML. Rejected — heavier weight, requires a new goroutine, and busd is already the established IPC channel.

### D5: Open pipeline file with `tea.ExecProcess`

When `p` is pressed in the cron jobs pane, resolve the pipeline YAML path (same logic used in other pipeline-launch flows: search `~/.config/orcai/pipelines/<target>.yaml` and the XDG paths). Open with `$EDITOR`, falling back to `vi`. Use `tea.ExecProcess` so BubbleTea suspends and restores the TUI cleanly.

## Risks / Trade-offs

- **Cron TUI height budget** — Adding topBar + padding row reduces available panel height by 2 rows. The existing `splitHeight` caps the jobs pane at 14 rows, so on small terminals the log pane may get squeezed. Mitigation: enforce a minimum of 4 rows for the log pane inside `splitHeight`.
- **busd availability in crontui** — crontui may run standalone without a live bus. The `publisher.Publish` call must be a no-op when no bus is connected (the existing scheduler pattern already handles this with `_ =` ignore).
- **`$EDITOR` not set** — Fall back to `vi`; document in the hint bar as `p pipeline`.

## Migration Plan

No database or config schema changes. All changes are in-process Go code. Deploy by rebuilding the binary; no rollback steps needed.

## Open Questions

- Should the padding row respect the theme background color (filled with theme BG) or be a blank line? Blank line is simpler and consistent with existing inter-panel gaps.
