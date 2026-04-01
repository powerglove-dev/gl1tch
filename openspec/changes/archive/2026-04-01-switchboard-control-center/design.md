## Context

The current UX has three separate screens: the welcome dashboard (ANSI art + keybinding cheatsheet), the sysop panel (live session monitor, three-column BBS layout), and the pipeline/prompt builder (accordion step editor). All three are launched from different keybindings or subcommands. The shell (`c` key) is the primary way to actually run a pipeline or fire an agent, which is backwards — the TUI should be the launchpad, not the shell.

The sidebar/sysop package (`internal/sidebar/`) already has the right skeleton: a BubbleTea model, live session telemetry via busd, a three-column layout, and keybinding handling. The welcome package (`internal/welcome/`) provides the ANSI art header and keybinding cheatsheet but is otherwise just a key dispatcher. The promptbuilder package is a full accordion-step editor — useful for authoring, but too heavy as an operational control.

The switchboard merges these two live-pane concepts and adds active launch controls, without carrying over the promptbuilder's editing complexity.

## Goals / Non-Goals

**Goals:**
- Single full-screen entry point replacing both `orcai welcome` and `orcai sysop`.
- Left column: two sections — Pipeline Launcher (list of saved `.pipeline.yaml` files) and Agent Runner (provider + model picker + inline prompt input).
- Center/main area: Activity Feed — real-time output from pipeline runs and agent invocations, with status badges and timestamps.
- Bottom bar: compact keybinding reference (single line or two-line strip), replaces the welcome cheatsheet.
- `orcai welcome`, `orcai sysop`, `orcai-welcome`, `orcai-sysop` all launch the switchboard.
- Pipeline runs execute in-process (via `pipeline.Run`) with output streamed to the activity feed via a tea.Cmd goroutine.
- Agent invocations are single-step pipelines: selecting a provider/model/prompt creates an in-memory `pipeline.Pipeline` with one plugin step and runs it through `pipeline.Run` — the same path as saved pipelines.
- Provider and model lists come from `picker.BuildProviders()` (sidecar-aware).

**Non-Goals:**
- Pipeline authoring (the promptbuilder stays for `orcai pipeline build` but is not a switchboard entry point).
- Replacing the tmux session workflow — the switchboard lives inside an existing pane; sessions and worktrees still use tmux.
- Removing the `orcai pipeline build` subcommand.
- Multi-pipeline concurrency (v1 runs one job at a time; a running job disables the launcher).

## Decisions

### Decision: Extend sidebar.Model rather than create a new package

**Chosen**: Rename `internal/sidebar/` to `internal/switchboard/` and expand the existing `Model`. The sidebar already has busd subscription, session telemetry, BBS-style three-column rendering, and keybinding wiring. Adding the launcher column and activity feed is additive.

**Rationale**: Less code churn; the rendering patterns (lipgloss columns, ANSI color constants, tick-based refresh) are already proven. A from-scratch package would duplicate all of this.

**Alternative considered**: New `internal/switchboard/` package alongside sidebar. Rejected — creates a period where both exist and nothing uses sidebar; cleaner to rename in place.

---

### Decision: Activity Feed is a ring buffer of `feedEntry` structs rendered in the center column

**Chosen**: Center column holds a `[]feedEntry` (capped at N entries, newest-at-top). Each entry has a timestamp, status badge (`▶ running`, `✓ done`, `✗ failed`), title (pipeline name or "agent: <provider>/<model>"), and a `[]string` output lines. The selected entry expands to show output lines; others show one summary line.

**Rationale**: The sysop panel already renders a session list with selection. Extending this pattern to a general activity feed is familiar territory. Ring buffer prevents unbounded memory growth for long-running switchboard sessions.

**Alternative considered**: Streaming all output to a scrollable viewport widget. Rejected for v1 — adds scroll state complexity; the selected-entry expansion is sufficient for typical pipeline/agent outputs.

---

### Decision: Pipeline runs use a tea.Cmd that streams output via a channel

**Chosen**: Launching a pipeline dispatches a `tea.Cmd` that runs `pipeline.Run(...)` in a goroutine, sending `feedLineMsg{id, line}` messages back to the BubbleTea update loop as lines arrive. The pipeline `Publisher` interface is implemented as a channel-backed type that converts step events to `feedLineMsg`.

**Rationale**: BubbleTea programs cannot block Update; all I/O must be in Cmds. The existing `pipeline.NoopPublisher` interface shows the extension point. A `ChanPublisher` sends events through a `chan tea.Msg`.

**Alternative considered**: Writing output to a temp file and polling. Rejected — adds latency and file I/O overhead for no benefit.

---

### Decision: Agent runs are single-step pipelines

**Chosen**: There is no separate "agent runner" code path. Selecting a provider/model and entering a prompt creates an in-memory `pipeline.Pipeline` with one step (`executor: <providerID>`, `model: <modelID>`, `prompt: <input>`) and passes it to `pipeline.Run` — the exact same function used for saved YAML pipelines. The switchboard's Quick Run form is only responsible for building that `pipeline.Pipeline` struct; the execution infrastructure is shared.

**Rationale**: Eliminates an entire parallel execution path (`CliAdapter.Execute` goroutine + custom line writer). Everything goes through `pipeline.Run`, which already handles env var injection, var interpolation, builtins, error propagation, and the Publisher interface. A single unified path is less code, less surface for bugs, and easier to extend (e.g. adding retry or timeout applies to both saved and ad-hoc runs automatically).

**Alternative considered**: Separate `runAgentCmd` using `CliAdapter.Execute` directly. Rejected — duplicates execution logic already in the pipeline runner.

---

### Decision: Quick Run form uses inline three-step selection in the left column

**Chosen**: The left column has two sections (Saved Pipelines, Quick Run). The Quick Run section presents an inline three-step form: (1) provider dropdown, (2) model dropdown (skip if provider has no models), (3) text input for the prompt. Tab/Enter advances; Esc cancels. On submit, an in-memory single-step pipeline is built and handed to `launchPipelineCmd`.

**Rationale**: Matches the picker's existing dropdown pattern (`internal/picker/dropdown.go`). Reuses `picker.BuildProviders()` so sidecar plugins are automatically included. Inline form avoids pop-up overlay complexity.

**Alternative considered**: Pop-up modal overlay. Rejected — harder to implement in BubbleTea without a dedicated overlay library.

---

### Decision: welcome.go and sidebar.go both delegate to the new switchboard package

**Chosen**: `internal/welcome/welcome.go` is rewritten to be a thin shim that calls `switchboard.Run()`. `internal/sidebar/sidebar.go` is moved to `internal/switchboard/switchboard.go` (or sidebar is kept but aliased). Both `cmd/sysop.go` and `cmd/welcome.go` call the same `switchboard.Run()`.

**Rationale**: Zero-duplication entry points. Both binaries (`orcai-welcome`, `orcai-sysop`) and both subcommands end up at the same TUI.

## Risks / Trade-offs

- **BubbleTea re-render cost with live output** → Pipeline output can be high-frequency. Mitigation: throttle `feedLineMsg` delivery with a 50ms debounce or batch lines into `[]string` slices per tick.
- **Provider/model list is computed at startup** → If a new sidecar is installed while the switchboard is open, it won't appear. Mitigation: add a refresh keybinding (`R`) that re-calls `BuildProviders()`.
- **Single active job per v1** → A long-running pipeline blocks the launcher. Mitigation: show a `[running]` badge and disable launch controls; document multi-job as a v2 item.
- **Rename of internal/sidebar/** → Any external import paths break. Mitigation: orcai has no external consumers of internal packages; safe to rename.
