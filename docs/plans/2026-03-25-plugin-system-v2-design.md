# Plugin System v2 Design

**Date:** 2026-03-25
**Status:** Approved

## Philosophy

Orcai stays lean. Its job is tmux orchestration, session management, and acting as the local event bus. Everything else — providers, widgets, themes — lives outside core and speaks to orcai through defined contracts. Contributors never touch this repo; they write a manifest and a binary (or just a manifest for providers and themes) and drop it in the right config directory.

Three plugin kinds, each with their own manifest format and discovery path. The daemon is invisible infrastructure that contributors never need to think about.

---

## Provider Plugins

### Goal

Remove all hardcoded provider lists from core. `knownCLITools` in `internal/discovery/discovery.go` and `adapterDefs` in `internal/bridge/manager.go` both go away, replaced by profile loading.

### Capability Profiles

Bundled profiles ship embedded in orcai via `//go:embed` for popular tools (claude, gemini, opencode, aider, goose, copilot). Each is a YAML file — pure data, no Go code.

```yaml
name: claude
binary: claude
display_name: Claude Code
api_key_env: ANTHROPIC_API_KEY
models:
  - id: claude-opus-4-6
    display: "Opus 4.6"
    cost_input_per_1m: 15.00
    cost_output_per_1m: 75.00
  - id: claude-sonnet-4-6
    display: "Sonnet 4.6"
    cost_input_per_1m: 3.00
    cost_output_per_1m: 15.00
session:
  window_name: "{{.name}}:{{.model}}"
  launch_args: []
  env: {}
```

### Discovery

1. Load all bundled profiles (embedded in binary)
2. Merge `~/.config/orcai/providers/*.yaml` — user profiles win on name collision
3. Binary detection: `exec.LookPath(profile.Binary)` — dormant if not installed

Community contributors add a new provider by writing a YAML profile and placing it in `~/.config/orcai/providers/`. No Go code, no repo PR required.

### Deep Integration Surface

Profiles carry enough data for orcai to provide:
- Model picker (names, IDs)
- Cost tracking per session (input/output token rates)
- Session window naming templates
- API key environment variable hints
- Per-provider launch arguments and environment overrides

---

## Widget Plugins

### Goal

Decouple welcome splash, sysop panel, session list, and any future display components from core. Contributors can ship new widgets as standalone binaries with a simple manifest.

### Manifest

```yaml
# ~/.config/orcai/widgets/weather/widget.yaml
name: weather
binary: orcai-weather
description: Current conditions display
subscribe:
  - session.started
  - theme.changed
  - session.ended
```

### Lifecycle

Orcai discovers widget manifests in `~/.config/orcai/widgets/*/widget.yaml` at startup. It launches each widget binary in its own tmux pane (layout is tmux's business, not orcai's). Widgets connect to orcai's bus socket and receive subscribed events.

### Framed JSON Protocol

Communication is newline-delimited JSON frames over the orcai bus Unix socket.

**Orcai → Widget (events):**
```json
{"event": "theme.changed", "payload": {"accent": "#bd93f9", "bg": "#282a36", "fg": "#f8f8f2"}}
{"event": "session.started", "payload": {"provider": "claude", "model": "sonnet-4-6", "window": 2}}
{"event": "session.ended", "payload": {"window": 2}}
```

**Widget → Orcai (commands):**
```json
{"cmd": "notify", "payload": {"text": "Weather updated"}}
{"cmd": "session.launch", "payload": {"provider": "claude", "model": "claude-sonnet-4-6"}}
```

Widgets that need no interactivity can ignore the socket entirely and just render to their pane. Disconnected widgets are pruned from the subscriber list silently.

### First-Party Widgets

Built-in widgets (welcome splash, sysop panel, session list) are implemented as first-class widget binaries — they use the same manifest and protocol as contributor widgets. They ship with orcai in `internal/widgets/` and are registered at startup, but follow the same contract. This means they can be extracted to separate repos without changing the protocol.

---

## Theme Plugins

### Goal

Themes are full visual identity bundles covering palette, ANSI art, border styles, and status bar configuration. No binary required — pure data directories.

### Bundle Structure

```
~/.config/orcai/themes/gruvbox/
  theme.yaml
  splash.ans
  banner.ans        # optional
```

### Theme Manifest

```yaml
name: gruvbox
display_name: "Gruvbox Dark"
palette:
  bg:      "#282828"
  fg:      "#ebdbb2"
  accent:  "#d79921"
  dim:     "#928374"
  border:  "#504945"
  error:   "#cc241d"
  success: "#98971a"
borders:
  style: light          # light | heavy | ascii
statusbar:
  format: " {session} · {provider} · {model} "
  bg: palette.bg
  fg: palette.accent
splash: splash.ans      # relative path within bundle
```

### Discovery & Switching

- Bundled themes (ABS/Dracula default) ship via `//go:embed`
- User themes loaded from `~/.config/orcai/themes/*/theme.yaml`
- Active theme stored in orcai config
- Theme switching publishes `theme.changed` on the bus with the full resolved palette
- Widgets use the palette from the event — no hardcoded colors in widget binaries

---

## Daemon & Event Bus

### Socket

Orcai starts a local Unix socket server at startup:
- Primary: `$XDG_RUNTIME_DIR/orcai/bus.sock`
- Fallback: `~/.cache/orcai/bus.sock`

### Architecture

The existing `internal/bus` package is the backbone. A new `internal/busd` (bus daemon) package wraps it with:
- Unix socket listener
- Widget client registry (name → connection)
- Subscription routing (event type → []clients)
- Fanout delivery on publish

Core orcai components (session manager, theme switcher, pipeline runner) publish events to the bus. The socket layer fans them out to all subscribed widget connections.

No gRPC, no new dependencies. Newline-delimited JSON is the wire format.

---

## Config Directory Layout

```
~/.config/orcai/
  providers/          # user-installed provider profiles (*.yaml)
  widgets/            # user-installed widgets (*/widget.yaml + binary)
  themes/             # user-installed themes (*/theme.yaml + assets)
  plugins/            # raw CLI adapter sidecars (existing, unchanged)
  pipelines/          # pipeline definitions (existing, unchanged)
```

Bundled profiles, widgets, and themes for all built-in content ship via `//go:embed` in `internal/assets/`.

---

## Migration

| Current | Replaced By |
|---|---|
| `discovery.knownCLITools` hardcoded slice | Embedded provider profiles |
| `bridge/manager.go` `adapterDefs` hardcoded slice | Provider profile discovery |
| `internal/adapters/{claude,gemini,copilot}/` | Provider profiles + generic session launcher |
| `internal/welcome/` (hardcoded BubbleTea) | First-party welcome widget (manifest + binary) |
| `internal/ansiart/` (baked-in) | Theme bundle asset |

The existing `internal/plugin` (CliAdapter, Manager, Tier 1/2 taxonomy) and `internal/pipeline` remain unchanged — this design sits alongside them and eventually absorbs the provider detection role.
