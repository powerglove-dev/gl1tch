## Why

The current pipeline engine is a thin sequential runner with string-only state, no retry logic, and no parallel execution — functional for demos but not production-grade. The ensemble Python project (a distributed workflow orchestration system built by the same author) contains battle-tested patterns for lifecycle hooks, structured data flow, conditional branching, retries, parallel steps, and provider abstraction that we can port directly into orcai's single-binary architecture.

## What Changes

- **Pipeline step lifecycle**: Add `init` / `execute` / `cleanup` phases per step so plugins can allocate and release resources cleanly
- **Structured step output**: Replace string-only `step<ID>.out` variables with a typed `step.<id>.data.<key>` context map, enabling nested output from steps and JSON extraction
- **Parallel step execution**: Add `needs: [stepA, stepB]` dependency declarations to pipeline YAML; the runner builds a DAG and executes independent steps concurrently with goroutines
- **Retry policies**: Add `retry` block to pipeline steps — `max_attempts`, `interval`, and optional `on` condition (always / on_failure)
- **Error branching**: Add `on_failure` step routing alongside `condition` — pipelines can specify a recovery step on failure rather than aborting immediately
- **Loop / forEach**: Add `for_each` clause to step definition — iterates plugin execution over a list of inputs, collecting outputs into an array
- **Event publishing from steps**: Implement the existing `publish_to` field in the runner so step output is forwarded to the event bus topic
- **Plugin hierarchical naming**: Adopt `category.action` naming convention (e.g. `builtin.assert`, `providers.claude.chat`) in the plugin registry and discovery system
- **Builtin steps**: Add a small set of built-in step executors that don't require a plugin (assert, set_data, log, sleep, http_get) — compiled directly into the binary
- **Observability**: Emit structured step lifecycle events on `orcai.pipeline.*` bus topics (step started, step done, step failed) with timing and metadata

## Capabilities

### New Capabilities

- `pipeline-step-lifecycle`: Per-step `init` / `execute` / `cleanup` phases with structured typed output context
- `pipeline-parallel-execution`: DAG-based parallel step execution driven by `needs` declarations
- `pipeline-retry-and-error`: Retry policies and `on_failure` routing for resilient pipeline execution
- `pipeline-for-each`: `for_each` clause to iterate a step over a list, collecting results
- `pipeline-builtins`: Compiled-in step executors: `builtin.assert`, `builtin.set_data`, `builtin.log`, `builtin.sleep`, `builtin.http_get`
- `pipeline-event-publishing`: Step results published to event bus topics via `publish_to` field
- `plugin-hierarchical-naming`: `category.action` naming convention for plugin registry and discovery

### Modified Capabilities

- `cli-adapter-discovery`: CLI adapters need to declare a `category` in sidecar YAML so they resolve under the hierarchical naming scheme

## Impact

- `internal/pipeline/pipeline.go` — extend `Step` struct with `Needs`, `Retry`, `ForEach`, `OnFailure` fields; add `RetryPolicy` struct
- `internal/pipeline/runner.go` — rewrite execution loop from sequential queue to DAG walk with goroutine fan-out; implement `publish_to`; add lifecycle phases
- `internal/pipeline/condition.go` — extend condition evaluation to support `step.<id>.data.<key>` path expressions
- `internal/pipeline/template.go` — extend `Interpolate` to support nested `step.<id>.data.<key>` path access
- `internal/pipeline/builtin.go` — new file for builtin step executors
- `internal/plugin/manager.go` — add `category.action` registry lookup
- `internal/discovery/discovery.go` — surface plugin category from sidecar YAML
- `internal/bus/bus.go` — no changes needed; existing pub/sub is sufficient
- No breaking changes to existing pipeline YAML — all new fields are optional with zero-value defaults
