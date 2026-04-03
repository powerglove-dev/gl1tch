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

Restart gl1tch once, then pick it with `glitch theme set my-theme`.

> **TIP:** Use `"palette.accent"` in status bar and modal fields instead of repeating hex values. Change the accent once and everything updates.


## Customizing


### Border styles

```yaml
borders:
  style: light    # thin unicode box-drawing characters
  style: heavy    # thick unicode box-drawing characters
  style: ascii    # + - | only, for minimal environments
```


### Status bar

```yaml
statusbar:
  format: " {session} · {model} "   # tokens: {session}, {model}
  bg: "palette.bg"
  fg: "palette.accent"
```


### Modal colors

```yaml
modal:
  bg: "palette.bg"
  border: "palette.accent"
  title_bg: "palette.accent"
  title_fg: "palette.bg"
```


### Overriding a built-in theme

Name your theme the same as a built-in. Your version takes precedence.

```yaml
name: nord          # overrides the built-in nord theme
display_name: "Nord (my fork)"
mode: dark
# ... your palette
```


## Examples


### Nord-inspired theme

```yaml
name: nord-custom
display_name: "Nord Custom"
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
```


### Solarized light theme

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
```


## Reference

### `theme.yaml` fields

| Field | Required | Type | Notes |
|-------|----------|------|-------|
| `name` | yes | string | Kebab-case, unique in your registry |
| `display_name` | yes | string | Shown in the theme picker |
| `mode` | yes | string | `dark` or `light` |
| `palette.bg` | yes | hex | Background |
| `palette.fg` | yes | hex | Primary text |
| `palette.accent` | yes | hex | Highlights and interactive elements |
| `palette.dim` | yes | hex | Secondary/muted text |
| `palette.border` | yes | hex | Panel borders |
| `palette.error` | yes | hex | Error messages |
| `palette.success` | yes | hex | Success messages |
| `borders.style` | yes | string | `light`, `heavy`, or `ascii` |
| `statusbar.format` | yes | string | Template string; supports `{session}` and `{model}` |
| `statusbar.bg` | yes | string | Hex or `palette.<key>` reference |
| `statusbar.fg` | yes | string | Hex or `palette.<key>` reference |
| `splash` | no | string | Relative path to an `.ans` ANSI art file |
| `modal.bg` | no | string | Hex or palette reference |
| `modal.border` | no | string | Hex or palette reference |
| `modal.title_bg` | no | string | Hex or palette reference |
| `modal.title_fg` | no | string | Hex or palette reference |

### Bundle directory layout

```text
~/.config/glitch/themes/
└── my-theme/
    ├── theme.yaml
    ├── splash.ans          (optional)
    ├── header-wide.ans     (optional)
    └── header-narrow.ans   (optional)
```


## See Also

- [Plugins](/docs/pipelines/plugins) — extend what your assistant can do
- [Prompts](/docs/pipelines/prompts) — pair a sharp theme with a sharp prompt library
