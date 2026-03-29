## Context

The `opencode-code-review` pipeline fails at `assert-reviewed` because `EvalCondition` doesn't implement `not_empty`. The Activity Feed renders per-step status badges but two gaps make them incomplete: "running" uses hardcoded ANSI yellow instead of the theme palette, and `StepInfo.lines` is never populated so no step output appears beneath the badge. Inbox items distinguish failure only by dot color, which is easy to miss.

All four issues touch the same user-visible surface (the pipeline execution feedback loop) and share a single root cause: the feed/inbox subsystems were built incrementally and some wiring was never completed.

## Goals / Non-Goals

**Goals:**
- `not_empty` condition works in `EvalCondition` so `assert-reviewed` (and any future assert using it) passes correctly
- Step badge color for "running" state derives from the active theme palette
- Step output from `StepDone` busd event appears as lines beneath the step badge in the Activity Feed
- Failed inbox items display a visible `⚠` attention marker

**Non-Goals:**
- Streaming per-step output in real time (we use the StepDone snapshot, not a live tail)
- Adding a dedicated `warn` color slot to the 55 theme YAML files
- Reworking the inbox list model beyond the attention flag

## Decisions

### 1. `not_empty` in `EvalCondition` — simple string case

Add `case expr == "not_empty": return strings.TrimSpace(output) != ""` to the switch in `condition.go`. This is unambiguous and matches the semantics implied by the existing `len > N` case.

**Alternative considered**: A generic expression evaluator. Rejected — massively over-engineered for one missing keyword.

### 2. Theme-aware "running" color — derive from palette, no new YAML field

`ANSIPalette` gains a `Warn` field. `BundleANSI` populates it by blending the theme's FG toward the accent (or simply using FG), giving a neutral highlight that contrasts with both Success and Error regardless of theme. The fallback (no bundle) keeps `aYlw`. This avoids touching all 55 theme YAML files.

**Alternative considered**: Adding a `warn` key to `Palette` and all theme YAMLs. Rejected — high churn, themes don't need a semantic warning color for any use case beyond this one.

**Derivation strategy**: Use the FG color (usually near-white). "Running" is not an error state; a neutral-bright color communicates "in progress" without alarming the user. The hardcoded yellow was already a compromise.

### 3. Step output — populate from `StepDone` busd payload

`handlePipelineBusEvent` for `topics.StepDone` already decodes `run_id` and `step`. Extend that decode to also parse `output.value` (a string). Split on newlines, take the last 5 lines, store in `step.lines` via a new helper `appendStepLines`. No format changes to the pipeline runner or log watcher.

**Alternative considered**: Tagged log lines `[step:<id>] output:<line>` emitted by the runner. Rejected — requires format changes in the runner, adds latency (log watcher polls at 150ms), and duplicates data already in the StepDone event.

**Limitation**: Only the final snapshot of step output is available, not a live stream. This is acceptable for v1; real-time streaming can be added later via a dedicated bus topic.

### 4. Inbox attention flag — augment `item.Description()`

`item.Description()` prepends a `⚠` marker (rendered in the error palette color) when `run.ExitStatus != nil && *run.ExitStatus != 0`. This is the least-invasive change point and keeps the flag visible in the list without altering the `Title()`.

## Risks / Trade-offs

- **Step output truncation**: We store only the last 5 lines from `StepDone`. Long outputs are silently truncated. → Acceptable for an overview; users can open the tmux window for full scrollback.
- **StepDone-only output**: Steps that produce no `StepDone` event (crash, cancel) show no output. → Mitigated by existing "settle running steps" logic that marks them failed.
- **`Warn` = FG color**: For light-mode themes, FG may be dark and barely visible against the background. → Acceptable; all current themes are dark-mode Dracula variants. Can be revisited when a light theme ships.
