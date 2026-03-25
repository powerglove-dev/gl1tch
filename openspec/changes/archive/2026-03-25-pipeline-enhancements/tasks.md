## 1. Step Struct and YAML Schema Extensions

- [x] 1.1 Add `Needs []string`, `Retry *RetryPolicy`, `OnFailure string`, `ForEach string` fields to `Step` struct in `internal/pipeline/pipeline.go`
- [x] 1.2 Add `RetryPolicy` struct: `MaxAttempts int`, `Interval time.Duration`, `On string` (yaml tag `interval` parses duration string)
- [x] 1.3 Add `MaxParallel int` field to `Pipeline` struct (default 8 when zero)
- [x] 1.4 Update YAML unmarshalling so `interval` string (e.g. `"2s"`) parses into `time.Duration` via a custom `UnmarshalYAML`

## 2. Step Lifecycle and Structured Output

- [x] 2.1 Define `StepExecutor` interface in `internal/pipeline/executor.go`: `Init(ctx) error`, `Execute(ctx, args map[string]any) (map[string]any, error)`, `Cleanup(ctx) error`
- [x] 2.2 Add `stepState` struct to runner: `status stepStatus`, `output map[string]any`, `pendingDeps atomic.Int32`
- [x] 2.3 Rewrite `internal/pipeline/template.go` `Interpolate` function to support dot-path expressions (`step.<id>.data.<key>`) by walking a nested `map[string]any` context
- [x] 2.4 Add backwards-compat shim in `Interpolate`: detect `{{<ID>.out}}` pattern (no dots), resolve to `step.<id>.data.value`, log deprecation warning
- [x] 2.5 Update execution context shape to `{"param": {...}, "step": {"<id>": {"state": "...", "data": {...}}}}`

## 3. Builtin Step Registry

- [x] 3.1 Create `internal/pipeline/builtin.go` with `type BuiltinFunc func(ctx context.Context, args map[string]any) (map[string]any, error)` and `var builtinRegistry map[string]BuiltinFunc`
- [x] 3.2 Implement `builtin.assert`: evaluate `condition` arg (`true`, `false`, `contains:<s>`, `matches:<re>`, `len > <n>`) against optional `value` arg; return error with optional `message` on failure
- [x] 3.3 Implement `builtin.set_data`: merge `data` map arg into output map and return it
- [x] 3.4 Implement `builtin.log`: interpolate `message` arg against context, write to pipeline output writer, return nil output
- [x] 3.5 Implement `builtin.sleep`: parse `duration` arg, call `time.Sleep` respecting context cancellation
- [x] 3.6 Implement `builtin.http_get`: HTTP GET to `url` arg with optional `timeout` arg (default 10s); return `{"status": <int>, "body": <string>}` on 2xx, error on non-2xx or network failure

## 4. DAG Execution Engine

- [x] 4.1 Add `buildDAG(steps []Step) (map[string][]string, error)` in `internal/pipeline/dag.go`: builds adjacency list of `stepID → []dependentIDs`; returns error on cycle detected (topological sort via DFS)
- [x] 4.2 Add `detectCycle(graph map[string][]string) error` using DFS with grey/black coloring; return descriptive error including the cycle path
- [x] 4.3 Rewrite `pipeline.Run` in `internal/pipeline/runner.go` to use the DAG: initialise `pendingDeps` counter for each step from its `needs` count; steps with 0 deps go into a ready channel immediately
- [x] 4.4 Add a semaphore channel of capacity `Pipeline.MaxParallel` (default 8) that goroutines acquire before executing and release on completion
- [x] 4.5 Implement the dispatcher loop: receive from a `completed chan stepResult` channel; decrement dependent counters; enqueue newly ready steps; stop when all steps reach a terminal state
- [x] 4.6 For_each expansion: before DAG construction, detect steps with `ForEach` set, resolve the template to a string list, and replace the step with N cloned steps (`id[0]`…`id[N-1]`) each with `{{item}}` injected into their args

