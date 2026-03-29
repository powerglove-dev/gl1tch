## ADDED Requirements

### Requirement: Prompt manager opens as a full-screen BubbleTea modal
The prompt manager SHALL be a full-screen BubbleTea TUI launched via the CLI subcommand `orcai _promptmgr`. It SHALL use the same Dracula palette and box-drawing border conventions as the inbox detail and crontui modals. It SHALL inherit the active theme via `tuikit.ThemeState`.

#### Scenario: Launched from jump window
- **WHEN** the user selects the "prompts" entry in the jump window
- **THEN** the prompt manager opens full-screen in a tmux window named `orcai-prompts`

#### Scenario: Theme is applied on open
- **WHEN** the prompt manager initialises
- **THEN** all rendered borders, text, and accents use the active Dracula-based theme colours

### Requirement: Three-panel layout — list, editor, test runner
The prompt manager SHALL render a three-panel layout: a left panel showing the prompt list, a right-top panel for the prompt editor, and a right-bottom panel for test runner output. Focus SHALL cycle between panels via `tab`/`shift+tab`.

#### Scenario: Left panel shows prompt list
- **WHEN** prompts exist in the store
- **THEN** the left panel lists each prompt by title and model slug, one per row

#### Scenario: Right-top panel shows editor for selected prompt
- **WHEN** the user selects a prompt in the left panel
- **THEN** the right-top panel populates with the prompt's title, body, and model slug

#### Scenario: Right-bottom panel shows test output
- **WHEN** the user runs a prompt
- **THEN** the right-bottom panel displays the streamed response

### Requirement: Jump window exposes a "prompts" sysop entry
The jump window SHALL include a synthetic "prompts" sysop entry. Selecting it SHALL open (or switch to) a tmux window running `orcai _promptmgr`, following the same pattern as the existing "cron" entry.

#### Scenario: Prompts entry appears in sysop column
- **WHEN** the jump window is opened
- **THEN** a "prompts" entry is visible in the left (sysop) column

#### Scenario: Selecting prompts entry navigates to prompt manager
- **WHEN** the user selects "prompts" and presses enter
- **THEN** a tmux window named `orcai-prompts` is created (or focused) running `orcai _promptmgr`

### Requirement: Browse and navigate prompts
The left panel SHALL support keyboard navigation through the prompt list. Arrow keys or `j`/`k` SHALL move the cursor. The selected prompt SHALL be highlighted with the accent background colour.

#### Scenario: Navigate down
- **WHEN** the user presses `j` or `↓`
- **THEN** the cursor moves to the next prompt in the list

#### Scenario: Navigate up
- **WHEN** the user presses `k` or `↑`
- **THEN** the cursor moves to the previous prompt in the list

#### Scenario: Selected prompt is highlighted
- **WHEN** the cursor is on a prompt
- **THEN** that row renders with the selection background

### Requirement: Search prompts with fuzzy filter
The left panel SHALL include a fuzzy search input (same pattern as crontui filter). Typing SHALL filter the prompt list in real time. Clearing the input SHALL restore the full list.

#### Scenario: Typing filters list
- **WHEN** the user types in the search input
- **THEN** only prompts whose title or body matches the fuzzy query are shown

#### Scenario: Clearing filter restores list
- **WHEN** the user clears the search input
- **THEN** all prompts are shown

### Requirement: Create a new prompt
Pressing `n` in the left panel SHALL create a new blank prompt, focus the editor panel, and place the cursor in the title field.

#### Scenario: New prompt created
- **WHEN** the user presses `n`
- **THEN** a blank prompt form opens in the right-top panel with focus on the title field

#### Scenario: Saving a new prompt persists it
- **WHEN** the user fills in title and body and presses `ctrl+s`
- **THEN** the prompt is saved to the store and appears in the left panel list

### Requirement: Edit an existing prompt
Pressing `e` or `enter` on a selected prompt SHALL open it in the editor panel for editing. Changes are saved with `ctrl+s`.

