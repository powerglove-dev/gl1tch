## Context

The principal review found ten issues across `internal/plugin/` and `internal/pipeline/` that are cheap to fix individually but collectively form a trap: if the DAG pipeline rewrite (`pipeline-enhancements`) lands on top of them, every structural decision in the new code inherits the old type constraints. The existing code is small (~400 lines total across both packages), has zero test coverage, and is used by only one external caller (the pipeline CLI command). This is the lowest-risk moment to make these changes.

Current state snapshot:
- `internal/plugin/manager.go` — flat `map[string]Plugin`, `Register` panics on duplicate
- `internal/pipeline/pipeline.go` — `Step.Vars map[string]string`, no `Needs`, no `Executor`
- `internal/pipeline/runner.go` — single loop, execution context is a raw `map[string]string`
- `internal/pipeline/template.go` + `condition.go` — both take `map[string]string`
- `proto/orcai/v1/plugin.proto` — `input string` + `vars map<string,string>` in Execute RPC
- Zero `*_test.go` files in either package

## Goals / Non-Goals

**Goals:**
- Establish a test harness for both packages before the rewrite
- Eliminate the `map[string]string` → `map[string]any` mismatch that would propagate into DAG code
- Give the pipeline enhancements a stable `ExecutionContext` type, `executeStep` function, and `EventPublisher` interface to target
- Update the proto wire format while no external plugins are deployed (one-time breaking change)
- Make `Register` safe for duplicate names (no panic)

**Non-Goals:**
- Implementing DAG execution, retry, for_each, or builtins (those are `pipeline-enhancements`)
- Changing the observable behaviour of existing pipeline execution — all pipelines that work today must still work after this change
- Adding new gRPC RPCs for lifecycle (Init/Cleanup) — out of scope
- Implementing `EventPublisher` (define the interface only, no concrete bus wiring)

## Decisions

### 1. `ExecutionContext` is a struct wrapping `map[string]any`, not a type alias

Introducing `type ExecutionContext = map[string]any` is tempting but aliases provide no encapsulation — callers still mutate the map directly. A thin struct with `Get(path string) (any, bool)` and `Set(path string, value any)` accessor methods gives the DAG refactor a stable API to extend with per-step scoping, snapshot/restore for retry, and concurrent read access. The accessor overhead is negligible.

```go
type ExecutionContext struct {
    mu   sync.RWMutex
    data map[string]any
}
func (c *ExecutionContext) Get(path string) (any, bool)
func (c *ExecutionContext) Set(path string, value any)
func (c *ExecutionContext) Snapshot() map[string]any  // for retry/fork
```

### 2. `executeStep` signature targets the new types immediately

```go
func executeStep(ctx context.Context, step *Step, ec *ExecutionContext, mgr *plugin.Manager, w io.Writer) error
```

The function takes the new `ExecutionContext` and `*Step` (which now has `Executor`/`Args`). The existing runner loop is replaced by a call to `executeStep` in a simple for-loop. This is a zero-behaviour-change refactor — the loop body moves, nothing else.

### 3. Template and condition packages: signature change only, no semantics change

`Interpolate(tmpl string, vars map[string]any) string` — the underlying `text/template` engine already handles `map[string]any` natively via reflection. The change is: delete `map[string]string`, insert `map[string]any`. Callers that pass `map[string]string` literals will get a compile error; they are updated to use `map[string]any` literals. No template syntax changes.

`EvalCondition(expr string, vars map[string]any) bool` — condition expressions that reference variables currently do `vars["key"].(string)` coercions internally. With `map[string]any`, add a `toString(v any) string` helper that handles string, int, float, bool coercions for backwards compatibility.

### 4. Proto: `google.protobuf.Struct` replaces string fields

`google.protobuf.Struct` serialises to/from `map[string]interface{}` in Go via `structpb`. The `Execute` RPC changes from:

```protobuf
message ExecuteRequest {
  string input = 1;
  map<string, string> vars = 2;
}
```

to:

```protobuf
message ExecuteRequest {
  google.protobuf.Struct args = 1;
}
```

And `ExecuteResponse` adds a structured output field alongside the existing stream of bytes:

```protobuf
message ExecuteResponse {
  oneof payload {
    bytes  chunk  = 1;  // streaming text (existing)
    google.protobuf.Struct output = 2;  // final structured output (new)
  }
}
```

This preserves streaming text output (which `CLIAdapter` and provider adapters depend on) while adding the structured return path the `StepExecutor` design needs. The `CLIAdapter` sends only `chunk` payloads; future structured-output plugins use `output`.

### 5. `Register` returns error; panic removed

```go
func (m *Manager) Register(p Plugin) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    if _, exists := m.plugins[p.Name()]; exists {
        return fmt.Errorf("plugin %q already registered", p.Name())
    }
    m.plugins[p.Name()] = p
    return nil
}
```

The one call site (`host.go`) logs the error and continues loading other plugins. This matches how `LoadWrappersFromDir` already handles partial failures.

### 6. `EventPublisher` interface lives in `internal/pipeline/event.go`

```go
type EventPublisher interface {
    Publish(ctx context.Context, topic string, payload []byte) error
}

// NoopPublisher is a nil-safe EventPublisher for testing and CLI-standalone use.
type NoopPublisher struct{}
func (NoopPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
```

`Run` accepts `EventPublisher` as a parameter (defaulting to `NoopPublisher`). No global state, no init-time bus connection.

## Risks / Trade-offs

- **Proto regeneration touches generated files**: The `.pb.go` files are generated; any stale generation will cause build failures. Mitigation: the `Makefile` should have a `proto` target; run it as part of this change and commit the generated output.
- **`map[string]any` template change is not truly backwards-compatible**: Go `text/template` accesses map keys via `.Key` syntax with `map[string]any`, but the current templates may use `{{.key}}` against `map[string]string`. Test coverage of existing templates must be written first, before the signature change, to catch any template syntax incompatibilities.
- **`ExecutionContext` mutex**: The struct adds a `sync.RWMutex`. In the current sequential runner this is dead weight. In the DAG runner it is necessary. Accept the cost now.

## Migration Plan

1. Write tests first (captures current behaviour)
2. Make mechanical type changes (`map[string]any`, `Register` → error)
3. Extract `executeStep`, introduce `ExecutionContext`
4. Add `EventPublisher` interface
5. Update proto, regenerate
6. Run `go test ./...` and `go build ./...`
7. Commit

No database migrations, no deployment coordination, no API version bump required.
