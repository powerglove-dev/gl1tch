# Theme Bundles — Contributor Guide

A theme bundle is a directory containing a `theme.yaml` manifest and an optional `splash.ans` ANSI art file. Themes control the colour palette, border style, status bar format, and welcome splash shown when orcai starts.

orcai ships with the built-in ABS theme. You can override it or add entirely new themes by placing a bundle in `~/.config/orcai/themes/`.

## Bundle directory layout

```
~/.config/orcai/themes/
└── my-theme/
    ├── theme.yaml
    └── splash.ans      (optional)
```

## `theme.yaml` schema

```yaml
name: my-theme
display_name: "My Theme"

palette:
  bg: "#1a1a2e"
  fg: "#e0e0ff"
  accent: "#7b68ee"
  dim: "#555577"
  border: "#333355"
  error: "#ff4444"
  success: "#44ff88"

borders:
  style: light   # light, heavy, or ascii

statusbar:
  format: " {session} "
  bg: "palette.bg"      # may reference a palette key
  fg: "palette.accent"

splash: splash.ans      # optional; path relative to the theme directory
```

### Field reference

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Kebab-case unique identifier. Used as the registry key. |
| `display_name` | yes | Human-readable label shown in the sysop panel theme picker. |
| `palette.bg` | yes | Background colour (hex). |
| `palette.fg` | yes | Foreground / primary text colour (hex). |
| `palette.accent` | yes | Accent / highlight colour (hex). |
| `palette.dim` | yes | Dimmed / secondary text colour (hex). |
| `palette.border` | yes | Border colour (hex). |
| `palette.error` | yes | Error / warning colour (hex). |
| `palette.success` | yes | Success / confirmation colour (hex). |
| `borders.style` | no | One of `light`, `heavy`, or `ascii`. Defaults to `light`. |
| `statusbar.format` | no | Status bar template. `{session}` is replaced with the active session name. |
| `statusbar.bg` | no | Status bar background. May be a hex colour or a `palette.X` reference. |
| `statusbar.fg` | no | Status bar foreground. May be a hex colour or a `palette.X` reference. |
| `splash` | no | Relative path to an ANSI art file rendered on the welcome screen. |

### Palette keys

All seven palette keys (`bg`, `fg`, `accent`, `dim`, `border`, `error`, `success`) are required. orcai will refuse to load a theme with a missing palette key and fall back to the bundled ABS theme.

### Palette references in `statusbar`

The `statusbar.bg` and `statusbar.fg` fields accept either a literal hex colour or a `palette.X` reference string (e.g. `"palette.accent"`). References are resolved by `Bundle.ResolveRef()` at load time — the result is the corresponding hex value from the palette.

### Border styles

| Value | Description |
|-------|-------------|
| `light` | Thin Unicode box-drawing characters (`─`, `│`, `┌`, etc.) |
| `heavy` | Thick Unicode box-drawing characters (`━`, `┃`, `┏`, etc.) |
| `ascii` | ASCII-only (`-`, `|`, `+`) for terminals that lack Unicode support |

## User themes and override behaviour

orcai loads bundled themes first, then user themes from `~/.config/orcai/themes/`. When two themes share the same `name`, the **user theme wins**. This lets you fully replace the ABS theme by creating a bundle named `abs` in your themes directory.

## Switching the active theme

```
orcai theme set my-theme
```

The active theme is persisted to `~/.config/orcai/active_theme` by `themes.Registry.SetActive()` and restored automatically on next launch. You can also switch themes via the sysop panel (when available) which publishes a `theme.changed` event to all connected widgets.

## ANSI art (`splash.ans`)

The optional `splash.ans` file is rendered on the orcai welcome screen. Keep the following in mind:

- The path in `theme.yaml` is relative to the theme directory.
- Strip cursor-movement escape sequences (e.g. `\033[H`, `\033[2J`) — BubbleTea manages its own cursor and these sequences will corrupt the TUI layout.
- Colour escape sequences (`\033[38;2;R;G;Bm`, `\033[0m`, etc.) are fully supported.
- Use `ClampWidth` (from `internal/ansiart`) if you need to trim the art to a maximum column width at runtime.

## Example: minimal dark theme

```yaml
name: midnight
display_name: "Midnight"

palette:
  bg: "#0d0d0d"
  fg: "#cccccc"
  accent: "#00bfff"
  dim: "#444444"
  border: "#222222"
  error: "#ff3333"
  success: "#33ff99"

borders:
  style: heavy

statusbar:
  format: " {session} "
  bg: "palette.bg"
  fg: "palette.accent"
```

Install it:

```
mkdir -p ~/.config/orcai/themes/midnight
# save the YAML above as ~/.config/orcai/themes/midnight/theme.yaml
orcai theme set midnight
```
