## Context

gl1tch currently has three executor registration paths that converge in `executor.Manager` but are initialized through separate code paths with different information densities:

1. **Bundled providers** (`internal/providers/`) — structured YAML, binary detection, model lists, no system prompt
2. **Sidecar wrappers** (`~/.config/glitch/wrappers/*.yaml`) — user-defined command+args, no prompt layer, loaded once at startup via `LoadWrappersFromDir`
3. **APM agents** (`.apm/agents/*.agent.md`) — rich frontmatter (name, description, version, capabilities list) + markdown system prompt; installed on-demand via `ApmManager.InstallAndWrap()`

The brain system (`internal/pipeline/capability.go`) already seeds all registered executors as capability notes (`type:capability`) injected into every pipeline step's context. APM agents have capability semantics in their frontmatter but this data is discarded after the `--system` flag is constructed. The router's embedding cache (`internal/router/`) is populated only from pipeline YAML descriptions, not from APM agent descriptions.

The gap: APM agents are richer than sidecar wrappers but treated as second-class — their capability metadata is thrown away, they don't seed routable pipeline templates, and the router can't find them by intent.

## Goals / Non-Goals

**Goals:**
- Persist APM capability frontmatter as queryable brain notes at install time
- Materialize a pipeline template file per APM agent so the router can discover and dispatch it
- Enable lazy APM install triggered by router intent matching

**Non-Goals:**
- Changing how bundled providers are loaded or described
- Publishing APM agents or modifying the APM CLI itself
- Replacing the existing `RequireAgent` / `AgentCapabilityProvider` interface
- Building a UI for browsing the APM registry

## Decisions

### 1. Pipeline materialization at install time, not on-demand

**Decision:** When `InstallAndWrap()` runs, if `apm.yml` declares a `pipeline` stanza for that agent, write `~/.config/glitch/pipelines/apm.<name>.pipeline.yaml` immediately.

**Rationale:** The router loads pipeline descriptions at startup and builds its embedding cache then. Deferring materialization to first-use would require cache invalidation and hot-reload plumbing. Writing at install time fits the existing startup model — if a new pipeline appears, the next gl1tch invocation picks it up.

**Alternative considered:** Store pipeline fragments in the brain store and reconstruct at startup. Rejected — the brain store is for transient run context, not durable config artifacts. Pipeline YAML on disk is the canonical source of truth.

### 2. APM capability brain notes use `run_id=0` (system-scope)

**Decision:** APM capability notes are written with `run_id=0` (same as the capability seeder) so they persist across runs and are always included in step context without consuming the per-run 10-note cap.

**Rationale:** Capabilities are static facts about what the system can do — they belong in the system scope, not tied to a specific run. The existing `CapabilitySeeder` already uses this pattern.

**Alternative considered:** Upsert on every `InstallAndWrap()` call. Accepted as a side effect — writing the same note content twice is idempotent at the store level.

### 3. Router APM awareness via description injection into the embedding cache

**Decision:** After `buildFullManager()`, iterate APM-registered executors (those with ID prefix `apm.`) and synthesize a minimal pipeline description string (`"[apm] <name>: <description>"`) for embedding. Cache these alongside standard pipeline embeddings. On a confident match, emit a synthetic one-step pipeline that routes to the `apm.<name>` executor.

**Rationale:** Avoids requiring every APM agent to ship a full pipeline YAML just to be router-discoverable. The description in the `.agent.md` frontmatter is sufficient signal.

**Alternative considered:** Require a `pipeline` stanza in `apm.yml` for router discovery. Too much friction — agents should be discoverable by default.

### 4. `source` tag on capability notes

**Decision:** `CapabilitySeeder` adds `source:builtin` to provider-derived notes; `InstallAndWrap()` adds `source:apm` to APM-derived notes. The tag is informational — no filtering logic changes.

**Rationale:** Gives future pipeline steps the ability to express "prefer APM agents for this" without requiring it now.

## Risks / Trade-offs

- **Stale pipeline templates** → If an APM agent is updated (re-installed), `InstallAndWrap()` overwrites the pipeline template. User customizations to `~/.config/glitch/pipelines/apm.*.pipeline.yaml` will be lost. Mitigation: only write if the file does not exist; log a notice if skipping.

- **Router false positives on APM descriptions** → Short or generic agent descriptions may match unrelated prompts at ≥ 0.85. Mitigation: threshold is already tunable; APM-sourced descriptions are prefixed with `[apm]` to reduce collision with pipeline descriptions.

- **Brain note accumulation** → Each `apm install` writes new capability notes. Over time the system-scope notes grow. Mitigation: upsert by `(title, source:apm)` composite key rather than appending.

## Migration Plan

All changes are additive:
1. `apm.yml` `pipeline` stanza is optional — existing entries continue to work unchanged.
2. Brain notes written with `source:apm` are new rows — no schema migration needed.
3. Pipeline templates written to `~/.config/glitch/pipelines/` only if not already present.

No rollback steps needed — removing the new fields leaves behavior identical to today.

## Open Questions

- Should the synthetic router pipeline for APM agents support multi-step composition (e.g., `pre-flight` → `apm-agent` → `summarize`)? Deferred — start with single-step dispatch.
- Should `apm.yml` support a `router_description` override distinct from the `.agent.md` description? Likely yes for agents with terse frontmatter descriptions — punt to a follow-on change.