#### Scenario: Edit opens prompt in editor
- **WHEN** the user presses `e` on a selected prompt
- **THEN** the right-top panel shows the prompt's current title, body, and model slug in editable fields

#### Scenario: Saving edits updates the store
- **WHEN** the user modifies the body and presses `ctrl+s`
- **THEN** the prompt's `body` and `updated_at` are updated in the store

### Requirement: Delete a prompt
Pressing `d` on a selected prompt SHALL prompt for confirmation (`y`/`n`). Confirming SHALL delete the prompt from the store and remove it from the list.

#### Scenario: Delete requires confirmation
- **WHEN** the user presses `d` on a prompt
- **THEN** a confirmation prompt appears asking the user to confirm deletion

#### Scenario: Confirmed deletion removes prompt
- **WHEN** the user presses `y` to confirm
- **THEN** the prompt is deleted from the store and no longer appears in the list

#### Scenario: Cancelling deletion leaves prompt intact
- **WHEN** the user presses `n` to cancel
- **THEN** the prompt remains in the store and list

### Requirement: Select working directory for a prompt run
The editor panel SHALL include a directory path field. The user can type a path or browse with a fuzzy directory picker (same pattern as the agent runner CWD picker). The selected directory SHALL be stored in the `prompts` table as `cwd TEXT DEFAULT ''`. When the test runner executes the prompt, the selected directory is passed to the model invocation as the working directory context.

#### Scenario: Directory field visible in editor
- **WHEN** the editor panel is open
- **THEN** a CWD field is visible showing the current stored directory (or empty if unset)

#### Scenario: Directory stored with prompt
- **WHEN** the user saves a prompt with a directory set
- **THEN** the `cwd` column in the `prompts` table reflects the selected path

#### Scenario: Directory passed to test runner
- **WHEN** the user triggers a run with a directory set
- **THEN** the model invocation receives the directory as working directory context

#### Scenario: Run without directory is unaffected
- **WHEN** the CWD field is empty and the user triggers a run
- **THEN** the run proceeds without a working directory constraint

### Requirement: Select model/agent for a prompt
The editor panel SHALL include a model selector field (dropdown or cycling selector) listing available models from the plugin/provider registry. The selected model slug SHALL be stored with the prompt.

#### Scenario: Model selector lists available models
- **WHEN** the editor panel opens
- **THEN** the model selector shows models available from the plugin registry

#### Scenario: Selected model is saved with prompt
- **WHEN** the user saves a prompt
- **THEN** the `model_slug` field reflects the selected model

### Requirement: Prompt pre-selection in agent runner modal
The switchboard agent runner modal SHALL include a "Prompt" picker field. Selecting a saved prompt SHALL pre-populate the agent's initial input with the prompt body.

#### Scenario: Prompt picker appears in agent runner
- **WHEN** the agent runner modal opens
- **THEN** a "Prompt" field is visible above the existing fields

#### Scenario: Selecting a prompt injects body
- **WHEN** the user selects a saved prompt and launches the agent
- **THEN** the prompt body is passed as the initial input to the agent run

#### Scenario: No prompt selected leaves run unaffected
- **WHEN** the user leaves the Prompt field empty and launches the agent
- **THEN** the agent run behaves identically to the current behaviour

### Requirement: Prompt selection in pipeline steps
A pipeline step SHALL support an optional `prompt_id` field. When set, the executor SHALL resolve the prompt body from the store and prepend it to the step's input.

#### Scenario: Step with prompt_id resolves prompt
- **WHEN** a step has `prompt_id` set and the step executes
- **THEN** the prompt body is fetched from the store and prepended to the step input

#### Scenario: Step without prompt_id is unaffected
- **WHEN** a step has no `prompt_id` field
- **THEN** step execution is identical to current behaviour

#### Scenario: Missing prompt_id causes step failure
- **WHEN** a step has `prompt_id` set to an ID that does not exist in the store
- **THEN** the step fails with a descriptive error message
