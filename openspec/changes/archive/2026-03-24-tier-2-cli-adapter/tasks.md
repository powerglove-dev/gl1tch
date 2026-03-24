## 1. SidecarSchema and NewCliAdapterFromSidecar

- [x] 1.1 Add `SidecarSchema` struct to `internal/plugin/cli_adapter.go` with fields: `Name string`, `Description string`, `Command string`, `Args []string`, `InputSchema string`, `OutputSchema string` (all with yaml tags)
- [x] 1.2 Add `caps []Capability` field to `CliAdapter` struct to hold sidecar-loaded capabilities
- [x] 1.3 Implement `NewCliAdapterFromSidecar(path string) (*CliAdapter, error)` that reads the YAML file, unmarshals into `SidecarSchema`, validates `Command` is non-empty, and returns a populated `*CliAdapter`
- [x] 1.4 Update `Capabilities()` on `CliAdapter` to return `c.caps` instead of `nil` (non-sidecar adapters have nil caps; sidecar adapters have one entry)
- [x] 1.5 In `NewCliAdapterFromSidecar`, populate `c.caps` with a single `Capability{Name: schema.Name, InputSchema: schema.InputSchema, OutputSchema: schema.OutputSchema}`

## 2. SidecarSchema Tests

- [x] 2.1 Add `TestNewCliAdapterFromSidecar_Valid` in `cli_adapter_test.go`: write a temp YAML file with all fields, call `NewCliAdapterFromSidecar`, assert name/description/command match
- [x] 2.2 Add `TestNewCliAdapterFromSidecar_MissingCommand`: write a temp YAML without `command`, assert error returned
- [x] 2.3 Add `TestNewCliAdapterFromSidecar_FileNotFound`: call with nonexistent path, assert error returned
- [x] 2.4 Add `TestCliAdapter_Capabilities_FromSidecar`: load sidecar with input/output schema, assert `Capabilities()` returns one entry with correct schema values
- [x] 2.5 Add `TestCliAdapter_Capabilities_NoSidecar`: existing adapter via `NewCliAdapter`, assert `Capabilities()` returns nil (existing test behavior preserved)
- [x] 2.6 Run `go test ./internal/plugin/...` and confirm all pass

## 3. Discovery

- [x] 3.1 Create `internal/plugin/discovery.go` with `LoadWrappers(dir string) ([]Plugin, []error)` — returns plugins and a slice of per-file errors
- [x] 3.2 In `LoadWrappers`: return empty slice + nil errors when dir does not exist (use `os.IsNotExist`)
- [x] 3.3 In `LoadWrappers`: use `os.ReadDir(dir)` to list entries, filter to `*.yaml` files only
- [x] 3.4 For each YAML file: call `NewCliAdapterFromSidecar`, append to plugins on success or append error on failure — never abort the loop

## 4. Discovery Tests

- [x] 4.1 Create `internal/plugin/discovery_test.go`
- [x] 4.2 Add `TestLoadWrappers_Valid`: write two valid sidecar YAML files to a temp dir, assert two plugins returned
- [x] 4.3 Add `TestLoadWrappers_DirNotExist`: call with nonexistent dir, assert empty slice and nil errors
- [x] 4.4 Add `TestLoadWrappers_MixedValidity`: one valid + one invalid YAML in temp dir, assert one plugin and one error returned
- [x] 4.5 Add `TestLoadWrappers_IgnoresNonYAML`: place a `.txt` file in temp dir, assert it is not loaded
- [x] 4.6 Run `go test ./internal/plugin/...` and confirm all pass

## 5. Manager Integration

- [x] 5.1 Add `LoadWrappersFromDir(dir string) []error` method to `Manager` in `internal/plugin/manager.go` — calls `LoadWrappers`, registers each plugin, returns the error slice
- [x] 5.2 Add `TestManager_LoadWrappersFromDir_Valid` in `plugin_test.go`: write two sidecars to temp dir, call `LoadWrappersFromDir`, assert both plugins retrievable via `Get`
- [x] 5.3 Add `TestManager_LoadWrappersFromDir_EmptyDir`: empty temp dir, assert no plugins registered and no errors

## 6. Final Verification

- [x] 6.1 Run `go test ./internal/plugin/...` — all tests pass
- [x] 6.2 Run `go build ./...` — no compilation errors
- [x] 6.3 Commit: `feat(plugin): add sidecar YAML loading and wrapper discovery for Tier 2 CliAdapter`
