## ADDED Requirements

### Requirement: Nav bar includes Labs and Docs entries
The site navigation SHALL include `[8] LABS` and `[9] DOCS` entries that activate the corresponding screens.

#### Scenario: Labs nav entry present
- **WHEN** the site loads
- **THEN** the nav bar SHALL render a button labeled `[8] LABS` with `data-nav-screen="labs"`

#### Scenario: Docs nav entry present
- **WHEN** the site loads
- **THEN** the nav bar SHALL render a button labeled `[9] DOCS` with `data-nav-screen="docs"`

#### Scenario: Key `8` activates Labs
- **WHEN** a user presses the `8` key
- **THEN** `KeyboardRouter.switchScreen('labs')` SHALL be called and the labs screen SHALL become visible

#### Scenario: Key `9` activates Docs
- **WHEN** a user presses the `9` key
- **THEN** `KeyboardRouter.switchScreen('docs')` SHALL be called and the docs screen SHALL become visible

### Requirement: Key hints updated in HelpOverlay
The `[?]` help overlay SHALL list the `8` and `9` key bindings alongside the existing bindings.

#### Scenario: Help overlay shows 8 and 9
- **WHEN** the help overlay is open
- **THEN** it SHALL display entries for `8 → LABS` and `9 → DOCS`
