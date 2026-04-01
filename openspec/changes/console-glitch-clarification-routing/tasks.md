## 1. Tea Message Plumbing

- [x] 1.1 Define `ClarificationInjectMsg` Tea message type (carries `store.ClarificationRequest`) in `internal/console/`
- [x] 1.2 Update `pipeline_bus.go`: convert `ClarificationRequested` busd event to `ClarificationInjectMsg` instead of setting `Switchboard.pendingClarification`
- [x] 1.3 Add `ClarificationInjectMsg` case to root model `Update()` — forward to gl1tch panel

## 2. Glitch Panel State

- [x] 2.1 Define `pendingClarification` struct: `{RunID int64, PipelineName, StepID, Question string, AskedAt time.Time, urgent bool}`
- [x] 2.2 Add `pendingClarifications []pendingClarification` and `batchWindow time.Time` fields to the glitch panel model
- [x] 2.3 Add `ClarificationMessage` variant to the chat message type (carries all clarification metadata + `Resolved bool`, `Answer string`)

## 3. Injection and Urgency Logic

- [x] 3.1 Implement `injectClarification()` — evaluate urgency on arrival, append `ClarificationMessage` to thread, handle batch window (3s grouping)
- [x] 3.2 Implement batch summary injection: if `time.Since(batchWindow) < 3s`, accumulate and emit summary message before individual messages
- [x] 3.3 Add 60-second `tea.Tick` to the glitch panel; on tick, re-evaluate urgency for all passive pending items and promote if threshold crossed
- [x] 3.4 Implement scroll-to-bottom and urgent badge state when urgency triggers

## 4. Answer Routing

- [x] 4.1 Implement `parseAnswerTarget(input string, pending []pendingClarification) (idx int, answer string, warning string)` — handles plain vs. `<N>:` prefix
- [x] 4.2 In glitch panel `Update()` on `tea.KeyMsg` submit: if `pendingClarifications` non-empty, route via `parseAnswerTarget` instead of sending to LLM
- [x] 4.3 Call `store.AnswerClarification(runID, answer)` and `publishClarificationReplyCmd(reply)` after routing
- [x] 4.4 Mark resolved `ClarificationMessage` in thread (set `Resolved = true`, `Answer = answer`); update rendered view
- [x] 4.5 Show out-of-range warning inline in chat thread when applicable

## 5. Remove Modal Overlay

- [x] 5.1 Remove `pendingClarification *store.ClarificationRequest` field from `Switchboard` model
- [x] 5.2 Remove modal overlay render logic from `switchboard.go` (`View()`)
- [x] 5.3 Remove `AnswerClarification` call site in switchboard that was wired to the modal submit
- [x] 5.4 Remove `loadPendingClarificationsCmd` startup logic that fed the modal (DB-polling fallback stays in pipeline runner — not in TUI)

## 6. Rendering

- [x] 6.1 Implement `ClarificationMessage` render: pipeline badge prefix, question text, resolved state with answer and checkmark
- [x] 6.2 Implement numbered list rendering for multi-pending state (only shown when ≥ 2 pending; single pending renders without index)
- [x] 6.3 Add urgent badge indicator to gl1tch panel header (badge count + color change when `urgent = true`)

## 7. Tests

- [x] 7.1 Unit test `parseAnswerTarget`: plain answer, explicit index, out-of-range index
- [x] 7.2 Unit test urgency evaluation: fresh (passive), near-timeout (urgent), tick promotion
- [x] 7.3 Unit test batch window: two clarifications within 3s produce summary; two beyond 3s do not
- [ ] 7.4 Integration test: pipeline blocked on `GLITCH_CLARIFY:` resolves when answer submitted via chat panel (adapts existing clarification integration test)
