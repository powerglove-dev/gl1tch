## 1. Tests for Existing Pipeline Package

- [x] 1.1 Create `internal/pipeline/runner_test.go`: happy-path sequential execution with a stub plugin, assert final output returned
- [x] 1.2 `runner_test.go`: step with `If: "false"` is skipped, assert plugin not called
- [x] 1.3 `runner_test.go`: step output stored in vars; subsequent step template `{{stepA.out}}` resolves correctly
- [x] 1.4 `runner_test.go`: plugin `Execute` error propagates as `Run` return error
- [x] 1.5 Create `internal/pipeline/template_test.go`: `{{key}}` interpolation with string value
- [x] 1.6 `template_test.go`: missing key leaves placeholder unchanged
- [x] 1.7 Create `internal/pipeline/condition_test.go`: `"always"` → true, `"false"` → false
- [x] 1.8 `condition_test.go`: `"contains:foo"` against output containing/not containing `"foo"`
- [x] 1.9 `condition_test.go`: `"matches:^ok"` regex match

## 2. Tests for Existing Plugin Package

- [x] 2.1 Create `internal/plugin/manager_test.go`: `Register` then `Get` returns the plugin
- [x] 2.2 `manager_test.go`: `Get` on unknown name returns `(nil, false)`
- [x] 2.3 `manager_test.go`: duplicate `Register` currently panics — document the test as `t.Skip("panic on duplicate — fixed in next task")`

## 3. Make Manager.Register Safe

- [x] 3.1 Change `Manager.Register(p Plugin)` signature to `Register(p Plugin) error` in `internal/plugin/manager.go`
- [x] 3.2 Replace panic with `return fmt.Errorf("plugin %q already registered", p.Name())`
- [x] 3.3 Update `internal/host/host.go` call site: log the error and continue (do not abort plugin loading)
- [x] 3.4 Update `manager_test.go` duplicate test: remove skip, assert error returned, assert first registration unchanged

## 4. Extend Step and Pipeline Structs

- [x] 4.1 Add `Executor string \`yaml:"executor"\`` to `Step` in `internal/pipeline/pipeline.go`; add comment: "Executor supersedes Plugin when set; use 'builtin.*' or 'category.action' form"
- [x] 4.2 Add `Args map[string]any \`yaml:"args"\`` to `Step`; add comment: "Args supersedes Vars when set; supports nested values"
- [x] 4.3 Add `Needs []string \`yaml:"needs"\`` to `Step`; add comment: "Step IDs that must complete before this step runs (DAG — implemented in pipeline-enhancements)"
- [x] 4.4 Add deprecated comment above `Plugin string` and `Vars map[string]string` fields: "// Deprecated: use Executor and Args"
- [x] 4.5 Add `Vars map[string]any \`yaml:"vars"\`` to `Pipeline` struct for pipeline-level seed context

## 5. ExecutionContext Named Type

- [x] 5.1 Create `internal/pipeline/context.go` with `ExecutionContext` struct: `mu sync.RWMutex`, `data map[string]any`
- [x] 5.2 Implement `NewExecutionContext() *ExecutionContext` — initialises empty `data` map
- [x] 5.3 Implement `Get(path string) (any, bool)` — splits path on `.`, walks nested maps
- [x] 5.4 Implement `Set(path string, value any)` — sets value at leaf; creates intermediate maps as needed
- [x] 5.5 Implement `Snapshot() map[string]any` — returns a shallow copy of `data` (deep copy of top level)
- [x] 5.6 Add `TestExecutionContext` in `internal/pipeline/context_test.go`: round-trip Set/Get, dot-path traversal, missing path returns false, snapshot independence

## 6. Extract executeStep and Update runner.go

- [x] 6.1 Extract the loop body in `internal/pipeline/runner.go` into `func executeStep(ctx context.Context, step *Step, ec *ExecutionContext, mgr *plugin.Manager, defaultInput string) (string, error)`
- [x] 6.2 Replace the loop body with a call to `executeStep`; verify `go test ./internal/pipeline/...` still passes
- [x] 6.3 Inside `executeStep`, store output via `ec.Set(step.ID+".out", output)` instead of writing to the raw map

## 7. Update template.go and condition.go Signatures

- [x] 7.1 Change `Interpolate(tmpl string, vars map[string]string) string` to accept `map[string]any` in `internal/pipeline/template.go`
- [x] 7.2 Update the template engine call to use the `map[string]any`; add `fmt.Sprint` coercion for non-string values
- [x] 7.3 Change `EvalCondition(expr string, vars map[string]string) bool` to accept `map[string]any` in `internal/pipeline/condition.go`
- [x] 7.4 Add a `toString(v any) string` helper in `condition.go` that coerces string/int/float/bool via `fmt.Sprint`; use it wherever a string comparison is made
- [x] 7.5 Update all call sites in `runner.go` to pass `*ExecutionContext` data (via `Snapshot()`) instead of the old raw map
- [x] 7.6 Update template and condition tests to use `map[string]any` literals

## 8. EventPublisher Interface

- [x] 8.1 Create `internal/pipeline/event.go` with `EventPublisher` interface: `Publish(ctx context.Context, topic string, payload []byte) error`
- [x] 8.2 Add `NoopPublisher` struct with `Publish` that always returns nil
- [x] 8.3 Add `publisher EventPublisher` parameter to `Run` function signature; default to `NoopPublisher{}` when nil
- [x] 8.4 Add `TestNoopPublisher` in `internal/pipeline/event_test.go`: assert `Publish` returns nil

## 9. Proto Update

- [x] 9.1 Update `proto/orcai/v1/plugin.proto`: add `import "google/protobuf/struct.proto"`
- [x] 9.2 Replace `ExecuteRequest` fields `input string` and `map<string,string> vars` with `google.protobuf.Struct args = 1`
- [x] 9.3 Add `google.protobuf.Struct output = 2` as a `oneof` alternative alongside existing `bytes chunk` in `ExecuteResponse`
- [x] 9.4 Regenerate Go proto code: `make proto` (or `protoc` directly); commit generated `.pb.go` files
- [x] 9.5 Update any Go code that references the old `ExecuteRequest.Input` / `ExecuteRequest.Vars` fields to use `ExecuteRequest.Args` via `structpb`

## 10. Documentation and Convention Comments

- [x] 10.1 Add doc comment to `plugin.Plugin` interface in `internal/plugin/plugin.go`: document `input` as "primary data payload / stdin"; `vars` as "string metadata passed as env vars — not structured data"
- [x] 10.2 Add matching comment to `CLIAdapter.Execute` in `internal/plugin/cli_adapter.go`
- [x] 10.3 Add comment to `internal/host/host.go` above plugin loading: "Plugin process lifecycle (start/stop) is managed here. Per-step execution lifecycle (init/execute/cleanup) is managed by the pipeline runner — do not conflate."
- [x] 10.4 Add comment to `internal/discovery/discovery.go` above name derivation: document the filename → plugin name mapping convention

## 11. Verification

- [x] 11.1 Run `go test ./internal/pipeline/... ./internal/plugin/...` — all tests pass
- [x] 11.2 Run `go build ./...` — no compilation errors
- [x] 11.3 Run existing pipeline YAML smoke test (if available) to confirm backwards compatibility
- [x] 11.4 Commit: `refactor(pipeline,plugin): prereqs for DAG pipeline enhancements`
