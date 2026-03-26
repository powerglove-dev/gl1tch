## ADDED Requirements

### Requirement: Panel body rows use theme accent color for highlighted text
All panel body rows (pipelines list, agent runner provider/model lists, signal board rows, activity feed entries) SHALL use the active theme's accent color for highlighted/interactive text instead of hardcoded bright-cyan (`\x1b[96m`).

#### Scenario: Pipeline list selected item uses theme accent
- **WHEN** the pipelines panel renders a selected pipeline item while unfocused
- **THEN** the item text color is the active theme's `palette.accent` ANSI sequence

#### Scenario: Activity feed entry title uses theme accent
- **WHEN** the activity feed renders a job entry title
- **THEN** the title text uses the active theme's `palette.accent` ANSI sequence

### Requirement: Panel body rows use theme dim color for secondary text
Timestamps, dimmed labels, "no items" placeholders, and output lines SHALL use the active theme's `palette.dim` ANSI sequence.

#### Scenario: Activity feed timestamp is dim
- **WHEN** the activity feed renders a job entry
- **THEN** the timestamp portion uses the active theme's `palette.dim` ANSI sequence

#### Scenario: Empty state uses theme dim
- **WHEN** a panel has no items (no pipelines, no providers, no activity)
- **THEN** the "no items" placeholder text uses the active theme's `palette.dim` ANSI sequence

### Requirement: Signal board LEDs use theme success/error/accent colors
Signal board status LEDs SHALL use `palette.success` for done, `palette.error` for failed, and `palette.accent` for running (blinking on) / `palette.dim` for running (blinking off).

#### Scenario: Done job LED is theme success color
- **WHEN** the signal board renders a job with `FeedDone` status
- **THEN** the LED `●` uses the active theme's `palette.success` ANSI sequence

#### Scenario: Failed job LED is theme error color
- **WHEN** the signal board renders a job with `FeedFailed` status
- **THEN** the LED `●` uses the active theme's `palette.error` ANSI sequence

### Requirement: Selection highlight background uses theme border color
Selected rows SHALL use the active theme's `palette.border` as a background color (24-bit ANSI BG sequence) for the selection highlight, replacing the hardcoded blue background (`\x1b[44m`).

#### Scenario: Selected and focused pipeline uses theme selection background
- **WHEN** the pipelines panel renders a selected item while focused
- **THEN** the row background is derived from the active theme's `palette.border` as a 24-bit BG sequence

### Requirement: ANSIPalette includes SelBG field
`styles.ANSIPalette` SHALL include a `SelBG` field containing the 24-bit ANSI background sequence (`\x1b[48;2;R;G;Bm`) derived from `palette.border`.

#### Scenario: BundleANSI populates SelBG
- **WHEN** `styles.BundleANSI(bundle)` is called with any valid bundle
- **THEN** the returned palette's `SelBG` field is a valid 24-bit ANSI background sequence for the bundle's border color

### Requirement: boxTop label color uses theme accent
The `boxTop` panel header function SHALL use the theme accent color for the panel title label instead of hardcoded bright-cyan.

#### Scenario: Panel header title uses theme accent
- **WHEN** any panel renders its header via `boxTop`
- **THEN** the title text color matches the active theme's accent color
