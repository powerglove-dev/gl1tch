## Audit update (2026-04-08)

The implementation audit (task 1.1) turned up significant existing substrate that changes the shape of this change:

- **`pkg/glitchd/brain.go:554` already has `QueryRecentCollectorEvents`** — a workspace-scoped, source-filtered, limit-clamped ES query that returns a curated `RecentEvent` shape. The comment says it "powers the expand-to-see-content view in ActivitySidebar" but the desktop has no caller yet; the Wails binding and frontend wiring are missing. Task 3.1 is therefore "expose the existing query, add an optional `since` param", not "add a new query from scratch".
- **`pkg/glitchd/triage.go` already implements the LLM judge pass** (`TriageEvents`) and **already defaults to `qwen2.5:7b`** via the local Ollama HTTP API with `format: "json"`. It produces structured `{severity, title, why, source}` alerts but does not carry the referenced document ids. Task group 6 is therefore "refactor triage to add event refs + move `triageSystemPrompt` out of a Go const and into `pkg/glitchd/prompts/judge.md` + add batch hash dedupe", not "build a new subsystem".
- **`pkg/glitchd/deep_analysis.go` `Analyzer`** already shells out to `opencode run --model <cfg> --format json` with the `ollama/<model>` prefix. Model selection is already parameterized via `capability.Config.Analysis.Model`. Current default is `ollama/qwen2.5-coder:latest`. No wiring work needed for single-shot invocation; the new streaming path, however, is entirely new code because the existing `runOne` uses blocking `cmd.Output()`.
- **`opencode run` is a one-shot invocation with no session persistence.** Design decision #6 below as originally written assumed session continuation was possible. It is not. The decision is corrected below to treat every refinement as an independent `opencode run` whose prompt includes the same document set plus the new user question. This is strictly simpler, has no tool-state issues (because there was no tool state to preserve), and matches what actually works today.

### Scope for the initial implementation (Option A)

Because the audit shows this change is smaller than originally framed, it is split into two landable slices:

1. **This change ships**: prompts/loader, extended `brain:activity` payload with `items` preview, `ListIndexedDocs` Wails binding over existing ES query, `AnalyzeActivityChunks` streaming binding using `opencode run` + `StdoutPipe`, inline preview rendering on indexing rows, and a new `IndexedDocsModal` with list pane, "Analyze all" / "Analyze selected" buttons, streamed markdown output, and independent-run follow-up refinement.
2. **Deferred to a follow-up change**: the triage refactor (event refs, prompt file, batch hash dedupe), alert-click-opens-modal wiring, unit/integration test coverage beyond build validation. The follow-up change will be proposed once the Option A slice is shipped and we have real behavior to refine.

## Context

The desktop activity sidebar is fed by the Wails event `brain:activity`, emitted from a single entry point in `glitch-desktop/app.go` at `emitBrainActivity()` (line ~1761). Collector indexing deltas come from `refreshCollectorActivity()` (line ~2046), which calls `pkg/glitchd/brain.QueryCollectorActivityScoped()` — a query that currently returns only aggregated counts per source. The underlying `Event` documents exist with rich metadata in the `glitch-events` Elasticsearch index (`internal/esearch/events.go`), but there is no query path that returns the actual documents behind a count delta.

A deep analyzer already exists at `pkg/glitchd/deep_analysis.go` and uses the OpenCode tool-using loop. Today it is invoked only by the autonomous triage loop and indexes its results into `glitch-analyses`. The frontend already has an expandable-row affordance for `kind: "analysis"` entries in `ActivitySidebar.tsx` (~lines 196–362).

Hard constraints carried into this design:

- **AI-first, nothing hardcoded.** No regex error detection, static severity tables, or keyword lists in Go. All judgment lives in LLM prompts.
- **qwen2.5:7b is the hard default.** All analysis runs through OpenCode + qwen2.5:7b via a local Ollama-compatible backend. The judge pass uses the same model with tools disabled.
- **Pre-1.0, no migrations.** If ES shape changes, wipe the index.
- **TUI is being removed.** No TUI surfaces may be touched.

## Goals / Non-Goals

**Goals:**

- Give the user a preview of *what* was indexed inline on every indexing activity row.
- Provide a full drill-in modal that lists every indexed document for a source/window and supports multi-selection.
- Let the user run on-demand analyses (all or selected) with an optional free-form prompt, streamed back into the modal, through the OpenCode + qwen2.5:7b tool-using loop.
- Support iterative refinement: follow-up prompts chain new analyses via `parent_analysis_id`.
- Add a background LLM judge pass that raises alert-kind activity rows for noteworthy batches — entirely LLM-driven, no Go heuristics.
- Keep all prompts in editable files under `pkg/glitchd/prompts/` so operators can tune behavior without recompiling.

**Non-Goals:**

