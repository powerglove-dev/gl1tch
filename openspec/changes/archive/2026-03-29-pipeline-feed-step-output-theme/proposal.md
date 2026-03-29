## Why

Four connected gaps make the pipeline Activity Feed unreliable and visually inconsistent: the `not_empty` condition used by `assert-reviewed` is unimplemented (always fails), step status colors ignore the active theme, step output is never routed to the per-step display, and failed inbox items carry no visual urgency marker.

## What Changes

- **Add `not_empty` to `EvalCondition`** — `condition: "not_empty"` falls through to `default: return false`; the `opencode-code-review` pipeline's `assert-reviewed` step always fails because of this.
- **Theme-aware step colors in Activity Feed** — the "running" step badge uses hardcoded `aYlw` (ANSI yellow); add a `Warn` field to `ANSIPalette`, populate it from a theme-derivation strategy, and use `pal.Warn` in step rendering.
- **Populate `step.lines` with step output** — `StepInfo.lines` is declared and rendered but never written; extract the `output.value` from `StepDone` busd events and store the last N lines so output appears under each step badge.
- **Inbox "needs attention" flag for failed runs** — failed inbox items look identical to successful ones except for dot color; add a visible `⚠` or `!` attention marker on failed runs.

## Capabilities

### New Capabilities

- `feed-step-output`: Per-step output lines rendered in the Activity Feed beneath each step badge.
- `feed-step-theme-colors`: Step badge colors derived from the active theme palette rather than hardcoded ANSI constants.
- `inbox-attention-flag`: Visual "needs attention" indicator on failed inbox items.

### Modified Capabilities

- `pipeline-step-lifecycle`: `EvalCondition` extended with `not_empty` condition; the `assert` builtin now correctly validates non-empty output.

## Impact

- `internal/pipeline/condition.go` — add `not_empty` case
- `internal/styles/styles.go` — add `Warn` field to `ANSIPalette`, populate in `BundleANSI`
- `internal/switchboard/switchboard.go` — use `pal.Warn` for "running" step color
- `internal/switchboard/pipeline_bus.go` — parse `StepDone` output payload and store in `step.lines`
- `internal/inbox/model.go` — add attention flag to failed `item.Description()`
