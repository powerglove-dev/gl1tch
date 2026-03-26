## ADDED Requirements

### Requirement: Five new built-in themes available in theme picker
The theme registry SHALL include five additional bundled themes discoverable via the theme picker: Catppuccin Mocha, Tokyo Night, Rose Pine, Solarized Dark, and Kanagawa.

#### Scenario: New themes appear in theme picker
- **WHEN** the user opens the theme picker overlay
- **THEN** all five new themes are listed alongside existing themes

#### Scenario: New themes load without error
- **WHEN** `themes.NewRegistry` initializes
- **THEN** all five new theme bundles load without error and have valid palette fields

### Requirement: Each new theme has a complete palette and modal config
Each new theme YAML SHALL define `palette` (bg, fg, accent, dim, border, error, success), `statusbar`, `header_style` (all five panels with same accent/text), and `modal` (bg, border, title_bg, title_fg) sections.

#### Scenario: Catppuccin Mocha palette is correct
- **WHEN** the Catppuccin Mocha theme is loaded
- **THEN** `palette.bg` is `#1e1e2e`, `palette.accent` is `#cba6f7`, `palette.success` is `#a6e3a1`

#### Scenario: Tokyo Night palette is correct
- **WHEN** the Tokyo Night theme is loaded
- **THEN** `palette.bg` is `#1a1b26`, `palette.accent` is `#7aa2f7`, `palette.error` is `#f7768e`

#### Scenario: Rose Pine palette is correct
- **WHEN** the Rose Pine theme is loaded
- **THEN** `palette.bg` is `#191724`, `palette.accent` is `#c4a7e7`, `palette.dim` is `#6e6a86`

#### Scenario: Solarized Dark palette is correct
- **WHEN** the Solarized Dark theme is loaded
- **THEN** `palette.bg` is `#002b36`, `palette.accent` is `#268bd2`, `palette.border` is `#073642`

#### Scenario: Kanagawa palette is correct
- **WHEN** the Kanagawa theme is loaded
- **THEN** `palette.bg` is `#1f1f28`, `palette.accent` is `#7e9cd8`, `palette.success` is `#76946a`
