## Context

ORCAI pipelines dispatch steps to AI agent plugins (e.g. `claude.chat`, `ollama.chat`). Each plugin receives a `prompt` string and a flat `vars` map. Currently there is no mechanism for the pipeline runner to automatically enrich that prompt with knowledge from the ORCAI SQLite store — agents are blind to historical runs, stored notes, or any other persistent data unless a pipeline author manually writes `db` steps and threads the results through template variables. The `db` step is awkward, requires SQL literacy, pollutes the pipeline YAML with boilerplate, and provides no guardrails against accidental writes to operational tables.

The target state: pipeline YAML gains two optional boolean flags — `use_brain` (read) and `write_brain` (write) — that activate automatic, transparent pre-context injection around agent steps. A new `BrainInjector` component in `internal/pipeline` assembles the context string, and a new `brain_notes` table isolates agent-written knowledge from the operational `runs` table.

## Goals / Non-Goals

**Goals:**
- `use_brain: true` on a pipeline or step causes the runner to prepend a read-context block to the agent's prompt, describing the ORCAI schema and supplying recent relevant data from the DB
- `write_brain: true` on a pipeline or step causes the runner to (a) include a write-context block instructing the agent to embed `<brain>` XML in its response, and (b) parse and persist that XML to `brain_notes` after the step completes
- `brain_notes` entries written by `write_brain` steps are automatically surfaced in subsequent `use_brain` steps in the same pipeline run, creating a feedback loop
- `BrainInjector` is an interface so it can be swapped for testing or future vector-store backends
- Remove the `db` step type; cover the narrow read use-case with `use_brain`

**Non-Goals:**
- Vector/semantic search over brain notes (plain SQL full-text search only for now)
- Multi-user or role-based brain note isolation
- Brain notes across pipeline runs (initial implementation is per-run; cross-run is a future phase)
- Structured output schemas or typed brain note fields beyond free-text + metadata JSON

## Decisions

### Decision: Flags at both pipeline and step level; step overrides pipeline

**Rationale**: Some pipelines want all agent steps to be brain-aware (pipeline-level default). Others need surgical injection only on specific steps. Step-level flags override the pipeline-level flag using a tri-state: `unset` (inherit), `true`, `false`. This avoids a separate pipeline-level config section and keeps YAML readable.

**Alternative considered**: Separate `brain:` block with `read:` and `write:` sub-keys. Rejected — too verbose for common cases.

### Decision: Pre-context injected as a preamble prefix to the `prompt` string

**Rationale**: Plugins receive a `prompt` string. Injecting context as a prefix keeps the plugin protocol unchanged — no new plugin API, no new vars key. The agent sees the full context at the beginning of the prompt exactly as if the pipeline author had written it.

**Alternative considered**: Pass brain context as a reserved `vars["__brain__"]` key and let plugins handle it. Rejected — requires changes to every plugin that wants to respect brain context; preamble injection is transparent.

### Decision: `<brain>` XML block convention for write responses

**Rationale**: Agents must emit structured output that the runner can reliably parse without depending on JSON mode or a specific model capability. XML with a named root tag is LLM-natural, unambiguous to parse with a simple regex/SAX scan, and doesn't conflict with Markdown output.

**Alternative considered**: Require JSON output mode. Rejected — not all providers support structured output; forces agent responses to be pure JSON which breaks natural language answers.

### Decision: `brain_notes` table owned exclusively by agents; `runs` is read-only from agent perspective

**Rationale**: Keeps operational data safe. The brain context preamble for `use_brain` steps includes explicit instructions: "Do NOT modify the `runs` table." All agent writes go through the runner's `<brain>` parser, which only calls `store.InsertBrainNote`.

**Alternative considered**: Let agents issue arbitrary `exec` SQL. Rejected — no guardrails, any hallucinated SQL could corrupt runs.

### Decision: Remove `step_db.go` (the `db` step type)

**Rationale**: The read use-case is better served by `use_brain`. No existing `.pipeline.yaml` in the repo uses `type: db` steps (grep confirms zero matches). Keeping it would create confusion about which mechanism to use.

**Migration**: Any pipeline needing raw SQL reads that `use_brain` cannot satisfy can use a `builtin.db_query` shim (thin wrapper kept only if a future need arises); for now, the type is simply removed.

### Decision: `BrainInjector` is an interface with a default `StoreBrainInjector` implementation

**Rationale**: Enables testing with a mock injector; enables future swap to a vector store without touching the runner.

## Risks / Trade-offs

- [Risk: Brain preamble bloats prompts and increases token cost] → Mitigation: Preamble is only injected for steps with `use_brain` active. The `BrainInjector` implementation caps recent brain notes to a configurable N (default 10) and truncates individual note bodies to 500 chars.
- [Risk: Agent ignores `<brain>` instruction or emits malformed XML] → Mitigation: The runner treats `<brain>` parsing as best-effort. Missing or malformed blocks are silently skipped with a debug log line — the step does not fail. Future: add a `require_brain_write: true` flag that fails the step if no `<brain>` block is found.
- [Risk: Cross-run brain note contamination] → Mitigation: Phase 1 only injects notes written within the current pipeline run. A `run_id` column on `brain_notes` allows future scoping; notes from other runs are excluded from the preamble unless the pipeline sets `brain_scope: all`.
- [Risk: Removing `db` step breaks existing pipelines] → Mitigation: Grep confirms no current pipeline YAML uses `type: db`. The type is removed with no migration needed.
- [Risk: LLM generates SQL that reads sensitive data even with read-only intent] → Mitigation: Phase 1 brain preamble does not grant free SQL access. It only surfaces pre-fetched summaries assembled by `BrainInjector`. Raw query capability is a future opt-in.

## Migration Plan

1. Add `brain_notes` table via schema migration in `store/schema.go` — backward compatible, additive.
2. Add `BrainInjector` interface and `StoreBrainInjector` in `pipeline/brain.go`.
3. Extend `Pipeline` and `Step` structs with `UseBrain`/`WriteBrain` fields.
4. Extend `runner.go` prompt-building path to call the injector before dispatching.
5. Add `<brain>` block parser and `store.InsertBrainNote` call after step completion.
6. Delete `step_db.go`.
7. Update `pipeline.go`'s `resolveExecutor` to remove `db` case.
8. Rollback: revert is a clean set of file changes with no data-destructive migrations.

## Open Questions

- Should `brain_scope` be a Phase 1 field or deferred? Currently leaning toward Phase 1 as a simple bool (`cross_run: false`), since the `run_id` column is cheap to add now.
- What is the right default cap for injected brain notes? 10 notes × 500 chars = 5 KB preamble seems safe, but needs validation against Ollama's context window for local models.
