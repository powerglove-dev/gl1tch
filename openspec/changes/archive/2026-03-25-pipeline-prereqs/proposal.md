## Why

A principal-level review of the `pipeline-enhancements` plan identified ten structural issues in the existing plugin and pipeline packages that, if left unaddressed, would force the DAG execution rewrite to make large compromises under delivery pressure. This change addresses all of them before `pipeline-enhancements` implementation begins.

## What Changes

- **Tests for pipeline and plugin packages**: Write `*_test.go` files covering the existing sequential runner, template interpolation, condition evaluation, and plugin manager — zero test coverage exists today
- **`Manager.Register` returns error instead of panicking**: `plugin/manager.go` currently panics on duplicate plugin name; change to return error and fix the one call site in `host.go`
- **`Step` struct extended with `executor`, `args`, `needs` fields**: Add typed fields to `internal/pipeline/pipeline.go`; mark `Plugin` and `Vars` as deprecated in comments (not removed — backwards compatible)
- **`Pipeline` struct gains pipeline-level `Vars map[string]any`**: Seeds the execution context for the DAG runner
- **`template.go` and `condition.go` signatures changed to `map[string]any`**: Mechanical type change; compiler finds every call site
- **`executeStep` extracted from runner loop**: ~20-line extraction turns the loop body into a standalone function that the DAG scheduler can call without restructuring
- **`ExecutionContext` named type introduced**: Replaces raw `map[string]string` in the runner; gives the DAG refactor a stable type to extend
- **`EventPublisher` interface defined in pipeline package**: Minimal interface the runner will depend on; nil-safe for testing; no implementation yet
- **Proto updated to `google.protobuf.Struct`**: Replace `input: string` + `vars: map<string,string>` with `args: google.protobuf.Struct` in the plugin RPC; regenerate Go code — one-time wire-breaking change while no external plugins exist
- **Calling convention documented**: Add comments to `plugin.Plugin` interface clarifying `input` (stdin/prompt) vs `vars` (env/metadata) semantics; same for `CLIAdapter`

## Capabilities

### New Capabilities

- `pipeline-execution-context`: Named `ExecutionContext` type and `executeStep` extraction as pipeline execution primitives
- `pipeline-event-publisher`: `EventPublisher` interface definition for bus integration

### Modified Capabilities

- `cli-adapter-discovery`: `plugin.Manager.Register` changes signature to return error; `cli-adapter-sidecar` field documentation updated with calling convention
- `cli-adapter-sidecar`: No requirement changes — documentation only

## Impact

- `internal/plugin/plugin.go` — add calling convention doc comments
- `internal/plugin/manager.go` — `Register` returns `error`; add `ByCategory` stub
- `internal/plugin/cli_adapter.go` — add calling convention doc comments
- `internal/plugin/manager_test.go` — new file
- `internal/pipeline/pipeline.go` — add `Executor`, `Args map[string]any`, `Needs []string` to `Step`; add `Vars map[string]any` to `Pipeline`
- `internal/pipeline/runner.go` — extract `executeStep`; introduce `ExecutionContext`; update template/condition call sites
- `internal/pipeline/runner_test.go` — new file
- `internal/pipeline/template.go` — signature: `map[string]string` → `map[string]any`
- `internal/pipeline/template_test.go` — new file
- `internal/pipeline/condition.go` — signature: `map[string]string` → `map[string]any`
- `internal/pipeline/condition_test.go` — new file
- `internal/pipeline/event.go` — new file (`EventPublisher` interface)
- `internal/host/host.go` — handle `Register` error return; add lifecycle boundary comment
- `internal/discovery/discovery.go` — add filename-to-plugin-name convention comment
- `proto/orcai/v1/plugin.proto` — switch to `google.protobuf.Struct`; regenerate
- No breaking changes to existing pipeline YAML — `plugin` and `vars` fields remain functional
