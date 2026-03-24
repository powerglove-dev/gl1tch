## Context

`internal/plugin` already has a working `CliAdapter` that spawns a subprocess, writes string input to stdin, and streams stdout/stderr to an `io.Writer`. `plugin.Manager` registers and retrieves plugins by name. There is no way to load adapters from the filesystem — every registration is in-code. The Tier 2 design calls for a sidecar YAML at `~/.config/orcai/wrappers/<name>.yaml` that declares schema and CLI invocation details, plus a discovery pass that auto-registers all sidecar files in that directory. `gopkg.in/yaml.v3` is already a project dependency.

## Goals / Non-Goals

**Goals:**
- Define `SidecarSchema` struct and YAML loading for `~/.config/orcai/wrappers/<name>.yaml`
- `NewCliAdapterFromSidecar(path string)` enriches `CliAdapter` with name, description, command, args, and capabilities from the sidecar
- `LoadWrappers(dir string)` scans `*.yaml` files in a directory and returns `[]Plugin`
- `Manager.LoadWrappersFromDir(dir string)` calls `LoadWrappers` and registers results
- All changes are additive — existing `NewCliAdapter` and `Execute` are unchanged

**Non-Goals:**
- JSON envelope protocol (raw stdin/stdout is sufficient for the current integration targets)
- PATH scanning (only sidecar-declared tools are registered; PATH-scanning is a future concern)
- Hot-reload of sidecar files at runtime
- Schema validation beyond YAML unmarshalling

## Decisions

**1. `SidecarSchema` as a plain struct in `cli_adapter.go`, not a separate file**
The sidecar type is tightly coupled to `CliAdapter` construction. Keeping it in the same file avoids a new file for a small type. If it grows (e.g., JSON envelope flags, env var declarations), it moves to its own file then.

**2. `NewCliAdapterFromSidecar` returns `(*CliAdapter, error)`**
Options: (a) return `Plugin` interface, (b) return `*CliAdapter`. Returning `*CliAdapter` lets callers access sidecar-specific fields if needed without type-asserting. The interface is preserved at the registration layer.

**3. `Capabilities()` returns schema from sidecar when available**
The existing `Capabilities()` returns `nil`. After sidecar loading, it returns a single `Capability` with `InputSchema` and `OutputSchema` from the YAML. If the sidecar omits these fields, the `Capability` entry still exists but with empty schema strings — the name + description are always populated.

**4. `LoadWrappers` in a new `discovery.go` file**
Discovery is a different concern from adapter construction. `discovery.go` already appears in the plugin test files (`discovery_test.go`) suggesting it's a planned file. Keeping it separate makes the boundary clear.

**5. `Manager.LoadWrappersFromDir` as a convenience method**
Callers shouldn't need to call `LoadWrappers` + loop + `Register` manually. A single method on Manager reduces boilerplate at call sites (e.g., `cmd/` startup code).

## Risks / Trade-offs

- **Malformed YAML silently skipped vs. hard error** — `LoadWrappers` will log and skip invalid sidecar files rather than returning an error for one bad file, since a misconfigured sidecar shouldn't prevent other wrappers from loading. → The error slice returned allows callers to surface warnings.
- **Directory does not exist** — `LoadWrappers` returns an empty slice (not an error) when the wrappers dir is absent; first-run experience should not be noisy. → Callers that expect the dir can `os.MkdirAll` it themselves.

## Migration Plan

No migration required. Existing in-code `NewCliAdapter` registrations are unaffected. New sidecar-based loading is opt-in — nothing calls `LoadWrappersFromDir` until wired into startup.

## Open Questions

- Should `LoadWrappers` also accept non-`*.yaml` files? No — stick to `.yaml` for discoverability.
- Should the sidecar support an `enabled: false` flag to disable a wrapper? Defer — YAGNI.
