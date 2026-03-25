## 1. Core Widget Subcommands

- [x] 1.1 Register `sysop` cobra subcommand in `cmd/` that starts the sysop BubbleTea component and blocks until exit
- [x] 1.2 Register `picker` cobra subcommand in `cmd/` that starts the session picker and blocks until exit
- [x] 1.3 Register `welcome` cobra subcommand in `cmd/` that starts the welcome dashboard and blocks until exit
- [x] 1.4 Add `--bus-socket` flag to all three subcommands; wire to bus client connect when flag is non-empty
- [x] 1.5 Write tests: each subcommand exits cleanly; `--bus-socket` flag present but socket absent returns a clear error

## 2. Widget Dispatch Layer

- [x] 2.1 Create `internal/widgetdispatch` package with `Dispatch(ctx, name string, opts Options) error`
- [x] 2.2 Implement PATH lookup: `exec.LookPath("orcai-" + name)` — return override path if found
- [x] 2.3 Implement self-referential detection: compare resolved override path to `os.Executable()`; skip and warn if equal
- [x] 2.4 Implement fallback: if no override found, build `exec.Command("orcai", name, ...)` for built-in
- [x] 2.5 Pass `--bus-socket` arg to both override and built-in invocations when bus socket path is set in opts
- [x] 2.6 Surface non-zero exit codes and stderr output as structured errors
- [x] 2.7 Write tests: override found → override binary called; no override → built-in called; self-referential override → built-in called with warning

## 3. Override Precedence for Sidecars

- [x] 3.1 Update widget dispatch to check PATH override before consulting the sidecar/manifest registry
- [x] 3.2 Write test: sidecar named `weather` + `orcai-weather` on PATH → override binary used, sidecar skipped

## 4. Layout Config

- [x] 4.1 Create `internal/layout` package with `Config` struct matching `layout.yaml` schema (`panes` list with `name`, `widget`, `position`, `size`)
- [x] 4.2 Implement `LoadConfig(path string) (*Config, error)` — return empty config (no-op) if file absent
- [x] 4.3 Implement `Apply(ctx context.Context, cfg *Config, dispatch widgetdispatch.Dispatcher) error` — iterate panes, check for existing named pane, create split and launch widget
- [x] 4.4 Wire `layout.Apply` into `orcai attach` after session init, before TUI handoff
- [x] 4.5 Validate `position` field on load; log error and skip invalid pane, continue remaining
- [x] 4.6 Write tests: absent file → no-op; empty panes → no-op; duplicate pane name → skip without error; invalid position → skip with log

## 5. Keybindings Config

- [x] 5.1 Create `internal/keybindings` package with `Config` struct matching `keybindings.yaml` schema (`bindings` list with `key`, `action`)
- [x] 5.2 Implement `LoadConfig(path string) (*Config, error)` — return empty config if file absent
- [x] 5.3 Implement `Apply(cfg *Config) error` — for each binding, run `tmux bind-key <key> <resolved-action>`
- [x] 5.4 Implement action resolver: map action names (e.g. `launch-session-picker`) to tmux command strings
- [x] 5.5 Wire `keybindings.Apply` into `orcai attach` before TUI handoff
- [x] 5.6 Write tests: absent file → no bindings applied; listed key → bind-key called; unlisted key → not touched

## 6. `orcai config init` Command

- [x] 6.1 Register `config init` cobra subcommand in `cmd/`
- [x] 6.2 Write default `layout.yaml` template (classic orcai pane layout with sysop + welcome)
- [x] 6.3 Write default `keybindings.yaml` template (classic orcai keybindings)
- [x] 6.4 Implement overwrite prompt: if file exists, ask user to confirm before overwriting
- [x] 6.5 Print created file paths on success
- [x] 6.6 Write tests: files absent → both created; file exists + user confirms → overwritten; file exists + user declines → unchanged

## 7. Integration

- [x] 7.1 Update `orcai attach` to call `layout.Apply` and `keybindings.Apply` using loaded configs
- [x] 7.2 Verify that a clean install with no config files produces zero layout or keybinding changes (regression test)
- [x] 7.3 Update README / user docs to describe subcommand usage, override convention, and `config init`
