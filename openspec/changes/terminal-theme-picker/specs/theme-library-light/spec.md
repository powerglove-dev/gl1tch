## ADDED Requirements

### Requirement: Bundle mode classification
Each theme bundle YAML SHALL include an optional `mode` field with value `dark` or `light`. Bundles without a `mode` field SHALL be treated as `dark`.

#### Scenario: Dark bundle without mode field
- **WHEN** a `theme.yaml` does not include a `mode` field
- **THEN** the loaded bundle SHALL report `Mode == "dark"`

#### Scenario: Light bundle with mode field
- **WHEN** a `theme.yaml` includes `mode: light`
- **THEN** the loaded bundle SHALL report `Mode == "light"`

### Requirement: Registry mode filtering
The theme registry SHALL expose a `BundlesByMode(mode string) []Bundle` method that returns only bundles matching the given mode.

#### Scenario: Filtering dark bundles
- **WHEN** `BundlesByMode("dark")` is called
- **THEN** only bundles with `Mode == "dark"` SHALL be returned

#### Scenario: Filtering light bundles
- **WHEN** `BundlesByMode("light")` is called
- **THEN** only bundles with `Mode == "light"` SHALL be returned

### Requirement: Total theme library of 55 bundles
The embedded theme library SHALL contain exactly 55 bundles across dark and light modes, sourced from terminalcolors.com alongside the existing ORCAI-original themes.

#### Scenario: Registry loads all 55 bundles
- **WHEN** the registry is initialized with default assets
- **THEN** `len(registry.All())` SHALL equal 55

### Requirement: 40 bundled dark themes
The following 40 dark theme YAML bundles SHALL be included as embedded assets in `internal/assets/themes/` (10 existing + 30 new):

**Existing (retain as-is, add `mode: dark`):**
- `abs`, `nord`, `dracula`, `borland`, `catppuccin-mocha`, `gruvbox`, `kanagawa`, `rose-pine`, `solarized-dark`, `tokyo-night`

**New dark bundles:**
- `apprentice`, `ayu-dark`, `catppuccin-macchiato`, `catppuccin-frappe`, `cobalt2`, `deus`, `everforest-dark`, `github-dark`, `gotham`, `iceberg-dark`, `jellybeans`, `kanagawa-dragon`, `lucario`, `miasma`, `moonfly`, `night-owl`, `nightfly`, `nordic`, `one-dark`, `one-half-dark`, `panda`, `seoul256-dark`, `shades-of-purple`, `sonokai`, `srcery`, `tender`, `tokyo-night-storm`, `tokyo-night-moon`, `tomorrow-night`, `zenbones-dark`

#### Scenario: All dark themes load from registry
- **WHEN** the registry is initialized
- **THEN** `BundlesByMode("dark")` SHALL return 40 bundles

### Requirement: 15 bundled light themes
The following 15 light theme YAML bundles SHALL be included as embedded assets with `mode: light`:
- `ayu-light`, `catppuccin-latte`, `everforest-light`, `github-light`, `gruvbox-light`, `iceberg-light`, `night-owl-light`, `one-light`, `one-half-light`, `rose-pine-dawn`, `seoul256-light`, `solarized-light`, `tomorrow`, `zenbones-light`, `noctis-lux`

#### Scenario: All light themes load from registry
- **WHEN** the registry is initialized
- **THEN** `BundlesByMode("light")` SHALL return 15 bundles

### Requirement: Light theme activation
Any light theme SHALL be activatable via the picker and the theme CLI command.

#### Scenario: Light theme activation
- **WHEN** a light theme is activated
- **THEN** the palette colors SHALL reflect that theme's light background and foreground values
- **AND** the active theme SHALL be persisted to `~/.config/orcai/active_theme`

### Requirement: Backward compatibility for existing themes
All 10 existing bundled themes SHALL continue to load and activate without change to their palette, borders, or display name.

#### Scenario: Existing theme unchanged after mode field addition
- **WHEN** an existing theme YAML gains a `mode: dark` field
- **THEN** the bundle's palette, borders, and display name SHALL be identical to pre-change values
