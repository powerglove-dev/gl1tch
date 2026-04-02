## Why

The main TUI entry point is named "switchboard" — a passive telephony metaphor that undersells what it is: the primary command interface into gl1tch. Separately, OTel tracing was just wired across the execution stack, but traces currently write to stdout where they corrupt the feed with raw JSON and are invisible to the operator at runtime. Both gaps undermine the daily-driver experience: one is a naming inconsistency that leaks into every file and conversation, the other is instrumentation that exists but provides zero UX value.

## What Changes

- **BREAKING** Rename `switchboard.go` → `deck.go` and all internal references from "switchboard" to "deck" across `internal/console/`; the `console` package name is unchanged
- Redirect OTel trace output from stdout to `~/.glitch/traces.jsonl` (file exporter, not stdout) so the TUI is never corrupted
- Wire OTel span lifecycle events into the feed's existing `StepInfo` / `StepStatusMsg` machinery so step timing appears inline in feed entries
- Add a `/trace` slash command that opens a detail view for the most recent (or selected) run's trace — span tree, durations, status

## Capabilities

### New Capabilities

- `console-deck-rename`: Rename the switchboard concept to "deck" throughout `internal/console/` — file, type references, comments, and any external callers
- `otel-trace-feed`: Wire OTel span events into the feed UX — file exporter, step timing in `StepInfo`, and `/trace` detail view

### Modified Capabilities

- `feed-step-output`: Feed entries gain per-step timing (duration_ms) sourced from OTel span attributes alongside existing step status

## Impact

- `internal/console/switchboard.go` → renamed; all `*_panel.go`, `*_test.go`, and `cmd/` files that reference the type updated
- `internal/telemetry/telemetry.go` — exporter swapped from stdout to JSONL file writer
- `internal/console/switchboard.go` (deck) — `Update()` gains a `TraceSpanMsg` handler; `feedEntry.steps` gains `DurationMS`
- `internal/pipeline/runner.go`, `internal/orchestrator/conductor.go` — span completion events published as `tea.Msg` via existing BUSD/channel path
- No new external dependencies; OTel SDK already in `go.mod`
