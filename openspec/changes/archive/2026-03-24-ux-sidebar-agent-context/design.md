## Context

The sidebar (`internal/sidebar/sidebar.go`) is a standalone bubbletea process launched in a dedicated tmux pane. It queries tmux directly for window list and has no awareness of what agents are doing. The chatui package already captures token counts in `StreamDone{InputTokens, CacheTokens, OutputTokens}` but never publishes them anywhere. The bus is a gRPC pub/sub server whose address is known only to bootstrap-launched processes — the sidebar has no way to connect to it today. Bootstrap (`internal/bootstrap/bootstrap.go`) owns all tmux key bindings including the backtick leader and the ESC capture.

## Goals / Non-Goals

**Goals:**
- Leader key changed to `ctrl+;` (`C-\;` in tmux); all chord bindings updated accordingly
- Global ESC intercept removed — ESC passes through to pane content unmodified
- Sidebar UI replaced with agent context panel: per-session provider name, running/idle status, token counts, cost estimate
- Sidebar is togglable: hidden by default, `ctrl+; t` shows/hides it
- New-session picker and prompt-builder hints visible in tmux status bar right-side
- Telemetry data (tokens, cost) flows from chatui → bus → sidebar via a new `orcai.telemetry` bus topic
- Bus address persisted to `~/.config/orcai/bus.addr` so the sidebar process can connect at startup

**Non-Goals:**
- Per-message cost breakdown or invoicing
- Streaming token counts mid-response (only on StreamDone)
- Sidebar plugin system (sidebar remains a standalone process, not a go-plugin)
- Mouse support for status bar buttons

## Decisions

**1. Bus address via file (`~/.config/orcai/bus.addr`)**
Options: (a) env var in sidebar's tmux pane, (b) well-known file, (c) fixed port. Env vars don't survive `tmux split-window` subshell reliably across configurations. A well-known file is simple, already used by other tools (e.g. ssh-agent). Fixed port risks collisions. → chosen: file.

Bootstrap writes the address after `bus.Listen()`. Sidebar reads it on startup and retries for up to 3 seconds if the file isn't present yet.

**2. Telemetry topic: `orcai.telemetry`**
One topic, structured payload: `{"session_id":"...", "provider":"claude", "status":"done", "input_tokens":1200, "output_tokens":89, "cost_usd":0.004}`. The sidebar subscribes once and merges incoming events into its session map by session_id. Status events (`"status":"streaming"`) are published when a stream starts; `"status":"done"` when `StreamDone` fires.

Alternatively, per-session topics (`orcai.telemetry.<id>`) avoid all-session fan-out but require the sidebar to re-subscribe whenever new sessions appear. A single topic is simpler.

**3. Cost estimation in chatui, not sidebar**
Cost depends on model name (known to chatui at stream time). Calculating in chatui and publishing the result keeps the sidebar dumb. Rates table is a `map[string][2]float64` (input, output $/MTok) — hardcoded for Claude models initially.

**4. Sidebar toggle via `ctrl+; t` chord + marker file**
`tmux kill-pane -t .0` hides the sidebar; `orcai _sidebar` in a split respawns it. A marker file at `~/.config/orcai/.sidebar-visible` tracks state. Bootstrap writes it to `false` on session start (hidden by default). The `t` chord checks the file, toggles, and runs the appropriate tmux command.

**5. Status bar session controls as text hints (no clickable buttons)**
Tmux status bar supports `#(command)` dynamic content but not interactive buttons without mouse bindings. We add human-readable hints: `ctrl+; n  new   ctrl+; p  build` on the right side. No mouse binding needed — the chord still works.

**6. `ctrl+;` as leader: `C-\;` in tmux syntax**
Semicolon is a tmux command separator, so it must be escaped as `\;` in bind-key strings. The full binding: `bind-key -n C-\; switch-client -T orcai-chord`. All existing `orcai-chord` bindings are unchanged; only the entry key changes.

## Risks / Trade-offs

- **Bus address file race** — if the sidebar spawns before bootstrap writes `bus.addr`, the sidebar gets no telemetry until it retries. → 3-second retry loop with 250ms sleep.
- **Cost rates go stale** — hardcoded model pricing will drift. → Rates are in one `var` block; easy to update. A future change can pull from a config file.
- **Sidebar hidden by default** — users who relied on the always-visible window list lose it. → The chord hint in the status bar makes `ctrl+; t` discoverable. The window list is removed in favor of the agent panel; session switching still works via `ctrl+; n`.

## Migration Plan

1. `bootstrap.go`: update `buildTmuxConf` — new leader binding, remove ESC binding, write `bus.addr` file, update status bar format, add toggle chord
2. `internal/sidebar/sidebar.go`: replace model entirely with agent context panel; add bus client subscription
3. `internal/chatui/`: publish `orcai.telemetry` events on stream start and `StreamDone`
4. Wire: existing sessions continue working; sidebar just shows "no data yet" for sessions that haven't produced a StreamDone

Rollback: revert `buildTmuxConf` to restore backtick leader and ESC binding.

## Open Questions

- Should the sidebar show all historical sessions or only currently-open tmux windows? → Only open windows for now (sidebar already knows window list from tmux).
- Should `ctrl+;` conflict with any default terminal bindings? → `ctrl+;` is not bound by default in bash/zsh or common terminal emulators.
