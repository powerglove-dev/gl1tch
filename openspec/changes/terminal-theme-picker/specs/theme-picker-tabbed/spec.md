## ADDED Requirements

### Requirement: Tabbed dark/light navigation
The theme picker overlay SHALL display two tabs — **Dark** and **Light** — and the user SHALL be able to switch between them using the `tab` key.

#### Scenario: Default tab is Dark
- **WHEN** the theme picker is opened
- **THEN** the Dark tab SHALL be active and only dark-mode bundles SHALL be listed

#### Scenario: Tab key switches to Light
- **WHEN** the picker is open on the Dark tab and the user presses `tab`
- **THEN** the Light tab SHALL become active and only light-mode bundles SHALL be listed

#### Scenario: Tab key cycles back to Dark
- **WHEN** the picker is open on the Light tab and the user presses `tab`
- **THEN** the Dark tab SHALL become active

### Requirement: Two-column theme grid
Within each tab, themes SHALL be arranged in a 2-column grid. The left column contains the first half of the bundles (indices 0…n/2-1) and the right column the remainder.

#### Scenario: Column focus starts on left
- **WHEN** the theme picker is opened
- **THEN** the left column SHALL have focus

#### Scenario: Right arrow or l moves focus to right column
- **WHEN** the left column has focus and the user presses `l` or the right arrow key
- **THEN** the right column SHALL gain focus

#### Scenario: Left arrow or h moves focus to left column
- **WHEN** the right column has focus and the user presses `h` or the left arrow key
- **THEN** the left column SHALL gain focus

#### Scenario: j/k navigate within the focused column
- **WHEN** a column has focus and the user presses `j`
- **THEN** the cursor in that column SHALL advance by one (clamped to last item)
- **WHEN** a column has focus and the user presses `k`
- **THEN** the cursor in that column SHALL retreat by one (clamped to first item)

### Requirement: Color swatch preview per theme
Each theme entry SHALL display a 7-block color swatch (bg, fg, accent, dim, border, error, success) next to the theme display name, matching the current single-column picker behavior.

#### Scenario: Swatch renders for each bundle
- **WHEN** the picker renders a theme row
- **THEN** seven colored block characters SHALL precede the theme display name

### Requirement: Active theme indicator
The currently active theme SHALL be marked with a checkmark (✓) regardless of which tab or column it appears in.

#### Scenario: Active theme shows checkmark on correct tab
- **WHEN** the active theme is a dark theme and the Dark tab is shown
- **THEN** the active theme entry SHALL display a ✓ suffix

#### Scenario: Active light theme shows checkmark on Light tab
- **WHEN** the active theme is a light theme and the Light tab is shown
- **THEN** the active theme entry SHALL display a ✓ suffix

### Requirement: Enter applies selected theme
Pressing `enter` SHALL activate the theme under the cursor in the focused column, persist it, apply tmux colors, and close the picker.

#### Scenario: Enter on dark theme applies it
- **WHEN** the user navigates to a dark theme and presses `enter`
- **THEN** the registry active theme SHALL be updated
- **AND** tmux colors SHALL be refreshed
- **AND** a `theme.changed` bus event SHALL be published
- **AND** the picker SHALL close

#### Scenario: Enter on light theme applies it
- **WHEN** the user navigates to a light theme and presses `enter`
- **THEN** the same apply-and-close behavior SHALL occur as for dark themes

### Requirement: Real-time theme preview on navigation
As the cursor moves to a new theme entry, the picker SHALL immediately apply that theme's colors to tmux and publish a `theme.changed` busd event — without persisting the selection. This gives a live preview as the user navigates.

#### Scenario: Cursor move triggers live preview
- **WHEN** the user presses `j`, `k`, `h`, or `l` and the cursor lands on a new theme
- **THEN** tmux status bar colors SHALL update to that theme within the same key event
- **AND** a `theme.changed` busd event SHALL be published with the previewed theme name
- **AND** the `~/.config/orcai/active_theme` file SHALL NOT be written

#### Scenario: Escape restores previous active theme
- **WHEN** the user presses `esc` to cancel the picker
- **THEN** the originally active theme (at picker-open time) SHALL be re-applied via tmux and busd
- **AND** the `~/.config/orcai/active_theme` file SHALL remain unchanged

#### Scenario: Enter commits the previewed theme
- **WHEN** the user presses `enter` while previewing a theme
- **THEN** that theme SHALL be persisted to `~/.config/orcai/active_theme`
- **AND** the picker SHALL close

### Requirement: Hint bar shows all active keys
The picker hint bar SHALL show keybindings for all active controls: `tab` (switch mode), `h/l` (switch column), `j/k` (navigate), `enter` (apply), `esc` (cancel).

#### Scenario: Hint bar content
- **WHEN** the picker is rendered
- **THEN** the hint bar SHALL display at minimum: tab, h/l, j/k, enter, esc bindings
