## Context

gl1tch's console layer (`internal/console/`) is a BubbleTea application centered on a monolithic `Switchboard` model (4,584 lines). The AI chat interface lives in `glitch_panel.go` (1,831 lines) as a sub-panel. Clarification requests from pipelines currently flow through a modal overlay on the `Switchboard` model — a single `pendingClarification *store.ClarificationRequest` field that serializes all questions one at a time.

The event path today:
```
pipeline executor → GLITCH_CLARIFY marker → clarification.go → SQLite + busd
→ pipeline_bus.go (ClarificationRequested) → Switchboard.pendingClarification → modal overlay
→ user types answer → Switchboard writes store.AnswerClarification → busd ClarificationReply
```

We are re-routing the TUI half of this path through the gl1tch chat panel.

## Goals / Non-Goals

**Goals:**
- Remove the single-at-a-time constraint on clarification handling
- Make the gl1tch chat panel the authoritative UI surface for all pipeline ↔ user communication
- Surface clarifications with context-aware urgency (proactive vs. passive)
- Begin the `Switchboard` → `console.Model` naming transition in new code only

**Non-Goals:**
- Rewriting or decomposing `switchboard.go` beyond removing the modal
- gl1tch LLM generating answers on the user's behalf
- Changing the `GLITCH_CLARIFY:` executor protocol
- Changing the busd event schema or SQLite schema

## Decisions

### D1: Event routing via Tea message, not direct subscription

The gl1tch panel does not subscribe to busd directly. Instead, `pipeline_bus.go` continues to own the busd subscription and converts `ClarificationRequested` events into a `ClarificationInjectMsg` Tea message dispatched to the root model, which forwards it to the glitch panel.

**Why**: busd subscription lifecycle is already managed in `pipeline_bus.go`. A second subscriber would require a second socket connection. Staying with Tea messages keeps the panel unit-testable without a live bus.

**Alternative considered**: Have the glitch panel subscribe directly via a `waitForClarificationCmd`. Rejected — duplicates bus connection logic and couples the panel to busd internals.

### D2: Pending clarification state lives in the glitch panel, not the root model

A new `pendingClarifications []pendingClarification` slice is added to `GlitchPanel` (or equivalent struct in `glitch_panel.go`). The root model (`Switchboard` / future `console.Model`) holds no clarification state after this change.

**Why**: All rendering and answer routing for clarifications is in the glitch panel. Keeping state co-located with behavior avoids cross-component state threading.

### D3: Urgency evaluated on injection and on a periodic tick

When a `ClarificationInjectMsg` arrives, the panel immediately checks `time.Since(req.AskedAt)` against the 10-minute timeout:
- **< 5 min elapsed**: inject message into thread, increment badge count, do not scroll
- **≥ 5 min elapsed**: inject message, scroll chat to bottom, set `urgentClarification = true` (panel header flashes badge)

A `tea.Tick` every 60 seconds re-evaluates urgency on all pending items and may promote passive → urgent.

**Why**: Pipelines are often launched and immediately blocked — most clarifications will arrive fresh. Only promotions to urgent (mid-wait) need the ticker.

### D4: Answer matching — queue-first with optional explicit index

When the user submits a message and `len(pendingClarifications) > 0`, the panel checks:
- If the message starts with `<N>:` (e.g., `2: yes`) → answer goes to `pendingClarifications[N-1]`
- Otherwise → answer goes to `pendingClarifications[0]` (oldest pending)

After routing, the pending entry is removed from the slice and the chat message is updated in-thread to show the answer as resolved.

**Why**: In the common case (one pending question), zero friction. For concurrent pipelines, numbered prefix gives explicit control without requiring a separate UI widget.

**Alternative considered**: Require explicit index always. Rejected — degrades the single-pipeline case unnecessarily.

### D5: Clarification messages are a distinct message type in the chat history

The glitch panel's chat history (currently `[]ChatMessage` or equivalent) gains a `ClarificationMessage` variant that carries `{RunID, PipelineName, StepID, Question, Answer, Resolved bool}`. This enables:
- Distinct rendering (pipeline badge prefix, resolve checkmark)
- Answer-detection logic that reads pending state from the message itself

**Why**: Embedding clarification metadata in the message avoids a parallel data structure and makes thread replay coherent.

## Risks / Trade-offs

**[Risk] User replies to gl1tch during an active clarification for unrelated reasons**
→ Mitigation: The answer-detection only triggers when `pendingClarifications` is non-empty. If the user's message looks like a question or free-form statement (heuristic: ends with `?`, or no pending items), it routes to gl1tch normally. Worst case, a false-positive writes a non-answer to the pipeline — the pipeline will re-ask or time out. Acceptable for v1; a smarter classifier can replace the heuristic later.

**[Risk] Clock skew between pipeline host and TUI host if running remotely**
→ Mitigation: `askedAt` comes from the SQLite row written by the pipeline runner. If hosts differ, urgency evaluation uses the DB timestamp — same clock as the 10-min timeout. No skew introduced by this change.

**[Risk] glitch_panel.go grows further (already 1,831 lines)**
→ Mitigation: The clarification state and render logic is self-contained and can be extracted to `clarification_panel.go` inside `internal/console/` if size becomes unmanageable. Out of scope for this change.

## Migration Plan

1. Add `ClarificationInjectMsg` Tea message type and re-wire `pipeline_bus.go` to emit it instead of updating `Switchboard.pendingClarification`.
2. Add clarification state + rendering to `glitch_panel.go`.
3. Remove `pendingClarification` field and modal render from `switchboard.go`.
4. Verify with an existing integration test that a pipeline blocked on `GLITCH_CLARIFY:` resolves when an answer is submitted via the chat panel.

Rollback: revert the three files above; the busd subscription and SQLite schema are unchanged.

## Open Questions

- Should answered clarifications persist in the chat thread permanently (full history) or be pruned after N messages? For v1, persist — removal logic is additive.
- Should gl1tch emit any LLM-generated framing around a clarification injection ("Pipeline `deploy-staging` needs your input:") or render it as raw structured data? Lean toward structured data for v1 to avoid latency on urgent requests.
