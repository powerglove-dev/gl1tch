---
title: "Themes"
description: "Switch your assistant's look instantly, or build a theme that's completely yours."
order: 50
---

gl1tch ships with a set of built-in themes. Switch between them with one command, or drop a `theme.yaml` into your config folder and make your workspace look exactly how you want it. Colors, borders, status bar, panel headers — all yours.


## Quick Start

Open the theme picker from anywhere in gl1tch:

```bash
glitch theme pick
```

Navigate to a theme, press `Enter`. Done. Your choice persists across restarts.

To switch directly without the picker:

```bash
glitch theme set dracula
glitch theme set nord
glitch theme set tokyo-night
```


## Built-in Themes

| Name | Mode | Description |
|------|------|-------------|
| `dracula` | dark | Purple-accented, high contrast |
| `nord` | dark | Cool blues and muted tones |
| `tokyo-night` | dark | Neon city vibes |
| `catppuccin-mocha` | dark | Warm, pastel dark palette |
| `solarized-light` | light | Classic warm light theme |
| `minimal` | dark | Black background, single accent |

List what's available on your system:

```bash
glitch theme list
```


## Creating Your Own Theme

Place a `theme.yaml` in `~/.config/glitch/themes/<your-name>/`. gl1tch loads it at startup — no restart needed after that first load.

### Minimal dark theme

Save this as `~/.config/glitch/themes/my-theme/theme.yaml`:

```yaml
name: my-theme
display_name: "My Theme"
mode: dark

palette:
  bg: "#0a0a0a"
  fg: "#e8e8e8"
  accent: "#00d9ff"
  dim: "#555555"
  border: "#333333"
  error: "#ff4444"
  success: "#44ff88"

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

### Using your custom theme

After saving a `theme.yaml` in `~/.config/glitch/themes/<name>/`:

1. Reload gl1tch — the system loads all user themes at startup
2. Open the theme picker (usually keybind `T` in the main view)
3. Navigate to your theme and press `enter` to activate it
4. Your choice is immediately applied and persisted for future sessions

To override a bundled theme, name your user theme identically (e.g. `name: nord`). Your version takes precedence.


## Concepts

**Theme bundle** — a directory containing `theme.yaml` (metadata and color definitions) and optional `.ans` ANSI art files for panel headers or splash screens.

**Palette** — seven semantic color slots your theme must define:
- `bg` — background color
- `fg` — foreground/primary text
- `accent` — highlights and interactive elements
- `dim` — secondary text and less important UI
- `border` — panel dividers and edges
- `error` — error messages
- `success` — success messages

Colors are hex strings like `#1a1b26`.

**Palette reference** — a string like `palette.accent` in your YAML that points to a palette color. The system resolves these to actual colors at runtime, letting your modal and status bar colors derive from the palette without duplication.

**Mode** — theme classification as `dark` or `light`. Used by the picker to group themes and by your UI to query bundles by appearance preference.

**Headers** — optional per-panel ANSI art sprites (`.ans` files) that replace plain-text panel titles. Stored as a map from panel name to file paths, ordered from widest to narrowest, so the system picks the first sprite that fits your current panel width.

**Modal config** — colors for popup overlays (quit confirmation, agent launch, theme switcher). Fields:
- `bg` — overlay background
- `border` — border color
- `title_bg` — title bar background
- `title_fg` — title bar text color

**Header style** — dynamic header generation when no fixed-width `.ans` sprite fits your panel. Includes top/bottom/border characters and per-panel accent/text color pairs, allowing headers to render at any width.


## Configuration / YAML Reference

A theme bundle is a directory containing `theme.yaml`. Place it at `~/.config/glitch/themes/<name>/` to load at startup.

### `theme.yaml` schema

```yaml
name: example-theme                    # required: kebab-case identifier
display_name: "Example Theme"          # required: human-readable label
mode: dark                             # required: "dark" or "light"

palette:
  bg: "#1a1a2e"                        # required: background hex color
  fg: "#e0e0ff"                        # required: foreground hex color
  accent: "#7b68ee"                    # required: accent hex color
  dim: "#555577"                       # required: dimmed text hex color
  border: "#333355"                    # required: border hex color
  error: "#ff4444"                     # required: error message hex color
  success: "#44ff88"                   # required: success message hex color

borders:
  style: light                         # required: "light", "heavy", or "ascii"

statusbar:
  format: " {session} "                # status bar format string
  bg: "palette.bg"                     # may reference a palette key
  fg: "palette.accent"                 # or use literal hex color like "#ffffff"

modal:
  bg: "palette.bg"
  border: "palette.accent"
  title_bg: "palette.accent"
  title_fg: "palette.bg"

header_style:
  top_char: "▄"                        # optional: character for header top (default "▄")
  bot_char: "▀"                        # optional: character for header bottom (default "▀")
  border_char: "█"                     # optional: character for borders (default "█")
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

headers:                               # optional: per-panel ANSI art file paths
  pipelines: ["splash.ans"]
  agent_runner: ["wide.ans", "narrow.ans"]
```

### Field reference

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Kebab-case unique identifier |
| `display_name` | yes | Human-readable label in the theme picker |
| `mode` | yes | "dark" or "light" |
| `palette.bg` | yes | Background color (hex) |
| `palette.fg` | yes | Foreground text color (hex) |
| `palette.accent` | yes | Accent/highlight color (hex) |
| `palette.dim` | yes | Dimmed/secondary text color (hex) |
| `palette.border` | yes | Panel border color (hex) |
| `palette.error` | yes | Error message color (hex) |
| `palette.success` | yes | Success message color (hex) |
| `borders.style` | yes | "light", "heavy", or "ascii" |
| `statusbar` | no | Status bar configuration |
| `modal` | no | Modal overlay colors |
| `header_style` | no | Dynamic header generation settings |
| `headers` | no | Per-panel ANSI art file paths |


## Tips

**Reusing built-in palettes** — Start by copying a bundled theme's YAML and editing the colors. To override a bundled theme, name your user theme identically (e.g. `name: nord`). Your version takes precedence.

**Using ANSI art for headers** — Place `.ans` files in your theme directory (e.g. `~/.config/glitch/themes/my-theme/splash.ans`), then reference them in the `headers` map with relative paths. List paths from widest to narrowest so the picker selects the best fit for your current panel width.

**Testing palette changes** — Open the theme picker to preview your colors instantly. No reload needed.


## See Also

- [Dracula palette reference](https://draculatheme.com/contribute) — official color specs for extending Dracula-based themes
- [terminalcolors.com](https://terminalcolors.com) — curated repository of terminal themes with YAML exports

