## Why

The current window 0 "welcome" screen is a one-shot splash — it shows static help text and exits to `$SHELL` on any keypress, leaving window 0 idle and informationless for the rest of the session. Converting it into a live dashboard turns window 0 into a persistent command centre showing what every AI session is doing, how many tokens have been consumed, and what it has cost — without needing to open the sidebar.

## What Changes

- The welcome screen is replaced by a persistent live dashboard that does **not** exit on keypress.
- The dashboard subscribes to the `orcai.telemetry` bus and renders a session card for each active tmux window.
- Each card shows: provider, status (● streaming / ○ idle), input tokens, output tokens, and cost USD.
- An aggregate summary row shows cumulative tokens and cost across all sessions.
- The existing ANSI/BBS banner (`buildWelcomeArt`) is kept and reused as the dashboard header.
- Chord-key hints (`^spc n new · ^spc p build`) replace the "any key continue" footer.
- Enter still opens the provider picker popup.
- `q` / `ctrl+c` quit back to `$SHELL` (current behaviour preserved via `execShell`).
- The static `helpMarkdown` / glamour rendered block is removed.

## Capabilities

### New Capabilities

- `welcome-dashboard`: Live dashboard view in window 0 — session cards with telemetry, aggregate totals, BBS-themed layout, bus subscription, keyboard hints.

### Modified Capabilities

- `agent-context-panel`: The welcome dashboard reads from the same `orcai.telemetry` bus topic and uses the same `SessionTelemetry` data shape — no requirement changes, the sidebar and dashboard are independent consumers.

## Impact

- `internal/welcome/welcome.go` — full rewrite of the BubbleTea model; `Run()` signature unchanged.
- New dependency on `proto/orcai/v1` (EventBus client) and `google.golang.org/grpc` — already in `go.mod`.
- No changes to bootstrap, sidebar, chatui, or picker.
