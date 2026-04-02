---
title: "Themes"
description: "Bundle system for palette, borders, status bar, and modal styling in gl1tch."
order: 50
---

Themes drive all color, border style, and status bar rendering in gl1tch without requiring code changes. The theme system loads bundled palettes at startup, discovers user-provided overrides from `~/.config/glitch/themes/`, and persists the active theme choice across restarts. Themes communicate changes via a bus event, allowing the UI to repaint live when a new theme is selected.


## Architecture Overview

gl1tch's theme system lives in `internal/themes` and consists of four components:

**Registry** — `registry.go` holds the master list of available themes and tracks which one is active. It loads bundled themes from the embedded `assets.ThemeFS` and user themes from `~/.config/glitch/themes/`, with user themes winning on name collisions. The active theme is persisted to `~/.config/glitch/active_theme` and restored on startup.

**Loader** — `loader.go` reads theme bundles from both embedded and filesystem sources. A bundle is a directory containing `theme.yaml` (metadata and color definitions) and optional `.ans` ANSI art files for splash screens or panel headers.

**Global Registry** — `global.go` stores a process-level singleton so any component in gl1tch can call `GlobalRegistry()` to get the active theme without passing it through function arguments.

**Tmux Integration** — `tmux.go` applies theme colors to the running tmux session via `set-option` commands, updating the status bar colors and pane borders immediately when the user switches themes.

When a user selects a new theme, the system publishes a `theme.changed` bus event with the theme name as payload. Subscribers (switchboard, panels, modals) receive this event and re-render using the new palette.


## Technologies

- **YAML** — theme manifests use YAML for readability and ease of editing
- **tmux** — status bar and pane border colors are applied via tmux `set-option` commands
- **ANSI escape codes** — color values are hex strings (e.g. `#ff79c6`) that are converted to ANSI sequences or tmux color codes where needed
- **go:embed** — bundled themes are compiled into the binary as read-only assets


## Concepts

**Bundle** — a complete theme definition. Contains a palette (seven semantic colors), border style, status bar configuration, optional ANSI splash art, modal styling, and optional panel header sprites.

**Palette** — seven semantic color slots that every theme must define: `bg` (background), `fg` (foreground), `accent` (highlights), `dim` (secondary text), `border` (panel borders), `error` (error messages), `success` (success messages). Colors are hex strings like `#1a1b26`.

**Palette reference** — a string like `palette.accent` in a YAML field that points to a palette color. The registry resolves these to hex values at runtime, allowing modal and status bar colors to derive from the palette without duplication.

**Mode** — theme classification as `dark` or `light`. Used by the theme picker to group themes and by consumers to query bundles by appearance preference.

**Headers** — optional per-panel ANSI art sprites (`.ans` files) that replace plain-text panel titles. Stored as a map from panel name to a list of file paths, ordered from widest to narrowest, so the system can pick the first sprite that fits the current panel width.

**Modal config** — colors for popup overlays (quit confirmation, agent launch, theme switcher). Fields: `bg` (overlay background), `border` (border color), `title_bg` (title bar background), `title_fg` (title bar text color).

**Header style** — dynamic header generation when no fixed-width `.ans` sprite fits the panel. Includes top/bottom/border characters and per-panel accent/text color pairs, allowing headers to render at any width.


## Configuration / YAML Reference

A theme bundle is a directory containing `theme.yaml`. Place it at `~/.config/glitch/themes/<name>/` to load at startup, or include it in the bundled assets folder to ship with gl1tch.

### `theme.yaml` schema

```yaml
name: example-theme                    # required: kebab-case identifier
display_name: "Example Theme"          # required: human-readable name
mode: dark                             # required: "dark" or "light"

palette:
  bg: "#1a1a2e"                        # required: background hex
  fg: "#e0e0ff"                        # required: foreground hex
  accent: "#7b68ee"                    # required: highlight hex
  dim: "#555577"                       # required: dimmed text hex
  border: "#333355"                    # required: border hex
  error: "#ff4444"                     # required: error color hex
  success: "#44ff88"                   # required: success color hex

borders:
  style: light                         # "light", "heavy", or "ascii"

statusbar:
  format: " {session} "                # template string
  bg: "palette.bg"                     # may reference palette key
  fg: "palette.accent"                 # may reference palette key

splash: splash.ans                     # optional: relative path to .ans file

modal:
  bg: "palette.bg"                     # optional: modal background
  border: "palette.border"             # optional: modal border
  title_bg: "palette.accent"           # optional: modal title bar BG
  title_fg: "palette.bg"               # optional: modal title bar FG

header_style:                          # optional: dynamic header generation
  top_char: "▄"                        # top block character (default "▄")
  bot_char: "▀"                        # bottom block character (default "▀")
  border_char: "█"                     # border character (default "█")
  panels:                              # map of panel name to colors
    pipelines:
      accent: "palette.accent"
      text: "palette.fg"
    agent_runner:
      accent: "palette.accent"
      text: "palette.fg"
    signal_board:
      accent: "palette.accent"
      text: "palette.fg"
    activity_feed:
      accent: "palette.accent"
      text: "palette.fg"

headers:                               # optional: .ans sprite paths
  pipelines:
    - header-wide.ans
    - header-narrow.ans
  agent_runner:
    - header.ans
```

### Palette reference resolution

