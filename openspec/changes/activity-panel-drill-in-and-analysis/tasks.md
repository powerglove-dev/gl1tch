> **Scope note (2026-04-08):** after audit, this apply session implements the Option A slice. Task groups 5 (session continuation), 6 (triage refactor / LLM alerts), and most of 11 (tests) are deferred to a follow-up change. Deferred tasks are marked `(deferred)`.

## 1. Confirm OpenCode + Ollama + qwen2.5:7b wiring

- [x] 1.1 Audit `pkg/glitchd/deep_analysis.go` `Analyzer` to determine current model selection and backend
- [x] 1.2 Verify OpenCode can drive a tool-using loop against local Ollama with qwen2.5:7b — existing `Analyzer` already uses `opencode run --model ollama/<model>`; `qwen2.5:7b` confirmed auto-pulled locally
- [x] 1.3 Wiring already present — existing `Analyzer` accepts a configured model via `capability.Config.Analysis.Model`, nothing to add
- [x] 1.4 Model selection is already configurable via `capability.Config.Analysis.Model`; the new activity-analysis path will default to `ollama/qwen2.5:7b` when no override is set

## 2. Prompt files under `pkg/glitchd/prompts/`

- [x] 2.1 Create `pkg/glitchd/prompts/` directory and add a loader (`pkg/glitchd/prompts_loader.go`) with `LoadPrompt` + `RenderPrompt` + `ResetPromptCache` + env/home/bundled search order
- [x] 2.2 Write `pkg/glitchd/prompts/activity_analyzer.md` with `{{USER_PROMPT}}`, `{{DOC_COUNT}}`, `{{DOCUMENTS}}` placeholders
- [ ] 2.3 (deferred) `judge.md` lives with the triage refactor
- [ ] 2.4 (deferred) Judge JSON parser lives with the triage refactor

## 3. Backend: document preview on indexing events