- TUI parity (TUI is being removed).
- Any regex/keyword/severity-table detection path.
- Elasticsearch index migrations.
- Changes to the assistant router or the observer query engine.
- Sharing analyses across machines (persistence is local-only via existing ES).
- Supporting cloud models — local Ollama only.

## Decisions

### 1. Attach document preview to the existing `brain:activity` payload, don't add a new event

**Decision:** Extend the existing `brain:activity` payload with new optional fields (`items`, `parent_id`, `event_refs`) rather than introducing a second event stream. The frontend tolerates unknown fields today, so adding optional fields is backwards-compatible within the desktop app; older cached state on disk can simply be ignored because pre-1.0 rules allow wiping local state.

**Alternatives considered:**

- *New `brain:indexing` event type.* Rejected — doubles frontend subscription paths and splits ordering guarantees across two streams for no real gain.
- *Fetch preview on demand from the frontend when a row is expanded.* Rejected for the inline preview because it breaks the "see what just changed at a glance" affordance the user asked for. It is, however, exactly the right pattern for the full drill-in modal (see decision 3).

### 2. Preview cap = 5 items, sorted by timestamp descending

**Decision:** The preview attached to an indexing event contains at most 5 items, the most recent by timestamp. This keeps payloads small, matches the row-expansion UI that already renders well for short lists, and matches the number the user implied in the spec discussion.

**Alternatives considered:**

- *No cap, whole batch.* Rejected — some sources index 30+ docs per tick, which would bloat activity payloads and duplicate work with the drill-in modal.
- *Configurable cap.* Deferred — not worth the config surface for a number we can tune if needed.

### 3. Full drill-in uses a Wails binding, not pre-attached data

**Decision:** The drill-in modal fetches its document list on demand via a new Wails binding `ListIndexedDocs(source, sinceUnix, limit)`. Payload size for the sidebar stays small (preview only), and the modal can paginate or widen its window without touching the activity stream.

**Alternatives considered:**

- *Pre-attach the full batch to the activity payload.* Rejected — unbounded payload growth and wasted work when the user never opens the modal.
- *Query ES directly from the frontend.* Rejected — ES client lives in Go and we don't want to expose ES auth to the renderer process.

### 4. Analysis always runs through OpenCode + qwen2.5:7b

**Decision:** Reuse the existing `Analyzer` in `pkg/glitchd/deep_analysis.go`, but parameterize its model selection so we can set qwen2.5:7b as the default via an Ollama backend. Do not introduce a second, tool-less analysis path for "quick" analyses — the user explicitly asked for the tool-using loop everywhere, and making the loop the only path keeps behavior predictable.

**Alternatives considered:**

- *Two analyzer paths (tool-using for "deep", tool-less for "quick").* Rejected by the user — single loop, single model.
- *Direct Ollama HTTP calls from Go, bypassing OpenCode.* Rejected — we lose the tool-using loop and the already-wired session/observability.

### 5. Streaming output via a dedicated Wails event keyed by a stream id

**Decision:** On `AnalyzeActivityChunks`, the backend returns a `streamID` immediately and streams tokens via a new Wails event `brain:analysis:stream` payload `{streamID, kind: "token"|"done"|"error", data, error}`. The frontend subscribes per open analysis pane. This mirrors the existing `brain:activity` pattern and does not require Wails' experimental stream binding.

**Alternatives considered:**

- *Single blocking call that returns the whole markdown.* Rejected — defeats the "live analysis" UX the user asked for.
- *Wails v2 experimental streaming bindings.* Rejected — extra coupling to experimental API for no advantage over a keyed event.

### 6. Refinement is independent `opencode run` invocations (corrected 2026-04-08)

**Decision:** Each refinement is a fresh, independent `opencode run` invocation whose prompt contains the same selected documents plus the new user question. There is no session continuation. The `parent_analysis_id` chain is metadata only — used by the UI to render a threaded view and by ES to link related analyses — and does not imply shared process/session state on the backend.

**Why corrected:** The audit (task 1.1) confirmed that `opencode run --format json` is a one-shot command with no session handle to reuse. The original decision to "continue the same session for 2 turns, then auto-summarize" assumed an API that does not exist in the version of opencode gl1tch drives today. Re-reading the same docs on each refinement is a real cost (more input tokens per turn), but the tradeoff is simplicity and zero session-lifecycle bugs.

**Alternatives considered:**

- *Investigate an opencode interactive/daemon mode that holds sessions.* Deferred — worth a follow-up spike, but not a blocker for shipping Option A.
- *Concatenate prior turns into the prompt manually.* Rejected for the first cut. The backend could do this later without changing the frontend contract, so it is a pure backend optimization that can ship when we need it.
- *Cap refinement chain length.* Not needed — each refinement has deterministic `event_key` dedupe, and the user pays for their own depth.

### 7. Prompts live as editable files under `pkg/glitchd/prompts/`

