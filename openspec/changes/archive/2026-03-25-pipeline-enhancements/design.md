## Context

The current pipeline runner (`internal/pipeline/runner.go`) is a simple for-loop that executes steps sequentially, passes string-only variables via `{{key}}` interpolation, and propagates the first error immediately. The ensemble project demonstrated that a production-grade workflow engine needs: a DAG execution model, typed per-step output context, retry/error policies, and built-in step executors compiled directly into the binary. All of this must remain a single binary — no external orchestration daemon.

Current state constraints:
- `Step.Plugin` maps to a `plugin.Manager` lookup — works for both native gRPC plugins and CLI adapters
- `Step.Condition` already supports if/then/else branching with substring/regex/length predicates
- `Step.PublishTo` field exists in the struct but is not wired in the runner
- The event bus (`orcai.telemetry`) is already live and used by chatui

## Goals / Non-Goals

**Goals:**
- DAG execution: steps with `needs` run in parallel once their dependencies complete
- Structured output: steps return `map[string]any` accessible as `step.<id>.data.<key>` in templates
- Retry policies: per-step `retry` block with `max_attempts`, `interval`, optional `on` condition
- `on_failure` routing: step can nominate a recovery step to run on failure
- `for_each`: iterate a step over a slice of strings, collecting all outputs
- Builtin steps: `builtin.assert`, `builtin.set_data`, `builtin.log`, `builtin.sleep`, `builtin.http_get`
- Event publishing: `publish_to` writes step output JSON to the named bus topic
- Hierarchical plugin naming: `category.action` registry so sidecar YAML can declare category
- Pipeline bus events: `orcai.pipeline.step.started`, `orcai.pipeline.step.done`, `orcai.pipeline.step.failed`