## 5. Retry and Error Routing

- [x] 5.1 Extract single-step execution into `executeStep(ctx, step, state) (map[string]any, error)` that calls `Init`, `Execute`, `Cleanup` in order; always calls `Cleanup` even on failure
- [x] 5.2 Wrap `executeStep` in a retry loop: if `step.Retry != nil`, loop up to `MaxAttempts`, sleep `Interval` between attempts, stop early on context cancellation
- [x] 5.3 After final failure, check `step.OnFailure`: if set, enqueue the named on_failure step as a ready step; mark the original step `failed`
- [x] 5.4 When a step is marked `failed` with no `on_failure`, mark all transitive dependents (via DAG walk) as `skipped`

## 6. Plugin Hierarchical Naming

- [x] 6.1 Add a `categoryIndex map[string]Plugin` field to `plugin.Manager` in `internal/plugin/manager.go`
- [x] 6.2 Add `RegisterCategory(category, action string, p Plugin)` method that stores under `category` and tracks valid actions
- [x] 6.3 Update `Manager.Get(name string)` to: (a) check direct registry, (b) if not found, split on last `.`, check category index, return plugin with `_action` attached in a wrapper
- [x] 6.4 Add `category` field to CLI wrapper sidecar YAML schema in `internal/plugin/cli_adapter.go`; if present, call `RegisterCategory` on load
- [x] 6.5 Update the runner to check `builtinRegistry[step.Type]` before calling `manager.Get`; return clear error for unknown `builtin.*` types

## 7. Event Bus Integration in Runner

- [x] 7.1 Add `connectBus() (pb.EventBusClient, error)` helper to `internal/pipeline/runner.go` that reads `~/.config/orcai/bus.addr` and dials gRPC (same pattern as `chatui/provider_bridge.go`)
- [x] 7.2 At pipeline start, attempt bus connection; store client or nil (no retry); log if unavailable
- [x] 7.3 In the step dispatcher, publish `orcai.pipeline.step.started` before each step goroutine launches
- [x] 7.4 After each step goroutine completes, publish `orcai.pipeline.step.done` or `orcai.pipeline.step.failed` with `duration_ms`
- [x] 7.5 After step succeeds, check `step.PublishTo`: if set and bus client non-nil, publish output JSON to the named topic

## 8. Tests and Verification

- [x] 8.1 Add `TestDAGCycleDetection` in `internal/pipeline/dag_test.go`: verify cycle returns error; valid DAG returns nil
- [x] 8.2 Add `TestParallelExecution` in `internal/pipeline/runner_test.go`: two independent steps; verify they run concurrently (use a latched channel or time assertion)
- [x] 8.3 Add `TestRetryPolicy` in `internal/pipeline/runner_test.go`: step fails twice then succeeds; verify only 3 executions occur
- [x] 8.4 Add `TestOnFailure` in `internal/pipeline/runner_test.go`: step fails; verify on_failure step runs; verify dependent marked skipped
- [x] 8.5 Add `TestForEach` in `internal/pipeline/runner_test.go`: for_each over 3 items; verify 3 executions and output at `step.<id>.items[*].data`
- [x] 8.6 Add `TestBuiltins` in `internal/pipeline/builtin_test.go`: one test per builtin covering success and failure paths
- [x] 8.7 Add `TestHierarchicalNaming` in `internal/plugin/manager_test.go`: register plugin under category; verify `category.action` resolves; verify `_action` var injected
- [x] 8.8 Add `TestTemplateInterpolation` in `internal/pipeline/template_test.go`: nested `step.<id>.data.<key>` resolution; missing path left unchanged; legacy `{{ID.out}}` compatibility
- [x] 8.9 Run `go test ./internal/pipeline/... ./internal/plugin/...` — all tests pass
- [x] 8.10 Run `go build ./...` — no compilation errors
- [x] 8.11 Commit: `feat(pipeline): ensemble-inspired DAG execution, retries, builtins, and hierarchical plugin naming`
