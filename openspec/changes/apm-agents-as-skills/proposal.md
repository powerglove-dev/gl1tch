## Why

gl1tch has three parallel executor registration paths — bundled providers, sidecar YAML wrappers, and APM agents — that all converge in `executor.Manager` but are treated as fundamentally different things. APM agents carry rich semantic content (capabilities, system prompts) that the brain system can reason about, while sidecar wrappers are opaque command invocations. Unifying them under a single "skill" model lets the APM ecosystem become gl1tch's plugin marketplace: install an agent, get a routable pipeline step, a brain-visible capability, and a composable skill — automatically.

## What Changes

- `apm.yml` gains an optional `pipeline` stanza per entry — a minimal pipeline fragment materialized to `~/.config/glitch/pipelines/apm.<name>.pipeline.yaml` on install
- `ApmManager.InstallAndWrap()` persists APM agent `capabilities` frontmatter as brain notes tagged `type:capability source:apm` — making them query-visible to `RequireAgent` and pipeline steps
- Router embedding cache includes APM agent descriptions; a confident match (≥ 0.85) triggers lazy install + dispatch without user intervention
- `CapabilitySeeder` distinguishes `source:apm` vs `source:builtin` in seeded notes so steps can filter by origin

## Capabilities

### New Capabilities

- `apm-skill-pipeline-materialization`: On APM agent install, generate a pipeline template file in the user's pipelines directory from the agent's `pipeline` stanza in `apm.yml`
- `apm-capability-brain-seeding`: Persist APM agent `capabilities` frontmatter as brain notes (`type:capability source:apm`) so pipeline steps and `RequireAgent` can resolve agents by capability tag
- `apm-router-awareness`: Extend the router's embedding cache to include APM agent descriptions; auto-install and dispatch when cosine similarity ≥ 0.85

## Impact

- `apm.yml` schema (new `pipeline` stanza per entry)
- `internal/apmmanager/model.go` — `InstallAndWrap()` extended with brain seeding + pipeline materialization
- `internal/router/` — embedding cache population includes APM agent descriptions
- `internal/pipeline/capability.go` — `CapabilitySeeder` tags notes with `source:apm` or `source:builtin`
- No breaking changes to existing executor, pipeline, or brain APIs
