## ADDED Requirements

### Requirement: FuzzyPickerModel provides inline fuzzy selection for static lists
The system SHALL provide a `FuzzyPickerModel` in `internal/modal` that accepts a static `[]string` item list, supports fuzzy filtering via a text input, and renders as an inline dropdown within parent rows using the theme-aware ANSI palette.

#### Scenario: Open with items
- **WHEN** `Open(items []string)` is called
- **THEN** the picker becomes open, items are set, the filter input is cleared and focused, and the shown list is reset to all items

#### Scenario: Fuzzy filter narrows results
- **WHEN** the user types into the filter input
- **THEN** only items whose fuzzy score against the query is > 0 are shown, sorted by score descending

#### Scenario: Confirm selection
- **WHEN** the user presses Enter with at least one item shown
- **THEN** the picker closes and emits `FuzzyPickerConfirmed` with `SelectedItem()` returning the selected item

#### Scenario: Cancel with Escape
- **WHEN** the user presses Escape
- **THEN** the picker closes and emits `FuzzyPickerCancelled` without changing the caller's selection state

#### Scenario: Inline render when open
- **WHEN** `ViewInline(w, maxVisible int, pal ANSIPalette)` is called while open
- **THEN** the rendered string contains a filter input row, a bordered list of up to `maxVisible` items with cursor highlight, and hint text — all at width `w`

### Requirement: DirPickerModel supports inline rendering
The `DirPickerModel` in `internal/modal` SHALL expose a `ViewInline(w int, pal ANSIPalette) string` method that renders the picker list within the calling component's row layout rather than as a centered overlay box.

#### Scenario: Inline render matches overlay content
- **WHEN** `ViewInline` is called while the picker has items
- **THEN** the output contains the filter input, directory list with cursor, and navigation hints at the given width — without any centering padding

#### Scenario: Inline render while walking
- **WHEN** `ViewInline` is called before the dir walk has completed
- **THEN** the output contains "scanning directories…" placeholder text

### Requirement: Saved Prompt field is a tab-reachable focus slot
The agent modal SHALL include Saved Prompt as focus slot 1 in the tab cycle: 0 (provider/model) → 1 (saved prompt) → 2 (prompt) → 3 (use brain) → 4 (cwd) → 5 (schedule) → 0.

#### Scenario: Tab reaches saved prompt slot
- **WHEN** the agent modal is open and focus is at slot 0 (provider/model confirmed)
- **THEN** pressing Tab moves focus to slot 1 (saved prompt)

#### Scenario: Enter opens inline fuzzy picker
- **WHEN** focus is at slot 1 and the user presses Enter
- **THEN** the `FuzzyPickerModel` opens inline below the Saved Prompt row with the list of saved prompt titles (plus a "(none)" option)

#### Scenario: Confirming a saved prompt sets it
- **WHEN** the user selects a title in the fuzzy picker and presses Enter
- **THEN** `agentPromptIdx` is updated to the matching prompt and the picker closes

#### Scenario: `[` and `]` cycling removed
- **WHEN** the user presses `[` or `]` while the agent modal is open
- **THEN** no change occurs to `agentPromptIdx` (keys are no longer handled)

### Requirement: Working Directory field uses inline dir picker
The agent modal's Working Directory field (focus slot 4) SHALL render the `DirPickerModel` inline within the modal rows when active instead of launching a full-screen centered overlay.

#### Scenario: Enter opens inline dir picker
- **WHEN** focus is at slot 4 and the user presses Enter
- **THEN** `dirPickerOpen` is set true and `dirPickerCtx` is set to "agent", and `ViewInline` rows appear below the CWD value row within the agent modal

#### Scenario: No overlay for agent context
- **WHEN** `dirPickerOpen` is true and `dirPickerCtx` is "agent"
- **THEN** the main `View()` does NOT apply `OverlayCenter` for the dir picker

#### Scenario: Pipeline context still uses overlay
- **WHEN** `dirPickerOpen` is true and `dirPickerCtx` is "pipeline"
- **THEN** the main `View()` continues to apply `OverlayCenter` as before
