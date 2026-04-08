---
name: observer-query
description: Build or review an observer query that grounds the LLM in workspace state by combining Elasticsearch results from internal/esearch with the glitchd workspace state bundle. Use when implementing or fixing scoped Ask paths in internal/observer.
---

Apply the canonical pattern for observer queries in gl1tch.

Background: Phase 1 of the observer rewrite (project_observer_rewrite.md) is the ES + git collector + observer query engine. The known gap (project_observer_workspace_grounding.md) is that the scoped Ask path skips generateQuery and has no workspace context — the fix was blocked on the capabilities refactor, which has now landed (project_capability_unified.md). This skill captures the unblock pattern so it gets applied consistently.

The pattern, in order:

1. **Pull workspace state from glitchd**, do not reconstruct it locally.
   - The state bundle includes: cwd, recent git refs, open buffers, recent capability events, active pipeline (if any).
   - Read internal/observer/ for the current state-bundle accessor. If it does not exist yet, it must be added before the query path can be considered grounded — flag this and stop.

2. **Generate the ES query via generateQuery**, never via free-form LLM string concatenation.
   - Read internal/esearch/ for the index mapping and query builder helpers.
   - The LLM's role is to pick fields and filters from the structured builder, not to emit raw JSON DSL.
   - Always include a filter scoped to the workspace from step 1 (cwd or workspace_id) — this is the whole point of the unblock.

3. **Run the query through internal/esearch**, capture both hits and the query that was run.
   - Persist the executed query for replay/debugging — observer queries must be reproducible.

4. **Hand results back to the assistant via capability.Router**, not directly into a prompt loop.
   - The router exposes the observer as a named capability. The LLM asks for it by name; the observer returns structured results.
   - This preserves the contract from project_assistant_router.md: the LLM never constructs commands.

5. **Default model for any LLM step is qwen2.5:7b via local Ollama** (project_local_llm_default.md).

When reviewing an existing observer query path, walk it against these five steps in order and flag the first one that's missing. The most common failure today is step 1 — workspace-blind queries that go straight to step 2.

When implementing a new one, write the smallest code that closes the loop end-to-end before adding any options or knobs. Pre-1.0 rule: no migration shims, no compatibility fallbacks for old query formats — wipe and restart instead.

Files to always read before touching this path:
- internal/observer/
- internal/esearch/
- internal/router/ and internal/capability/router.go
- internal/assistant/ (Ask entry point)
