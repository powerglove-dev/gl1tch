## Why

Core UI components (sysop panel, session picker, welcome dashboard) are currently hardwired into the orcai binary with no replacement path — users who want a custom sysop panel or a different welcome experience must fork the repo. Additionally, orcai claims tmux keybindings and pane layout at startup, leaving power users no way to arrange widgets to fit their workflow without patching core.

## What Changes

- Core UI widgets (sysop, picker, welcome) are invoked as `orcai <name>` subcommands, keeping a single binary while making them addressable by the widget dispatch layer
- Before invoking a built-in subcommand, orcai checks `PATH` for an `orcai-<name>` binary and defers to it if found — the same override convention used by Git plugins
- Widget pane layout is driven by a user-editable layout config (`~/.config/orcai/layout.yaml`) rather than hardcoded tmux split geometry; orcai applies the config at startup but does not re-assert it after that, so users retain full tmux control
- Keybinding overrides: orcai only binds keys listed in `~/.config/orcai/keybindings.yaml`; keys absent from that file are left untouched, resolving conflicts with existing tmux users

## Capabilities

### New Capabilities

- `core-subcommand-dispatch`: Mechanism for orcai to invoke built-in widget subcommands (`orcai sysop`, `orcai picker`, `orcai welcome`) and expose them to the widget layer as first-party implementations
- `plugin-binary-override`: PATH-based lookup that checks for `orcai-<name>` before falling back to the built-in subcommand — zero config for the override path, mirrors Git plugin convention
- `widget-layout-config`: Declarative pane layout config that controls where and how orcai opens widget panes at startup, with a no-op default that leaves existing tmux layouts untouched

### Modified Capabilities

- `cli-adapter-sidecar`: Widget dispatch path now checks the binary-override resolution order before launching a sidecar — override binaries are preferred over sidecar declarations of the same name

## Impact

- `cmd/` — new subcommands registered: `orcai sysop`, `orcai picker`, `orcai welcome`
- `internal/widget` (new) — dispatch logic: PATH lookup → built-in subcommand fallback
- `internal/layout` (new) — parse and apply `layout.yaml` at session init
- `~/.config/orcai/layout.yaml` — new optional user config file
- `~/.config/orcai/keybindings.yaml` — existing concept, now controls which keys orcai binds (absent = untouched)
- No breaking changes to the pipeline or plugin manager layers
