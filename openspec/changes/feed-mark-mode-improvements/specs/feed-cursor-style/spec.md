## ADDED Requirements

### Requirement: Feed cursor indicator overlays content without layout shift
The Activity Feed cursor indicator (`> `) SHALL overlay the first two visible characters of the row content rather than prepending to it. The total visible width of a cursor row SHALL equal the total visible width of a non-cursor row. Content SHALL NOT be shifted right when the cursor is on a line.

#### Scenario: Cursor row has the same width as a non-cursor row
- **WHEN** the feed cursor is on a line
- **THEN** the rendered row width in visible columns equals the width of a non-cursor row at the same position

#### Scenario: Cursor indicator replaces leading characters rather than prepending
- **WHEN** the feed cursor is on a line with leading whitespace
- **THEN** the `> ` indicator occupies the first two character columns and the remaining content follows without shifting

### Requirement: Feed cursor indicator uses theme accent color
The `> ` cursor indicator in the Activity Feed SHALL be rendered using `ANSIPalette.Accent` from the active theme palette rather than a hardcoded ANSI color constant.

#### Scenario: Cursor uses accent color from active theme
- **WHEN** a theme bundle is active and the feed cursor is on a line
- **THEN** the `> ` indicator is rendered in the palette's accent color

#### Scenario: Cursor color matches other panel cursor indicators
- **WHEN** the feed and another panel both show cursor indicators
- **THEN** both indicators use the same theme accent color