**Decision:** Both the analyzer rubric and the judge prompt live as plain markdown/text files under `pkg/glitchd/prompts/` and are loaded at runtime. They are not embedded string literals in Go source. The user can edit them without recompiling, which is the practical expression of the "AI-first, nothing hardcoded" rule for this feature.

**Alternatives considered:**

- *Go `embed` of prompt files.* Rejected — would still require a rebuild to tweak prompts.
- *Config file / env var.* Rejected — prompts are too long for env vars and a dedicated directory is clearer.

### 8. Judge pass rate-limiting via delta threshold + batch hash dedupe

**Decision:** Before running the judge on a refresh cycle, skip it if either (a) the new-doc delta is below a configured minimum, or (b) the content hash of the batch matches a recently judged batch (remembered in a small in-memory ring). Both limits are configurable, both default to sensible values, and neither involves inspecting document content with Go heuristics — the hash is opaque.

**Alternatives considered:**

- *Unrestricted judge on every poll.* Rejected — wastes local LLM time and generates activity-sidebar noise when nothing is happening.
- *Time-window rate limit only.* Rejected — misses cases where genuine new batches arrive back-to-back.

### 9. Alert rows reference docs via `event_refs` rather than reprocessing on click

**Decision:** When the judge raises an alert, the referenced doc ids it returned are stored directly on the alert activity event (`event_refs`). Clicking the row opens the modal scoped to those ids and pre-fills the prompt box with the judge's hook. The modal does not re-run the judge.

**Alternatives considered:**

- *Re-run the judge on click to get up-to-date ids.* Rejected — stale-but-explainable is better than fresh-but-surprising.

### 10. All three new capabilities ship together; no feature flag

**Decision:** Preview, modal, and LLM alerts ship as one change. They share payload, prompt infrastructure, and UI wiring, and the value of any one without the others is limited.

**Alternatives considered:**

- *Ship preview first, then modal, then alerts.* Rejected — the preview is not useful without the modal to drill into, and the modal is where alerts already have to land.

## Risks / Trade-offs

- **qwen2.5:7b latency for on-demand analyses** → mitigation: stream tokens immediately, show a visible progress state, and keep the rubric prompt compact. Latency is acceptable because the user controls when analysis runs.
- **Judge pass producing noisy alerts** → mitigation: rate-limit via delta threshold + batch hash dedupe; prompt can be tuned without recompile; if it still misbehaves, operators can raise the delta threshold to effectively disable it.
- **Unbounded refinement chains growing ES** → mitigation: deterministic `event_key` dedupes identical inputs; chain length not otherwise capped because chains are the point.
- **Extended `brain:activity` payload breaking older frontend builds** → mitigation: new fields are optional, desktop app ships backend + frontend together, and pre-1.0 we accept the break explicitly (flagged BREAKING in the proposal).
- **Prompt files being edited in ways that break parsing** → mitigation: the judge's response format is a small JSON shape validated in Go; on parse failure the brain logs the error and skips the cycle (no alert is raised, no crash). The analyzer prompt produces free-form markdown with no parsing required.
- **OpenCode not currently wired for Ollama/qwen2.5:7b** → mitigation: part of this change is making that wiring real and configurable; if it turns out OpenCode cannot drive Ollama in the required tool-using mode, we will raise this as a blocker before the frontend work begins.
- **ES query cost for large windows** → mitigation: `ListIndexedDocs` takes a `limit` parameter and the modal caps it to a sane default (e.g. 500); users needing more are expected to narrow the window.

## Migration Plan

- No database or schema migrations. The `glitch-events` and `glitch-analyses` mappings remain unchanged: `event_refs` and `parent_id` ride on `Event.Metadata` (a generic dict field already in the mapping).
- No user-facing migration. On first launch of the updated desktop app, existing activity history may not render previews for historical rows — they will simply show as before. New indexing events carry previews from that point forward.
- Rollback: revert the desktop binary. No persisted state changes are written by this change beyond standard `glitch-analyses` entries, which the old binary already tolerates.

## Open Questions

1. **Does OpenCode's current wiring support an Ollama-compatible backend driving a tool-using loop with qwen2.5:7b?** If not, the first task is to confirm or add that wiring before building on top of `Analyzer`. Flagged as a risk above.
2. **What's the default judge delta threshold and ring size?** Starting values: delta ≥ 1 (run on any new docs), ring size 32 for hash dedupe. Tune after live observation.
3. **Should the modal support cross-source analysis** (e.g. "analyze the last github batch *and* the last claude batch together")? Not in scope for this change — the modal is scoped to a single source/window — but the analyzer invocation shape (`eventIDs []string`) does not preclude adding it later.
4. **Where does the user configure the model override** (per decision 4)? Initial plan: a new key in the existing glitchd config; exact key name to be decided during implementation. Not worth blocking the proposal on.