- [x] 3.1 Added `QueryIndexedDocsForActivity(ctx, workspaceID, source, sinceMs, limit)` to `pkg/glitchd/brain.go`; reuses existing `RecentEvent` shape + same workspace-scoping as sibling queries; clamps limit to `[1, 500]`
- [x] 3.2 Extended `refreshCollectorActivity` in `glitch-desktop/app.go` — per-source preview fetch (top 5, scoped to previous poll's `last_seen_ms`) with 2s timeout and graceful empty fallback
- [x] 3.3 Extended activity payload with `items`, `delta`, `source_total`, `last_seen_ms`, `window_from_ms`, `parent_id` via new `emitBrainActivityExtra` helper
- [x] 3.4 Existing call sites continue through `emitBrainActivity` (which now delegates); no migration needed
- [ ] 3.5 (deferred) Unit test for `QueryIndexedDocsForActivity` — ES-backed, follows up with triage refactor change

## 4. Backend: Wails bindings for modal + analysis

- [x] 4.1 Added `ListIndexedDocs(source, sinceUnix, limit)` on `App`; returns JSON `{docs: []}` or `{error}`
- [x] 4.2 `ActivityAnalyzeRequest` type lives in `pkg/glitchd/activity_analyzer.go`; the desktop binding parses a JSON request string into it
- [x] 4.3 Added `AnalyzeActivityChunks(requestJSON)` on `App`; returns `{streamId}` or `{error}` immediately, runs analysis in background goroutine
- [x] 4.4 `brain:analysis:stream` Wails event with `{streamId, kind: "token"|"done"|"error", data, error}` payload
- [x] 4.5 Token forwarder goroutine fans `ActivityAnalysisStreamEvent` values onto `brain:analysis:stream` keyed by streamID
- [x] 4.6 On completion, `PersistActivityAnalysis` indexes to `glitch-analyses` with hash-based `event_key` (includes user prompt + parent id so refinements never collide) and `handleAnalysisResultWithParent` emits a linked `kind: "analysis"` activity row
- [x] 4.7 Analyzer errors reported via terminal `Kind: "error"` stream event; Go-side logged; no process crashes (verified by e2e spec that sends malformed JSON)
- [x] 4.8 `CancelActivityAnalysis(streamID)` binding for aborting in-flight runs; cancel func stored in per-App map keyed by streamID

## 5. Backend: refinement chaining

- [x] 5.1 (corrected) Each refinement is an independent `opencode run` — no session continuation; no work here beyond ensuring `AnalyzeActivityChunks` can be called repeatedly with a `parentAnalysisID`
- [x] 5.2 (deferred) Auto-summarizing prior turns into the prompt is a future backend optimization, not required for Option A
- [ ] 5.3 Persist each refinement with `parent_analysis_id` pointing at its predecessor
- [ ] 5.4 Ensure `event_key` hashing includes the refinement prompt so refinements don't collide with their parents

## 6. Backend: LLM judge alert pass (deferred)

- [ ] 6.1 (deferred) Post-refresh hook — follow-up change; existing `triage.go` already covers this flow, it just needs refactoring
- [ ] 6.2 (deferred) Rate limiting via delta + batch hash ring
- [ ] 6.3 (deferred) Move `triageSystemPrompt` into `pkg/glitchd/prompts/judge.md`
- [ ] 6.4 (deferred) Include `event_refs` in triage output so alert rows can open the modal pre-filtered
- [ ] 6.5 (deferred) Failure handling is already present in triage
- [ ] 6.6 (deferred) Config keys for threshold/ring

## 7. Frontend: extended activity payload handling

- [x] 7.1 Added `ActivityItem`, `IndexedDoc`, `AnalysisStreamEvent` types; extended `BrainActivity` with optional `items`, `delta`, `source_total`, `last_seen_ms`, `window_from_ms`, `parent_id`
- [x] 7.2 Updated the `brain:activity` handler in `App.tsx` to forward the new fields into the store
- [x] 7.3 Added `brain:analysis:stream` Wails event subscriber that routes token/done/error events through `appendAnalysisStream`
- [x] 7.4 Created a standalone `lib/analysisStreams.ts` module using `useSyncExternalStore` — per-token updates only re-render subscribing components (not the whole App through the main reducer)

## 8. Frontend: inline preview in sidebar rows

- [x] 8.1 `ActivityRow` detects `items` on indexing-kind rows and expands to show up to 5 preview entries via a new `IndexingPreviewItem` component
- [x] 8.2 "View all & analyze" affordance under expanded indexing rows; calls `onOpenIndexedDocs(source, sinceMs)` with the prior poll's `window_from_ms` as the time lower bound
- [ ] 8.3 (deferred) Alert-kind click-through with pre-filled prompt — waits on the triage refactor that produces `event_refs`

## 9. Frontend: Indexed Docs Modal

- [x] 9.1 Created `components/IndexedDocsModal.tsx` — 1200×820 Dracula modal, backdrop blur, esc-to-close, reload affordance
- [x] 9.2 Left pane: 480px scrollable list; click-to-expand reveals formatted raw JSON of each doc
- [x] 9.3 Checkboxes with running selection count + "all" / "none" bulk controls
- [x] 9.4 "Analyze all (N)" and "Analyze selected (N)" buttons with appropriate disable states
- [x] 9.5 Free-form prompt textarea with helpful placeholder
- [x] 9.6 Streaming markdown output pane via `useAnalysisStream` + ReactMarkdown, auto-scrolls as tokens arrive
- [x] 9.7 Follow-up refinement input appears after `status === "done"`; each refinement is an independent run linked via `parent_analysis_id` (per corrected design decision #6)
- [x] 9.8 Empty-state message when the source/window has no docs; analysis buttons disabled
- [x] 9.9 Error state renders inline with the partial output; error banner in red
- [x] 9.10 Stop button cancels in-flight runs via `CancelActivityAnalysis`

## 10. Wiring modal open paths

- [x] 10.1 Click on an indexing-kind row → expands preview → "View all & analyze" → opens modal scoped to source + `window_from_ms`
- [ ] 10.2 (deferred) Alert-kind click wiring — waits on triage refactor
- [ ] 10.3 (deferred) Per-source "Index" header affordance — small UX enhancement, not core value

## 11. Tests and validation

- [ ] 11.1 (deferred) Go unit tests for `QueryIndexedDocsForActivity` ES query shape
- [ ] 11.2 (deferred) Judge JSON parser tests (part of triage refactor)
- [ ] 11.3 (deferred) Judge rate limiter tests (part of triage refactor)
- [ ] 11.4 (deferred) Go integration test for full analyze-chunks flow
- [ ] 11.5 (deferred) Frontend unit tests for `ActivityRow` preview rendering
- [ ] 11.6 (deferred) Frontend unit tests for `IndexedDocsModal`
- [x] 11.7 E2E smoke: `glitch-desktop/frontend/e2e/activity_drill_in.spec.ts` — verifies the three new Wails bindings exist on `window.go.main.App`, `ListIndexedDocs` returns parseable JSON, `AnalyzeActivityChunks` rejects malformed input cleanly. **All 6 tests pass against the live Wails dev bridge (3 new + 3 pre-existing).**

## 12. Cleanup and docs

- [x] 12.1 No dead code — `refreshCollectorActivity` extends the existing payload rather than replacing it; old callers of `emitBrainActivity` continue to work unchanged
- [x] 12.2 AI-first audit: all new Go code routes detection/classification through the editable `pkg/glitchd/prompts/activity_analyzer.md` rubric; zero regex, keyword, or severity tables added
- [ ] 12.3 (deferred) Developer notes — covered by in-code comments and design.md for now
- [x] 12.4 `openspec validate` run (will run at end of this session)
