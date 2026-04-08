## 1. Event schema additions

- [ ] 1.1 Add `brain_feedback` event type to `internal/brain` with fields: answer_id, verdict, source (explicit | implicit), classification, config_hash, timestamp
- [ ] 1.2 Add `brain_config_change` event type with fields: field, old_value, new_value, timestamp
- [ ] 1.3 Add a `config_hash` helper that produces a stable hash of the current brain/research-loop config
- [ ] 1.4 Embed `config_hash` in every `research_score` event (touches `glitch-research-loop` integration)
- [ ] 1.5 Add unit tests for event encoding/decoding round-trips

## 2. Stats engine

- [ ] 2.1 Create `internal/brainaudit/stats` (or sibling) package with `Series`, `Bucket`, `Stats` types
- [ ] 2.2 Implement daily-30 + weekly-beyond bucket storage colocated with brain events (no new DB)
- [ ] 2.3 Implement `Stats.Update(event)` for `research_score`, `research_escalation`, `brain_feedback`, `brain_config_change`
- [ ] 2.4 Implement `Stats.Get(metric, range) (Series, error)` reading only from cached buckets
- [ ] 2.5 Implement bucket eviction/rollup (daily → weekly at 30 days)
- [ ] 2.6 Add unit tests covering update, get, eviction, and unknown-metric error

## 3. Metric computation

- [ ] 3.1 Implement `accept_rate`: rolling fraction over last 7 days
- [ ] 3.2 Implement `confidence_calibration` Brier score over the same window
- [ ] 3.3 Implement `retrieval_precision` from `research_score` per-claim labels
- [ ] 3.4 Implement `iteration_count` average from `research_attempt` events
- [ ] 3.5 Implement `escalation_rate` from presence/absence of `research_escalation` per accepted answer
- [ ] 3.6 Document each metric's formula in `internal/brainaudit/stats/metrics.md`
- [ ] 3.7 Add unit tests with synthetic event streams covering each metric independently

## 4. `/brain stats` slash command

- [ ] 4.1 Register `/brain stats` in the slash dispatcher (depends on `chat-first-ui`)
- [ ] 4.2 Build a `score_card` per metric with current value, sparkline, and direction-aware trend arrow
- [ ] 4.3 Add a one-line interpretation under each metric ("0.12 — calibration is good", etc.)
- [ ] 4.4 Render config-change annotations as marks on the sparkline
- [ ] 4.5 Smoke test with seeded events

## 5. `/brain config` widget and editor

- [ ] 5.1 Register `/brain config` in the slash dispatcher
- [ ] 5.2 Build the `widget_card` listing every editable knob with current value and edit action
- [ ] 5.3 Implement constrained inputs (numeric range, enum picker, toggle) per knob type
- [ ] 5.4 Implement client-side and server-side validation
- [ ] 5.5 Implement preview-and-confirm flow for threshold and budget edits
- [ ] 5.6 Wire successful edits to emit `brain_config_change` events
- [ ] 5.7 Smoke test: edit threshold via widget, verify event lands and active config updates

## 6. Next-prompt chip generator

- [ ] 6.1 Add a chip generator function in `internal/assistant` that takes (question, answer) and returns up to 3 short prompts
- [ ] 6.2 Author the system prompt with the no-destructive-actions rule
- [ ] 6.3 Hook the generator to run async after each assistant answer
- [ ] 6.4 Append the generated chips as an `action_chips` message under the answer
- [ ] 6.5 Drop chips classified as destructive
- [ ] 6.6 Verify chip clicks flow through the existing `widget-action-protocol` and reach either the slash dispatcher or the assistant

## 7. Explicit thumbs feedback

- [ ] 7.1 Add thumbs-up / thumbs-down controls to the assistant answer rendering in `internal/chatui`
- [ ] 7.2 Wire clicks to emit `brain_feedback` events with `source=explicit`
- [ ] 7.3 Visually mark answers that have received feedback so the user knows their click registered

## 8. Implicit contradicting-follow-up classifier

- [ ] 8.1 Add an inline qwen2.5:7b classifier in `internal/assistant` that runs on every `glitch ask` with a recent prior answer in scope
- [ ] 8.2 Define the four labels: `rephrase`, `challenge`, `correct`, `ignore_previous` (and `unrelated` for the no-op case)
- [ ] 8.3 Emit `brain_feedback` events for the four contradicting labels
- [ ] 8.4 Persist the raw classifier verdict alongside the event for auditability
- [ ] 8.5 Add unit tests with mock follow-ups for each label

## 9. Retention policy

- [ ] 9.1 Implement 30-day TTL on full `research_attempt` payloads (truncate to hash after expiry)
- [ ] 9.2 Confirm `research_score`, `brain_feedback`, `brain_config_change` are kept indefinitely
- [ ] 9.3 Add a brain audit smoke test verifying eviction behavior on a seeded store

## 10. Wiring and validation

- [ ] 10.1 End-to-end smoke: ask a question, accept the answer with thumbs-up, run `/brain stats`, see the event reflected in `accept_rate`
- [ ] 10.2 End-to-end smoke: edit threshold via `/brain config`, run `/brain stats`, see the annotation on the sparkline
- [ ] 10.3 Run `openspec validate brain-observability --strict` and fix findings
