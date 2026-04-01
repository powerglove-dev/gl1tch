## ADDED Requirements

### Requirement: Panel header label is "agents"
The center panel header SHALL display the label "agents" (replacing the former "signal_board" label). The panel header is rendered via the existing `PanelHeader` / `boxTop` helpers using the key `"agents"`.

#### Scenario: Panel header shows agents label
- **WHEN** the center panel is rendered
- **THEN** the panel title bar contains the text "agents"

### Requirement: Grid layout for agent cards
Agents SHALL be displayed as a 2-D grid of fixed-width cards. The number of columns SHALL be `max(1, midW / 24)` so each card is at least 24 characters wide. Each card SHALL display the agent name, its current status badge, and a short status label on a single line.

#### Scenario: Grid columns computed from available width
- **WHEN** center column width is 48 characters
- **THEN** grid renders 2 columns of cards

#### Scenario: Minimum one column at narrow widths
- **WHEN** center column width is less than 24 characters
- **THEN** grid renders 1 column of cards

### Requirement: h/j/k/l cursor navigation
The agents grid SHALL support cursor navigation using `h` (left), `j` (down), `k` (up), `l` (right). The cursor SHALL be clamped to valid grid indices. Navigation SHALL only apply when the center panel is focused.

#### Scenario: l moves cursor right
- **WHEN** center panel is focused and the user presses `l`
- **THEN** grid cursor column increments by 1 (clamped to last column)

#### Scenario: h moves cursor left
- **WHEN** center panel is focused and the user presses `h`
- **THEN** grid cursor column decrements by 1 (clamped to 0)

#### Scenario: j moves cursor down
- **WHEN** center panel is focused and the user presses `j`
- **THEN** grid cursor row increments by 1 (clamped to last row)

#### Scenario: k moves cursor up
- **WHEN** center panel is focused and the user presses `k`
- **THEN** grid cursor row decrements by 1 (clamped to 0)

#### Scenario: Cursor clamped when agent list shrinks
- **WHEN** an agent is removed and the cursor would be out of bounds
- **THEN** cursor is clamped to the last valid index

### Requirement: Cursor highlight
The agent card under the cursor SHALL be rendered with the accent color border or background to distinguish it from non-selected cards.

#### Scenario: Selected card uses accent style
- **WHEN** cursor is at row 0, col 1
- **THEN** the card at that grid position renders with accent color highlighting