Any field in `statusbar`, `modal`, or `header_style` may contain a palette reference string like `"palette.accent"`. At runtime, the registry resolves this to the hex color from `palette.accent`. Use palette references to keep your theme cohesive — if you change the accent color, all references update automatically.

Literal hex strings like `"#ff79c6"` bypass palette resolution and render as-is.

### Field reference

| Field | Required | Type | Notes |
|-------|----------|------|-------|
| `name` | yes | string | Kebab-case, must be unique in the registry |
| `display_name` | yes | string | Shown in theme picker |
| `mode` | yes | string | `dark` or `light` for grouping |
| `palette.bg` | yes | hex | Background color |
| `palette.fg` | yes | hex | Foreground/primary text color |
| `palette.accent` | yes | hex | Highlight/interactive color |
| `palette.dim` | yes | hex | Secondary/muted text color |
| `palette.border` | yes | hex | Panel border color |
| `palette.error` | yes | hex | Error message color |
| `palette.success` | yes | hex | Success message color |
| `borders.style` | yes | string | `light`, `heavy`, or `ascii` |
| `statusbar.format` | yes | string | Template with `{session}`, `{model}` tokens |
| `statusbar.bg` | yes | string | Hex or palette reference |
| `statusbar.fg` | yes | string | Hex or palette reference |
| `splash` | no | string | Relative path to `.ans` file in bundle |
| `modal.bg` | no | string | Hex or palette reference; falls back to hardcoded default if absent |
| `modal.border` | no | string | Hex or palette reference |
| `modal.title_bg` | no | string | Hex or palette reference |
| `modal.title_fg` | no | string | Hex or palette reference |
| `header_style` | no | object | Enables dynamic header rendering |
| `headers` | no | map | Panel name → list of `.ans` paths |

### Bundle directory layout

```
~/.config/glitch/themes/
└── my-theme/
    ├── theme.yaml
    ├── splash.ans             (optional)
    ├── header-wide.ans        (optional)
    └── header-narrow.ans      (optional)
```

Files referenced in `headers` and `splash` are relative to the bundle directory.


## Examples

### Minimal dark theme

Save this as `~/.config/glitch/themes/minimal/theme.yaml`:

```yaml
name: minimal
display_name: "Minimal"
mode: dark

palette:
  bg: "#0a0a0a"
  fg: "#e8e8e8"
  accent: "#00d7ff"
  dim: "#626262"
  border: "#3a3a3a"
  error: "#ff5555"
  success: "#55ff55"

borders:
  style: light

statusbar:
  format: " {session} "
  bg: "palette.bg"
  fg: "palette.accent"
```

### Theme with palette references in modal

```yaml
name: nord-themed
display_name: "Nord-based"
mode: dark

palette:
  bg: "#2e3440"
  fg: "#eceff4"
  accent: "#88c0d0"
  dim: "#4c566a"
  border: "#3b4252"
  error: "#bf616a"
  success: "#a3be8c"

borders:
  style: light

statusbar:
  format: " {session} · {model} "
  bg: "palette.bg"
  fg: "palette.accent"

modal:
  bg: "palette.bg"
  border: "palette.accent"
  title_bg: "palette.accent"
  title_fg: "palette.bg"

header_style:
  top_char: "▄"
  bot_char: "▀"
  border_char: "█"
  panels:
    pipelines:
      accent: "palette.accent"
      text: "palette.fg"
    agent_runner:
      accent: "palette.accent"
      text: "palette.fg"
    signal_board:
      accent: "palette.accent"
      text: "palette.fg"
    activity_feed:
      accent: "palette.accent"
      text: "palette.fg"
```

### Light theme

```yaml
name: solarized-light
display_name: "Solarized Light"
mode: light

palette:
  bg: "#fdf6e3"
  fg: "#657b83"
  accent: "#268bd2"
  dim: "#93a1a1"
  border: "#eee8d5"
  error: "#dc322f"
  success: "#859900"

borders:
  style: light

statusbar:
  format: " {session} "
  bg: "palette.bg"
  fg: "palette.accent"

modal:
  bg: "palette.bg"
  border: "palette.accent"
  title_bg: "palette.accent"
  title_fg: "palette.bg"

header_style:
  panels:
    pipelines:
      accent: "palette.accent"
      text: "palette.fg"
    agent_runner:
      accent: "palette.accent"
      text: "palette.fg"
    signal_board:
      accent: "palette.accent"
      text: "palette.fg"
    activity_feed:
      accent: "palette.accent"
      text: "palette.fg"
```

### Accessing your custom theme

After saving a `theme.yaml` in `~/.config/glitch/themes/<name>/`:

1. Restart gl1tch — the registry loads all user themes at startup
2. Open the theme picker (usually keybind `T` in the switchboard)
3. Navigate to your theme and press `enter` to activate it
4. The theme is immediately applied and persisted for future sessions

To override a bundled theme, name your user theme identically (e.g. `name: nord`). The user version takes precedence.


## See Also

- [Dracula palette reference](https://draculatheme.com/contribute) — official color specs for extending Dracula-based themes
- [terminalcolors.com](https://terminalcolors.com) — curated repository of terminal themes with YAML exports
- Internal: `internal/themes/themes.go` — `Bundle` struct and color resolution
- Internal: `internal/console/switchboard.go` — switchboard model holds the registry and subscribes to theme changes
- Internal: `internal/styles/styles.go` — bundle-aware style factories for lipgloss components

