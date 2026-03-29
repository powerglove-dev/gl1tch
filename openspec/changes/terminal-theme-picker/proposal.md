## Why

The current theme picker presents all themes in a flat single-column list with no separation between dark and light modes, and the bundled theme library is exclusively dark themes — users who prefer light terminals have no options. Expanding the library with curated dark and light themes from terminalcolors.com and redesigning the picker with a tabbed 2-column layout makes theme selection faster and visually organized.

## What Changes

- Add a curated set of light theme YAML bundles sourced from terminalcolors.com (Catppuccin Latte, Solarized Light, Gruvbox Light, One Half Light, Everforest Light, Ayu Light, GitHub Light, Rosé Pine Dawn)
- Add additional dark theme YAML bundles (Everforest Dark, Ayu Dark, One Dark, One Half Dark, Night Owl, Nightfly, Iceberg Dark, Tokyo Night Storm)
- Redesign `ViewThemePicker` / `HandleThemePicker` in `internal/tuikit/theme_picker.go` to use a 2-column layout matching the jump window modal style
- Add tab/shift-tab key handling to switch the active tab between **Dark** and **Light** theme groups
- Update `ThemePicker` struct to carry a `Tab` field (0 = dark, 1 = light) and per-tab cursors

## Capabilities

### New Capabilities
- `theme-library-light`: Curated light terminal theme bundles (YAML assets) ready for activation in ORCAI
- `theme-picker-tabbed`: Redesigned theme picker overlay with dark/light tabs and 2-column theme grid

### Modified Capabilities
<!-- None — theme picker is a UI component, not a spec-tracked capability with existing requirements. -->

## Impact

- `internal/assets/themes/` — new subdirectories for each added theme (dark and light)
- `internal/tuikit/theme_picker.go` — layout rewrite; `ViewThemePicker` and `HandleThemePicker` signatures may gain a tab parameter or the `ThemePicker` struct absorbs state
- `internal/switchboard/theme_picker.go` — minor wiring update if picker struct fields change
- No breaking changes to `themes.Bundle`, registry, or bus events
