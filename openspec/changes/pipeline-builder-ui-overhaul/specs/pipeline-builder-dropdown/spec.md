## ADDED Requirements

### Requirement: Dropdown renders an inline overlay of selectable items
The pipeline builder's `Dropdown` component SHALL render an open dropdown as a bordered overlay box immediately below the focused field, using box-drawing characters (`┌─┐│└┘`) and the ABBS Dracula palette. The overlay SHALL show at most 8 items at a time, scrolling if the item list is longer.

#### Scenario: Dropdown opens on Enter or Space
- **WHEN** the user presses `Enter` or `Space` on a focused dropdown field
- **THEN** the overlay appears below the field label showing all available items, with the current selection highlighted in pink

#### Scenario: Dropdown scrolls when items exceed 8
- **WHEN** the dropdown is open and the item list contains more than 8 entries
- **THEN** only 8 items are visible at a time and Up/Down arrows scroll the visible window

#### Scenario: Dropdown closes and applies selection on Enter
- **WHEN** the dropdown is open and the user presses `Enter`
- **THEN** the overlay closes and the focused item becomes the selected value

#### Scenario: Dropdown closes without change on Escape
- **WHEN** the dropdown is open and the user presses `Escape`
- **THEN** the overlay closes and the previously selected value is retained unchanged

### Requirement: Dropdown uses ABBS Dracula palette
The dropdown overlay border SHALL use purple (`\x1b[38;5;141m`), the selected item SHALL be highlighted in pink (`\x1b[38;5;212m`), and unselected items SHALL be rendered in the dim color (`\x1b[38;5;66m`). No other color schemes are permitted in the dropdown component.

#### Scenario: Selected item is visually distinct
- **WHEN** the dropdown is open
- **THEN** the currently highlighted item is rendered in pink and prefixed with `▶`, while all other items are dim

#### Scenario: Overlay border uses box-drawing characters
- **WHEN** the dropdown is open
- **THEN** the overlay uses `┌`, `─`, `┐`, `│`, `└`, `┘` for its border, rendered in purple

### Requirement: Dropdown supports separator items
The dropdown SHALL support separator entries (visual dividers) in its item list. Separators SHALL be rendered as a dim horizontal rule and SHALL NOT be selectable — the cursor skips over them.

#### Scenario: Cursor skips separator on Down arrow
- **WHEN** the dropdown is open and the cursor is on the item directly above a separator
- **THEN** pressing `Down` moves the cursor to the first non-separator item below the separator

#### Scenario: Separator is not selectable via Enter
- **WHEN** the dropdown cursor is on a separator (not possible by design — cursor skips them)
- **THEN** the Enter key has no effect (invariant: cursor is never on a separator)
