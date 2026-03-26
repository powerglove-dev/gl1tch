## ADDED Requirements

### Requirement: Shared modal color resolver
A `resolveModalColors()` method on `Model` SHALL return a `modalColors` struct containing all colors needed by any modal (border, titleBG, titleFG, fg, accent, dim, error), derived from the active bundle with Dracula fallbacks.

#### Scenario: resolveModalColors uses active bundle
- **WHEN** an active theme bundle is set
- **THEN** `resolveModalColors()` returns colors from that bundle's `Modal` and `Palette` fields

#### Scenario: resolveModalColors falls back to Dracula
- **WHEN** no active theme bundle is set (nil registry)
- **THEN** `resolveModalColors()` returns Dracula hex values as fallbacks

### Requirement: Quit confirmation modal is fully themed
The quit modal SHALL derive all colors (border, title background/foreground, body text, key hint accent, key hint dim) from the active theme bundle.

#### Scenario: Quit modal border matches theme
- **WHEN** the quit confirmation modal renders with an active theme
- **THEN** the rounded border color matches `bundle.Modal.Border` resolved value

#### Scenario: Quit modal yes/no keys use theme accent and dim
- **WHEN** the quit modal renders key hints
- **THEN** `[y]` uses the theme's accent color and `[n]` uses the theme's dim color

### Requirement: Delete confirmation modal is fully themed
The delete pipeline modal SHALL derive all colors from the active theme bundle, replacing all hardcoded `styles.Purple`, `styles.Pink`, `styles.Comment` constants.

#### Scenario: Delete modal header uses theme title colors
- **WHEN** the delete confirmation modal renders
- **THEN** the header background is `bundle.Modal.TitleBG` and foreground is `bundle.Modal.TitleFG`

#### Scenario: Delete modal pipeline name uses theme accent
- **WHEN** the delete modal renders the pipeline name
- **THEN** the name text uses the theme's accent color

### Requirement: Agent runner modal content is fully themed
The agent runner modal SHALL derive all content colors from the active bundle's `ansiPalette()`, replacing hardcoded ANSI constants for section labels, provider/model lists, selection highlights, key hints, and warning text.

#### Scenario: Agent modal section headers use theme accent when active
- **WHEN** the agent modal renders a focused section header
- **THEN** the header text uses the theme's accent ANSI sequence

#### Scenario: Agent modal selection highlight uses theme SelBG
- **WHEN** the agent modal renders the focused selected item
- **THEN** the row background uses the theme's `SelBG` sequence
