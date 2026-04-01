## Why

When multiple pipelines run concurrently and need user input, gl1tch serializes clarifications through a single blocking modal overlay — only one question surfaces at a time while the rest queue silently in SQLite. Routing clarifications through the gl1tch chat panel instead eliminates this bottleneck, makes the AI the natural conversation layer for all pipeline interactions, and begins the `Switchboard` → `console.Model` rename.

## What Changes

- **Remove** the `pendingClarification *store.ClarificationRequest` modal overlay from the console model; the single-at-a-time constraint is eliminated.
- **Add** clarification injection into the gl1tch chat panel: when a pipeline emits `GLITCH_CLARIFY:`, a structured message is pushed into the chat thread (pipeline badge, step context, question text).
- **Add** smart urgency surfacing: gl1tch evaluates elapsed time vs. 10-min timeout and either proactively draws attention or queues the notification for the next conversational pause.
- **Add** answer routing: the gl1tch input handler detects replies to pending clarifications by in-thread context, writes the answer via `store.AnswerClarification(runID, answer)`, and publishes `ClarificationReply` on busd.
- **Add** multi-clarification thread view: multiple pending questions are rendered as a numbered inline list; answers can arrive in any order.
- **Begin** `Switchboard` → `Console` rename: new code in this change references `console.Model`; existing internal names are not yet renamed.

## Capabilities

### New Capabilities

- `glitch-clarification-routing`: gl1tch chat panel receives pipeline clarification requests, injects them into the conversation thread with smart urgency logic, and routes user answers back to the correct pipeline run via runID.
- `clarification-smart-surfacing`: Urgency heuristic — proactive notification when >50% of the 10-min timeout has elapsed or when a batch of clarifications arrives; passive queuing otherwise.
- `clarification-multi-inbox`: Multiple concurrent pending clarifications rendered as an inline numbered list in the chat thread; answers matched to runs by thread position.

### Modified Capabilities

- `pipeline-event-publishing`: `ClarificationRequested` and `ClarificationReply` busd events are now consumed and published by the gl1tch panel, not the switchboard modal. Event schema unchanged.

## Impact

- `internal/console/switchboard.go`: Remove `pendingClarification` field, modal overlay render, and `AnswerClarification` call site wired to the overlay.
- `internal/console/glitch_panel.go`: Add clarification message injection, pending-clarification state, urgency evaluator, and answer-detection in the input handler.
- `internal/console/pipeline_bus.go`: Re-route `ClarificationRequested` event subscription to the gl1tch panel instead of the switchboard model.
- No changes to `internal/pipeline/clarification.go`, `internal/clarify/`, or the busd event schema.
