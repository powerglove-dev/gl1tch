## ADDED Requirements

### Requirement: Switchboard is the single full-screen entry point
The switchboard SHALL be a full-screen BubbleTea TUI launched by `orcai sysop`, `orcai welcome`, `orcai-sysop`, and `orcai-welcome`. All four entry points SHALL call `switchboard.Run()` from `internal/switchboard/`. The old separate welcome dashboard and sysop panel SHALL no longer exist as independent TUI programs.

#### Scenario: orcai sysop launches switchboard
- **WHEN** the user runs `orcai sysop`
- **THEN** the full-screen switchboard opens

#### Scenario: orcai welcome launches switchboard
- **WHEN** the user runs `orcai welcome`
- **THEN** the full-screen switchboard opens (not the old ANSI art dashboard)

#### Scenario: orcai-welcome binary launches switchboard
- **WHEN** the `orcai-welcome` binary is executed
- **THEN** the full-screen switchboard opens

### Requirement: Switchboard layout has three regions
The switchboard SHALL render in three persistent regions:
1. **Left column** (≈30% width): two sections — Pipeline Launcher (top) and Agent Runner (bottom).
2. **Center/main area** (remaining width): Activity Feed showing pipeline and agent output.
3. **Bottom bar** (1–2 lines): compact keybinding reference strip.

#### Scenario: Three regions visible simultaneously
- **WHEN** the switchboard is rendered at any terminal width ≥ 80 columns
- **THEN** all three regions are visible without truncation

#### Scenario: Regions resize with terminal
- **WHEN** the terminal is resized
- **THEN** the three regions reflow to maintain proportional widths

### Requirement: Bottom keybinding bar replaces welcome dashboard cheatsheet
The switchboard bottom bar SHALL display the most-used keybindings in a single- or two-line compact strip using Dracula palette colors. It SHALL include at minimum: launch pipeline, run agent, quit, refresh providers. The standalone welcome dashboard full-screen cheatsheet SHALL be removed.

#### Scenario: Keybinding bar visible at bottom
- **WHEN** the switchboard is rendered
- **THEN** the bottom one or two lines show keybinding hints (e.g. `enter launch · a agent · r refresh · q quit`)

### Requirement: ANSI/BBS banner displays at top of left column
The switchboard SHALL retain the Dracula-palette ANSI/BBS banner at the top of the left column or as a header above all columns. The banner SHALL use box-drawing characters and the ORCAI logotype.

#### Scenario: Banner visible in switchboard
- **WHEN** the switchboard renders
- **THEN** the ORCAI BBS banner is visible using Dracula palette colors

### Requirement: Switchboard subscribes to busd for live telemetry
The switchboard SHALL connect to the event bus on startup (same logic as current sysop panel) and update the activity feed when `orcai.telemetry` events arrive. If the bus is unavailable the switchboard SHALL render normally without telemetry.

#### Scenario: Telemetry updates activity feed
- **WHEN** an `orcai.telemetry` event arrives from the bus
- **THEN** the activity feed updates without requiring manual refresh

#### Scenario: Bus unavailable does not crash
- **WHEN** the bus daemon is not running
- **THEN** the switchboard opens and renders without telemetry data

### Requirement: q or ctrl+c exits the switchboard
Pressing `q` or `ctrl+c` SHALL exit the switchboard. If a job is running, the switchboard SHALL prompt for confirmation before exiting.

#### Scenario: q exits when idle
- **WHEN** no job is running and the user presses `q`
- **THEN** the switchboard exits

#### Scenario: q prompts when job is running
- **WHEN** a pipeline or agent job is currently running and the user presses `q`
- **THEN** the switchboard shows a confirmation prompt before exiting
