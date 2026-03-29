# Agent Modal: use_brain Toggle + Top Border Fix

## Summary

Two changes to the AGENT modal in the switchboard:

1. **Fix top border shifting left** — `OverlayCenter` can place the modal at `startRow=0`, merging the `┌─── AGENT ───┐` border with the ORCAI title bar and making it appear left-shifted.
2. **Add `use_brain` toggle** — a checkbox row in the modal so agent runner one-shots and scheduled runs can inject brain context into the prompt.

---

## Fix 1: Top Border (panelrender.OverlayCenter)

**Root cause:** When `popH >= h`, `startRow = pmax((h-popH)/2, 0) = 0`. The agent modal's top border row overwrites `baseLines[0]` (the ORCAI title bar). The title bar's `─` dashes visually extend the `┌─── AGENT ───┐` border to the left.

**Fix:**
- Clamp `startRow` to a minimum of 1: `startRow := pmax((h-popH)/2, 1)`
- Add row clipping in the render loop: `if row >= h { break }`

This affects only modals tall enough that `(h-popH)/2 < 1` — in practice only the agent modal. All other modals (quit, help, delete, pipeline) are short enough that `startRow` is already ≥ 1.

---

## Feature: use_brain Toggle

### Model

Add `agentUseBrain bool` to `Model` (alongside `agentCWD`, `agentScheduleErr`). Not reset on modal close — preserves the user's last choice.

### Focus States (5 → 6)

| Focus | Section |
|-------|---------|
| 0 | PROVIDER |
| 1 | MODEL |
| 2 | PROMPT |
| **3** | **USE BRAIN (new)** |
| 4 | WORKING DIRECTORY |
| 5 | SCHEDULE |

Tab cycling: `% 6` (was `% 5`). Shift-tab backward wrap: `+ 5` (was `+ 4`).

### Rendering

A single checkbox row rendered after the PROMPT textarea and before the WORKING DIRECTORY spacer:

```
  [ ] use brain context      (agentUseBrain=false, any focus)
  [x] use brain context      (agentUseBrain=true, any focus)
```

The row is styled with the accent color when focused (focus==3), dim otherwise.

### Key Handling

- **Focus 3 (use_brain):** space or enter toggles `agentUseBrain`; other keys ignored.
- **Focus 4 (cwd):** enter opens dir picker (unchanged, was focus 3).
- **Focus 5 (schedule):** textarea key forwarding (unchanged, was focus 4).
- **up/down pass-through:** update condition from `focus==4` to `focus==5` (schedule textarea).

### Immediate-Run Path (submitAgentJob)

After `storeRunID` is recorded, if `agentUseBrain && m.store != nil`:

```go
inj := pipeline.NewStoreBrainInjector(m.store)
if preamble, err := inj.ReadContext(context.Background(), jh.storeRunID); err == nil && preamble != "" {
    input = preamble + "\n\n" + input
}
```

This uses the same `StoreBrainInjector` and `ReadContext` logic as the pipeline executor.

### Scheduled-Run Path (writeSingleStepPipeline)

Add a `useBrain bool` parameter. When true, emit `use_brain: true` in the step YAML:

```yaml
steps:
  - id: run
    executor: opencode
    model: llama3.2
    use_brain: true
    prompt: |
      ...
```

The cron-launched pipeline then runs through the standard pipeline executor, which handles brain injection natively.
