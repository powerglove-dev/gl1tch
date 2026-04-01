## Context

The switchboard currently has four panels (Pipelines, Agent Runner, Inbox, Activity Feed) plus a Signal Board view. Cron jobs are managed exclusively via `cron.yaml` and the bare `orcai cron run` daemon—there is no interactive surface. The `orcai-cron` tmux session exists but hosts only a log stream, not a TUI.

`internal/cron` already has `LoadConfig`, `Entry`, and `Scheduler`. `robfig/cron` is already a dependency and exposes `Entry.Next` (next scheduled fire time). `sahilm/fuzzy` is already used by the Signal Board.

## Goals / Non-Goals

**Goals:**
- Add a `c`-keyed Cron panel to the switchboard showing the next N upcoming scheduled jobs (read-only summary).
- Add `m` shortcut in that panel to switch the active tmux window to `orcai-cron`.
- Build a `cmd/orcai-cron/` BubbleTea TUI that runs inside the `orcai-cron` tmux session and wraps the scheduler in-process.
- TUI: split-pane layout — top = job list with fuzzy filter, bottom = live log stream using `charmbracelet/log` with job-name prefix.
- TUI: `e` edit (form overlay), `d` delete (confirm dialog), `enter` / `r` run-now.
- TUI reads and applies the active ABBS theme bundle (Dracula palette).
- `orcai cron start` launches the TUI binary instead of the bare daemon.

**Non-Goals:**
- Creating new cron entries from the orcai-cron TUI (creation stays in the switchboard modal for now).
- Editing `cron.yaml` outside of the TUI's own overlay.
- Real-time log streaming from outside the TUI process (log tail is in-process only).

## Decisions

### D1 — TUI embeds the scheduler (single process)
**Decision:** `cmd/orcai-cron/main.go` creates a `cron.Scheduler` and drives it from within the BubbleTea model's `Init`/`Update` loop.

**Rationale:** Tight coupling between live job state and UI makes it trivial to push "job started / completed" events directly to the model without IPC. The alternative—a separate daemon + TUI that poll a file or socket—adds complexity with no benefit at this scale.

**Alternative considered:** Separate daemon process + TUI reading a shared log file. Rejected: file-tailing in BubbleTea requires a polling goroutine and adds latency; direct event injection is simpler.

### D2 — Log viewer uses an in-memory ring buffer fed by a BubbleTea `tea.Cmd`
**Decision:** A `LogSink` type implements `io.Writer` and posts `logLineMsg` messages to the BubbleTea runtime via a channel. The TUI model accumulates lines in a fixed-size ring buffer (default 500 lines). `charmbracelet/log` is configured with the job name as a prefix field.

**Rationale:** `charmbracelet/log` already produces ANSI-colored structured output. Prefixing the job name per-line gives operators at-a-glance attribution. The ring buffer avoids unbounded memory growth.

**Alternative considered:** Write logs to a temp file and `tail -f` it. Rejected: adds a disk dependency; in-memory is sufficient for a session-scoped daemon.

### D3 — Fuzzy search reuses `sahilm/fuzzy` (same as Signal Board)
**Decision:** The job list in the TUI top pane uses the same `fuzzy.FindFrom` pattern as `signal_board.go`, matching against entry `Name`.

**Rationale:** Consistency with existing code; no new dependency.

### D4 — Switchboard Cron panel is a lightweight read-only widget
**Decision:** The switchboard panel calls `cron.LoadConfig()` + `robfig/cron`'s parser to compute next run times and renders a compact list (same ANSI box style as other panels). It does not hold a live `Scheduler`.

**Rationale:** The switchboard is already complex; keeping it read-only avoids duplicate scheduler state. All mutations go through the `orcai-cron` TUI.

### D5 — Edit overlay uses a multi-field form (name, schedule, kind, target, timeout)
**Decision:** The edit overlay is a BubbleTea component with five `textinput` fields, pre-populated from the selected entry, committed on `enter`. It calls `cron.WriteEntry` (replacing the existing entry by name) and triggers a scheduler reload.

**Rationale:** The CRUD surface is contained; a bespoke form is simpler than importing a full form library.

### D6 — Theme awareness via active bundle loader
**Decision:** The TUI calls the existing switchboard `LoadBundle` / `LoadActiveBundle` function to obtain Dracula palette colors and applies them to lipgloss styles at startup (and on `tea.WindowSizeMsg`).

**Rationale:** Keeps theme logic in one place; the TUI gets ABBS visual coherence for free.

### D7 — `orcai cron start` launches the TUI
**Decision:** `cronStartCmd` resolves the binary path and passes `cron tui` (a new hidden subcommand) as the tmux session command. The bare `cron run` daemon command is kept as a fallback for headless/CI contexts.

**Rationale:** Users in a terminal context get the full TUI automatically; `cron run` still works for scripted use.

## Risks / Trade-offs

- **Scheduler restart on edit**: When an entry is edited, the scheduler does a full reload (remove all + re-register). If a job is mid-run during reload, it completes normally (robfig/cron handles this safely). → Mitigation: reload is already implemented in `Scheduler.reload()`; no change needed.
- **TUI binary path in tmux**: `cron start` resolves `os.Executable()` for the binary path; if orcai is invoked via `go run` in development, the tmp path changes between runs. → Mitigation: document that `make install` is required for session-launch; dev mode falls back to `cron run`.
- **Bottom log pane height**: Fixed 40%/60% split. Very small terminals (< 20 rows) may make one pane unusable. → Mitigation: enforce a minimum height of 6 rows per pane; collapse log pane below threshold.
- **No persistence across TUI restarts**: The in-memory ring buffer is lost when the TUI exits. → Mitigation: `charmbracelet/log` already writes to the tmux pane scrollback; users can scroll back in tmux.

## Migration Plan

1. Add `NextRun` helper to `internal/cron/scheduler.go` (additive, no API break).
2. Build `cmd/orcai-cron/` TUI; gated behind `orcai cron tui` subcommand initially.
3. Update `cmd/cmd_cron.go` `cron start` to invoke `cron tui` instead of `cron run`.
4. Add switchboard Cron panel (`c`/`m` keys) to `internal/switchboard/switchboard.go`.
5. Update bottom-bar hint strip to include `c`.

Rollback: revert `cron start` to call `cron run`; remove switchboard panel block. No data migrations required (`cron.yaml` schema unchanged).

## Open Questions

- Should the TUI show the last-run status (pass/fail) per job in the job list? The store already tracks this — worth surfacing if the extra column fits.
- Should `cron tui` become the default (replacing `cron run` entirely) or remain an alias? Decision deferred to implementation.
