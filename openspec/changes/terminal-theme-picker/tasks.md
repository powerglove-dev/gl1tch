## 1. Theme Bundle Struct & Registry

- [x] 1.1 Add `Mode string` field to `themes.Bundle` struct in `internal/themes/themes.go`
- [x] 1.2 Update YAML loader in `internal/themes/loader.go` to read `mode` field; default to `"dark"` when absent
- [x] 1.3 Add `BundlesByMode(mode string) []Bundle` method to registry in `internal/themes/registry.go`
- [x] 1.4 Add `mode: dark` field to all 10 existing theme YAMLs in `internal/assets/themes/`

## 2. Dark Theme Bundles (30 new)

- [x] 2.1 Add `apprentice` theme YAML (dark)
- [x] 2.2 Add `ayu-dark` theme YAML (dark)
- [x] 2.3 Add `catppuccin-macchiato` theme YAML (dark)
- [x] 2.4 Add `catppuccin-frappe` theme YAML (dark)
- [x] 2.5 Add `cobalt2` theme YAML (dark)
- [x] 2.6 Add `deus` theme YAML (dark)
- [x] 2.7 Add `everforest-dark` theme YAML (dark)
- [x] 2.8 Add `github-dark` theme YAML (dark)
- [x] 2.9 Add `gotham` theme YAML (dark)
- [x] 2.10 Add `iceberg-dark` theme YAML (dark)
- [x] 2.11 Add `jellybeans` theme YAML (dark)
- [x] 2.12 Add `kanagawa-dragon` theme YAML (dark)
- [x] 2.13 Add `lucario` theme YAML (dark)
- [x] 2.14 Add `miasma` theme YAML (dark)
- [x] 2.15 Add `moonfly` theme YAML (dark)
- [x] 2.16 Add `night-owl` theme YAML (dark)
- [x] 2.17 Add `nightfly` theme YAML (dark)
- [x] 2.18 Add `nordic` theme YAML (dark)
- [x] 2.19 Add `one-dark` theme YAML (dark)
- [x] 2.20 Add `one-half-dark` theme YAML (dark)
- [x] 2.21 Add `panda` theme YAML (dark)
- [x] 2.22 Add `seoul256-dark` theme YAML (dark)
- [x] 2.23 Add `shades-of-purple` theme YAML (dark)
- [x] 2.24 Add `sonokai` theme YAML (dark)
- [x] 2.25 Add `srcery` theme YAML (dark)
- [x] 2.26 Add `tender` theme YAML (dark)
- [x] 2.27 Add `tokyo-night-storm` theme YAML (dark)
- [x] 2.28 Add `tokyo-night-moon` theme YAML (dark)
- [x] 2.29 Add `tomorrow-night` theme YAML (dark)
- [x] 2.30 Add `zenbones-dark` theme YAML (dark)

## 3. Light Theme Bundles (15 new)

- [x] 3.1 Add `ayu-light` theme YAML (light)
- [x] 3.2 Add `catppuccin-latte` theme YAML (light)
- [x] 3.3 Add `everforest-light` theme YAML (light)
- [x] 3.4 Add `github-light` theme YAML (light)
- [x] 3.5 Add `gruvbox-light` theme YAML (light)
- [x] 3.6 Add `iceberg-light` theme YAML (light)
- [x] 3.7 Add `night-owl-light` theme YAML (light)
- [x] 3.8 Add `one-light` theme YAML (light)
- [x] 3.9 Add `one-half-light` theme YAML (light)
- [x] 3.10 Add `rose-pine-dawn` theme YAML (light)
- [x] 3.11 Add `seoul256-light` theme YAML (light)
- [x] 3.12 Add `solarized-light` theme YAML (light)
- [x] 3.13 Add `tomorrow` theme YAML (light — the original Tomorrow light base)
- [x] 3.14 Add `zenbones-light` theme YAML (light)
- [x] 3.15 Add `noctis-lux` theme YAML (light)

## 4. Theme Picker — Real-Time Preview

- [x] 4.1 Extract `PreviewTheme(bundle themes.Bundle)` helper in `internal/tuikit/theme_picker.go` — applies tmux + publishes busd event but does NOT write `active_theme` file
- [x] 4.2 Update `HandleThemePicker` to call `PreviewTheme` on each cursor move (j/k)
- [x] 4.3 Store `originalTheme themes.Bundle` in `ThemePicker` struct to restore on `esc`
- [x] 4.4 On `esc`, call `PreviewTheme(originalTheme)` to revert tmux + busd without persisting

## 5. Theme Picker — Tabbed 2-Column Layout

- [x] 5.1 Add `Tab int`, `DarkCursor int`, `LightCursor int`, `ColFocus int`, `OriginalTheme *themes.Bundle` fields to `ThemePicker` struct
- [x] 5.2 Rewrite `ViewThemePicker` to accept split dark/light bundle slices and active tab; render tab bar header (`[Dark] / Light` or `Dark / [Light]`)
- [x] 5.3 Implement 2-column grid layout in `ViewThemePicker` using `lipgloss.JoinHorizontal`, mirroring jump window column pattern
- [x] 5.4 Update `HandleThemePicker` to handle `tab` key (cycle Dark/Light tabs), `h`/`left` and `l`/`right` for column focus, and `j`/`k` per column cursor
- [x] 5.5 Call `PreviewTheme` on every cursor/tab change in `HandleThemePicker`
- [x] 5.6 Update hint bar to include `tab`, `h/l`, `j/k`, `enter`, `esc` bindings

## 6. Call-Site Updates

- [x] 6.1 Update `internal/switchboard/theme_picker.go` to split bundles by mode and pass to new `ViewThemePicker` / `HandleThemePicker`
- [x] 6.2 Update `internal/crontui/` theme picker integration for new struct fields and function signatures
- [x] 6.3 Verify `jumpwindow` is unaffected (read-only check, no changes expected)

## 7. Validation

- [x] 7.1 Confirm `BundlesByMode("dark")` returns 40 bundles and `BundlesByMode("light")` returns 15 bundles
- [x] 7.2 Manually test picker: navigate dark themes, verify tmux updates in real time, press esc, confirm revert
- [x] 7.3 Manually test picker: tab to Light, navigate light themes, press enter, confirm persistence
- [x] 7.4 Test at 80-column terminal width — both columns must render without overflow
