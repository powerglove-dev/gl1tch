## Context

Window 0 currently runs `welcome.Run()`, which renders a one-shot BubbleTea TUI. The model has no ticker, no bus connection, and quits on any keypress before replacing the process with `$SHELL`. The sidebar (`internal/sidebar`) already implements a persistent live TUI that subscribes to `orcai.telemetry` via gRPC and renders session cards — the dashboard will use the same pattern but rendered full-screen in window 0 rather than a narrow side panel.

The `orcai.telemetry` bus emits two event shapes: `{status: "streaming"}` on stream start and `{status: "done", input_tokens, output_tokens, cost_usd}` on completion. The sidebar's `TelemetryMsg` / `SessionTelemetry` types in `internal/sidebar` already model this correctly.

## Goals / Non-Goals

**Goals:**
- Replace the static splash with a live, always-on dashboard in window 0.
- Display one card per active tmux window with provider, status, token counts, and cost.
- Show an aggregate totals row summing all sessions.
- Preserve the existing ANSI/BBS banner (`buildWelcomeArt`) as the header.
- Keep Enter → picker popup and q/ctrl+c → quit to shell.
- Subscribe to `orcai.telemetry` and update in real time without polling.
- Tick every 5 seconds to refresh the window list from tmux.

**Non-Goals:**
- Historical / persisted cost tracking across sessions (future work).
- Interacting with individual session windows from the dashboard (that's the sidebar's job).
- Replacing or merging with the sidebar — both can coexist.

## Decisions

### D1: Reuse sidebar's telemetry subscription pattern, not its types

The sidebar's `SessionTelemetry`, `TelemetryMsg`, and `subscribeCmd` are in `internal/sidebar` — an internal package. Rather than creating a shared package, the welcome package will duplicate the small structs and `connectBus` / `subscribeCmd` helpers locally. This keeps packages independent and avoids circular imports.

**Alternatives considered:**
- Extract to `internal/telemetry` shared package — deferred; only two consumers so far, not worth the abstraction yet.
- Import sidebar from welcome — rejected, creates coupling between two independent UI packages.

### D2: Dashboard does not exit to shell automatically

The current `Run()` calls `execShell()` after the BubbleTea program ends. The new dashboard only calls `execShell()` when the user explicitly quits (`q` or `ctrl+c`). This keeps window 0 alive as a persistent dashboard rather than dropping to a shell prompt.

**Alternatives considered:**
- Keep the "any key exits" behaviour with a short timeout — rejected, makes the live telemetry useless.

### D3: Session cards rendered with pure ANSI, not glamour/lipgloss

The existing banner (`buildWelcomeArt`) uses raw ANSI escapes and box-drawing characters to stay consistent with the Dracula/BBS aesthetic. Session cards will use the same approach: `╔═╗ ║ ╚═╝` box per card, coloured with the established palette constants (pink, teal, blue, green, yellow, dimT). No lipgloss dependency is introduced.

**Alternatives considered:**
- lipgloss — adds a dependency and diverges from the established raw-ANSI style.

### D4: Layout — stacked cards, two columns when terminal is wide enough

Narrow (< 100 cols): cards stack vertically, full width.
Wide (≥ 100 cols): cards flow in two columns.
Aggregate totals row always spans full width below the cards.

### D5: Aggregate totals computed on every render, not cached

Total input tokens, output tokens, and cost are summed over `sessions` map on each `View()` call. Given the number of sessions is small (< 20), this is negligible.

## Risks / Trade-offs

- **Bus not running at welcome start** → `connectBus()` reads `bus.addr` with a 3-second retry (same pattern as sidebar). If unavailable, dashboard renders with "no data" placeholders and no crash.
- **Window list stale between ticks** → 5-second tick is fast enough for human perception; tmux command overhead is minimal.
- **Large terminal redraws** → BubbleTea handles alt-screen redraws; no special mitigation needed.

## Migration Plan

1. Rewrite `internal/welcome/welcome.go` in place — `Run()` signature is unchanged so `main.go` needs no change.
2. No database migrations, no config changes, no new binaries.
3. Rollback: revert the single file.
