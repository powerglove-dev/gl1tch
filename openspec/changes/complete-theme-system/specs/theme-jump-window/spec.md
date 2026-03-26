## ADDED Requirements

### Requirement: Jump window loads active theme at startup
The `orcai _jump` popup SHALL load the active theme from the persisted registry (`~/.config/orcai/active_theme`) at program startup and apply its colors to all UI elements.

#### Scenario: Jump window uses persisted theme colors
- **WHEN** the jump window starts and an active theme is persisted
- **THEN** all UI colors (header, input, selection, list items, hints) derive from that theme's bundle

#### Scenario: Jump window falls back to Dracula when no theme persisted
- **WHEN** the jump window starts and no active theme file exists
- **THEN** all UI colors use Dracula fallback values

### Requirement: Jump window input field uses theme colors
The search input SHALL use the theme's accent color for the prompt, fg for typed text, and dim for placeholder text.

#### Scenario: Input prompt uses theme accent
- **WHEN** the jump window renders the search input
- **THEN** the `>` prompt color matches the active theme's accent

### Requirement: Jump window list selection uses theme colors
The selected window row SHALL use the theme's border color as background and fg as foreground, replacing hardcoded Dracula `SelBg`/`Pink` constants.

#### Scenario: Selected row uses theme selection colors
- **WHEN** the jump window renders the selected window item
- **THEN** the row uses the active theme's border color as background

### Requirement: Jump window header uses theme modal title colors
The "ORCAI  Jump to Window" header SHALL use `bundle.Modal.TitleBG` as background and `bundle.Modal.TitleFG` as foreground.

#### Scenario: Jump window header uses theme title colors
- **WHEN** the jump window renders its header row
- **THEN** the background matches `bundle.Modal.TitleBG` and foreground matches `bundle.Modal.TitleFG`
