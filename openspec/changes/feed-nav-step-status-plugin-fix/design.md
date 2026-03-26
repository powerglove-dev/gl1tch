## Context

The Switchboard TUI has three focusable sections: Pipeline Launcher, Agent Runner, and Activity Feed. Tab cycles focus between sections, but the Activity Feed has only partial focus support — it accepts `f` to enter per-entry expanded view but has no line-by-line navigation. Users cannot page through long output, jump to the top/bottom, or select specific lines without leaving to a tmux window.

Pipeline output is captured as unstructured log lines via a log-file watcher. The `pipeline.Run` call ultimately calls `runDAG` which accepts an `EventPublisher` but passes `_ = publisher` — step lifecycle events are never emitted. The switchboard therefore has no way to know which step is running or what its status is.

The "ollama already registered" warning is caused by `buildProviders()` in `internal/picker/picker.go` populating the `Providers` slice copy without setting `SidecarPath` from the discovery layer. The `pipelineRunCmd` sidecar-skip guard (`if prov.SidecarPath != ""`) therefore never fires for ollama, so it registers the plugin manually and then `LoadWrappersFromDir` registers it again.

## Goals / Non-Goals

**Goals:**
- Tab focus lands on the Activity Feed; j/k, arrow keys, PgUp/PgDn, and g/G navigate within it.
- A visible selection cursor tracks the focused line.
- Each feed entry shows an expandable step list with live `pending / running / done / failed` badges.
- Pipeline runner emits structured step-status log lines that the log-watcher can parse.
- `buildProviders` propagates `SidecarPath` to static-provider entries; "already registered" warning disappears.

**Non-Goals:**
- Rewriting the log-watcher to use a structured event bus (out of scope for this change).
- Full text search or filtering within the feed.
- Editing or replaying a pipeline from within the feed.

## Decisions

### 1. Step status via structured log lines, not a side-channel

**Decision**: The pipeline runner prints `[step:<id>] status:<state>` to stdout. The log-watcher (already reading the log file line by line) parses lines matching this pattern and sends a new `StepStatusMsg` into the BubbleTea channel.

**Alternatives considered**:
- *Bus events*: The `EventPublisher` interface exists but the bus is not wired in the `orcai pipeline run` subprocess. Wiring it would require IPC between the subprocess and the TUI, significantly more scope.
- *Polling the pipeline YAML*: The feed would need to reload YAML and cross-reference run state; brittle and racy.

**Rationale**: The log file is already the single source of truth for the feed. Piggybacking structured annotations on it keeps the change self-contained and reversible.

### 2. Feed navigation uses the same focus model as other panels

**Decision**: `feedFocused` already exists. When focused, the feed intercepts `j`/`k`/`↑`/`↓`, `PgUp`/`PgDn`, `g`/`G` and moves a `feedCursor` (line index within the currently expanded entry). Tab advances focus through panels in the existing cycle.

**Alternatives considered**:
- *Entry-level cursor* (select which pipeline run to view): Useful but the existing `feedSelected` already tracks this; the user request was specifically about line navigation within the feed.

### 3. SidecarPath fix is a one-liner in `buildProviders`

**Decision**: After the static-Providers loop adds an entry to `out`, copy the `SidecarPath` from the corresponding `extras` entry (keyed by `p.ID`) into the `ProviderDef` before appending to `out`.

**Alternatives considered**:
- Fix the skip guard in `pipelineRunCmd` to use name-based lookup instead of SidecarPath — more surgical but treats the symptom, not the root cause.

## Risks / Trade-offs

- [Risk] Log-line parsing is fragile if the runner output format changes → Mitigation: use a stable prefix `[step:<id>] status:<state>` and a documented constant; parser is isolated in `parseStepStatus(line string)`.
- [Risk] High-frequency step output could cause excessive UI redraws → Mitigation: step-status messages are cheap (no screen clear), and the existing 256-deep channel buffer absorbs bursts.
- [Risk] Changing focus behaviour might conflict with existing keyboard shortcuts → Mitigation: feed navigation keys (`j`, `k`, `g`, `G`) are only active when `feedFocused == true`; they do not conflict with the global `f` (filter) or `enter` (open in window) bindings.

## Migration Plan

1. Add `SidecarPath` propagation to `buildProviders` (no migration needed — removes a warning).
2. Add structured step-log emission to `runDAG` (additive; existing pipelines gain step lines in their log).
3. Add `StepStatusMsg` parsing and `feedCursor` navigation to the switchboard (purely additive UI change).
4. Update status-bar hints to reflect new feed navigation keys when feed is focused.
