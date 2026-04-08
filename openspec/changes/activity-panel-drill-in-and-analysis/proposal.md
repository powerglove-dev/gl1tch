## Why

The desktop activity sidebar announces that indexing is happening ("claude · 8 new", "github · 31 new") but gives the user no way to see *what* was indexed, no way to inspect the underlying documents, and no way to ask the AI "what do these mean and what should I do next?" without leaving the panel and hand-constructing an Elasticsearch query. This is the gap between a passive event log and the AI-first dashboard gl1tch is supposed to be — today the activity panel is read-only noise, tomorrow it should be the primary surface for turning raw indexed signal into actionable next steps.

## What Changes

- **Preview** — indexing activity events (`kind: "indexing"` / collector-source check-ins) carry a small list of the actual new docs (id, title, timestamp, source badge) alongside the existing count, and the sidebar row expands inline to show them.
- **Drill-in modal** — clicking an indexing row opens a full-size modal scoped to that source + time window. Left pane: scrollable list of all indexed docs with checkboxes and raw-JSON expansion. Right pane: analysis workspace.
- **On-demand AI analysis** — from the modal, the user can "Analyze all" or multi-select chunks and "Analyze selected", optionally with a free-form prompt. Analysis runs through the existing OpenCode tool-using loop driven by **qwen2.5:7b** (local Ollama, per the hard default). Output streams into the modal as markdown.
- **Iterative refinement** — after the first analysis, a follow-up prompt box lets the user refine ("what's the first thing I should fix?"), producing chained analyses linked by `parent_analysis_id`.
- **Background alert pass** — after each collector refresh, a lightweight qwen2.5:7b judge pass (no tools) looks at each new batch and decides whether to raise a `kind: "alert"` row. Clicking an alert opens the modal pre-filtered to the referenced docs with the alert's hook pre-filled as the prompt. **No Go-side regex, keyword matching, or static severity tables** — all judgment is in the LLM.
- Every analysis run is persisted into the existing `glitch-analyses` ES index and emitted as a `kind: "analysis"` activity row with `parent_id` pointing at the triggering indexing/alert event.
- Analyzer model path is pinned/defaulted to qwen2.5:7b via OpenCode's local backend (if it's currently hardcoded elsewhere, it is made configurable with qwen2.5:7b as the default).
- **BREAKING** for the `brain:activity` Wails event payload: adds new optional fields (`items`, `parent_id`, `event_refs`). Frontend consumers must tolerate the new shape. No ES index migrations — if `glitch-events` or `glitch-analyses` shape changes, the index is wiped per the pre-1.0 rule.

## Capabilities

### New Capabilities

- `activity-panel-drill-in`: Ability to inspect the actual indexed documents behind any activity sidebar indexing/alert row via a full-size modal with per-doc detail and raw-JSON expansion.
- `activity-panel-analysis`: Ability to run and iteratively refine an on-demand, tool-using LLM analysis (OpenCode + qwen2.5:7b) over a selected subset of indexed documents from within the activity modal, with streamed output persisted as a linked analysis event.
- `activity-panel-llm-alerts`: Background LLM-driven alert detection — after each collector refresh, a qwen2.5:7b judge pass inspects new doc batches and raises alert-kind activity rows without any Go-side heuristic code.

### Modified Capabilities

<!-- None. No existing spec in openspec/specs/ covers the desktop activity sidebar yet. -->

## Impact

- **Backend (Go)**
  - `pkg/glitchd/brain.go` — new `ListIndexedDocs(ctx, source, since, limit)` query alongside `QueryCollectorActivityScoped`; `refreshCollectorActivity` attaches a preview list to emitted events.
  - `pkg/glitchd/deep_analysis.go` — `Analyzer` parameterized for qwen2.5:7b via OpenCode's Ollama backend; exposed for on-demand invocation (not just the autonomous triage loop).
  - New brain-loop stage: per-refresh qwen2.5:7b judge pass over new batches producing alert events. Prompt lives under `pkg/glitchd/prompts/` as an editable file, not as a Go string literal.
- **Wails bindings** (`glitch-desktop/app.go`)
  - New: `ListIndexedDocs(source, sinceUnix, limit)`, `AnalyzeActivityChunks(req)` returning a stream id.
  - New Wails event: `brain:analysis:stream` keyed by stream id for token streaming.
  - Extended `brain:activity` payload with `items`, `parent_id`, `event_refs`.
- **Frontend (React)** (`glitch-desktop/frontend/src/`)
  - `components/IndexedDocsModal.tsx` — new big modal (Dracula styling).
  - `components/ActivitySidebar.tsx` — inline preview rendering + click-to-open-modal wiring for indexing and alert rows.
  - `App.tsx` / store — handle extended `brain:activity` payload and new `brain:analysis:stream` event.
- **Elasticsearch** — no mapping changes required if `Event.Metadata` remains a generic dict. Any schema change means wiping `glitch-events` / `glitch-analyses` (pre-1.0 rule).
- **Dependencies** — no new runtime dependencies. Assumes OpenCode is already wired with an Ollama-compatible backend; if not, wiring that is part of this change.
- **Out of scope** — no TUI changes (TUI is being removed), no changes to the assistant router, no regex/keyword-based detection anywhere.
