## Why

The `plugin.CliAdapter` struct and `plugin.Manager` exist but have no way to discover or schema-describe wrappers from the filesystem — every adapter must be registered in code. Adding sidecar YAML support and a discovery loader lets any CLI tool in `~/.config/orcai/wrappers/` be registered automatically with full schema metadata, unblocking the openspec plugin integration and making orcai's Tier 2 layer self-service.

## What Changes

- Add a `SidecarSchema` type that loads `~/.config/orcai/wrappers/<name>.yaml` (name, description, command, args, input_schema, output_schema)
- Add `NewCliAdapterFromSidecar(path string)` constructor that creates a `CliAdapter` from a sidecar file
- Enrich `CliAdapter.Capabilities()` to return schema data when loaded from a sidecar
- Add `LoadWrappers(dir string) ([]Plugin, error)` to `internal/plugin` that scans a directory for `*.yaml` sidecar files and returns CliAdapters
- Wire `Manager.LoadWrappersFromDir(dir string)` to call `LoadWrappers` and register all resulting plugins
- Add `gopkg.in/yaml.v3` sidecar parsing (already a project dependency)

## Capabilities

### New Capabilities

- `cli-adapter-sidecar`: Load a CliAdapter from a YAML sidecar file declaring name, description, command, args, and input/output schemas
- `cli-adapter-discovery`: Scan a directory for `*.yaml` sidecar files and register all resulting CliAdapters into the plugin Manager

### Modified Capabilities

<!-- No existing specs — no delta specs needed -->

## Impact

- `internal/plugin/cli_adapter.go` — add `SidecarSchema` struct, `NewCliAdapterFromSidecar`, enrich `Capabilities()` with schema fields
- `internal/plugin/discovery.go` — new file: `LoadWrappers(dir string) ([]Plugin, error)`
- `internal/plugin/manager.go` — add `LoadWrappersFromDir(dir string) error`
- `internal/plugin/cli_adapter_test.go` — add sidecar loading tests
- `internal/plugin/discovery_test.go` — new file: tests for `LoadWrappers`
- No changes to `plugin.Plugin` interface, `Manager.Register`, or existing `CliAdapter.Execute`
