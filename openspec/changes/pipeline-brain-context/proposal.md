## Why

The existing `db` step type gives pipelines raw SQL access to the ORCAI result store, but it forces pipeline authors to write brittle SQL and provides no automatic context for AI agents to reason about the database — agents run blind to the knowledge already stored in the system. We need a first-class mechanism for agent steps to optionally receive curated context about the database schema and stored data, and a safe write path where agents can record insights to their own isolated tables without touching operational data.

## What Changes

- Add a `use_brain` flag (boolean) to `Pipeline` and `Step` structs. When set, the pipeline runner automatically prepends a system-level pre-context block to any agent step's prompt, describing the ORCAI SQLite schema, how to issue read queries, and how to interpret the results.
- Add a `write_brain` flag (boolean) to `Pipeline` and `Step` structs. When set, the runner injects an additional pre-context block granting write access to a protected `brain_notes` table, with instructions on schema, insert format, and the expectation that responses include a structured `<brain>` XML block that the runner will parse and persist.
- Add a `brain_notes` table to the ORCAI SQLite store via schema migration. This table is owned by agents; the operational `runs` table is read-only from the agent perspective.
- Add a `BrainInjector` component to `internal/pipeline` responsible for assembling the pre-context strings from live schema introspection and recent `brain_notes` entries.
- Extend the pipeline runner's `resolveExecutor` / prompt-building path to call `BrainInjector` when flags are active before dispatching to the plugin.
- **BREAKING**: Remove the `db` step type from `step_db.go`. Its read use-case is superseded by `use_brain`; its write use-case is superseded by `write_brain`. Pipelines using `type: db` must migrate to one of the new flags or use a `builtin.db_query` wrapper if raw SQL is truly needed.

## Capabilities

### New Capabilities

- `pipeline-brain-read`: Pipeline and step-level `use_brain` flag; `BrainInjector` read-context assembly; schema introspection; injection into agent prompt before plugin dispatch.
- `pipeline-brain-write`: Pipeline and step-level `write_brain` flag; `brain_notes` SQLite table; `<brain>` XML block parsing from agent response; runner persistence of brain notes.
- `pipeline-brain-feedback`: Post-execution injection of recent `brain_notes` entries back into subsequent steps' context when both `use_brain` and `write_brain` are active on the same pipeline, forming a feedback loop that improves agent decisions across pipeline runs.

### Modified Capabilities

- `pipeline-execution-context`: `ExecutionContext` gains a `BrainSummary() string` method that returns a pre-formatted summary of recent brain notes for injection; `WithBrain(injector BrainInjector)` option passed to `pipeline.Run`.

## Impact

- `internal/pipeline/step_db.go` — removed (or gutted to a thin `builtin.db_query` shim if backward compat is needed)
- `internal/pipeline/pipeline.go` — `Pipeline` and `Step` structs gain `UseBrain bool` and `WriteBrain bool` fields
- `internal/pipeline/runner.go` — prompt-building path extended with brain injection
- `internal/pipeline/brain.go` — new file; `BrainInjector` interface + default SQLite implementation
- `internal/store/schema.go` — `brain_notes` table migration
- `internal/store/store.go` — `InsertBrainNote`, `RecentBrainNotes` methods
- `.pipeline.yaml` files using `type: db` steps must be updated
- No changes to plugin binary protocol; brain injection is transparent to plugins
