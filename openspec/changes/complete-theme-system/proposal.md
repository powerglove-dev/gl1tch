## Why

The ORCAI switchboard currently uses hardcoded Dracula color constants throughout panel content, modals, and the jump window popup — meaning theme selection only affects headers and borders, while everything else stays Dracula regardless of which theme is active. Users need every visible element to respond to the active theme.

## What Changes

- All panel content (pipelines list, agent runner list, signal board LEDs/labels, activity feed rows) derive colors from the active theme's `ANSIPalette`
- All modals (quit confirm, delete confirm, agent runner) fully themed via a shared `resolveModalColors` helper
- Jump window popup (`orcai _jump`) loads the persisted active theme from `~/.config/orcai/active_theme` at startup
- `styles.ANSIPalette` gains a `SelBG` field for theme-driven selection highlight backgrounds
- `boxTop` label color uses theme accent instead of hardcoded bright-cyan
- `statusIcon()` accepts palette parameter instead of returning hardcoded escape codes
- 5 new bundled themes added: Catppuccin Mocha, Tokyo Night, Rose Pine, Solarized Dark, Kanagawa

## Capabilities

### New Capabilities

- `theme-panel-content`: All panel body rows (pipelines, agent runner, signal board, activity feed) use theme-derived ANSI colors for text, selection highlights, status LEDs, and dim/accent contrast
- `theme-modals`: Quit, delete, and agent runner modals derive all colors (border, title, body text, key hints) from the active bundle's Modal and Palette fields
- `theme-jump-window`: The `orcai _jump` display-popup loads the persisted active theme via `themes.Registry` and applies it to all UI colors
- `theme-new-bundles`: Five additional built-in themes (Catppuccin Mocha, Tokyo Night, Rose Pine, Solarized Dark, Kanagawa) available in the theme picker

### Modified Capabilities

## Impact

- `internal/styles/styles.go`: `ANSIPalette` struct gains `SelBG` field; `BundleANSI` populates it
- `internal/switchboard/switchboard.go`: `ansiPalette()` fallback gains `SelBG`; `resolveModalColors()` helper added; `buildLauncherSection`, `buildAgentSection`, `viewActivityFeed`, `viewQuitModalBox`, `viewDeleteModalBox`, `viewAgentModalBox`, `boxTop`, `statusIcon` all updated
- `internal/switchboard/signal_board.go`: `buildSignalBoard` LED and label colors updated
- `internal/jumpwindow/jumpwindow.go`: gains `themes` import, `jumpPalette` struct, `loadPalette()`, updates `newModel()` and `View()`
- `internal/assets/themes/`: 5 new YAML theme directories added
