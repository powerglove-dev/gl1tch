## 1. Embedded Assets Foundation

- [x] 1.1 Create `internal/assets/` directory with `//go:embed` setup for providers and themes
- [x] 1.2 Write bundled provider profiles: `claude.yaml`, `gemini.yaml`, `opencode.yaml`, `aider.yaml`, `goose.yaml`, `copilot.yaml`
- [x] 1.3 Create bundled ABS theme bundle: `internal/assets/themes/abs/theme.yaml` + `splash.ans`

## 2. Provider Profile Package

- [x] 2.1 Create `internal/providers/` package with `Profile` struct matching the YAML schema (name, binary, display_name, api_key_env, models, session)
- [x] 2.2 Implement `LoadBundled()` — loads embedded profiles from `internal/assets/providers/`
- [x] 2.3 Implement `LoadUser(dir string)` — scans `~/.config/orcai/providers/*.yaml`
- [x] 2.4 Implement `Registry` — merges bundled + user profiles (user wins on name collision), binary detection via `exec.LookPath`
- [x] 2.5 Write table-driven tests for profile loading, merging, and binary detection
- [x] 2.6 Replace `knownCLITools` in `internal/discovery/discovery.go` with `providers.Registry`
- [x] 2.7 Replace `adapterDefs` in `internal/bridge/manager.go` with `providers.Registry`
- [x] 2.8 Remove `internal/adapters/{claude,gemini,copilot}/` packages

## 3. Theme Package

- [x] 3.1 Create `internal/themes/` package with `Bundle` and `Palette` structs matching the theme.yaml schema
- [x] 3.2 Implement `LoadBundled()` — loads embedded ABS theme from `internal/assets/themes/`
- [x] 3.3 Implement `LoadUser(dir string)` — scans `~/.config/orcai/themes/*/theme.yaml`
- [x] 3.4 Implement `Registry` — merges bundled + user themes, resolves palette references
- [x] 3.5 Implement `SetActive(name string)` — persists active theme to orcai config
- [x] 3.6 Write tests for bundle loading, palette resolution, and fallback behavior on missing assets
- [x] 3.7 Wire theme registry into orcai startup; publish `theme.changed` on theme switch

## 4. Bus Daemon

- [ ] 4.1 Create `internal/busd/` package with Unix socket server wrapping `internal/bus`
- [ ] 4.2 Implement client registration frame: widget sends `{"name": "...", "subscribe": [...]}` on connect
- [ ] 4.3 Implement subscription routing — only deliver events matching a client's declared subscriptions
- [ ] 4.4 Implement fanout publish — on `bus.Publish`, deliver to all matching subscriber connections
- [ ] 4.5 Implement socket path resolution (`$XDG_RUNTIME_DIR/orcai/bus.sock` with `~/.cache/orcai/bus.sock` fallback)
- [ ] 4.6 Implement graceful shutdown — close all connections and remove socket file on orcai exit
- [ ] 4.7 Wire `busd.Start()` into orcai main init sequence before any widget launch
- [ ] 4.8 Write tests for subscription filtering, fanout, and client prune on disconnect

## 5. Widget Plugin Package

- [ ] 5.1 Create `internal/widgets/` package with `Manifest` struct matching `widget.yaml` schema (name, binary, description, subscribe)
- [ ] 5.2 Implement `Discover(dir string)` — scans `~/.config/orcai/widgets/*/widget.yaml`
- [ ] 5.3 Implement `Launch(manifest, tmuxSession)` — starts widget binary in a new tmux window via `tmux new-window`
- [ ] 5.4 Implement widget client prune on disconnect in busd (wire into fanout error path)
- [ ] 5.5 Write tests for manifest discovery and malformed manifest handling

## 6. First-Party Widget Migration

- [x] 6.1 Create `cmd/orcai-welcome/` — welcome dashboard widget binary with `widget.yaml` manifest
- [x] 6.2 Port dashboard BubbleTea model from `internal/welcome/` to the welcome widget binary
- [x] 6.3 Replace hardcoded ANSI color constants in welcome widget with palette values from `theme.changed` bus event
- [x] 6.4 Subscribe welcome widget to `theme.changed`, `session.started`, `session.ended`, `orcai.telemetry`
- [x] 6.5 Remove `internal/welcome/` package after migration is validated
- [x] 6.6 Create `cmd/orcai-sysop/` — sysop panel widget binary with `widget.yaml` manifest (stub implementation acceptable initially)

## 7. Theme Integration Cleanup

- [x] 7.1 Move ANSI art assets from `internal/ansiart/` into the ABS theme bundle (`internal/assets/themes/abs/`)
- [x] 7.2 Update all references to `internal/ansiart/` to use the theme registry
- [x] 7.3 Remove `internal/ansiart/` package after references are cleared

## 8. Config Directory Layout

- [x] 8.1 Update bootstrap (`internal/bootstrap/`) to create `~/.config/orcai/providers/`, `~/.config/orcai/widgets/`, and `~/.config/orcai/themes/` on first run
- [x] 8.2 Add active theme storage to orcai config struct and persistence

## 9. Documentation

- [x] 9.1 Write `docs/plugins/providers.md` — contributor guide for writing a provider profile YAML
- [x] 9.2 Write `docs/plugins/widgets.md` — contributor guide for the widget manifest and bus protocol
- [x] 9.3 Write `docs/plugins/themes.md` — contributor guide for theme bundle format and asset files