**Non-Goals:**
- Distributed / multi-node execution (ensemble's control_plane + worker split) — single binary only
- Persistent checkpointing or resume-from-failure across process restarts
- Dynamic plugin hot-reload at runtime
- GUI pipeline editor (handled separately by `_promptbuilder`)
- Breaking existing pipeline YAML — all new fields are optional

## Decisions

### 1. DAG execution: goroutine fan-out with a dependency counter

**Decision**: Represent the step graph as `map[stepID][]stepID` adjacency list. Each step has an atomic counter of unsatisfied dependencies. A dispatcher goroutine receives step completion events on a channel, decrements dependents' counters, and launches ready steps as goroutines. A WaitGroup tracks all in-flight steps.

**Why over alternatives**:
- *topological sort + fixed thread pool*: simpler but limits concurrency to pool size and requires pre-sorting, which breaks error-branch routing
- *channel pipeline (Rob Pike style)*: elegant but doesn't handle arbitrary DAG shapes or fan-in
- The counter approach is what ensemble's `JobPlanner.__recalculate_job_status` converges to, just made explicit

**Data structure**:
```go
type stepState struct {
    mu          sync.Mutex
    status      stepStatus  // waiting / running / done / failed / skipped
    output      map[string]any
    pendingDeps atomic.Int32
}
```

### 2. Structured output context: `step.<id>.data.<key>`

**Decision**: Each step executor returns `map[string]any` (nil = no output). The runner maintains a `context map[string]any` with shape:
```json
{
  "param": { "key": "value" },
  "step": {
    "fetch": { "state": "done", "data": { "url": "https://..." } }
  }
}
```
Template interpolation is upgraded from simple `strings.ReplaceAll` to a minimal path-expression evaluator: `{{step.fetch.data.url}}` walks the nested map.

**Why not Go `text/template`**: Overkill for variable substitution; introduces whitespace/error semantics that differ from the current simple `{{key}}` convention. A custom 50-line path walker is sufficient and keeps the YAML authoring experience simple.

**Why not Jinja2 equivalent (pongo2/etc.)**: External dependency for marginal benefit at this scale.

### 3. Builtin steps: statically registered map in `internal/pipeline/builtin.go`

**Decision**: A `var builtinRegistry = map[string]BuiltinFunc{...}` checked before the plugin manager. Keys use `builtin.*` prefix. `BuiltinFunc` signature:
```go
type BuiltinFunc func(ctx context.Context, args map[string]any) (map[string]any, error)
```
The runner checks `builtinRegistry[step.Type]` first, falls back to plugin manager.

**Why over alternatives**:
- *Separate plugin binary for builtins*: defeats single-binary goal
- *Interface-based registry*: adds indirection for no gain when all builtins are in-process

### 4. Retry: manual loop with exponential backoff option

**Decision**: No external retry library. The runner wraps the step execution in a loop:
```go
for attempt := 0; attempt < step.Retry.MaxAttempts; attempt++ {
    out, err = executeStep(...)
    if err == nil { break }
    time.Sleep(step.Retry.Interval)
}
```
`Retry.On` supports `"always"` (default) and `"on_failure"`. Exponential backoff is a follow-up; fixed interval is sufficient for the first version.

**Why not tenacity-style library**: The ensemble project uses tenacity because Python lacks native retry idioms. Go's explicit loop is cleaner and more debuggable.

### 5. `for_each`: expand step at run time

**Decision**: When `step.ForEach` is set, the runner expands it into N virtual step executions before DAG construction. Each expansion gets a unique ID suffix (`stepID[0]`, `stepID[1]`…) and the special variable `{{item}}` bound to the current element. Outputs are collected into `step.<id>.items[i].data`. Steps that `needs` a `for_each` step must wait for all expansions.

**Why not runtime iteration**: Expanding before DAG construction reuses the same DAG execution machinery without special-casing loops in the dispatcher.

### 6. Plugin hierarchical naming: `category.action` lookup chain

**Decision**: `plugin.Manager` adds a secondary index keyed by `category.action`. When a pipeline step's `Type` is `"providers.claude.chat"`, the manager splits on the last `.` to find category `"providers.claude"` and action `"chat"`, then looks up the plugin named `"providers.claude"` and calls `Execute` with `action` in vars. CLI wrapper sidecar YAML gains an optional `category` field; if present, the adapter registers under `category.name`.

**Why not rename existing plugins**: Backwards compatible — single-word names still resolve directly.

### 7. Event bus pipeline events

**Decision**: The runner publishes to `orcai.pipeline.step.started` and `orcai.pipeline.step.done` / `orcai.pipeline.step.failed` using the existing bus client. Payload is JSON:
```json
{ "pipeline": "my-pipe", "step": "fetch", "status": "done", "duration_ms": 42, "output": {...} }
```
The runner reads `bus.addr` from the filesystem (same pattern as chatui's `connectBus()`). If the file is absent, events are silently dropped — pipeline execution is not gated on bus availability.

## Risks / Trade-offs

- **DAG cycle detection**: If a user creates a cycle in `needs`, the dispatcher will deadlock waiting for steps that never unblock. → Mitigation: add a topological sort pass at pipeline load time; return an error if a cycle is detected.
- **Goroutine leak on cancel**: If `ctx` is cancelled mid-pipeline, in-flight goroutines must drain. → Mitigation: pass `ctx` through to each step goroutine; use `context.WithCancel` to propagate cancellation; WaitGroup ensures all goroutines exit before `Run` returns.
- **for_each expansion memory**: A `for_each` over a 10,000-item list expands to 10,000 goroutines. → Mitigation: add a concurrency cap (`MaxParallel` field on Pipeline, default 8) that limits simultaneous in-flight steps.
- **Backwards compatibility**: Existing pipeline YAML uses `{{stepID.out}}` style variables. The new context structure changes this to `step.<id>.data.value`. → Mitigation: the template interpolator checks `{{step.<id>.out}}` first, then falls back to `step.<id>.data.value`, with a deprecation log line; old pipelines keep working.

## Migration Plan

1. All changes are additive to existing structs — no YAML schema version bump required
2. Deploy by replacing the binary; existing pipelines continue to run unchanged
3. Rollback: revert to previous binary; no persistent state is written by the pipeline engine

## Open Questions

- Should `builtin.http_get` support TLS client certificates, or is plain GET sufficient for v1?
- Should `for_each` support structured objects (list of maps) or strings only for v1?
- Does the pipeline builder UI (`_promptbuilder`) need updates to expose `needs` / `retry` fields, or is that a separate change?
